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

// linkDualHubs finds and links dual-interface hub counterparts.
// Identifies USB 2.0 and USB 3.0 interfaces of the same physical hub.
func linkDualHubs(hub *Hub) {
	if hub.Counterpart != nil {
		return // Already linked
	}

	hubs, err := listHubsLinux()
	if err != nil {
		return
	}

	var candidates []*Hub
	for _, other := range hubs {
		// Skip self
		if other.Location == hub.Location {
			continue
		}

		// Only match hubs at same hierarchy level (same number of dots)
		if strings.Count(hub.Location, ".") != strings.Count(other.Location, ".") {
			continue
		}

		// Must be same vendor
		if other.Vendor != hub.Vendor {
			continue
		}

		// Must be different SuperSpeed state
		if other.SuperSpeed == hub.SuperSpeed {
			continue
		}

		// Check if related via sysfs parentage
		if areDualHubPartners(hub, &other) {
			candidates = append(candidates, &other)
		}
	}

	// Select best candidate by downstream hub port matching
	if len(candidates) > 0 {
		best := selectBestCounterpart(hub, candidates)
		hub.Counterpart = best
		// Link back
		otherHub := findHubInCache(best.Location)
		if otherHub != nil {
			otherHub.Counterpart = hub
		}
	}
}

// selectBestCounterpart selects the best counterpart from candidates.
// Prefers hubs with downstream hubs on the same port numbers.
func selectBestCounterpart(hub *Hub, candidates []*Hub) *Hub {
	// Check for candidates with downstream hubs on the same ports
	hubPorts := getDownstreamHubPorts(hub.Location)

	var best *Hub
	var bestScore int = -1

	for _, c := range candidates {
		cPorts := getDownstreamHubPorts(c.Location)
		score := countMatchingPorts(hubPorts, cPorts)
		if score > bestScore {
			bestScore = score
			best = c
		}
	}

	if best == nil && len(candidates) > 0 {
		best = candidates[0]
	}
	return best
}

// getDownstreamHubPorts returns set of port numbers that have downstream hubs.
func getDownstreamHubPorts(hubLoc string) map[int]bool {
	ports := make(map[int]bool)
	hubPath := filepath.Join(usbDevicesPath, hubLoc)

	entries, _ := os.ReadDir(hubPath)
	for _, entry := range entries {
		// Check if it's a downstream hub (format: X-Y.Z)
		name := entry.Name()
		if strings.Contains(name, ".") && !strings.Contains(name, ":") {
			// Parse port number from "1-5.3" -> 3
			parts := strings.Split(name, ".")
			if len(parts) >= 2 {
				var port int
				fmt.Sscanf(parts[1], "%d", &port)
				ports[port] = true
			}
		}
	}
	return ports
}

// countMatchingPorts counts how many port numbers are in both sets.
func countMatchingPorts(a, b map[int]bool) int {
	count := 0
	for port := range a {
		if b[port] {
			count++
		}
	}
	return count
}

// areDualHubPartners checks if two hubs are dual-interface partners.
func areDualHubPartners(h1, h2 *Hub) bool {
	// Check sysfs hierarchy: USB 3.0 hub is often child of USB 2.0 hub
	// or they share the same parent port

	h1Path := filepath.Join(usbDevicesPath, h1.Location)
	h2Path := filepath.Join(usbDevicesPath, h2.Location)

	// Read parent/child relationship
	// For dual hubs: check if they're on same physical port
	// by comparing bus number and device number

	h1Bus, _ := parseLocationToBusDevice(h1.Location)
	h2Bus, _ := parseLocationToBusDevice(h2.Location)

	// If same bus and device-1, they might be dual (USB 3.0 appears as child)
	if h1Bus == h2Bus {
		// Check if one is parent of other in sysfs
		if isChildHub(h1Path, h2.Location) || isChildHub(h2Path, h1.Location) {
			return true
		}

		// Alternative: check for product ID pair pattern
		// Realtek: 0411 (USB 3.0) pairs with 5411 (USB 2.0)
		if isRealtekDualPair(h1.Product, h2.Product) {
			return true
		}
	}

	// Try Realtek pair check even on different buses (USB 3.0 hubs are on separate bus)
	if isRealtekDualPair(h1.Product, h2.Product) {
		return true
	}

	return false
}

// isChildHub checks if a hub location is a child of the given path.
func isChildHub(parentPath, childLoc string) bool {
	// Check if childLoc appears as a subdirectory under parentPath
	entries, err := os.ReadDir(parentPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.Name() == childLoc {
			return true
		}
	}
	return false
}

// isRealtekDualPair checks if product IDs form a Realtek dual-hub pair.
func isRealtekDualPair(p1, p2 string) bool {
	// Realtek USB hub dual-interface pairs:
	// 0x5411 (USB 2.0) ↔ 0x0411 (USB 3.0)
	// 0x5413 (USB 2.0) ↔ 0x0413 (USB 3.0)
	// 0x5414 (USB 2.0) ↔ 0x0414 (USB 3.0)

	pairs := [][2]string{
		{"5411", "0411"},
		{"5413", "0413"},
		{"5414", "0414"},
	}

	for _, pair := range pairs {
		if (p1 == pair[0] && p2 == pair[1]) || (p1 == pair[1] && p2 == pair[0]) {
			return true
		}
	}
	return false
}

// findHubInCache finds a hub pointer in the cache by location.
func findHubInCache(loc string) *Hub {
	hubCacheMutex.RLock()
	defer hubCacheMutex.RUnlock()

	for i := range hubCache {
		if hubCache[i].Location == loc {
			return &hubCache[i]
		}
	}
	return nil
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

		// Skip parent hubs that have downstream sub-hubs on their ports.
		// Only control leaf hubs (hubs with no downstream hubs).
		if hasDownstreamHub(entry.Name(), hubPath) {
			continue
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

// hasDownstreamHub checks if a hub has any downstream sub-hubs on its ports.
// A downstream hub appears as a subdirectory with format "X-Y.Z" (contains dot but not colon)
// and has bDeviceClass == 0x09 (USB hub class).
func hasDownstreamHub(hubLoc string, hubPath string) bool {
	entries, err := os.ReadDir(hubPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		name := entry.Name()
		// Skip interface directories (contain colon)
		if strings.Contains(name, ":") {
			continue
		}
		// Check if it looks like a sub-hub: contains dot
		if !strings.Contains(name, ".") {
			continue
		}

		// Verify it's actually a hub by checking bDeviceClass
		subPath := filepath.Join(hubPath, name)
		classBytes, err := os.ReadFile(filepath.Join(subPath, "bDeviceClass"))
		if err != nil {
			continue
		}
		classStr := strings.TrimSpace(string(classBytes))
		class, err := strconv.ParseInt(classStr, 16, 32)
		if err != nil || int(class) != usbClassHub {
			continue
		}

		return true
	}

	return false
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
