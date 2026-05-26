package flash

import (
	"os"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"codeberg.org/georgik/espbrew-go/internal/flash/testutil"
)

// TestELFSectionSize checks if .flash.appdesc section is fully extracted
func TestELFSectionSize(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	sections, err := ParseELFSections(data)
	if err != nil {
		t.Fatalf("ParseELFSections failed: %v", err)
	}

	// Find .flash.appdesc section
	var appDesc *ELFSection
	for _, sec := range sections {
		if sec.Name == ".flash.appdesc" {
			appDesc = &sec
			break
		}
	}

	if appDesc == nil {
		t.Fatal(".flash.appdesc section not found")
	}

	t.Logf("=== .flash.appdesc section ===")
	t.Logf("Address: 0x%08x", appDesc.Addr)
	t.Logf("Size: %d bytes (0x%x)", appDesc.Size, appDesc.Size)
	t.Logf("File offset: 0x%x", appDesc.Offset)
	t.Logf("Data length: %d bytes", len(appDesc.Data))

	// Check if section data is complete
	if len(appDesc.Data) != int(appDesc.Size) {
		t.Errorf("Section data incomplete: got %d bytes, want %d bytes", len(appDesc.Data), appDesc.Size)
	}

	// Show section content
	t.Logf("\n=== Section content (first 256 bytes) ===")
	for i := 0; i < 256 && i < len(appDesc.Data); i += 16 {
		end := i + 16
		if end > len(appDesc.Data) {
			end = len(appDesc.Data)
		}
		t.Logf("0x%04x: % x", i, appDesc.Data[i:end])
	}

	// Check specific offsets
	t.Logf("\n=== Specific offsets ===")
	checkOffset := func(offset int, desc string) {
		if offset < len(appDesc.Data) {
			t.Logf("Offset 0x%02x (%d): % x - %s", offset, offset, appDesc.Data[offset:offset+16], desc)
		}
	}
	checkOffset(0x00, "Magic (expect AB CD 5D E5)")
	checkOffset(0x30, "Version (expect '0.0.0')")
	checkOffset(0x50, "SHA256 (espflash leaves zeros)")
	checkOffset(0x70, "Reserved4 (espflash may have data here)")
	checkOffset(0x90, "Offset where differences start")

	// Count non-zero bytes in different regions
	countNonZero := func(start, end int) int {
		count := 0
		for i := start; i < end && i < len(appDesc.Data); i++ {
			if appDesc.Data[i] != 0 {
				count++
			}
		}
		return count
	}

	t.Logf("\n=== Non-zero byte counts ===")
	t.Logf("Offset 0x00-0x30 (first 48 bytes): %d non-zero bytes", countNonZero(0, 0x30))
	t.Logf("Offset 0x30-0x50 (version area): %d non-zero bytes", countNonZero(0x30, 0x50))
	t.Logf("Offset 0x50-0x70 (SHA256 area): %d non-zero bytes", countNonZero(0x50, 0x70))
	t.Logf("Offset 0x70-0x90 (reserved4): %d non-zero bytes", countNonZero(0x70, 0x90))
	t.Logf("Offset 0x90-0xC0: %d non-zero bytes", countNonZero(0x90, 0xC0))

	// Check if section is padded or truncated
	if len(appDesc.Data) < 256 {
		t.Logf("\nWARNING: Section data is only %d bytes, app_desc should be at least 256 bytes", len(appDesc.Data))
	} else {
		t.Logf("\nSection has sufficient data (>= 256 bytes)")
	}
}

// TestROMSegmentContent checks what data is in our ROM segments
func TestROMSegmentContent(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	romSegments, ramSegments, err := ParseELF(data, chips.ChipESP32S3)
	if err != nil {
		t.Fatalf("ParseELF failed: %v", err)
	}

	t.Logf("=== ROM Segments ===")
	for i, seg := range romSegments {
		t.Logf("ROM[%d]: addr=0x%08x, len=%d", i, seg.Addr, len(seg.Data))

		// Check if this is the DROM segment (containing app_desc)
		if seg.Addr == 0x3C000020 {
			t.Logf("\n=== DROM Segment (contains .flash.appdesc) ===")
			t.Logf("Total size: %d bytes", len(seg.Data))

			// Check offset 0x90 (where differences start)
			if len(seg.Data) > 0x90+16 {
				t.Logf("Offset 0x90: % x", seg.Data[0x90:0x90+16])
			}

			// Count non-zero bytes
			nonZero := 0
			for _, b := range seg.Data {
				if b != 0 {
					nonZero++
				}
			}
			t.Logf("Non-zero bytes: %d / %d (%.1f%%)", nonZero, len(seg.Data), float64(nonZero)*100/float64(len(seg.Data)))
		}
	}

	t.Logf("\n=== RAM Segments ===")
	for i, seg := range ramSegments {
		t.Logf("RAM[%d]: addr=0x%08x, len=%d", i, seg.Addr, len(seg.Data))
	}
}

// TestConvertedImageSegment0 checks segment 0 in our converted image
func TestConvertedImageSegment0(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	result, err := ConvertELFToESPImage(data, chips.ChipESP32S3)
	if err != nil {
		t.Fatalf("ConvertELFToESPImage failed: %v", err)
	}

	parts, err := ParseMultiPartImage(result)
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	var appData []byte
	for _, part := range parts {
		if part.Offset == 0x10000 {
			appData = part.Data
			break
		}
	}

	if appData == nil {
		t.Fatal("App part not found")
	}

	// Parse segments from ESP image format
	// Extended header is 24 bytes, then segment headers
	segCount := int(appData[1])
	t.Logf("=== App image segments: %d ===", segCount)

	offset := 24 // Start after extended header
	for i := 0; i < segCount && offset+8 < len(appData); i++ {
		// Read segment header (little-endian)
		segAddr := uint32(appData[offset]) | uint32(appData[offset+1])<<8 |
			uint32(appData[offset+2])<<16 | uint32(appData[offset+3])<<24
		segLen := uint32(appData[offset+4]) | uint32(appData[offset+5])<<8 |
			uint32(appData[offset+6])<<16 | uint32(appData[offset+7])<<24

		t.Logf("Segment %d: addr=0x%08x, len=%d", i, segAddr, segLen)

		dataStart := offset + 8
		dataEnd := dataStart + int(segLen)

		if dataEnd > len(appData) {
			t.Logf("  WARNING: Segment extends beyond image (end=%d, image=%d)", dataEnd, len(appData))
			break
		}

		segData := appData[dataStart:dataEnd]

		// Count non-zero bytes
		nonZero := 0
		for _, b := range segData {
			if b != 0 {
				nonZero++
			}
		}
		t.Logf("  Non-zero bytes: %d / %d (%.1f%%)", nonZero, len(segData), float64(nonZero)*100/float64(len(segData)))

		// Check specific offsets for DROM segment
		if segAddr == 0x3c000020 && len(segData) > 0x90 {
			t.Logf("  Offset 0x90: % x", segData[0x90:min(0x90+16, len(segData))])
			t.Logf("  Offset 0x70: % x", segData[0x70:min(0x70+16, len(segData))])
		}

		offset = dataEnd
	}
}
