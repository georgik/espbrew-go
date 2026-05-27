package rom

import (
	"testing"
)

func TestEncodeSLIP(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "simple data",
			input:    []byte{0x01, 0x02, 0x03},
			expected: []byte{0x01, 0x02, 0x03, SLIP_END},
		},
		{
			name:     "data with END",
			input:    []byte{0x01, SLIP_END, 0x03},
			expected: []byte{0x01, SLIP_ESC, SLIP_ESC_END, 0x03, SLIP_END},
		},
		{
			name:     "data with ESC",
			input:    []byte{0x01, SLIP_ESC, 0x03},
			expected: []byte{0x01, SLIP_ESC, SLIP_ESC_ESC, 0x03, SLIP_END},
		},
		{
			name:     "data with both",
			input:    []byte{SLIP_END, SLIP_ESC},
			expected: []byte{SLIP_ESC, SLIP_ESC_END, SLIP_ESC, SLIP_ESC_ESC, SLIP_END},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeSLIP(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestDecodeSLIP(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "simple data",
			input:    []byte{0x01, 0x02, 0x03, SLIP_END},
			expected: []byte{0x01, 0x02, 0x03},
		},
		{
			name:     "data with escaped END",
			input:    []byte{0x01, SLIP_ESC, SLIP_ESC_END, 0x03, SLIP_END},
			expected: []byte{0x01, SLIP_END, 0x03},
		},
		{
			name:     "data with escaped ESC",
			input:    []byte{0x01, SLIP_ESC, SLIP_ESC_ESC, 0x03, SLIP_END},
			expected: []byte{0x01, SLIP_ESC, 0x03},
		},
		{
			name:     "multiple frames",
			input:    []byte{0x01, SLIP_END, 0x02, SLIP_END},
			expected: []byte{0x01, 0x02},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeSLIP(tt.input)
			if err != nil {
				t.Fatalf("DecodeSLIP failed: %v", err)
			}
			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestSLIPEncodeDecodeRoundtrip(t *testing.T) {
	testData := [][]byte{
		{0x01, 0x02, 0x03, 0x04, 0x05},
		{0x00, 0xFF, 0xAA, 0x55},
		{SLIP_END, SLIP_ESC, 0x01},
		{0x01, 0x02, SLIP_END, 0x03, SLIP_ESC, 0x04},
	}

	for _, data := range testData {
		encoded := EncodeSLIP(data)
		decoded, err := DecodeSLIP(encoded)
		if err != nil {
			t.Errorf("DecodeSLIP failed: %v", err)
			continue
		}
		if len(decoded) != len(data) {
			t.Errorf("length mismatch: got %d, want %d", len(decoded), len(data))
			continue
		}
		for i := range data {
			if decoded[i] != data[i] {
				t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, decoded[i], data[i])
			}
		}
	}
}
