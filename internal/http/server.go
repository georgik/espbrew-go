package http

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/georgik/esp-ci-cluster/internal/cluster"
	"github.com/georgik/esp-ci-cluster/internal/dashboard"
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
	addr    string
	node    cluster.Node
	router  *mux.Router
	server  *http.Server
	api     *APIHandler
	monitor *MonitorServer

	// WebSocket clients
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
}

func NewServer(addr string, node cluster.Node) *Server {
	s := &Server{
		addr:    addr,
		node:    node,
		clients: make(map[*websocket.Conn]bool),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.router = mux.NewRouter()

	// API routes
	s.api = NewAPIHandler(s.node)
	s.api.RegisterRoutes(s.router)

	// Monitor WebSocket routes
	s.monitor = NewMonitorServer()
	s.monitor.RegisterRoutes(s.router)

	// WebSocket - override the API placeholder
	s.router.HandleFunc("/api/v1/ws", s.handleWebSocket)

	// Health check
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Static files (dashboard)
	s.router.PathPrefix("/").Handler(s.handleStatic())
}

func (s *Server) Start(ctx context.Context) error {
	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
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
		<-ctx.Done()
		log.Info().Msg("HTTP server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
		s.closeAllClients()
	}()

	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
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
		if s.api.master != nil {
			firmware, _ := msg["firmware"].(string)
			device, _ := msg["device"].(string)
			job, err := s.api.master.EnqueueJob(firmware, device)
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
		"type":    "state",
		"nodes":   len(state.Nodes),
		"devices": len(state.Devices),
		"jobs":    len(state.Jobs),
	}

	if s.api.master != nil {
		data["queue_size"] = s.api.master.GetJobQueue().PendingCount()
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
			"jobs":    len(state.Jobs),
		}

		if s.api.master != nil {
			update["queue_size"] = s.api.master.GetJobQueue().PendingCount()
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

// Fallback dashboard if embedded files not available
const fallbackDashboard = `<!DOCTYPE html>
<html><head><title>ESPBrew Cluster</title>
<style>body{font-family:sans-serif;margin:40px;background:#1a1a2e;color:#eee}</style>
</head><body><h1>ESPBrew Cluster</h1><p>Dashboard files not embedded. Run: go generate ./internal/dashboard</p></body></html>
`
