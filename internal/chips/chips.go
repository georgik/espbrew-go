package chips

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
