package backend

import (
	"testing"

	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

func TestGetBackendConfig_Wokwi(t *testing.T) {
	configData := map[string]interface{}{
		"chip_type":    "ESP32",
		"diagram_json": `{"version":1}`,
	}

	cfg, err := GetBackendConfig(protocol.BackendWokwi, configData)
	if err != nil {
		t.Fatalf("GetBackendConfig failed: %v", err)
	}

	wokwiCfg, ok := cfg.(*protocol.WokwiConfig)
	if !ok {
		t.Fatalf("Expected WokwiConfig, got %T", cfg)
	}

	if wokwiCfg.ChipType != "ESP32" {
		t.Errorf("Expected ChipType ESP32, got %s", wokwiCfg.ChipType)
	}

	if wokwiCfg.DiagramJSON != `{"version":1}` {
		t.Errorf("Expected diagram_json, got %s", wokwiCfg.DiagramJSON)
	}
}

func TestGetBackendConfig_Wokwi_MissingChipType(t *testing.T) {
	configData := map[string]interface{}{
		"diagram_json": `{"version":1}`,
	}

	_, err := GetBackendConfig(protocol.BackendWokwi, configData)
	if err == nil {
		t.Error("Expected error for missing chip_type, got nil")
	}
}

func TestGetBackendConfig_Wokwi_MissingDiagram(t *testing.T) {
	configData := map[string]interface{}{
		"chip_type": "ESP32",
	}

	_, err := GetBackendConfig(protocol.BackendWokwi, configData)
	if err == nil {
		t.Error("Expected error for missing diagram_json, got nil")
	}
}

func TestGetBackendConfig_QEMU(t *testing.T) {
	configData := map[string]interface{}{
		"machine_type": "esp32",
		"memory_size":  4,
	}

	cfg, err := GetBackendConfig(protocol.BackendQEMU, configData)
	if err != nil {
		t.Fatalf("GetBackendConfig failed: %v", err)
	}

	qemuCfg, ok := cfg.(*protocol.QEMUConfig)
	if !ok {
		t.Fatalf("Expected QEMUConfig, got %T", cfg)
	}

	if qemuCfg.MachineType != "esp32" {
		t.Errorf("Expected MachineType esp32, got %s", qemuCfg.MachineType)
	}

	if qemuCfg.MemorySize != 4 {
		t.Errorf("Expected MemorySize 4, got %d", qemuCfg.MemorySize)
	}
}

func TestGetBackendConfig_UnknownBackend(t *testing.T) {
	configData := map[string]interface{}{}

	_, err := GetBackendConfig(protocol.BackendType("unknown"), configData)
	if err == nil {
		t.Error("Expected error for unknown backend type, got nil")
	}
}

func TestFactory_GetMonitor_Wokwi(t *testing.T) {
	factory := NewFactory()

	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1}`,
		},
	}

	monitor, err := factory.GetMonitor(device)
	if err != nil {
		t.Fatalf("GetMonitor failed: %v", err)
	}

	if monitor == nil {
		t.Error("Expected monitor, got nil")
	}
}

func TestFactory_GetMonitor_Physical(t *testing.T) {
	factory := NewFactory()

	device := &protocol.DeviceInfo{
		DeviceID: "test-physical",
		Path:     "/dev/ttyUSB0",
		Backend:  protocol.BackendPhysical,
	}

	monitor, err := factory.GetMonitor(device)
	if err != nil {
		t.Fatalf("GetMonitor failed: %v", err)
	}

	if monitor == nil {
		t.Error("Expected monitor, got nil")
	}
}

func TestFactory_GetFlasher_Wokwi(t *testing.T) {
	factory := NewFactory()

	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1}`,
		},
	}

	flasher, err := factory.GetFlasher(device)
	if err != nil {
		t.Fatalf("GetFlasher failed: %v", err)
	}

	if flasher == nil {
		t.Error("Expected flasher, got nil")
	}
}

func TestFactory_GetFlasher_Physical(t *testing.T) {
	factory := NewFactory()

	device := &protocol.DeviceInfo{
		DeviceID: "test-physical",
		Path:     "/dev/ttyUSB0",
		Backend:  protocol.BackendPhysical,
	}

	flasher, err := factory.GetFlasher(device)
	if err != nil {
		t.Fatalf("GetFlasher failed: %v", err)
	}

	if flasher == nil {
		t.Error("Expected flasher, got nil")
	}
}

func TestFactory_GetMonitor_QEMU_NotImplemented(t *testing.T) {
	factory := NewFactory()

	device := &protocol.DeviceInfo{
		DeviceID: "test-qemu",
		Backend:  protocol.BackendQEMU,
		BackendConfig: &protocol.QEMUConfig{
			MachineType: "esp32",
			MemorySize:  4,
		},
	}

	_, err := factory.GetMonitor(device)
	if err == nil {
		t.Error("Expected error for QEMU backend (not implemented), got nil")
	}
}

func TestFactory_GetMonitor_DefaultPhysical(t *testing.T) {
	factory := NewFactory()

	device := &protocol.DeviceInfo{
		DeviceID: "test-default",
		Path:     "/dev/ttyUSB0",
		Backend:  "", // Empty backend should default to physical
	}

	monitor, err := factory.GetMonitor(device)
	if err != nil {
		t.Fatalf("GetMonitor failed: %v", err)
	}

	if monitor == nil {
		t.Error("Expected monitor, got nil")
	}
}
