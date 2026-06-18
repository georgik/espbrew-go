package inventory

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.bug.st/serial"
)

// BootLogInfo contains device information parsed from boot log
type BootLogInfo struct {
	ChipType   string
	ChipRev    string
	FlashSize  uint32
	PSRAMSize  uint32
	Frequency  uint32
	MAC        string
	Strapping  string
	BootMode   string
	ResetCause string
}

// Boot patterns from ESP boot ROM
var (
	patternChip     = regexp.MustCompile(`ESP-ROM:(\w+)-Seq`)
	patternCPU      = regexp.MustCompile(`CPU:\s*(\d+)`)
	patternChipID   = regexp.MustCompile(`Chip:\s*(\w+)`)
	patternFeatures = regexp.MustCompile(`Features:\s*(.+)`)
	patternFreq     = regexp.MustCompile(`Freq:\s*(\d+)\s*MHz`)
	patternFreqAlt  = regexp.MustCompile(`Clock:\s*(\d+)MHz`)
	patternMAC      = regexp.MustCompile(`MAC:\s*([0-9a-fA-F:]{17})`)
	patternStrap    = regexp.MustCompile(`Strapping:\s*(0x[0-9a-fA-F]+)`)
	patternBoot     = regexp.MustCompile(`boot:0x([0-9a-fA-F]+)\s*\((\w+)\)`)
	patternReset    = regexp.MustCompile(`rst:0x([0-9a-fA-F]+)\s*\((\w+)\)`)
)

// MonitorBootLog opens a port, toggles DTR to reset device, and parses boot log
func MonitorBootLog(port string, timeout time.Duration) (*BootLogInfo, error) {
	mode := &serial.Mode{
		BaudRate: 115200,
	}

	p, err := serial.Open(port, mode)
	if err != nil {
		return nil, fmt.Errorf("open port: %w", err)
	}
	defer func() { _ = p.Close() }()

	// Set read timeout
	_ = p.SetReadTimeout(timeout)

	// Toggle DTR to reset device
	_ = p.SetDTR(false)
	time.Sleep(50 * time.Millisecond)
	_ = p.SetDTR(true)
	time.Sleep(50 * time.Millisecond)
	_ = p.SetDTR(false)

	info := &BootLogInfo{}
	scanner := bufio.NewScanner(p)
	deadline := time.Now().Add(timeout)

	for scanner.Scan() && time.Now().Before(deadline) {
		line := scanner.Text()
		parseBootLine(line, info)

		// If we have enough info, we can stop
		if info.ChipType != "" && info.MAC != "" {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read boot log: %w", err)
	}

	// Derive additional info from chip type
	if info.ChipType != "" {
		info.ChipType = normalizeChipName(info.ChipType)
		// Estimate flash size from features
		if info.FlashSize == 0 && info.ChipType != "" {
			info.FlashSize = estimateFlashSize(info.ChipType)
		}
	}

	if info.ChipType == "" {
		return nil, fmt.Errorf("could not identify chip from boot log")
	}

	return info, nil
}

func parseBootLine(line string, info *BootLogInfo) {
	line = strings.TrimSpace(line)

	// Chip type from ROM header
	if matches := patternChip.FindStringSubmatch(line); matches != nil {
		info.ChipType = matches[1]
	}

	// CPU ID (can help distinguish chip variants)
	if matches := patternCPU.FindStringSubmatch(line); matches != nil {
		// CPU[0] or just CPU: 0
	}

	// Chip ID (hex value)
	if matches := patternChipID.FindStringSubmatch(line); matches != nil {
		// Chip ID for strapping interpretation
		info.Strapping = matches[1]
	}

	// Features (PSRAM, etc.)
	if matches := patternFeatures.FindStringSubmatch(line); matches != nil {
		features := matches[1]
		if strings.Contains(features, "PSRAM") || strings.Contains(features, "SPIRAM") {
			// Parse PSRAM size from features
			if strings.Contains(features, "8MB") {
				info.PSRAMSize = 8 * 1024 * 1024
			} else if strings.Contains(features, "2MB") {
				info.PSRAMSize = 2 * 1024 * 1024
			} else if strings.Contains(features, "4MB") {
				info.PSRAMSize = 4 * 1024 * 1024
			}
		}
	}

	// Frequency
	if matches := patternFreq.FindStringSubmatch(line); matches != nil {
		_, _ = fmt.Sscanf(matches[1], "%d", &info.Frequency)
	} else if matches := patternFreqAlt.FindStringSubmatch(line); matches != nil {
		_, _ = fmt.Sscanf(matches[1], "%d", &info.Frequency)
	}

	// MAC address
	if matches := patternMAC.FindStringSubmatch(line); matches != nil {
		info.MAC = matches[1]
	}

	// Boot mode
	if matches := patternBoot.FindStringSubmatch(line); matches != nil {
		info.BootMode = matches[2]
	}

	// Reset cause
	if matches := patternReset.FindStringSubmatch(line); matches != nil {
		info.ResetCause = matches[2]
	}
}

func normalizeChipName(name string) string {
	// ESP boot ROM uses various names
	name = strings.ToLower(name)
	if strings.Contains(name, "esp32s3") || name == "esp32s3" {
		return "ESP32-S3"
	}
	if strings.Contains(name, "esp32s2") || name == "esp32s2" {
		return "ESP32-S2"
	}
	if strings.Contains(name, "esp32c3") || name == "esp32c3" {
		return "ESP32-C3"
	}
	if strings.Contains(name, "esp32c6") || name == "esp32c6" {
		return "ESP32-C6"
	}
	if strings.Contains(name, "esp32h2") || name == "esp32h2" {
		return "ESP32-H2"
	}
	if strings.Contains(name, "esp32") {
		return "ESP32"
	}
	return strings.ToUpper(name)
}

func estimateFlashSize(chipType string) uint32 {
	// Default flash sizes based on common modules
	switch chipType {
	case "ESP32-S3":
		return 8 * 1024 * 1024 // Common: 8MB
	case "ESP32-S2":
		return 4 * 1024 * 1024
	case "ESP32-C3":
		return 4 * 1024 * 1024
	case "ESP32-C6":
		return 4 * 1024 * 1024
	case "ESP32-H2":
		return 4 * 1024 * 1024
	case "ESP32":
		return 4 * 1024 * 1024
	default:
		return 4 * 1024 * 1024
	}
}

// BootLogToIdentity converts BootLogInfo to DeviceIdentity
func BootLogToIdentity(info *BootLogInfo, path string) *DeviceIdentity {
	identity := &DeviceIdentity{
		MAC:       info.MAC,
		Chip:      info.ChipType,
		ChipMajor: 0, // Boot log doesn't always have revision
		ChipMinor: 0,
		FlashSize: info.FlashSize,
		PSRAMSize: info.PSRAMSize,
	}

	// Try to get revision from chip type if available
	if strings.Contains(info.ChipType, "ESP32-S3") {
		identity.Chip = "ESP32-S3"
	}

	return identity
}
