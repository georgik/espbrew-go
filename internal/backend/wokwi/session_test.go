package wokwi

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/georgik/espbrew-go/pkg/protocol"
)

// wokwiDevice builds a Wokwi device whose API client points at the given mock
// server URL (via WOKWI_CLI_SERVER) and authenticates with the given token.
func wokwiDevice(path, serverURL, token string) *protocol.DeviceInfo {
	return &protocol.DeviceInfo{
		Path:    path,
		Backend: protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32-S3",
			DiagramJSON: `{"board":"board-esp32-s3-box-3"}`,
			APIToken:    token,
		},
	}
}

func TestSessionManagerStartsSimulation(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.Close()
	t.Setenv(envWokwiServer, srv.wsURL())

	dev := wokwiDevice("wokwi-1", srv.wsURL(), "tok")
	elfPath := filepath.Join(t.TempDir(), "app.elf")
	if err := writeFile(elfPath, buildMinimalELF()); err != nil {
		t.Fatal(err)
	}

	m := NewSessionManager()
	mon, err := m.Get(dev, elfPath)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if mon == nil {
		t.Fatal("Get returned nil monitor")
	}
	if !m.Running(dev.Path) {
		t.Error("Running = false after Get, want true")
	}
	defer m.Stop(dev.Path)

	// The simulation must have assembled the ELF and called sim:start.
	start := findCommand(srv.recorded(), "sim:start")
	if start == nil {
		t.Fatal("expected sim:start command")
	}
	firmware, ok := start.Params["firmware"].([]interface{})
	if !ok || len(firmware) != 3 {
		t.Fatalf("sim:start firmware = %#v, want 3 sections", start.Params["firmware"])
	}

	// The assembled image must have been uploaded as flash-<offset>.bin.
	var flashCount int
	for _, c := range srv.recorded() {
		if c.Command == "file:upload" {
			if name, _ := c.Params["name"].(string); strings.HasPrefix(name, "flash-") {
				flashCount++
			}
		}
	}
	if flashCount != 3 {
		t.Errorf("expected 3 flash section uploads, got %d", flashCount)
	}
}

func TestSessionManagerReuse(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.Close()
	t.Setenv(envWokwiServer, srv.wsURL())

	dev := wokwiDevice("wokwi-1", srv.wsURL(), "tok")
	elfPath := filepath.Join(t.TempDir(), "app.elf")
	if err := writeFile(elfPath, buildMinimalELF()); err != nil {
		t.Fatal(err)
	}

	m := NewSessionManager()
	first, err := m.Get(dev, elfPath)
	if err != nil {
		t.Fatalf("Get #1: %v", err)
	}
	defer m.Stop(dev.Path)

	// A second Get with an empty elfPath must reuse the running monitor (this
	// is how `monitor` attaches to a simulation a prior `flash` started).
	second, err := m.Get(dev, "")
	if err != nil {
		t.Fatalf("Get #2 (reuse): %v", err)
	}
	if first != second {
		t.Error("Get #2 returned a different monitor; expected reuse of the running session")
	}

	// A second Get with a new elfPath must still reuse (no fresh sim:start).
	before := findCommands(srv.recorded(), "sim:start")
	third, err := m.Get(dev, elfPath)
	if err != nil {
		t.Fatalf("Get #3: %v", err)
	}
	if first != third {
		t.Error("Get #3 returned a different monitor; expected reuse of the running session")
	}
	after := findCommands(srv.recorded(), "sim:start")
	if after != before {
		t.Errorf("reuse started a new simulation (%d -> %d sim:start calls)", before, after)
	}
}

func TestSessionManagerRequiresFirmwareForFreshSession(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.Close()
	t.Setenv(envWokwiServer, srv.wsURL())

	dev := wokwiDevice("wokwi-1", srv.wsURL(), "tok")

	m := NewSessionManager()
	_, err := m.Get(dev, "")
	if err == nil {
		t.Fatal("Get with empty elfPath and no existing session: expected error")
	}
	if !strings.Contains(err.Error(), "no firmware flashed") {
		t.Errorf("unexpected error: %v", err)
	}
	if m.Running(dev.Path) {
		t.Error("Running = true after failed Get, want false")
	}
}

func TestSessionManagerStopAndRestart(t *testing.T) {
	srv := newMockWokwiServer()
	defer srv.Close()
	t.Setenv(envWokwiServer, srv.wsURL())

	dev := wokwiDevice("wokwi-1", srv.wsURL(), "tok")
	elfPath := filepath.Join(t.TempDir(), "app.elf")
	if err := writeFile(elfPath, buildMinimalELF()); err != nil {
		t.Fatal(err)
	}

	m := NewSessionManager()
	if _, err := m.Get(dev, elfPath); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !m.Running(dev.Path) {
		t.Fatal("Running = false after Get, want true")
	}

	// Stop must tear the session down.
	m.Stop(dev.Path)
	if m.Running(dev.Path) {
		t.Error("Running = true after Stop, want false")
	}

	// A subsequent Get must start a fresh simulation.
	before := findCommands(srv.recorded(), "sim:start")
	if _, err := m.Get(dev, elfPath); err != nil {
		t.Fatalf("Get after stop: %v", err)
	}
	defer m.Stop(dev.Path)
	after := findCommands(srv.recorded(), "sim:start")
	if after <= before {
		t.Errorf("expected a fresh sim:start after restart (%d -> %d)", before, after)
	}
}

// findCommands returns how many recorded commands match name.
func findCommands(cmds []mockCommand, name string) int {
	n := 0
	for _, c := range cmds {
		if c.Command == name {
			n++
		}
	}
	return n
}
