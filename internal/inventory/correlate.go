package inventory

import (
	"strings"

	"codeberg.org/georgik/espbrew-go/internal/device"
)

// PortRecord represents a single serial port with its probe results
type PortRecord struct {
	Path        string
	MAC         string
	Chip        string
	PortType    device.PortType
	VID         uint16
	PID         uint16
	USBSerial   string
	USBLocation string
	Product     string
	Identified  bool
	ProbeError  string
}

// BoardIdentity represents a single ESP board that may have multiple ports
type BoardIdentity struct {
	MAC     string
	Chip    string
	Ports   []PortRecord
	HasJTAG bool
}

// PortRecommendation suggests which port to use for operations
type PortRecommendation struct {
	FlashPort   string
	MonitorPort string
	Reason      string
}

// CorrelatePorts groups ports by MAC address to identify boards
func CorrelatePorts(records []PortRecord) ([]BoardIdentity, []PortRecord) {
	// Group identified ports by MAC
	grouped := make(map[string][]PortRecord)
	for _, r := range records {
		if r.Identified && r.MAC != "" {
			grouped[r.MAC] = append(grouped[r.MAC], r)
		}
	}

	var boards []BoardIdentity
	for mac, ports := range grouped {
		board := BoardIdentity{
			MAC:   mac,
			Ports: ports,
		}

		// Determine chip type from ports
		for _, p := range ports {
			if p.Chip != "" {
				board.Chip = p.Chip
				break
			}
		}

		// Check if board has JTAG
		board.HasJTAG = hasUSJPort(ports)

		boards = append(boards, board)
	}

	// Separate unidentified ports
	var unidentified []PortRecord
	for _, r := range records {
		if !r.Identified {
			unidentified = append(unidentified, r)
		}
	}

	return boards, unidentified
}

// AttachUnidentifiedUSJ attempts to attach unidentified USJ ports by USB serial
func AttachUnidentifiedUSJ(boards []BoardIdentity, unidentified []PortRecord) {
	for _, port := range unidentified {
		if port.PortType != device.PortTypeUSBSerialJTAG || port.USBSerial == "" {
			continue
		}

		// Normalize MAC addresses for comparison
		portMAC := normalizeMAC(port.USBSerial)

		// Find matching board by MAC
		for i, board := range boards {
			boardMAC := normalizeMAC(board.MAC)
			if boardMAC == portMAC {
				// Attach this port to the board
				boards[i].Ports = append(boards[i].Ports, port)
				boards[i].HasJTAG = true
				break
			}
		}
	}
}

// RecommendPort selects the best port for flashing and monitoring
func RecommendPort(ports []PortRecord) PortRecommendation {
	if len(ports) == 0 {
		return PortRecommendation{
			FlashPort:   "",
			MonitorPort: "",
			Reason:      "no_ports",
		}
	}

	// Priority 1: USB Serial/JTAG port (preferred for flash/monitor)
	usj := findUSJPort(ports)
	if usj != nil {
		return PortRecommendation{
			FlashPort:   usj.Path,
			MonitorPort: usj.Path,
			Reason:      "usb_serial_jtag_preferred",
		}
	}

	// Priority 2: Only identified port
	if len(ports) == 1 {
		p := ports[0]
		if p.Identified {
			return PortRecommendation{
				FlashPort:   p.Path,
				MonitorPort: p.Path,
				Reason:      "only_identified_port",
			}
		}
	}

	// Priority 3: First identified port
	for _, p := range ports {
		if p.Identified {
			return PortRecommendation{
				FlashPort:   p.Path,
				MonitorPort: p.Path,
				Reason:      "first_identified_port",
			}
		}
	}

	// Fallback: First port
	return PortRecommendation{
		FlashPort:   ports[0].Path,
		MonitorPort: ports[0].Path,
		Reason:      "fallback_first_port",
	}
}

// hasUSJPort checks if any port is USB Serial/JTAG
func hasUSJPort(ports []PortRecord) bool {
	for _, p := range ports {
		if p.PortType == device.PortTypeUSBSerialJTAG {
			return true
		}
	}
	return false
}

// findUSJPort returns the first USB Serial/JTAG port
func findUSJPort(ports []PortRecord) *PortRecord {
	for _, p := range ports {
		if p.PortType == device.PortTypeUSBSerialJTAG {
			return &p
		}
	}
	return nil
}

// normalizeMAC converts MAC to consistent lowercase format
func normalizeMAC(mac string) string {
	macLower := strings.ToLower(mac)
	// Remove common separators
	macLower = strings.ReplaceAll(macLower, ":", "")
	macLower = strings.ReplaceAll(macLower, "-", "")
	macLower = strings.ReplaceAll(macLower, ".", "")
	return macLower
}
