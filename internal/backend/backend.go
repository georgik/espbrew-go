package backend

import (
	"fmt"

	"codeberg.org/georgik/espbrew-go/internal/backend/physical"
	"codeberg.org/georgik/espbrew-go/internal/backend/wokwi"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

// Factory creates backend instances based on device configuration
type Factory struct{}

// NewFactory creates a new backend factory
func NewFactory() *Factory {
	return &Factory{}
}

// GetMonitor returns the appropriate monitor for the device backend
func (f *Factory) GetMonitor(device *protocol.DeviceInfo) (protocol.Monitor, error) {
	switch device.Backend {
	case protocol.BackendPhysical, "":
		return physical.NewPhysicalMonitor(device)
	case protocol.BackendWokwi:
		return wokwi.NewMonitor(device)
	case protocol.BackendQEMU:
		return NewQEMUMonitor(device)
	default:
		return physical.NewPhysicalMonitor(device) // Default to physical
	}
}

// GetFlasher returns the appropriate flasher for the device backend
func (f *Factory) GetFlasher(device *protocol.DeviceInfo) (protocol.Flasher, error) {
	switch device.Backend {
	case protocol.BackendPhysical, "":
		return physical.NewPhysicalFlasher(device)
	case protocol.BackendWokwi:
		return wokwi.NewFlasher(device)
	case protocol.BackendQEMU:
		return NewQEMUFlasher(device)
	default:
		return physical.NewPhysicalFlasher(device) // Default to physical
	}
}

// GetBackendConfig creates and validates backend config from raw data
func GetBackendConfig(backendType protocol.BackendType, configData map[string]interface{}) (protocol.BackendConfig, error) {
	switch backendType {
	case protocol.BackendWokwi:
		cfg := &protocol.WokwiConfig{}
		if chipType, ok := configData["chip_type"].(string); ok {
			cfg.ChipType = chipType
		}
		if diagram, ok := configData["diagram_json"].(string); ok {
			cfg.DiagramJSON = diagram
		}
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("invalid wokwi config: %w", err)
		}
		return cfg, nil

	case protocol.BackendQEMU:
		cfg := &protocol.QEMUConfig{}
		if machineType, ok := configData["machine_type"].(string); ok {
			cfg.MachineType = machineType
		}
		if memSize, ok := configData["memory_size"].(int); ok {
			cfg.MemorySize = memSize
		}
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("invalid qemu config: %w", err)
		}
		return cfg, nil

	default:
		return nil, fmt.Errorf("unknown backend type: %s", backendType)
	}
}

// NewQEMUMonitor is a stub for future QEMU support
func NewQEMUMonitor(device *protocol.DeviceInfo) (protocol.Monitor, error) {
	return nil, fmt.Errorf("qemu backend not yet implemented")
}

// NewQEMUFlasher is a stub for future QEMU support
func NewQEMUFlasher(device *protocol.DeviceInfo) (protocol.Flasher, error) {
	return nil, fmt.Errorf("qemu backend not yet implemented")
}
