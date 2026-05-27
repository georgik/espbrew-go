package rom

import (
	"testing"
)

func TestDetectByMagic(t *testing.T) {
	tests := []struct {
		name        string
		magic       uint32
		expectedNil bool
	}{
		{"ESP32-S3", ESP32S3_MAGIC, false},
		{"ESP32-C3", ESP32C3_MAGIC, false},
		{"ESP32-C3 ECO3", ESP32C3_MAGIC_ECO3, false},
		{"ESP32-C3 ECO6", ESP32C3_MAGIC_ECO6, false},
		{"ESP32-C6", ESP32C6_MAGIC, false},
		{"Unknown", 0xFFFFFFFF, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chip := DetectByMagic(tt.magic)
			if tt.expectedNil {
				if chip != nil {
					t.Errorf("expected nil for magic 0x%08x, got %v", tt.magic, chip)
				}
			} else {
				if chip == nil {
					t.Errorf("expected chip for magic 0x%08x, got nil", tt.magic)
				}
			}
		})
	}
}

func TestDetectBySecurityID(t *testing.T) {
	tests := []struct {
		name        string
		chipID      uint32
		expectedNil bool
	}{
		{"ESP32-S3", SECURITY_ID_ESP32S3, false},
		{"ESP32-S2", SECURITY_ID_ESP32S2, false},
		{"ESP32-C3", SECURITY_ID_ESP32C3, false},
		{"ESP32-C6", SECURITY_ID_ESP32C6, false},
		{"Unknown", 0xFFFFFFFF, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chip := DetectBySecurityID(tt.chipID)
			if tt.expectedNil {
				if chip != nil {
					t.Errorf("expected nil for chip-id 0x%08x, got %v", tt.chipID, chip)
				}
			} else {
				if chip == nil {
					t.Errorf("expected chip for chip-id 0x%08x, got nil", tt.chipID)
				}
			}
		})
	}
}

func TestESP32S3Chip(t *testing.T) {
	chip := &ESP32S3Chip{}

	if chip.Name() != "ESP32-S3" {
		t.Errorf("expected name 'ESP32-S3', got %s", chip.Name())
	}

	expectedBase := uint32(0x60007044)
	if chip.BaseAddress() != expectedBase {
		t.Errorf("expected base address 0x%08x, got 0x%08x", expectedBase, chip.BaseAddress())
	}

	expectedMACReg := uint32(0x44)
	if chip.MACRegister() != expectedMACReg {
		t.Errorf("expected MAC register 0x%08x, got 0x%08x", expectedMACReg, chip.MACRegister())
	}
}

func TestESP32C3Chip(t *testing.T) {
	chip := &ESP32C3Chip{}

	if chip.Name() != "ESP32-C3" {
		t.Errorf("expected name 'ESP32-C3', got %s", chip.Name())
	}

	expectedBase := uint32(0x60008844)
	if chip.BaseAddress() != expectedBase {
		t.Errorf("expected base address 0x%08x, got 0x%08x", expectedBase, chip.BaseAddress())
	}
}

func TestFormatMAC(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "valid MAC",
			input:    []byte{0x84, 0xF7, 0x03, 0x12, 0x34, 0x56},
			expected: "84:f7:03:12:34:56",
		},
		{
			name:     "all zeros",
			input:    []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: "00:00:00:00:00:00",
		},
		{
			name:     "all FF",
			input:    []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
			expected: "ff:ff:ff:ff:ff:ff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMAC(tt.input)
			if result != tt.expected {
				t.Errorf("formatMAC() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestDeviceID(t *testing.T) {
	mac := "84:f7:03:12:34:56"
	expected := "esp-84:f7:03:12:34:56"
	result := DeviceID(mac)
	if result != expected {
		t.Errorf("DeviceID() = %s, want %s", result, expected)
	}
}
