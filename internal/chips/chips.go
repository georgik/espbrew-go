package chips

import "strings"

// Chip type for ESP chips
type Chip int

const (
	ChipESP32 Chip = iota
	ChipESP32S2
	ChipESP32S3
	ChipESP32C3
	ChipESP32C6
	ChipESP32H2
	ChipESP32C2
	ChipESP32C5
	ChipESP32C61
	ChipESP32P4
)

// String returns the chip name as a string
func (c Chip) String() string {
	switch c {
	case ChipESP32:
		return "esp32"
	case ChipESP32S2:
		return "esp32s2"
	case ChipESP32S3:
		return "esp32s3"
	case ChipESP32C3:
		return "esp32c3"
	case ChipESP32C6:
		return "esp32c6"
	case ChipESP32H2:
		return "esp32h2"
	case ChipESP32C2:
		return "esp32c2"
	case ChipESP32C5:
		return "esp32c5"
	case ChipESP32C61:
		return "esp32c61"
	case ChipESP32P4:
		return "esp32p4"
	default:
		return "unknown"
	}
}

// ParseChip parses a chip type string (e.g. "ESP32-C3", "esp32s3", "ESP32")
// into a Chip. It is case-insensitive and tolerates hyphens, so it accepts both
// the registry/API form ("ESP32-C3") and the lowercase String() form ("esp32c3").
// The second return value reports whether the string matched a known chip.
func ParseChip(s string) (Chip, bool) {
	key := chipKey(s)
	if c, ok := chipKeyIndex[key]; ok {
		return c, true
	}
	return 0, false
}

// chipKey normalizes a chip name for lookup: lowercase, trimmed, hyphens
// removed (e.g. "ESP32-C3" -> "esp32c3").
func chipKey(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "")
}

var chipKeyIndex = func() map[string]Chip {
	m := make(map[string]Chip)
	known := []Chip{
		ChipESP32, ChipESP32S2, ChipESP32S3, ChipESP32C3, ChipESP32C6,
		ChipESP32H2, ChipESP32C2, ChipESP32C5, ChipESP32C61, ChipESP32P4,
	}
	for _, c := range known {
		m[chipKey(c.String())] = c
	}
	return m
}()

// ESPChipID returns the actual ESP chip ID for image encoding
func (c Chip) ESPChipID() uint16 {
	switch c {
	case ChipESP32:
		return 0
	case ChipESP32S2:
		return 2
	case ChipESP32S3:
		return 9
	case ChipESP32C3:
		return 5
	case ChipESP32C6:
		return 13
	case ChipESP32H2:
		return 12
	case ChipESP32C2:
		return 10
	case ChipESP32C5:
		return 6
	case ChipESP32C61:
		return 19
	case ChipESP32P4:
		return 16
	default:
		return 0
	}
}
