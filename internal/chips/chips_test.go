package chips

import "testing"

func TestParseChip(t *testing.T) {
	tests := []struct {
		input    string
		want     Chip
		wantOK   bool
	}{
		// Registry/API form (uppercase, hyphenated).
		{"ESP32", ChipESP32, true},
		{"ESP32-S2", ChipESP32S2, true},
		{"ESP32-S3", ChipESP32S3, true},
		{"ESP32-C3", ChipESP32C3, true},
		{"ESP32-C6", ChipESP32C6, true},
		{"ESP32-H2", ChipESP32H2, true},
		{"ESP32-C2", ChipESP32C2, true},
		{"ESP32-C5", ChipESP32C5, true},
		// Lowercase / no-hyphen form (Chip.String()).
		{"esp32c3", ChipESP32C3, true},
		{"esp32s3", ChipESP32S3, true},
		// Tolerance: whitespace + mixed case.
		{"  Esp32-C3 ", ChipESP32C3, true},
		// Unknown.
		{"", 0, false},
		{"ESP32-X", 0, false},
		{"arduino", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseChip(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ParseChip(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("ParseChip(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseChipMatchesString round-trips every known chip through String() and
// ParseChip so a new chip added to the enum is covered automatically.
func TestParseChipMatchesString(t *testing.T) {
	known := []Chip{
		ChipESP32, ChipESP32S2, ChipESP32S3, ChipESP32C3, ChipESP32C6,
		ChipESP32H2, ChipESP32C2, ChipESP32C5, ChipESP32C61, ChipESP32P4,
	}
	for _, c := range known {
		if got, ok := ParseChip(c.String()); !ok || got != c {
			t.Errorf("ParseChip(%q) = %v, %v; want %v, true", c.String(), got, ok, c)
		}
	}
}
