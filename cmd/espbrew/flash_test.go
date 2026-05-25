package main

import (
	"fmt"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/flash"
)

func TestResolvePreset(t *testing.T) {
	tests := []struct {
		name           string
		preset         string
		chip           string
		expectedOffset int
		expectError    bool
	}{
		{"App preset", "app", "", flash.PresetOffsetApp, false},
		{"Partitions preset", "partitions", "", flash.PresetOffsetPartitions, false},
		{"Bootloader preset (default ESP32)", "bootloader", "", 0x1000, false},
		{"Unknown preset", "unknown", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset := 0
			var err error

			switch tt.preset {
			case "bootloader":
				offset = 0x1000 // ESP32 default
			case "partitions":
				offset = flash.PresetOffsetPartitions
			case "app":
				offset = flash.PresetOffsetApp
			default:
				err = fmt.Errorf("unknown preset")
			}

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError && offset != tt.expectedOffset {
				t.Errorf("Offset = %d, want %d", offset, tt.expectedOffset)
			}
		})
	}
}
