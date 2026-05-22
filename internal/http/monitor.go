package http

import (
	"encoding/json"
	"net/http"
	"sync"

	"codeberg.org/georgik/espbrew-go/internal/monitor"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var monitorUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type MonitorServer struct {
	streams *monitor.StreamManager
	mu      sync.RWMutex
}

func NewMonitorServer() *MonitorServer {
	return &MonitorServer{
		streams: monitor.NewStreamManager(),
	}
}

func (s *MonitorServer) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/monitor/{port}", s.handleMonitorWebSocket)
}

func (s *MonitorServer) handleMonitorWebSocket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	port := vars["port"]

	conn, err := monitorUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Str("port", port).Msg("WebSocket upgrade failed")
		return
	}
	defer conn.Close()

	log.Info().Str("port", port).Str("remote_addr", r.RemoteAddr).Msg("Monitor WebSocket connected")

	// Get monitor config from query params
	baud := 115200
	if b := r.URL.Query().Get("baud"); b != "" {
		var parsedBaud int
		if err := json.Unmarshal([]byte(b), &parsedBaud); err == nil {
			baud = parsedBaud
		}
	}

	exitOn := r.URL.Query().Get("exit_on")

	cfg := monitor.StreamConfig{
		Port:     port,
		BaudRate: baud,
		ExitOn:   exitOn,
	}

	sessionID := r.RemoteAddr + ":" + port
	session, err := s.streams.Create(sessionID, cfg)
	if err != nil {
		s.sendMonitorError(conn, err.Error())
		return
	}
	defer s.streams.Remove(sessionID)

	// Send start message
	conn.WriteJSON(map[string]interface{}{
		"type": "monitor_start",
		"port": port,
		"baud": baud,
	})

	// Handle incoming messages
	go s.handleMonitorMessages(conn, session)

	// Stream data to client
	for {
		select {
		case data, ok := <-session.Data():
			if !ok {
				return
			}
			msg := map[string]interface{}{
				"type": "data",
				"data": data,
			}
			if err := conn.WriteJSON(msg); err != nil {
				log.Debug().Err(err).Msg("WebSocket write error")
				return
			}

		case err, ok := <-session.Errors():
			if !ok {
				return
			}
			s.sendMonitorError(conn, err.Error())
			return
		}
	}
}

func (s *MonitorServer) handleMonitorMessages(conn *websocket.Conn, session *monitor.StreamSession) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Any("panic", r).Msg("Monitor message handler panic")
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		msgType, _ := msg["type"].(string)

		switch msgType {
		case "reset":
			session.SendControl(&monitor.ControlMessage{Type: "reset"})
			conn.WriteJSON(map[string]interface{}{
				"type": "reset_complete",
			})
		case "close":
			session.SendControl(&monitor.ControlMessage{Type: "close"})
			return
		}
	}
}

func (s *MonitorServer) sendMonitorError(conn *websocket.Conn, message string) {
	conn.WriteJSON(map[string]interface{}{
		"type":  "error",
		"error": message,
	})
}

func (s *MonitorServer) ListSessions() map[string]*monitor.StreamSession {
	return s.streams.List()
}
