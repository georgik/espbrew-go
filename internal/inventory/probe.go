package inventory

import (
	"fmt"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/inventory/rom"
)

// ProbeStatus represents the result of a probe attempt
type ProbeStatus string

const (
	ProbeStatusIdentified   ProbeStatus = "identified"
	ProbeStatusUnidentified ProbeStatus = "unidentified"
	ProbeStatusError        ProbeStatus = "error"
	ProbeStatusTimeout      ProbeStatus = "timeout"
)

// ProbeResult contains the probe outcome with status information
type ProbeResult struct {
	Status   ProbeStatus
	Identity *DeviceIdentity
	Error    string
	Port     string
	Duration time.Duration
}

// ProbeDeviceQuick reads device identity with short timeout.
// Use this immediately after device connect when device is in bootloader mode.
func ProbeDeviceQuick(port string) (*DeviceIdentity, error) {
	cfg := &rom.Config{
		BaudRate: 115200,
		Timeout:  500 * time.Millisecond, // Short timeout for quick probe
		Debug:    false,
	}

	conn, err := rom.Open(port, cfg)
	if err != nil {
		return nil, fmt.Errorf("open port: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Synchronize with bootloader
	if err := conn.Sync(); err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}

	// Detect chip type
	if err := conn.DetectChip(); err != nil {
		return nil, fmt.Errorf("detect chip: %w", err)
	}

	// Read MAC address
	mac, err := conn.ReadMAC()
	if err != nil {
		return nil, fmt.Errorf("read MAC: %w", err)
	}

	identity := &DeviceIdentity{
		MAC:  mac,
		Chip: conn.ChipType(),
	}

	// Read additional properties if supported (quick read, skip errors)
	if chip := conn.Chip(); chip != nil {
		psramSize, psramType, _ := chip.ReadPSRAM(conn)
		identity.PSRAMSize = psramSize
		identity.PSRAMType = psramType

		flashSize, _ := chip.ReadFlash(conn)
		identity.FlashSize = flashSize

		major, minor, _ := chip.ReadRevision(conn)
		identity.ChipMajor = major
		identity.ChipMinor = minor
	}

	return identity, nil
}

// ProbeDevice reads device identity from the given port using ROM protocol
func ProbeDevice(port string) (*DeviceIdentity, error) {
	cfg := &rom.Config{
		BaudRate: 115200,
		Timeout:  3 * time.Second,
		Debug:    false,
	}

	conn, err := rom.Open(port, cfg)
	if err != nil {
		return nil, fmt.Errorf("open port: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Synchronize with bootloader
	if err := conn.Sync(); err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}

	// Detect chip type
	if err := conn.DetectChip(); err != nil {
		return nil, fmt.Errorf("detect chip: %w", err)
	}

	// Read MAC address
	mac, err := conn.ReadMAC()
	if err != nil {
		return nil, fmt.Errorf("read MAC: %w", err)
	}

	identity := &DeviceIdentity{
		MAC:  mac,
		Chip: conn.ChipType(),
	}

	// Read additional properties if supported
	if chip := conn.Chip(); chip != nil {
		// Read PSRAM (may return 0, "" if not supported)
		psramSize, psramType, err := chip.ReadPSRAM(conn)
		if err == nil {
			identity.PSRAMSize = psramSize
			identity.PSRAMType = psramType
		}

		// Read Flash (may return 0 if not supported)
		flashSize, err := chip.ReadFlash(conn)
		if err == nil {
			identity.FlashSize = flashSize
		}

		// Read Revision
		major, minor, err := chip.ReadRevision(conn)
		if err == nil {
			identity.ChipMajor = major
			identity.ChipMinor = minor
		}
	}

	return identity, nil
}

// ProbeDeviceWithConfig reads device identity with custom configuration
func ProbeDeviceWithConfig(port string, cfg *rom.Config) (*DeviceIdentity, error) {
	if cfg == nil {
		cfg = rom.DefaultConfig()
	}

	conn, err := rom.Open(port, cfg)
	if err != nil {
		return nil, fmt.Errorf("open port: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.Sync(); err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}

	if err := conn.DetectChip(); err != nil {
		return nil, fmt.Errorf("detect chip: %w", err)
	}

	mac, err := conn.ReadMAC()
	if err != nil {
		return nil, fmt.Errorf("read MAC: %w", err)
	}

	identity := &DeviceIdentity{
		MAC:  mac,
		Chip: conn.ChipType(),
	}

	if chip := conn.Chip(); chip != nil {
		psramSize, psramType, _ := chip.ReadPSRAM(conn)
		identity.PSRAMSize = psramSize
		identity.PSRAMType = psramType

		flashSize, _ := chip.ReadFlash(conn)
		identity.FlashSize = flashSize

		major, minor, _ := chip.ReadRevision(conn)
		identity.ChipMajor = major
		identity.ChipMinor = minor
	}

	return identity, nil
}

// ProbeFromBootLog reads device identity by monitoring boot log after reset
// This works even if device is running an application (no manual bootloader entry needed)
func ProbeFromBootLog(port string) (*DeviceIdentity, error) {
	info, err := MonitorBootLog(port, 3*time.Second)
	if err != nil {
		return nil, err
	}

	return BootLogToIdentity(info, port), nil
}

// ProbeFromBootLogWithTimeout reads device identity with custom timeout
func ProbeFromBootLogWithTimeout(port string, timeout time.Duration) (*DeviceIdentity, error) {
	info, err := MonitorBootLog(port, timeout)
	if err != nil {
		return nil, err
	}

	return BootLogToIdentity(info, port), nil
}

// ProbeDeviceWithStatus probes a device and returns detailed status
func ProbeDeviceWithStatus(port string) *ProbeResult {
	start := time.Now()
	identity, err := ProbeDevice(port)
	duration := time.Since(start)

	if err != nil {
		status := ProbeStatusUnidentified
		if timeoutErr, ok := err.(interface{ Timeout() bool }); ok && timeoutErr.Timeout() {
			status = ProbeStatusTimeout
		}
		return &ProbeResult{
			Status:   status,
			Error:    err.Error(),
			Port:     port,
			Duration: duration,
		}
	}

	return &ProbeResult{
		Status:   ProbeStatusIdentified,
		Identity: identity,
		Port:     port,
		Duration: duration,
	}
}

// ProbeFromBootLogWithStatus probes via boot log and returns detailed status
func ProbeFromBootLogWithStatus(port string, timeout time.Duration) *ProbeResult {
	start := time.Now()
	identity, err := ProbeFromBootLogWithTimeout(port, timeout)
	duration := time.Since(start)

	if err != nil {
		status := ProbeStatusUnidentified
		if timeoutErr, ok := err.(interface{ Timeout() bool }); ok && timeoutErr.Timeout() {
			status = ProbeStatusTimeout
		}
		return &ProbeResult{
			Status:   status,
			Error:    err.Error(),
			Port:     port,
			Duration: duration,
		}
	}

	return &ProbeResult{
		Status:   ProbeStatusIdentified,
		Identity: identity,
		Port:     port,
		Duration: duration,
	}
}
