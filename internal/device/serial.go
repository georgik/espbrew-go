package device

import (
	"strings"

	"go.bug.st/serial"
)

// Scanner finds USB serial devices
type Scanner struct{}

func NewScanner() *Scanner {
	return &Scanner{}
}

// Scan returns all connected serial ports
func (s *Scanner) Scan() ([]Port, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}

	result := make([]Port, 0, len(ports))
	for _, p := range ports {
		result = append(result, Port{
			Path: p,
		})
	}
	return result, nil
}

// ScanESP returns only ESP devices
func (s *Scanner) ScanESP() ([]DeviceInfo, error) {
	ports, err := s.Scan()
	if err != nil {
		return nil, err
	}

	result := make([]DeviceInfo, 0)
	for _, port := range ports {
		// Try to open and identify device
		if info, ok := s.identifyESP(port.Path); ok {
			result = append(result, info)
		}
	}
	return result, nil
}

// identifyESP attempts to identify if a port is an ESP device
func (s *Scanner) identifyESP(path string) (DeviceInfo, bool) {
	// Open port with common ESP baud rate
	mode := &serial.Mode{
		BaudRate: 115200,
	}
	port, err := serial.Open(path, mode)
	if err != nil {
		return DeviceInfo{}, false
	}
	defer port.Close()

	// For now, assume all serial ports could be ESP devices
	// TODO: Use USB VID/PID via platform-specific APIs
	return DeviceInfo{
		Path: path,
		VID:  ESP_VID,
		PID:  ESP_PID_S3, // Default to S3
	}, s.isLikelyESP(path)
}

// isLikelyESP heuristically determines if port is likely an ESP device
func (s *Scanner) isLikelyESP(path string) bool {
	espPatterns := []string{
		"usb", "UART", "SLAB", "CP21", "FTDI", "CH340",
		"ttyUSB", "ttyACM", "cu.usb", "cu.usbserial",
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
	Path string
}
