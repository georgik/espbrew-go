package device

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"
	"go.bug.st/serial"
)

// Scanner finds USB serial devices
type Scanner struct {
	resolver *DeviceResolver
}

func NewScanner() *Scanner {
	return &Scanner{
		resolver: NewDeviceResolver(),
	}
}

// Scan returns all connected serial ports with stable paths
func (s *Scanner) Scan() ([]Port, error) {
	// On Linux, use by-id for stable paths
	if runtime.GOOS == "linux" {
		return s.scanStable()
	}

	// Fallback for other platforms
	return s.scanLegacy()
}

// scanStable scans devices using /dev/serial/by-id/ on Linux
func (s *Scanner) scanStable() ([]Port, error) {
	byIDDir := "/dev/serial/by-id"
	entries, err := os.ReadDir(byIDDir)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to read by-id directory, falling back to legacy scan")
		return s.scanLegacy()
	}

	result := make([]Port, 0, len(entries))
	for _, entry := range entries {
		symlink := filepath.Join(byIDDir, entry.Name())
		target, err := os.Readlink(symlink)
		if err != nil {
			continue
		}

		// Resolve relative symlink target
		resolved := filepath.Join(filepath.Dir(symlink), target)
		result = append(result, Port{
			Path:     symlink,  // Stable by-id path
			RealPath: resolved, // Actual /dev/ttyUSB path
		})
	}
	return result, nil
}

// scanLegacy scans devices using go.bug.st/serial (fallback)
func (s *Scanner) scanLegacy() ([]Port, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}

	result := make([]Port, 0, len(ports))
	for _, p := range ports {
		result = append(result, Port{
			Path:     p,
			RealPath: p,
		})
	}
	return result, nil
}

// ScanDetailed returns all connected serial ports with device info
func (s *Scanner) ScanDetailed() ([]DeviceInfo, error) {
	// On Linux, use resolver for detailed info
	if runtime.GOOS == "linux" {
		return s.resolver.ScanStable()
	}

	// Fallback for other platforms
	ports, err := s.Scan()
	if err != nil {
		return nil, err
	}

	result := make([]DeviceInfo, 0, len(ports))
	for _, port := range ports {
		info := DeviceInfo{
			Path: port.Path,
		}
		result = append(result, info)
	}
	return result, nil
}

// ScanESP returns only ESP devices with detailed info
func (s *Scanner) ScanESP() ([]DeviceInfo, error) {
	// On Linux, use stable paths with device info
	if runtime.GOOS == "linux" {
		devices, err := s.ScanDetailed()
		if err != nil {
			return nil, err
		}

		// Filter for ESP devices
		result := make([]DeviceInfo, 0)
		for _, dev := range devices {
			if s.isLikelyESP(dev.Path) {
				// Set default VID/PID if not already set
				if dev.VID == 0 {
					dev.VID = ESP_VID
					dev.PID = ESP_PID_S3
				}
				result = append(result, dev)
			}
		}
		return result, nil
	}

	// Fallback for other platforms
	ports, err := s.Scan()
	if err != nil {
		return nil, err
	}

	result := make([]DeviceInfo, 0)
	for _, port := range ports {
		// Try to get device info
		info, err := s.resolver.GetDeviceInfo(port.RealPath)
		if err != nil {
			// Fallback to basic info
			info = &DeviceInfo{
				Path: port.Path,
				VID:  ESP_VID,
				PID:  ESP_PID_S3,
			}
		}

		// Set default VID/PID if not available (happens on Windows)
		if info.VID == 0 {
			info.VID = ESP_VID
			info.PID = ESP_PID_S3
		}

		// Check if likely ESP
		if s.isLikelyESP(port.Path) || IsESPDevice(info.VID, info.PID) {
			result = append(result, *info)
		}
	}
	return result, nil
}

// isLikelyESP heuristically determines if port is likely an ESP device
func (s *Scanner) isLikelyESP(path string) bool {
	espPatterns := []string{
		"usb", "UART", "SLAB", "CP21", "FTDI", "CH340",
		"ttyUSB", "ttyACM", "cu.usb", "cu.usbserial",
		"COM",               // Windows COM ports
		"Espressif", "303a", // Espressif VID
	}

	lower := strings.ToLower(path)
	for _, pattern := range espPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// Port represents a serial port
type Port struct {
	Path     string // Stable path (by-id on Linux)
	RealPath string // Actual /dev/tty path
}
