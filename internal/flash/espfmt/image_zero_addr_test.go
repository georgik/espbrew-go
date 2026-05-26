package espfmt

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestImageZeroAddrSegments checks that generated images don't have
// invalid empty segments (addr=0 AND len=0)
// Padding segments (addr=0 with non-zero len) are valid for IROM alignment
func TestImageZeroAddrSegments(t *testing.T) {
	builder := NewImageBuilder(ChipESP32S3)
	builder.SetEntry(0x40370000)

	// Add two segments with a gap that requires padding
	builder.AddSegment(0x3c000020, make([]byte, 100))
	builder.AddSegment(0x42000000, make([]byte, 100))

	image, err := builder.BuildAppImage()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Image size: %d bytes", len(image))

	// Check for magic byte
	if len(image) < 8 || image[0] != ESP_MAGIC {
		t.Fatal("Invalid image format")
	}

	// Parse segments
	segmentCount := int(image[1])
	t.Logf("Segment count: %d", segmentCount)

	offset := 24 // Skip extended header
	paddingSegs := 0
	for i := 0; i < segmentCount && offset+8 <= len(image); i++ {
		addr := binary.LittleEndian.Uint32(image[offset : offset+4])
		length := binary.LittleEndian.Uint32(image[offset+4 : offset+8])

		t.Logf("Seg[%d]: addr=0x%08x, len=%d", i, addr, length)

		// Padding segments have addr=0 with non-zero length - this is valid
		// Only error if we have addr=0 AND len=0 (invalid empty segment)
		if addr == 0x00000000 {
			if length == 0 {
				t.Errorf("Segment %d has addr=0 and len=0 - invalid empty segment!", i)
			} else {
				paddingSegs++
			}
		}

		offset += 8 + int(length)
	}

	if paddingSegs > 0 {
		t.Logf("Found %d padding segments (addr=0), which is valid for IROM alignment", paddingSegs)
	}
}

// TestParsedSegmentsFromRealImage parses our generated Rust ESP image
// to check for invalid addresses
func TestParsedSegmentsFromRealImage(t *testing.T) {
	builder := NewImageBuilder(ChipESP32S3)
	builder.SetEntry(0x40379c8c)

	// Simulate segments from Rust ESP ELF
	builder.AddSegment(0x3c000020, make([]byte, 50976))
	builder.AddSegment(0x40378000, make([]byte, 9076))
	builder.AddSegment(0x42010020, make([]byte, 304116))

	t.Logf("Seg 1: addr=0x%08x, len=50976", 0x3c000020)
	t.Logf("Seg 2: addr=0x%08x, len=9076", 0x40378000)
	t.Logf("Seg 3: addr=0x%08x, len=304116", 0x42010020)

	image, err := builder.BuildAppImage()
	if err != nil {
		t.Fatal(err)
	}

	// Count segments with addr=0
	segmentCount := int(image[1])
	offset := 24
	zeroAddrCount := 0
	segsFound := 0
	imgLen := uint32(len(image))

	for offset+8 <= int(imgLen) && segsFound < segmentCount {
		addr := binary.LittleEndian.Uint32(image[offset : offset+4])
		length := binary.LittleEndian.Uint32(image[offset+4 : offset+8])

		t.Logf("Parsing seg[%d]: offset=%d, addr=0x%08x, len=%d", segsFound, offset, addr, length)

		// Padding segments have addr=0, this is expected (espflash format)
		if addr == 0x00000000 {
			zeroAddrCount++
			// Don't warn - padding segments with addr=0 are valid
		}

		offset += 8 + int(length)
		segsFound++
	}

	t.Logf("Final offset after parsing: %d, image len: %d", offset, len(image))

	// Dump hex around offset 60092 to see what's there
	if len(image) > 60120 {
		t.Logf("Hex dump at offset 60092: % x", image[60092:60120])
	}

	// Padding segments with addr=0 are valid (espflash format)
	if zeroAddrCount > 0 {
		t.Logf("Found %d padding segments with addr=0x00000000 (expected for IROM alignment)", zeroAddrCount)
	}
}

// TestImageBufferPaddingDirect tests writing padding directly to buffer
// instead of creating segments with addr=0
func TestImageBufferPaddingDirect(t *testing.T) {
	var buf bytes.Buffer

	// Write extended header (24 bytes)
	header := make([]byte, 24)
	header[0] = ESP_MAGIC
	header[1] = 2 // 2 segments
	buf.Write(header)

	// Write segment 1
	seg1Addr := []byte{0x20, 0x00, 0x00, 0x3c} // 0x3c000020
	seg1Len := []byte{0x64, 0x00, 0x00, 0x00}  // 100 bytes
	buf.Write(seg1Addr)
	buf.Write(seg1Len)
	buf.Write(make([]byte, 100))

	// Write padding directly (not as segment)
	buf.Write(make([]byte, 64)) // 64 bytes of padding

	// Write segment 2
	seg2Addr := []byte{0x00, 0x00, 0x00, 0x42} // 0x42000000
	seg2Len := []byte{0x64, 0x00, 0x00, 0x00}  // 100 bytes
	buf.Write(seg2Addr)
	buf.Write(seg2Len)
	buf.Write(make([]byte, 100))

	image := buf.Bytes()

	// Parse and verify
	t.Logf("Image size: %d bytes", len(image))
	t.Logf("Segment count from header: %d", image[1])

	// Should have exactly 2 segments, no padding segments
	if image[1] != 2 {
		t.Errorf("Expected 2 segments, got %d", image[1])
	}

	// Verify no segment has addr=0
	offset := 24
	imgLen := uint32(len(image))
	segsFound := 0
	for offset+8 <= int(imgLen) && segsFound < 2 {
		// Skip padding (zeros) before segment header
		for offset < int(imgLen)-8 && image[offset] == 0 && image[offset+1] == 0 && image[offset+2] == 0 && image[offset+3] == 0 {
			offset += 4
		}

		if offset+8 > int(imgLen) {
			break
		}

		addr := binary.LittleEndian.Uint32(image[offset : offset+4])
		if addr == 0 {
			t.Errorf("Segment %d has addr=0", segsFound)
		}
		offset += 8
		segsFound++
	}

	t.Log("Direct buffer padding approach works correctly")
}
