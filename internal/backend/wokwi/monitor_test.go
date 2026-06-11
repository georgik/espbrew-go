package wokwi

import (
	"context"
	"os"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

func TestNewMonitor(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1,"parts":[{"type":"esp32-devkitC","id":"chip"}]}`,
		},
	}

	monitor, err := NewMonitor(device)
	if err != nil {
		t.Fatalf("NewMonitor failed: %v", err)
	}

	if monitor == nil {
		t.Fatal("Expected monitor, got nil")
	}

	wokwiMonitor, ok := monitor.(*Monitor)
	if !ok {
		t.Fatal("Expected Wokwi Monitor, got different type")
	}

	if wokwiMonitor.config.ChipType != "ESP32" {
		t.Errorf("Expected ChipType ESP32, got %s", wokwiMonitor.config.ChipType)
	}
}

func TestNewMonitor_InvalidBackend(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-physical",
		Backend:  protocol.BackendPhysical,
	}

	_, err := NewMonitor(device)
	if err == nil {
		t.Error("Expected error for physical backend, got nil")
	}
}

func TestNewMonitor_MissingConfig(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		// No BackendConfig
	}

	_, err := NewMonitor(device)
	if err == nil {
		t.Error("Expected error for missing config, got nil")
	}
}

func TestNewMonitorFromConfig(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	if monitor == nil {
		t.Fatal("Expected monitor, got nil")
	}

	if monitor.elfPath != "/tmp/test.elf" {
		t.Errorf("Expected elfPath /tmp/test.elf, got %s", monitor.elfPath)
	}
}

func TestNewMonitorFromConfig_InvalidConfig(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		// Missing required fields
	}

	_, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err == nil {
		t.Error("Expected error for invalid config, got nil")
	}
}

func TestMonitor_SetELFPath(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	newPath := "/tmp/new.elf"
	monitor.SetELFPath(newPath)

	if monitor.elfPath != newPath {
		t.Errorf("Expected elfPath %s, got %s", newPath, monitor.elfPath)
	}
}

func TestMonitor_SetTimeout(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	newTimeout := 60 * time.Second
	monitor.SetTimeout(newTimeout)

	if monitor.timeout != newTimeout {
		t.Errorf("Expected timeout %v, got %v", newTimeout, monitor.timeout)
	}
}

func TestMonitor_SetExitPattern(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	newPattern := "Test complete"
	monitor.SetExitPattern(newPattern)

	if monitor.exitOn != newPattern {
		t.Errorf("Expected exitOn %s, got %s", newPattern, monitor.exitOn)
	}
}

func TestMonitor_Validate_NoConfig(t *testing.T) {
	monitor := &Monitor{
		config:  nil,
		elfPath: "/tmp/test.elf",
	}

	err := monitor.Validate()
	if err == nil {
		t.Error("Expected error for nil config, got nil")
	}
}

func TestMonitor_Validate_NoELF(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor := &Monitor{
		config:  cfg,
		elfPath: "",
	}

	err := monitor.Validate()
	if err == nil {
		t.Error("Expected error for missing ELF path, got nil")
	}
}

func TestMonitor_Validate_NonexistentELF(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor := &Monitor{
		config:  cfg,
		elfPath: "/tmp/nonexistent.elf",
	}

	err := monitor.Validate()
	if err == nil {
		t.Error("Expected error for nonexistent ELF, got nil")
	}
}

func TestMonitor_Validate_Valid(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test*.elf")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	monitor := &Monitor{
		config:  cfg,
		elfPath: tmpFile.Name(),
	}

	err = monitor.Validate()
	if err != nil {
		t.Errorf("Expected no error for valid config, got %v", err)
	}
}

func TestMonitor_IsRunning(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	if monitor.IsRunning() {
		t.Error("Expected IsRunning to return false for stopped monitor")
	}
}

func TestMonitor_Output(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	output := monitor.Output()
	if output == nil {
		t.Error("Expected output channel, got nil")
	}
}

func TestMonitor_GetConfig(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	retrievedCfg := monitor.GetConfig()
	if retrievedCfg == nil {
		t.Error("Expected config, got nil")
	}

	if retrievedCfg.ChipType != cfg.ChipType {
		t.Errorf("Expected ChipType %s, got %s", cfg.ChipType, retrievedCfg.ChipType)
	}
}

func TestMonitor_Send_NotSupported(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	err = monitor.Send([]byte("test"))
	if err == nil {
		t.Error("Expected error for Send (not supported), got nil")
	}
}

func TestMonitor_Reset_NotRunning(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "/tmp/test.elf")
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	err = monitor.Reset()
	if err == nil {
		t.Error("Expected error for Reset when not running, got nil")
	}
}

func TestMonitor_Start_NoELF(t *testing.T) {
	cfg := &protocol.WokwiConfig{
		ChipType:    "ESP32",
		DiagramJSON: `{"version":1}`,
	}

	monitor, err := NewMonitorFromConfig(cfg, "") // No ELF path
	if err != nil {
		t.Fatalf("NewMonitorFromConfig failed: %v", err)
	}

	ctx := context.Background()
	err = monitor.Start(ctx)
	if err == nil {
		t.Error("Expected error for Start without ELF, got nil")
	}
}

func TestCheckWokwiCliAvailable(t *testing.T) {
	// This test just checks that the function doesn't panic
	available := CheckWokwiCliAvailable()
	// Result depends on whether wokwi-cli is installed
	_ = available
}

func TestGetWokwiCliVersion(t *testing.T) {
	// This test just checks that the function doesn't panic
	// Result depends on whether wokwi-cli is installed
	_, err := GetWokwiCliVersion()
	// Error is expected if wokwi-cli is not installed
	_ = err
}
