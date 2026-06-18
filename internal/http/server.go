package http

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/dashboard"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Server struct {
	addr       string
	node       cluster.Node
	router     *mux.Router
	server     *http.Server
	api        *APIHandler
	monitor    *MonitorServer
	hub        *ProgressHub
	devMode    bool
	shutdownCh chan struct{}

	// WebSocket clients
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
}

func NewServer(addr string, node cluster.Node, store *persistence.Store) *Server {
	s := &Server{
		addr:    addr,
		node:    node,
		clients: make(map[*websocket.Conn]bool),
	}
	s.setupRoutes(store)
	return s
}

func (s *Server) setupRoutes(store *persistence.Store) {
	s.router = mux.NewRouter()
	s.hub = NewProgressHub()

	// API routes
	s.api = NewAPIHandler(s.node, store)
	s.api.RegisterRoutes(s.router)

	// Camera gallery routes
	cameraHandler := NewCameraHandler()
	cameraHandler.RegisterRoutes(s.router)

	// Camera settings routes
	cameraSettingsHandler := NewCameraSettingsHandler(store)
	cameraSettingsHandler.RegisterRoutes(s.router)

	// Bounding box mapping routes
	mappingHandler := NewMappingHandler(store)
	mappingHandler.RegisterRoutes(s.router)

	// Flash status routes
	flashStatusHandler := NewFlashStatusHandler(store)
	flashStatusHandler.RegisterRoutes(s.router)

	// Snap API routes
	snapAPI := NewSnapAPI(store, s.node)
	snapAPI.RegisterRoutes(s.router)

	// Device disable/enable routes
	deviceDisableHandler := NewDeviceDisableHandler(s.node, store)
	deviceDisableHandler.RegisterRoutes(s.router)

	// Device protect/unprotect routes
	deviceProtectHandler := NewDeviceProtectHandler(s.node, store)
	deviceProtectHandler.RegisterRoutes(s.router)

	// Flash API routes
	if leader, ok := s.node.(*cluster.LeaderNode); ok {
		progressHandler := NewProgressHandler(leader, s.hub)
		progressHandler.RegisterRoutes(s.router)

		flashHandler := NewFlashHandler(leader, os.TempDir(), progressHandler)
		flashHandler.RegisterRoutes(s.router)

		// Read-flash handler
		readFlashHandler := NewReadFlashHandler(s.node, leader, os.TempDir())
		readFlashHandler.RegisterRoutes(s.router)
	} else if peer, ok := s.node.(*cluster.PeerNode); ok {
		// Register flash handler on peer nodes too
		flashHandler := NewPeerFlashHandler(peer)
		flashHandler.RegisterRoutes(s.router)
	}

	// Monitor WebSocket routes
	s.monitor = NewMonitorServer()
	s.monitor.RegisterRoutes(s.router)

	// WebSocket - override the API placeholder
	s.router.HandleFunc("/api/v1/ws", s.handleWebSocket)

	// Health check
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Serial monitor page
	s.router.HandleFunc("/monitor", s.handleMonitor).Methods("GET")

	// Favicon
	s.router.HandleFunc("/favicon.ico", s.handleFavicon).Methods("GET")

	// Redirect root to v2 (must be before PathPrefix handlers)
	s.router.HandleFunc("/", s.handleRootRedirect).Methods("GET")

	// WASM UI (v2)
	s.router.PathPrefix("/v2/").Handler(s.handleWasmUI())

	// V1 HTML interface
	s.router.PathPrefix("/v1/").Handler(s.handleV1Static())

	// Developer mode shutdown endpoint
	if s.devMode {
		s.router.HandleFunc("/api/v1/dev/shutdown", s.handleDevShutdown).Methods("POST")
	}
}

func (s *Server) GetProgressCallback() func(string, int, string) {
	if s.hub == nil {
		return nil
	}
	return func(jobID string, progress int, status string) {
		s.hub.BroadcastProgress(jobID, progress, status)
	}
}

// SetDevMode enables developer mode features (unsafe for production)
func (s *Server) SetDevMode(enabled bool) {
	s.devMode = enabled
	if enabled && s.router != nil {
		s.router.HandleFunc("/api/v1/dev/shutdown", s.handleDevShutdown).Methods("POST")
		log.Warn().Msg("Developer mode enabled - shutdown endpoint exposed")
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.shutdownCh = make(chan struct{})
	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Long timeout for snap/flash operations
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", s.addr).Msg("HTTP server starting")
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Start state broadcast for WebSocket clients
	go s.broadcastState()

	go func() {
		select {
		case <-ctx.Done():
		case <-s.shutdownCh:
		}
		log.Info().Msg("HTTP server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
		s.closeAllClients()
	}()

	return nil
}

// handleDevShutdown handles developer mode shutdown request
func (s *Server) handleDevShutdown(w http.ResponseWriter, r *http.Request) {
	log.Warn().Msg("Developer mode shutdown requested")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "shutting_down",
	})
	close(s.shutdownCh)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleRootRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/v2/", http.StatusMovedPermanently)
}

func (s *Server) handleMonitor(w http.ResponseWriter, r *http.Request) {
	if dashboard.HasMonitor() {
		w.Header().Set("Content-Type", "text/html")
		w.Write(dashboard.MonitorHTML())
	} else {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fallbackMonitor))
	}
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Write(dashboard.FaviconSVG())
}

func (s *Server) handleStatic() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if dashboard.HasDashboard() {
				w.Header().Set("Content-Type", "text/html")
				w.Write(dashboard.IndexHTML())
			} else {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(fallbackDashboard))
			}
			return
		}
		http.FileServer(dashboard.StaticFS()).ServeHTTP(w, r)
	})
}

func (s *Server) handleV1Static() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip /v1/ prefix
		path := r.URL.Path[4:] // Remove "/v1"
		if path == "" || path == "/" {
			path = "/"
		}

		// Create a new request with the modified path
		r = r.Clone(r.Context())
		r.URL.Path = path

		if path == "/" {
			if dashboard.HasDashboard() {
				w.Header().Set("Content-Type", "text/html")
				w.Write(dashboard.IndexHTML())
			} else {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(fallbackDashboard))
			}
			return
		}
		http.FileServer(dashboard.StaticFS()).ServeHTTP(w, r)
	})
}

func (s *Server) handleWasmUI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip /v2/ prefix
		path := r.URL.Path[4:] // Remove "/v2"
		if path == "" || path == "/" {
			path = "index.html"
		}

		// Determine full file path
		fullPath := "web/" + path

		// Open the file
		f, err := os.Open(fullPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		// Get file info for Content-Type
		info, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Set proper content types before serving
		switch {
		case strings.HasSuffix(path, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case strings.HasSuffix(path, ".wasm"):
			w.Header().Set("Content-Type", "application/wasm")
		default:
			// Let http.ServeContent detect the type
		}

		// Serve the file with proper content type handling
		http.ServeContent(w, r, path, info.ModTime(), f)
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("WebSocket upgrade failed")
		return
	}

	log.Info().Str("remote_addr", r.RemoteAddr).Msg("WebSocket connected")

	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	// Send initial state
	s.sendState(conn)

	// Handle incoming messages
	go s.handleWebSocketMessages(conn)
}

func (s *Server) handleWebSocketMessages(conn *websocket.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		log.Debug().Bytes("message", message).Msg("WebSocket message received")

		// Handle incoming commands
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			s.handleWebSocketCommand(conn, msg)
		}
	}
}

func (s *Server) handleWebSocketCommand(conn *websocket.Conn, msg map[string]interface{}) {
	msgType, _ := msg["type"].(string)

	switch msgType {
	case "subscribe":
		// Client wants state updates
		log.Debug().Msg("Client subscribed to state updates")

	case "get_state":
		s.sendState(conn)

	case "create_job":
		if s.api.leader != nil {
			firmware, _ := msg["firmware"].(string)
			device, _ := msg["device"].(string)
			job, err := s.api.leader.EnqueueJob(firmware, device)
			if err != nil {
				s.sendError(conn, err.Error())
			} else {
				s.sendJSON(conn, map[string]interface{}{
					"type": "job_created",
					"job":  job.ToMap(),
				})
			}
		}
	}
}

func (s *Server) sendState(conn *websocket.Conn) {
	state := s.node.State()

	data := map[string]interface{}{
		"type":         "state",
		"nodes":        len(state.Nodes),
		"devices":      len(state.Devices),
		"cameras":      len(state.Cameras),
		"jobs":         len(state.Jobs),
		"devices_list": deviceListToMap(state.Devices),
		"cameras_list": cameraListToMap(state.Cameras),
	}

	if s.api.leader != nil {
		data["queue_size"] = s.api.leader.GetJobQueue().PendingCount()
	}

	s.sendJSON(conn, data)
}

func (s *Server) sendJSON(conn *websocket.Conn, data interface{}) {
	conn.WriteJSON(data)
}

func (s *Server) sendError(conn *websocket.Conn, message string) {
	conn.WriteJSON(map[string]interface{}{
		"type":  "error",
		"error": message,
	})
}

func (s *Server) broadcastState() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		if len(s.clients) == 0 {
			s.mu.RUnlock()
			continue
		}

		// Create state update
		state := s.node.State()
		update := map[string]interface{}{
			"type":    "state_update",
			"nodes":   len(state.Nodes),
			"devices": len(state.Devices),
			"cameras": len(state.Cameras),
			"jobs":    len(state.Jobs),
		}

		if s.api.leader != nil {
			update["queue_size"] = s.api.leader.GetJobQueue().PendingCount()
		}

		// Broadcast to all clients
		for conn := range s.clients {
			if err := conn.WriteJSON(update); err != nil {
				delete(s.clients, conn)
				conn.Close()
			}
		}
		s.mu.RUnlock()
	}
}

func (s *Server) closeAllClients() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.clients {
		conn.Close()
	}
	s.clients = make(map[*websocket.Conn]bool)
}

func deviceListToMap(devices map[string]*protocol.DeviceInfo) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(devices))
	for _, d := range devices {
		result = append(result, map[string]interface{}{
			"path":    d.Path,
			"vid":     d.VID,
			"pid":     d.PID,
			"status":  d.Status,
			"node_id": d.NodeID,
			"serial":  d.SerialNumber,
		})
	}
	return result
}

func cameraListToMap(cameras map[string]*protocol.CameraInfo) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(cameras))
	for _, c := range cameras {
		result = append(result, map[string]interface{}{
			"id":      c.ID,
			"name":    c.Name,
			"backend": c.Backend,
			"node_id": c.NodeID,
			"status":  c.Status,
		})
	}
	return result
}

// Fallback dashboard if embedded files not available
const fallbackDashboard = `<!DOCTYPE html>
<html><head><title>ESPBrew Cluster</title>
<style>body{font-family:sans-serif;margin:40px;background:#1a1a2e;color:#eee}</style>
</head><body><h1>ESPBrew Cluster</h1><p>Dashboard files not embedded. Run: go generate ./internal/dashboard</p></body></html>
`

// Fallback monitor page if embedded files not available
const fallbackMonitor = `<!DOCTYPE html>
<html><head><title>Serial Monitor - ESPBrew</title>
<style>body{font-family:sans-serif;margin:40px;background:#1a1a2e;color:#eee}</style>
</head><body><h1>Serial Monitor</h1><p>Monitor page not embedded. Run: go generate ./internal/dashboard</p></body></html>
`
