package espfmt

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestChecksumValidation tests the checksum calculation algorithm
func TestChecksumValidation(t *testing.T) {
	// Create a simple test image with known checksum
	// Format: header + one segment + padding + checksum (no SHA256)

	var buf bytes.Buffer

	// Write header: magic=0xE9, segment_count=0, rest zeros
	buf.Write([]byte{0xE9, 0x00}) // magic + seg_count=0
	buf.Write(make([]byte, 22))   // rest of header

	// Calculate checksum - only segment data, not headers (matching espflash)
	checksum := byte(ESP_CHECKSUM_MAGIC)

	// Write segment header (NOT included in checksum)
	segHeader := []byte{0x00, 0x10, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00}
	buf.Write(segHeader)

	// Write segment data (4 bytes) - INCLUDED in checksum
	data := []byte{0x01, 0x02, 0x03, 0x04}
	buf.Write(data)
	for _, b := range data {
		checksum ^= b
	}

	// Pad to 16-byte boundary (using espflash formula: 15 - len%16)
	padding := 15 - (buf.Len() % 16)
	buf.Write(make([]byte, padding))

	// Write checksum (patch segment_count after)
	buf.WriteByte(checksum)

	// Patch segment count
	imageData := buf.Bytes()
	imageData[1] = 1 // Segment count patched AFTER checksum written

	// Verify final image
	imageData = buf.Bytes()

	t.Logf("Image size: %d bytes", len(imageData))
	t.Logf("Segment count at position 1: %d", imageData[1])

	// For this simple test without SHA256, checksum is at last position
	checksumOffset := len(imageData) - 1
	storedChecksum := imageData[checksumOffset]
	t.Logf("Stored checksum: 0x%02X at position %d", storedChecksum, checksumOffset)

	// Verify: XOR of segment data should be 0x01^0x02^0x03^0x04 = 0x04
	// Starting from 0xEF: 0xEF ^ 0x01 ^ 0x02 ^ 0x03 ^ 0x04 = 0xEF ^ 0x04 = 0xEB
	expectedChecksum := ESP_CHECKSUM_MAGIC ^ 0x01 ^ 0x02 ^ 0x03 ^ 0x04
	t.Logf("Expected checksum (XOR of data): 0x%02X", expectedChecksum)

	if int(storedChecksum) != expectedChecksum {
		t.Errorf("Checksum mismatch: expected 0x%02X, stored 0x%02X",
			expectedChecksum, storedChecksum)
	}
}

// TestImageBuilderChecksum tests the full image builder checksum
func TestImageBuilderChecksum(t *testing.T) {
	builder := NewImageBuilder(ChipESP32S3)

	// Add a simple ROM segment
	testData := make([]byte, 64)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	builder.AddSegment(0x40000000, testData)

	// Build image
	imageData, err := builder.BuildAppImage()
	if err != nil {
		t.Fatalf("BuildAppImage failed: %v", err)
	}

	t.Logf("Built image size: %d bytes", len(imageData))

	// Verify header
	if imageData[0] != ESP_MAGIC {
		t.Errorf("Invalid magic: 0x%02X", imageData[0])
	}

	segmentCount := imageData[1]
	t.Logf("Segment count: %d", segmentCount)

	// Verify checksum
	// Format: [header+segments+padding][checksum][SHA256]
	// Checksum is XOR of segment data only (not headers), matching espflash
	checksumOffset := len(imageData) - 32 - 1 // -32 for SHA256, -1 for checksum byte
	storedChecksum := imageData[checksumOffset]
	t.Logf("Stored checksum: 0x%02X at position %d", storedChecksum, checksumOffset)

	// Recalculate checksum by parsing segments and XORing only data bytes
	// This matches espflash behavior: checksum only covers segment data, not headers
	calculatedChecksum := int(ESP_CHECKSUM_MAGIC)

	// Skip extended header (24 bytes) and parse segment headers
	offset := 24
	for offset < checksumOffset {
		if offset+8 > checksumOffset {
			break
		}
		// Read segment header
		/* addr */
		_ = uint32(imageData[offset]) | uint32(imageData[offset+1])<<8 |
			uint32(imageData[offset+2])<<16 | uint32(imageData[offset+3])<<24
		length := uint32(imageData[offset+4]) | uint32(imageData[offset+5])<<8 |
			uint32(imageData[offset+6])<<16 | uint32(imageData[offset+7])<<24
		offset += 8

		// XOR segment data only (not header)
		dataEnd := offset + int(length)
		if dataEnd > checksumOffset {
			dataEnd = checksumOffset
		}
		for i := offset; i < dataEnd; i++ {
			calculatedChecksum ^= int(imageData[i])
		}
		offset = dataEnd
	}

	t.Logf("Calculated checksum (segment data only): 0x%02X", calculatedChecksum)

	if calculatedChecksum != int(storedChecksum) {
		t.Errorf("Checksum mismatch: calculated 0x%02X, stored 0x%02X",
			calculatedChecksum, storedChecksum)
	}

	// Log first 64 bytes for debugging
	t.Logf("First 64 bytes:\n%s", hex.Dump(imageData[:min(64, len(imageData))]))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestPaddingAlignment tests 16-byte alignment padding
func TestPaddingAlignment(t *testing.T) {
	testCases := []struct {
		length       int
		expectPad    int
		expectOffset int
	}{
		{1000, 8, 1008},
		{1008, 0, 1008},
		{1016, 8, 1024},
		{1024, 0, 1024},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			padding := (16 - (tc.length % 16)) % 16
			offset := tc.length + padding

			if padding != tc.expectPad {
				t.Errorf("Length %d: expected padding %d, got %d", tc.length, tc.expectPad, padding)
			}

			if offset != tc.expectOffset {
				t.Errorf("Length %d: expected offset %d, got %d", tc.length, tc.expectOffset, offset)
			}

			if offset%16 != 0 {
				t.Errorf("Length %d: offset %d is not 16-byte aligned", tc.length, offset)
			}
		})
	}
}

// TestSegmentCountPatchEffect tests that patching segment_count affects checksum
func TestSegmentCountPatchEffect(t *testing.T) {
	// Simulate the checksum calculation with segment_count patch

	// Initial state: segment_count = 0
	originalChecksum := int(ESP_CHECKSUM_MAGIC)
	data := []byte{0xE9, 0x00, 0x02, 0x20} // magic, seg_count=0, flash_mode, flash_config
	for _, b := range data {
		originalChecksum ^= int(b)
	}
	t.Logf("Checksum with seg_count=0: 0x%02X", originalChecksum)

	// After patch: segment_count = 2
	patchedChecksum := int(ESP_CHECKSUM_MAGIC)
	data[1] = 2
	for _, b := range data {
		patchedChecksum ^= int(b)
	}
	t.Logf("Checksum with seg_count=2: 0x%02X", patchedChecksum)

	// Our adjustment formula: checksum ^ old ^ new
	adjustedChecksum := originalChecksum ^ 0 ^ 2
	t.Logf("Adjusted checksum: 0x%02X", adjustedChecksum)

	if adjustedChecksum != patchedChecksum {
		t.Errorf("Adjustment formula failed: expected 0x%02X, got 0x%02X",
			patchedChecksum, adjustedChecksum)
	}
}
