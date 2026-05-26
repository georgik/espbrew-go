package flash

import (
	"testing"
)

func TestParseMultiPartImage_Valid(t *testing.T) {
	// Create test data: [offset(4) + length(4) + data] x 3
	data := []byte{
		// Part 1: offset=0x0, length=4
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x04,
		0x01, 0x02, 0x03, 0x04,
		// Part 2: offset=0x1000, length=2
		0x00, 0x00, 0x10, 0x00,
		0x00, 0x00, 0x00, 0x02,
		0xAA, 0xBB,
		// Part 3: offset=0x10000, length=3
		0x00, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x03,
		0xCC, 0xDD, 0xEE,
	}

	parts, err := ParseMultiPartImage(data)
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	if len(parts) != 3 {
		t.Fatalf("Expected 3 parts, got %d", len(parts))
	}

	// Part 1
	if parts[0].Offset != 0x0 {
		t.Errorf("Part 1 offset: expected 0x0, got 0x%x", parts[0].Offset)
	}
	if len(parts[0].Data) != 4 {
		t.Errorf("Part 1 length: expected 4, got %d", len(parts[0].Data))
	}

	// Part 2
	if parts[1].Offset != 0x1000 {
		t.Errorf("Part 2 offset: expected 0x1000, got 0x%x", parts[1].Offset)
	}
	if len(parts[1].Data) != 2 {
		t.Errorf("Part 2 length: expected 2, got %d", len(parts[1].Data))
	}

	// Part 3
	if parts[2].Offset != 0x10000 {
		t.Errorf("Part 3 offset: expected 0x10000, got 0x%x", parts[2].Offset)
	}
	if len(parts[2].Data) != 3 {
		t.Errorf("Part 3 length: expected 3, got %d", len(parts[2].Data))
	}
}

func TestParseMultiPartImage_SinglePart(t *testing.T) {
	data := []byte{
		0x00, 0x01, 0x00, 0x00, // offset=0x10000 (big-endian)
		0x00, 0x00, 0x00, 0x01, // length=1
		0xFF,
	}

	parts, err := ParseMultiPartImage(data)
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	if len(parts) != 1 {
		t.Fatalf("Expected 1 part, got %d", len(parts))
	}

	if parts[0].Offset != 0x10000 {
		t.Errorf("Offset: expected 0x10000, got 0x%x", parts[0].Offset)
	}
	if len(parts[0].Data) != 1 || parts[0].Data[0] != 0xFF {
		t.Errorf("Data: expected [0xFF], got %v", parts[0].Data)
	}
}

func TestParseMultiPartImage_Empty(t *testing.T) {
	parts, err := ParseMultiPartImage([]byte{})
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	if len(parts) != 0 {
		t.Fatalf("Expected 0 parts, got %d", len(parts))
	}
}

func TestParseMultiPartImage_IncompleteHeader(t *testing.T) {
	data := []byte{0x00, 0x00} // Only 2 bytes

	_, err := ParseMultiPartImage(data)
	if err == nil {
		t.Error("Expected error for incomplete header, got nil")
	}
}

func TestParseMultiPartImage_IncompleteData(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x00, 0x00, // offset=0x0
		0x00, 0x00, 0x00, 0x04, // length=4
		0x01, 0x02, // Only 2 bytes of data
	}

	_, err := ParseMultiPartImage(data)
	if err == nil {
		t.Error("Expected error for incomplete data, got nil")
	}
}

func TestParseMultiPartImage_LargeValues(t *testing.T) {
	data := []byte{
		// Large offset: 0xFFFFF000 (big-endian)
		0xFF, 0xFF, 0xF0, 0x00,
		// Large length: 0x00001000 (4096 bytes, big-endian)
		0x00, 0x00, 0x10, 0x00,
	}
	// Append 4096 bytes of dummy data
	data = append(data, make([]byte, 4096)...)

	parts, err := ParseMultiPartImage(data)
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	if len(parts) != 1 {
		t.Fatalf("Expected 1 part, got %d", len(parts))
	}

	if parts[0].Offset != 0xFFFFF000 {
		t.Errorf("Offset: expected 0xFFFFF000, got 0x%x", parts[0].Offset)
	}

	if len(parts[0].Data) != 4096 {
		t.Errorf("Length: expected 4096, got %d", len(parts[0].Data))
	}
}
