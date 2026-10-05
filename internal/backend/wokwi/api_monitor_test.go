package wokwi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/websocket"
)

// mockCommand is a command recorded by mockWokwiServer.
type mockCommand struct {
	ID      string
	Command string
	Params  map[string]interface{}
}

// mockWokwiServer is a minimal in-process Wokwi Simulation API server used to
// validate APIMonitor.Start() end-to-end without the public endpoint. It sends
// a hello on connect, records every command, and answers each with a response.
type mockWokwiServer struct {
	*httptest.Server

	mu       sync.Mutex
	commands []mockCommand
}

func newMockWokwiServer() *mockWokwiServer {
	m := &mockWokwiServer{}
	m.Server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *mockWokwiServer) wsURL() string {
	return "ws" + m.Server.URL[len("http"):] + "/api/ws/beta"
}

func (m *mockWokwiServer) recorded() []mockCommand {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockCommand, len(m.commands))
	copy(out, m.commands)
	return out
}

func (m *mockWokwiServer) handle(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	// hello on connect (client reads protocolVersion/appVersion from the raw map)
	_ = conn.WriteMessage(websocket.TextMessage, mustJSON(mockMap(
		"type", "hello",
		"protocolVersion", float64(1),
		"appVersion", "mock-1.0",
	)))
	go m.readLoop(conn)
}

func (m *mockWokwiServer) readLoop(conn *websocket.Conn) {
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
		m.mu.Lock()
		m.commands = append(m.commands, mockCommand{ID: cmd.ID, Command: cmd.Command, Params: cmd.Params})
		m.mu.Unlock()

		resp := mockMap("type", "response", "command", cmd.Command, "id", cmd.ID, "result", mockMap())
		out, _ := json.Marshal(resp)
		_ = conn.WriteMessage(websocket.TextMessage, out)
	}
}

// mustJSON marshals v to JSON, panicking on error.
func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// writeFile writes b to path (test helper).
func writeFile(path string, b []byte) error {
	return os.WriteFile(path, b, 0o644)
}

// mockMap builds a flat map from alternating key/value pairs.
func mockMap(kv ...interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

// findCommand returns the first recorded command with the given name.
func findCommand(cmds []mockCommand, name string) *mockCommand {
	for i := range cmds {
		if cmds[i].Command == name {
			return &cmds[i]
		}
	}
	return nil
}

func TestAPIMonitorStartAssemblesAndUploadsSections(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.Close()

	// Write a minimal ELF (assembles into bootloader + partition + app).
	elfPath := filepath.Join(t.TempDir(), "app.elf")
	if err := writeFile(elfPath, buildMinimalELF()); err != nil {
		t.Fatal(err)
	}

	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32-S3",
		DiagramJSON: `{"board":"board-esp32-s3-box-3"}`,
	}
	mon, err := NewAPIMonitorFromConfig(cfg, elfPath, "tok", srv.wsURL())
	if err != nil {
		t.Fatalf("NewAPIMonitorFromConfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mon.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mon.Stop()

	cmds := srv.recorded()

	// diagram.json must be uploaded first.
	diagram := findCommand(cmds, "file:upload")
	if diagram == nil {
		t.Fatal("expected file:upload for diagram")
	}
	if name, _ := diagram.Params["name"].(string); name != "diagram.json" {
		t.Errorf("first file:upload name = %q, want diagram.json", name)
	}

	// Each assembled section must be uploaded as flash-<offset>.bin.
	var uploaded []string
	for _, c := range cmds {
		if c.Command == "file:upload" {
			if name, _ := c.Params["name"].(string); strings.HasPrefix(name, "flash-") {
				uploaded = append(uploaded, name)
			}
		}
	}
	want := []string{"flash-0.bin", "flash-8000.bin", "flash-10000.bin"}
	if len(uploaded) != len(want) {
		t.Fatalf("expected %d flash uploads, got %d: %v", len(want), len(uploaded), uploaded)
	}
	for i, name := range want {
		if uploaded[i] != name {
			t.Errorf("flash upload[%d] = %q, want %q", i, uploaded[i], name)
		}
	}

	// ELF must be uploaded as firmware.elf.
	var hasELF bool
	for _, c := range cmds {
		if c.Command == "file:upload" {
			if name, _ := c.Params["name"].(string); name == "firmware.elf" {
				hasELF = true
			}
		}
	}
	if !hasELF {
		t.Error("expected firmware.elf upload")
	}

	// sim:start must reference the uploaded sections (a list of {offset,file}).
	start := findCommand(cmds, "sim:start")
	if start == nil {
		t.Fatal("expected sim:start command")
	}
	firmware, ok := start.Params["firmware"].([]interface{})
	if !ok || len(firmware) != 3 {
		t.Fatalf("sim:start firmware = %#v, want 3 sections", start.Params["firmware"])
	}
	for _, f := range firmware {
		sec, ok := f.(map[string]interface{})
		if !ok {
			t.Errorf("firmware section not an object: %#v", f)
			continue
		}
		if _, hasOff := sec["offset"]; !hasOff {
			t.Errorf("firmware section missing offset: %#v", sec)
		}
		if _, hasFile := sec["file"]; !hasFile {
			t.Errorf("firmware section missing file: %#v", sec)
		}
	}

	// Serial listening must be enabled.
	if findCommand(cmds, "serial-monitor:listen") == nil {
		t.Error("expected serial-monitor:listen")
	}
}
