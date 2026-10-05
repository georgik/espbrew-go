package http

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/georgik/espbrew-go/internal/backend/wokwi"
	"github.com/georgik/espbrew-go/internal/monitor"
	"github.com/georgik/espbrew-go/pkg/protocol"
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
	// resolve maps a device base name (as sent by the CLI) to the full
	// device path stored on the leader, e.g. "usb-…-if00" ->
	// "/dev/serial/by-id/usb-…-if00". Nil when no leader registry is wired.
	resolve func(string) string
	// getDevice resolves the name the CLI sends (alias or path) to the full
	// device info on the leader, used to detect virtual backends (Wokwi/QEMU).
	// Nil when no leader registry is wired.
	getDevice func(string) (*protocol.DeviceInfo, bool)
	// sessions runs Wokwi simulations for the leader. Nil when no leader is
	// wired; Wokwi monitoring then reports the simulation is unavailable.
	sessions *wokwi.SessionManager
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
	portName := vars["port"]

	// Resolve the addressed board to its device info (alias or path). This is
	// needed to detect virtual backends (Wokwi/QEMU) before falling back to a
	// serial port.
	var dev *protocol.DeviceInfo
	if s.getDevice != nil {
		if d, ok := s.getDevice(portName); ok {
			dev = d
		}
	}

	// Wokwi devices run a simulation; stream its serial output over the
	// WebSocket instead of opening a serial port.
	if dev != nil && dev.Backend == protocol.BackendWokwi {
		s.handleWokwiMonitorWebSocket(w, r, dev)
		return
	}

	// Resolve the base name (as sent by the CLI) to the full device path
	// stored on the leader, e.g. "/dev/serial/by-id/usb-…-if00".
	if s.resolve != nil {
		portName = s.resolve(portName)
	}

	// Reconstruct full port path from name
	// On Windows, COM ports don't have a /dev/ prefix. A resolved path is
	// already absolute, so only prefix /dev/ when it isn't already.
	var port string
	if runtime.GOOS == "windows" {
		port = portName
	} else if strings.HasPrefix(portName, "/dev/") {
		port = portName
	} else {
		port = "/dev/" + portName
	}

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

	// Check if reset is requested
	resetRequested := r.URL.Query().Get("reset") == "1"

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
		"port": portName,
		"baud": baud,
	})

	// Handle incoming messages
	go s.handleMonitorMessages(conn, session)

	// Trigger reset after connection is established (if requested)
	if resetRequested {
		log.Info().Msg("Reset requested, sending control message")
		// Small delay to ensure client is ready
		time.Sleep(50 * time.Millisecond)
		session.SendControl(&monitor.ControlMessage{Type: "reset"})
		// Send reset confirmation
		conn.WriteJSON(map[string]interface{}{
			"type": "reset_complete",
		})
		log.Info().Msg("Reset control sent to device")
	}

	// Stream data to client with batching
	batch := make([]byte, 0, 4096)
	batchTimer := time.NewTimer(10 * time.Millisecond)
	batchTimer.Stop()

	flushBatch := func() error {
		if len(batch) == 0 {
			return nil
		}
		log.Info().Int("bytes", len(batch)).Msg("WebSocket: sending batched data")
		msg := map[string]interface{}{
			"type": "data",
			"data": base64.StdEncoding.EncodeToString(batch),
		}
		if err := conn.WriteJSON(msg); err != nil {
			log.Error().Err(err).Msg("WebSocket write error")
			return err
		}
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case data, ok := <-session.Data():
			if !ok {
				flushBatch()
				return
			}
			log.Info().Int("bytes", len(data)).Str("preview", string(data)).Msg("Received from serial port")

			// Append to batch
			batch = append(batch, data...)

			// Flush if batch is large enough
			if len(batch) >= 4096 {
				if err := flushBatch(); err != nil {
					return
				}
			} else {
				// Reset timer to flush after delay if more data arrives
				batchTimer.Reset(10 * time.Millisecond)
			}

		case <-batchTimer.C:
			log.Info().Msg("Batch timer fired, flushing partial batch")
			if err := flushBatch(); err != nil {
				return
			}

		case err, ok := <-session.Errors():
			if !ok {
				return
			}
			flushBatch()
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
			log.Info().Msg("Reset requested via WebSocket message")
			session.SendControl(&monitor.ControlMessage{Type: "reset"})
			conn.WriteJSON(map[string]interface{}{
				"type": "reset_complete",
			})
		case "close":
			session.SendControl(&monitor.ControlMessage{Type: "close"})
			return
		case "data":
			// Write data to serial port
			if dataStr, ok := msg["data"].(string); ok {
				if err := session.Write([]byte(dataStr)); err != nil {
					log.Error().Err(err).Msg("Failed to write to serial port")
					conn.WriteJSON(map[string]interface{}{
						"type":  "error",
						"error": "Failed to write to device",
					})
				}
			}
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

// handleWokwiMonitorWebSocket streams the serial output of a Wokwi simulation
// over the WebSocket. The simulation is started (or reused) via the leader's
// session manager, so `monitor` attaches to the simulation a prior `flash`
// started. Its serial output is sent as base64 "data" messages, the same
// frame format the serial monitor uses, so the CLI needs no changes.
func (s *MonitorServer) handleWokwiMonitorWebSocket(
	w http.ResponseWriter,
	r *http.Request,
	dev *protocol.DeviceInfo,
) {
	if s.sessions == nil {
		http.Error(w, "Wokwi simulation not available on this leader", http.StatusNotImplemented)
		return
	}

	resetRequested := r.URL.Query().Get("reset") == "1"

	conn, err := monitorUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Str("device", dev.Path).Msg("Wokwi monitor upgrade failed")
		return
	}
	defer conn.Close()

	monitor, err := s.sessions.Get(dev, "")
	if err != nil {
		log.Warn().Err(err).Str("device", dev.Path).Msg("Wokwi monitor: no simulation to attach to")
		_ = conn.WriteJSON(map[string]interface{}{
			"type":    "error",
			"message": err.Error(),
		})
		return
	}

	log.Info().Str("device", dev.Path).Msg("Wokwi monitor attached")

	// Reset first (if requested) to capture fresh boot logs, mirroring the
	// serial monitor's --reset behavior.
	if resetRequested {
		if rerr := monitor.Reset(); rerr != nil {
			log.Warn().Err(rerr).Msg("Wokwi reset failed")
		} else {
			_ = conn.WriteJSON(map[string]interface{}{"type": "reset_complete"})
		}
	}

	// Start message for parity with the serial monitor.
	_ = conn.WriteJSON(map[string]interface{}{
		"type": "monitor_start",
		"port": dev.Path,
	})

	// Handle client control messages (reset / close / data) in the background.
	// Reads happen here while streaming writes happen in the caller's goroutine
	// (gorilla is safe for one reader + one concurrent writer).
	go s.handleWokwiMessages(conn, monitor)

	// Stream the simulation serial output until the monitor stops or the client
	// disconnects.
	s.streamWokwiMonitorOutput(conn, monitor)
}

// streamWokwiMonitorOutput writes the monitor's serial output as base64 "data"
// messages until the monitor stops or the client disconnects. When the output
// channel closes it waits briefly for a possible Reset to restart the
// simulation before giving up, so a reset mid-stream does not drop the client.
func (s *MonitorServer) streamWokwiMonitorOutput(conn *websocket.Conn, monitor protocol.Monitor) {
	// The session manager only ever creates an *APIMonitor; the concrete type
	// is needed for IsRunning, which is not part of the protocol.Monitor
	// interface. Fall back to plain streaming if the assertion ever fails.
	apimon, ok := monitor.(*wokwi.APIMonitor)
	if !ok {
		for entry, keep := <-monitor.Output(); keep; entry, keep = <-monitor.Output() {
			if !writeWokwiData(conn, entry.Data) {
				return
			}
		}
		return
	}

	for {
		entry, ok := <-apimon.Output()
		if !ok {
			// Channel closed: either the simulation stopped, or a Reset is
			// restarting it. Wait briefly for the restart before giving up.
			if !waitForMonitorRunning(apimon, 500*time.Millisecond) {
				return
			}
			continue
		}
		if !writeWokwiData(conn, entry.Data) {
			return
		}
	}
}

// handleWokwiMessages dispatches client control messages to the monitor.
func (s *MonitorServer) handleWokwiMessages(conn *websocket.Conn, monitor protocol.Monitor) {
	defer func() {
		if recover() != nil {
			_ = conn.Close()
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

		switch msgType, _ := msg["type"].(string); msgType {
		case "reset":
			if rerr := monitor.Reset(); rerr != nil {
				log.Warn().Err(rerr).Msg("Wokwi reset failed")
			}
			_ = conn.WriteJSON(map[string]interface{}{"type": "reset_complete"})
		case "close":
			return
		case "data":
			if dataStr, ok := msg["data"].(string); ok {
				_ = monitor.Send([]byte(dataStr))
			}
		}
	}
}

// waitForMonitorRunning polls IsRunning until it returns true or the timeout
// elapses. It lets the streamer tell a transient Reset (which briefly stops the
// monitor) apart from a real shutdown.
func waitForMonitorRunning(m *wokwi.APIMonitor, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.IsRunning() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return m.IsRunning()
}

// writeWokwiData sends one chunk of serial output as a base64 "data" message.
// It returns false (and logs) when the client has disconnected so the streamer
// can stop.
func writeWokwiData(conn *websocket.Conn, data string) bool {
	if len(data) == 0 {
		return true
	}
	if err := conn.WriteJSON(map[string]interface{}{
		"type": "data",
		"data": base64.StdEncoding.EncodeToString([]byte(data)),
	}); err != nil {
		log.Debug().Err(err).Msg("Wokwi monitor: client disconnected, stopping stream")
		return false
	}
	return true
}
