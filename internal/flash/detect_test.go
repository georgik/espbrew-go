package flash

import (
	"testing"
)

func TestDetectFileType_ELF(t *testing.T) {
	// ELF magic: 0x7F 'E' 'L' 'F'
	data := []byte{0x7F, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	ft := DetectFileType(data)
	if ft != FileTypeELF {
		t.Errorf("Expected FileTypeELF, got %v", ft)
	}
}

func TestDetectFileType_ESP32Binary(t *testing.T) {
	// ESP32 binary magic: 0xE9
	data := []byte{0xE9, 0x07, 0x13, 0x20, 0x00, 0x00, 0x00, 0x00}
	ft := DetectFileType(data)
	if ft != FileTypeESP32Binary {
		t.Errorf("Expected FileTypeESP32Binary, got %v", ft)
	}
}

func TestDetectFileType_ESP8266Binary(t *testing.T) {
	// ESP8266 binary magic: 0xEA
	data := []byte{0xEA, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	ft := DetectFileType(data)
	if ft != FileTypeESP32Binary {
		t.Errorf("Expected FileTypeESP32Binary, got %v", ft)
	}
}

func TestDetectFileType_RawBinary(t *testing.T) {
	// Some other binary
	data := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	ft := DetectFileType(data)
	if ft != FileTypeRawBinary {
		t.Errorf("Expected FileTypeRawBinary, got %v", ft)
	}
}

func TestDetectFileType_Empty(t *testing.T) {
	ft := DetectFileType([]byte{})
	if ft != FileTypeUnknown {
		t.Errorf("Expected FileTypeUnknown, got %v", ft)
	}
}

func TestDetectFileType_SmallFile(t *testing.T) {
	ft := DetectFileType([]byte{0x7F})
	if ft != FileTypeUnknown {
		t.Errorf("Expected FileTypeUnknown, got %v", ft)
	}
}

func TestFileTypeString(t *testing.T) {
	tests := []struct {
		ft       FileType
		expected string
	}{
		{FileTypeELF, "ELF"},
		{FileTypeESP32Binary, "ESP32 Binary"},
		{FileTypeRawBinary, "Raw Binary"},
		{FileTypeUnknown, "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.ft.String(); got != tt.expected {
			t.Errorf("FileType.String() = %v, want %v", got, tt.expected)
		}
	}
}
