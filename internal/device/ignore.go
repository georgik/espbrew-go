package device

import (
	"strings"
)

// ignoredPortPatterns are case-insensitive substrings that identify serial
// ports which are definitely NOT ESP32 flashing devices. Any port whose path
// (or device name) contains one of these patterns is skipped during scanning
// so it never surfaces to the user.
//
// The most common offender is macOS's Bluetooth serial service port,
// "/dev/cu.Bluetooth-Incoming-Port", which appears alongside real USB-serial
// devices when calling serial.GetPortsList(). It is not a flashable USB-serial
// port, so we skip it outright instead of bothering the user with it.
var ignoredPortPatterns = []string{
	"bluetooth",     // Bluetooth serial/HID services (e.g. Bluetooth-Incoming-Port)
	"incoming",      // Bluetooth-Incoming-Port specifically
	"buds",          // wireless earbuds devices
	"debug-console", // Espressif "debug-console" USB port (not the flashing port)
}

// IsIgnoredPort reports whether the given device path should be skipped during
// device scanning because it is definitely not an ESP32 flashing device.
//
// Both the stable path (e.g. /dev/cu.Bluetooth-Incoming-Port) and the real
// path are checked so the filter works regardless of which representation is
// available on the current platform.
func IsIgnoredPort(path string) bool {
	if path == "" {
		return false
	}

	lower := strings.ToLower(path)
	for _, pattern := range ignoredPortPatterns {
		if pattern != "" && strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// AddIgnoredPortPattern appends a new case-insensitive substring to the ignore
// list. Ports whose path contains the pattern are skipped during scanning.
//
// The function is idempotent: adding an already-present pattern is a no-op. It
// is primarily useful for tests and for callers that want to extend the default
// ignore list at runtime.
func AddIgnoredPortPattern(pattern string) {
	pattern = strings.TrimSpace(strings.ToLower(pattern))
	if pattern == "" {
		return
	}
	for _, existing := range ignoredPortPatterns {
		if existing == pattern {
			return
		}
	}
	ignoredPortPatterns = append(ignoredPortPatterns, pattern)
}

// FilterIgnoredPorts returns a new slice with every ignored port removed. Ports
// that are not ignored are preserved in their original order. A nil or empty
// input is returned unchanged.
func FilterIgnoredPorts(ports []Port) []Port {
	if len(ports) == 0 {
		return ports
	}

	result := make([]Port, 0, len(ports))
	for _, p := range ports {
		if IsIgnoredPort(p.Path) || IsIgnoredPort(p.RealPath) {
			continue
		}
		result = append(result, p)
	}
	return result
}
