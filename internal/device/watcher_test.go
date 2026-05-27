package device

import (
	"testing"
)

func TestDeduplicatePorts(t *testing.T) {
	tests := []struct {
		name     string
		input    []Port
		expected []string // paths we expect to remain
	}{
		{
			name: "macOS cu/tty pair - cu preferred",
			input: []Port{
				{Path: "/dev/cu.usbmodem1401"},
				{Path: "/dev/tty.usbmodem1401"},
			},
			expected: []string{"/dev/cu.usbmodem1401"},
		},
		{
			name: "macOS multiple pairs",
			input: []Port{
				{Path: "/dev/cu.usbmodem1401"},
				{Path: "/dev/tty.usbmodem1401"},
				{Path: "/dev/cu.usbserial1420"},
				{Path: "/dev/tty.usbserial1420"},
			},
			expected: []string{"/dev/cu.usbmodem1401", "/dev/cu.usbserial1420"},
		},
		{
			name: "Linux ttyUSB - no deduplication",
			input: []Port{
				{Path: "/dev/ttyUSB0"},
				{Path: "/dev/ttyUSB1"},
			},
			expected: []string{"/dev/ttyUSB0", "/dev/ttyUSB1"},
		},
		{
			name: "Linux ttyACM - no deduplication",
			input: []Port{
				{Path: "/dev/ttyACM0"},
			},
			expected: []string{"/dev/ttyACM0"},
		},
		{
			name: "mixed macOS and Linux",
			input: []Port{
				{Path: "/dev/cu.usbmodem1401"},
				{Path: "/dev/tty.usbmodem1401"},
				{Path: "/dev/ttyUSB0"},
			},
			expected: []string{"/dev/cu.usbmodem1401", "/dev/ttyUSB0"},
		},
		{
			name: "only tty without cu - kept",
			input: []Port{
				{Path: "/dev/tty.usbmodem1401"},
			},
			expected: []string{"/dev/tty.usbmodem1401"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicatePorts(tt.input)

			// Check we have the expected number of results
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d ports, got %d", len(tt.expected), len(result))
				return
			}

			// Check each expected path is present
			resultPaths := make(map[string]bool)
			for _, p := range result {
				resultPaths[p.Path] = true
			}

			for _, expectedPath := range tt.expected {
				if !resultPaths[expectedPath] {
					t.Errorf("expected path %q not found in result", expectedPath)
				}
			}
		})
	}
}

func TestDeviceBaseName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/dev/cu.usbmodem1401", "usbmodem1401"},
		{"/dev/tty.usbmodem1401", "usbmodem1401"},
		{"/dev/cu.usbserial1420", "usbserial1420"},
		{"/dev/tty.usbserial1420", "usbserial1420"},
		{"/dev/ttyUSB0", ""}, // Linux - no base name
		{"/dev/ttyACM0", ""}, // Linux - no base name
		{"/dev/tty.wchusbserial", "wchusbserial"},
		{"/dev/null", ""}, // Not a serial device
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := deviceBaseName(tt.input)
			if result != tt.expected {
				t.Errorf("deviceBaseName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
