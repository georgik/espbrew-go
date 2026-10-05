package cluster

import "testing"

func TestDeviceName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/dev/cu.usbmodem1401", "cu.usbmodem1401"},
		{"/dev/ttyUSB0", "ttyUSB0"},
		{"cu.usbmodem1401", "cu.usbmodem1401"},
		{"ttyUSB0", "ttyUSB0"},
		// Windows: COM ports have no /dev/ prefix. Both the bare form
		// ("COM3") and the Windows device-namespace forms
		// ("\\.\\COM4" / "\\?\\COM7") must collapse to the port name.
		{"COM3", "COM3"},
		{"\\.\\COM4", "COM4"},
		{"\\?\\COM7", "COM7"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := DeviceName(tt.input)
			if result != tt.expected {
				t.Errorf("DeviceName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
