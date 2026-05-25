package flash

// Bootloader offsets from espflasher chip definitions.
// These are the flash offsets where bootloader images are located.
const (
	BootloaderOffsetESP8266     = 0x0
	BootloaderOffsetESP32       = 0x1000
	BootloaderOffsetESP32S2     = 0x1000
	BootloaderOffsetESP32S3     = 0x0
	BootloaderOffsetESP32C2     = 0x0
	BootloaderOffsetESP32C3     = 0x0
	BootloaderOffsetESP32C5     = 0x2000
	BootloaderOffsetESP32C6     = 0x0
	BootloaderOffsetESP32H2     = 0x0
	BootloaderOffsetESP32P4Rev1 = 0x2000
)

// Common preset offsets
const (
	PresetOffsetPartitions = 0x8000
	PresetOffsetApp        = 0x10000
)

// BootloaderOffset returns the bootloader flash offset for a given chip name.
// Returns false if chip name is unknown.
func BootloaderOffset(chipName string) (uint32, bool) {
	switch chipName {
	case "ESP8266":
		return BootloaderOffsetESP8266, true
	case "ESP32":
		return BootloaderOffsetESP32, true
	case "ESP32-S2":
		return BootloaderOffsetESP32S2, true
	case "ESP32-S3":
		return BootloaderOffsetESP32S3, true
	case "ESP32-C2":
		return BootloaderOffsetESP32C2, true
	case "ESP32-C3":
		return BootloaderOffsetESP32C3, true
	case "ESP32-C5":
		return BootloaderOffsetESP32C5, true
	case "ESP32-C6":
		return BootloaderOffsetESP32C6, true
	case "ESP32-H2":
		return BootloaderOffsetESP32H2, true
	case "ESP32-P4-Rev1":
		return BootloaderOffsetESP32P4Rev1, true
	default:
		return 0, false
	}
}
