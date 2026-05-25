package flash

import (
	"errors"
)

// FileType represents the type of firmware file
type FileType int

const (
	FileTypeUnknown FileType = iota
	FileTypeELF
	FileTypeESP32Binary
	FileTypeRawBinary
)

func (t FileType) String() string {
	switch t {
	case FileTypeELF:
		return "ELF"
	case FileTypeESP32Binary:
		return "ESP32 Binary"
	case FileTypeRawBinary:
		return "Raw Binary"
	default:
		return "Unknown"
	}
}

// ErrELFNotSupported is returned when an ELF file is detected
var ErrELFNotSupported = errors.New("ELF files require conversion. Use 'espflash save-image' or build a .bin file first")

// DetectFileType identifies the firmware file type from magic bytes
func DetectFileType(data []byte) FileType {
	if len(data) < 4 {
		return FileTypeUnknown
	}

	// Check for ELF magic: 0x7F 'E' 'L' 'F'
	if data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
		return FileTypeELF
	}

	// Check for ESP32 binary magic: 0xE9
	// Note: ESP8266 uses 0xEA, we treat it as ESP32 family binary
	if data[0] == 0xE9 || data[0] == 0xEA {
		return FileTypeESP32Binary
	}

	return FileTypeRawBinary
}
