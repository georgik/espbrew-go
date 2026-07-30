//go:build linux
// +build linux

package powercontrol

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	usbDevicesPath = "/sys/bus/usb/devices"

	// USB hub class code
	usbClassHub = 0x09

	// Port status bits
	usbPortStatConnection  = 0x0001
	usbPortStatEnable      = 0x0002
	usbPortStatSuspend     = 0x0004
	usbPortStatOvercurrent = 0x0008
	usbPortStatReset       = 0x0010
	usbPortStatPower       = 0x0100 // USB 2.0
	usbPortStatLowSpeed    = 0x0200
	usbPortStatHighSpeed   = 0x0400

	// USB 3.0 port status bits
	usbSsPortStatPower    = 0x0200
	usbPortStatLinkState  = 0x01e0
	usbPortStatSpeed5Gbps = 0x1c00
)

var (
	hubCache      []Hub
	hubCacheMutex sync.RWMutex
	hubCacheValid bool
)

func init() {
	listHubs = listHubsLinux
	setPortPower = setPortPowerIoctl
	getPortStatus = getPortStatusIoctl
}

// invalidateCache invalidates the hub cache.
func invalidateCache() {
	hubCacheMutex.Lock()
	defer hubCacheMutex.Unlock()
	hubCacheValid = false
}

// listHubsLinux returns all USB hubs with per-port power switching.
func listHubsLinux() ([]Hub, error) {
	hubCacheMutex.RLock()
	if hubCacheValid {
		hubs := make([]Hub, len(hubCache))
		copy(hubs, hubCache)
		hubCacheMutex.RUnlock()
		return hubs, nil
	}
	hubCacheMutex.RUnlock()

	entries, err := os.ReadDir(usbDevicesPath)
	if err != nil {
		return nil, fmt.Errorf("read usb devices: %w", err)
	}

	var hubs []Hub
	for _, entry := range entries {
		// Skip interface directories (contain colon)
		if strings.Contains(entry.Name(), ":") {
			continue
		}

		hubPath := filepath.Join(usbDevicesPath, entry.Name())

		// Check if it's a hub
		hub, err := parseHub(entry.Name(), hubPath)
		if err != nil {
			continue // Not a hub or error, skip
		}

		hubs = append(hubs, hub)
	}

	// Cache the results
	hubCacheMutex.Lock()
	hubCache = hubs
	hubCacheValid = true
	hubCacheMutex.Unlock()

	return hubs, nil
}

// parseHub parses a hub from the sysfs directory.
func parseHub(name, path string) (Hub, error) {
	// Read vendor ID
	vendorBytes, err := os.ReadFile(filepath.Join(path, "idVendor"))
	if err != nil {
		return Hub{}, fmt.Errorf("read vendor: %w", err)
	}
	vendor := strings.TrimSpace(string(vendorBytes))

	// Read product ID
	productBytes, err := os.ReadFile(filepath.Join(path, "idProduct"))
	if err != nil {
		return Hub{}, fmt.Errorf("read product: %w", err)
	}
	product := strings.TrimSpace(string(productBytes))

	// Check device class
	classBytes, err := os.ReadFile(filepath.Join(path, "bDeviceClass"))
	if err != nil {
		return Hub{}, fmt.Errorf("read device class: %w", err)
	}
	class, err := strconv.ParseInt(strings.TrimSpace(string(classBytes)), 16, 32)
	if err != nil {
		return Hub{}, fmt.Errorf("parse device class: %w", err)
	}

	// Must be a hub
	if int(class) != usbClassHub {
		return Hub{}, fmt.Errorf("not a hub")
	}

	// Find the hub interface
	interfaceDir, numPorts, superSpeed, err := findHubInterface(path, name)
	if err != nil {
		return Hub{}, fmt.Errorf("find hub interface: %w", err)
	}

	return Hub{
		Location:     name,
		Vendor:       vendor,
		Product:      product,
		NumPorts:     numPorts,
		interfaceDir: interfaceDir,
		SuperSpeed:   superSpeed,
	}, nil
}

// findHubInterface finds the hub interface and counts ports.
func findHubInterface(path, name string) (string, int, bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", 0, false, err
	}

	var interfaceDir string
	var superSpeed bool
	var ssInterfaceDir string // Track SuperSpeed interface separately

	// Check USB version as fallback for SuperSpeed detection
	// USB 3.x hubs have version >= 3.0
	if versionBytes, err := os.ReadFile(filepath.Join(path, "version")); err == nil {
		version := strings.TrimSpace(string(versionBytes))
		// Parse version like "3.20" or "2.10"
		var major, minor int
		if _, err := fmt.Sscanf(version, "%d.%d", &major, &minor); err == nil {
			if major >= 3 {
				superSpeed = true
			}
		}
	}

	// First pass: check hub interfaces
	for _, entry := range entries {
		if !strings.Contains(entry.Name(), ":") {
			continue
		}

		// Check if it's a hub interface
		intfPath := filepath.Join(path, entry.Name())
		classBytes, err := os.ReadFile(filepath.Join(intfPath, "bInterfaceClass"))
		if err != nil {
			continue
		}
		class, _ := strconv.ParseInt(strings.TrimSpace(string(classBytes)), 16, 32)
		if int(class) != usbClassHub {
			continue
		}

		// Track SuperSpeed interface
		if strings.HasSuffix(entry.Name(), ":2.0") {
			ssInterfaceDir = entry.Name()
			superSpeed = true
		}

		// Use first hub interface as fallback
		if interfaceDir == "" {
			interfaceDir = entry.Name()
		}
	}

	// Use SuperSpeed interface if found, otherwise use the first one
	if superSpeed && ssInterfaceDir != "" {
		interfaceDir = ssInterfaceDir
	}

	if interfaceDir == "" {
		return "", 0, false, fmt.Errorf("no hub interface found")
	}

	// Count ports by looking for port directories
	intfPath := filepath.Join(path, interfaceDir)
	entries, err = os.ReadDir(intfPath)
	if err != nil {
		return "", 0, false, err
	}

	portCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), name+"-port") {
			portCount++
		}
	}

	if portCount == 0 {
		return "", 0, false, fmt.Errorf("no ports found")
	}

	return interfaceDir, portCount, superSpeed, nil
}

// setPortPowerLinux sets power state for a port.
func setPortPowerLinux(hub *Hub, port int, on bool) error {
	disablePath := filepath.Join(
		usbDevicesPath,
		hub.interfaceDir,
		fmt.Sprintf("%s-port%d", hub.Location, port),
		"disable",
	)

	// Check if file exists (kernel >= 6.0)
	if _, err := os.Stat(disablePath); os.IsNotExist(err) {
		return ErrKernelTooOld
	}

	// Write power state
	value := "1"
	if on {
		value = "0"
	}

	if err := os.WriteFile(disablePath, []byte(value), 0644); err != nil {
		if os.IsPermission(err) {
			return ErrPermissionDenied
		}
		return fmt.Errorf("write disable: %w", err)
	}

	// Invalidate cache after power change
	invalidateCache()

	return nil
}

// getPortStatusLinux gets the current status of a port.
func getPortStatusLinux(hub *Hub, port int) (*PortStatus, error) {
	statusPath := filepath.Join(
		usbDevicesPath,
		hub.interfaceDir,
		fmt.Sprintf("%s-port%d", hub.Location, port),
		"status",
	)

	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil, fmt.Errorf("read status: %w", err)
	}

	// Parse hex status
	statusStr := strings.TrimSpace(string(data))
	status, err := strconv.ParseInt(statusStr, 0, 32)
	if err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}

	// Decode status bits
	ps := &PortStatus{
		Port:      port,
		Connected: (status & usbPortStatConnection) != 0,
		Enabled:   (status & usbPortStatEnable) != 0,
	}

	// Power status depends on USB version
	if hub.SuperSpeed {
		ps.Power = (status & usbSsPortStatPower) != 0

		// Decode speed for USB 3.0
		linkState := status & usbPortStatLinkState
		switch {
		case linkState == 0 && (status&usbPortStatSpeed5Gbps) != 0:
			ps.Speed = "5gbps"
		default:
			ps.Speed = "unknown"
		}
	} else {
		ps.Power = (status & usbPortStatPower) != 0

		// Decode speed for USB 2.0
		switch {
		case (status & usbPortStatLowSpeed) != 0:
			ps.Speed = "low"
		case (status & usbPortStatHighSpeed) != 0:
			ps.Speed = "high"
		case ps.Connected:
			ps.Speed = "full"
		}
	}

	return ps, nil
}
