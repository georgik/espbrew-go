package flash

import "testing"

func TestBootloaderOffset_AllChips(t *testing.T) {
	tests := []struct {
		chip     string
		expected uint32
	}{
		{"ESP8266", 0x0},
		{"ESP32", 0x1000},
		{"ESP32-S2", 0x1000},
		{"ESP32-S3", 0x0},
		{"ESP32-C2", 0x0},
		{"ESP32-C3", 0x0},
		{"ESP32-C5", 0x2000},
		{"ESP32-C6", 0x0},
		{"ESP32-H2", 0x0},
		{"ESP32-P4-Rev1", 0x2000},
	}

	for _, tt := range tests {
		offset, ok := BootloaderOffset(tt.chip)
		if !ok {
			t.Errorf("BootloaderOffset(%q) returned ok=false", tt.chip)
		}
		if offset != tt.expected {
			t.Errorf("BootloaderOffset(%q) = 0x%X, want 0x%X", tt.chip, offset, tt.expected)
		}
	}
}

func TestBootloaderOffset_UnknownChip(t *testing.T) {
	offset, ok := BootloaderOffset("UnknownChip")
	if ok {
		t.Errorf("BootloaderOffset(UnknownChip) returned ok=true with offset 0x%X", offset)
	}
	if offset != 0 {
		t.Errorf("BootloaderOffset(UnknownChip) returned non-zero offset 0x%X", offset)
	}
}
