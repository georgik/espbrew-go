package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/georgik/espbrew-go/internal/backend/wokwi"
	"github.com/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// mockWokwiServer is a minimal in-process Wokwi Simulation API server. It sends
// a hello on connect, answers commands, and can emit a serial-monitor:data
// event so the APIMonitor produces output to stream.
type mockWokwiServer struct {
	*httptest.Server

	mu      sync.Mutex
	writeMu sync.Mutex
	conns   map[*websocket.Conn]bool
}

func newMockWokwiServer() *mockWokwiServer {
	m := &mockWokwiServer{conns: map[*websocket.Conn]bool{}}
	m.Server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *mockWokwiServer) wsURL() string {
	return "ws" + m.Server.URL[len("http"):] + "/api/ws/beta"
}

func (m *mockWokwiServer) handle(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// hello on connect. All writes to a given connection go through writeMu so
	// the hello, command responses (readLoop), and serial events (emitSerial)
	// never race on the same *websocket.Conn (gorilla allows a single writer).
	m.writeMu.Lock()
	_ = conn.WriteMessage(websocket.TextMessage, mustWokwiJSON(map[string]interface{}{
		"type":            "hello",
		"protocolVersion": float64(1),
		"appVersion":      "mock-1.0",
	}))
	m.writeMu.Unlock()
	go m.readLoop(conn)
}

func (m *mockWokwiServer) readLoop(conn *websocket.Conn) {
	m.mu.Lock()
	m.conns[conn] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.conns, conn)
		m.mu.Unlock()
	}()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var cmd struct {
			ID      string                 `json:"id"`
			Type    string                 `json:"type"`
			Command string                 `json:"command"`
			Params  map[string]interface{} `json:"params"`
		}
		if json.Unmarshal(data, &cmd) != nil {
			continue
		}
		if cmd.Type != "command" {
			continue
		}
		resp := map[string]interface{}{
			"type":    "response",
			"command": cmd.Command,
			"id":      cmd.ID,
			"result":  map[string]interface{}{},
		}
		out, _ := json.Marshal(resp)
		m.writeMu.Lock()
		_ = conn.WriteMessage(websocket.TextMessage, out)
		m.writeMu.Unlock()
	}
}

// emitSerial sends a serial-monitor:data event to every connected client.
// Writes are serialized with every other writer to a connection (the hello
// in handle, command responses in readLoop) via writeMu, so gorilla's
// single-writer rule always holds.
func (m *mockWokwiServer) emitSerial(bytes []float64) {
	// The transport (see transport.go readLoop) reads the event name from
	// raw["event"] and the payload from raw["payload"] at the top level.
	ev := map[string]interface{}{
		"type":    "event",
		"event":   "serial-monitor:data",
		"payload": map[string]interface{}{"bytes": bytes},
	}
	out, _ := json.Marshal(ev)
	m.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(m.conns))
	for c := range m.conns {
		conns = append(conns, c)
	}
	m.mu.Unlock()

	m.writeMu.Lock()
	for _, c := range conns {
		_ = c.WriteMessage(websocket.TextMessage, out)
	}
	m.writeMu.Unlock()
}

func mustWokwiJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// startWokwiSession starts a simulation for dev against the mock server and
// returns the session manager and device.
func startWokwiSession(t *testing.T, srv *mockWokwiServer) (*wokwi.SessionManager, *protocol.DeviceInfo) {
	t.Helper()
	t.Setenv("WOKWI_CLI_SERVER", srv.wsURL())

	dev := &protocol.DeviceInfo{
		Path:    "wokwi-test",
		Backend: protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32-S3",
			DiagramJSON: `{"board":"board-esp32-s3-box-3"}`,
			APIToken:    "tok",
		},
	}

	sessions := wokwi.NewSessionManager()
	elfPath := filepath.Join(t.TempDir(), "app.elf")
	if err := os.WriteFile(elfPath, []byte{0xE9, 0x00, 0x00, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Get(dev, elfPath); err != nil {
		t.Fatalf("session.Get: %v", err)
	}
	t.Cleanup(func() { sessions.Stop(dev.Path) })
	return sessions, dev
}

// TestMonitorHandler_WokwiRoutingAndStreams verifies that a monitor request for
// a Wokwi device is routed to the simulation handler, that the client receives
// monitor_start, and that serial output from the simulation is streamed back as
// base64 "data" frames.
func TestMonitorHandler_WokwiRoutingAndStreams(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.Close()

	sessions, dev := startWokwiSession(t, srv)

	server := NewMonitorServer()
	server.getDevice = func(name string) (*protocol.DeviceInfo, bool) {
		if name == dev.Path {
			return dev, true
		}
		return nil, false
	}
	server.sessions = sessions

	router := mux.NewRouter()
	server.RegisterRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/monitor/wokwi-test"
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer conn.Close()

	// Reader goroutine forwards every message to msgCh.
	var allMu sync.Mutex
	var allMsgs []map[string]interface{}
	track := func(m map[string]interface{}) { allMu.Lock(); allMsgs = append(allMsgs, m); allMu.Unlock() }
	// Using a goroutine (with its own per-read deadline) avoids gorilla's
	// "repeated read on failed connection" panic that occurs when a deadline
	// fires mid-loop.
	msgCh := make(chan map[string]interface{}, 16)
	go func() {
		for {
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var msg map[string]interface{}
			if err := conn.ReadJSON(&msg); err != nil {
				close(msgCh)
				return
			}
			track(msg)
			select {
			case msgCh <- msg:
			default:
			}
		}
	}()

	waitMsg := func(predicate func(map[string]interface{}) bool) bool {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case msg, ok := <-msgCh:
				if !ok {
					return false
				}
				if predicate(msg) {
					return true
				}
			case <-time.After(100 * time.Millisecond):
			}
		}
		return false
	}

	// The client must receive monitor_start addressed to the Wokwi device path
	// (proving it took the Wokwi path, not the serial path).
	gotStart := waitMsg(func(m map[string]interface{}) bool {
		return m["type"] == "monitor_start" && m["port"] == dev.Path
	})
	if !gotStart {
		t.Fatal("did not receive monitor_start for the Wokwi device path")
	}

	// Now have the simulation emit serial output and verify it is streamed back
	// as base64 "data" frames.
	srv.emitSerial([]float64{72, 105}) // "Hi"

	gotData := waitMsg(func(m map[string]interface{}) bool {
		return m["type"] == "data"
	})
	if !gotData {
		allMu.Lock()
		got := allMsgs
		allMu.Unlock()
		t.Logf("received %d messages:", len(got))
		for i, m := range got {
			t.Logf("  [%d] type=%v port=%v data=%v", i, m["type"], m["port"], m["data"])
		}
		t.Fatal("did not receive streamed serial output from the Wokwi simulation")
	}
}
