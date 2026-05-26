package flash

import (
	"os"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"codeberg.org/georgik/espbrew-go/internal/flash/testutil"
)

// TestELFSectionDataExtraction verifies section data is read correctly
func TestELFSectionDataExtraction(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	sections, err := ParseELFSections(data)
	if err != nil {
		t.Fatalf("ParseELFSections failed: %v", err)
	}

	// Find .rodata section
	var rodata *ELFSection
	for _, sec := range sections {
		if sec.Name == ".rodata" {
			rodata = &sec
			break
		}
	}

	if rodata == nil {
		t.Fatal(".rodata section not found")
	}

	t.Logf("=== .rodata section ===")
	t.Logf("Address: 0x%08x", rodata.Addr)
	t.Logf("Size: %d bytes (0x%x)", rodata.Size, rodata.Size)
	t.Logf("File offset: 0x%x", rodata.Offset)
	t.Logf("Data length: %d bytes", len(rodata.Data))

	// Check if section data matches what's in the file
	if len(rodata.Data) != int(rodata.Size) {
		t.Errorf("Section data length mismatch: got %d, want %d", len(rodata.Data), rodata.Size)
	}

	// Verify data at section offset matches what we extract directly
	if int(rodata.Offset)+int(rodata.Size) > len(data) {
		t.Fatalf("Section extends beyond file")
	}

	directData := data[rodata.Offset : rodata.Offset+uint32(rodata.Size)]

	matches := true
	diffCount := 0
	for i := 0; i < len(rodata.Data) && i < len(directData); i++ {
		if rodata.Data[i] != directData[i] {
			matches = false
			diffCount++
			if diffCount <= 10 {
				t.Logf("Mismatch at offset %d: section=0x%02x, file=0x%02x", i, rodata.Data[i], directData[i])
			}
		}
	}

	if matches {
		t.Log("Section data matches file content")
	} else {
		t.Errorf("Section data differs from file content at %d offsets", diffCount)
	}

	// Count non-zero bytes in section data
	nonZero := 0
	for _, b := range rodata.Data {
		if b != 0 {
			nonZero++
		}
	}
	t.Logf("Non-zero bytes in section data: %d / %d (%.1f%%)", nonZero, len(rodata.Data), float64(nonZero)*100/float64(len(rodata.Data)))

	// Show first 64 bytes of section data
	t.Logf("\nFirst 64 bytes of section data:")
	for i := 0; i < 64 && i < len(rodata.Data); i += 16 {
		end := i + 16
		if end > len(rodata.Data) {
			end = len(rodata.Data)
		}
		t.Logf("0x%04x: % x", i, rodata.Data[i:end])
	}
}

// TestROMSegmentDataExtraction verifies ROM segment data extraction
func TestROMSegmentDataExtraction(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	sections, err := ParseELFSections(data)
	if err != nil {
		t.Fatalf("ParseELFSections failed: %v", err)
	}

	romSegments := GetROMSegments(sections, chips.ChipESP32S3)

	t.Logf("=== ROM Segments ===")
	for i, seg := range romSegments {
		t.Logf("ROM[%d]: addr=0x%08x, len=%d", i, seg.Addr, len(seg.Data))

		// For DROM segment (0x3c000020), check content
		if seg.Addr == 0x3c000020 {
			t.Logf("\n=== DROM Segment Analysis ===")

			// Count non-zero bytes
			nonZero := 0
			for _, b := range seg.Data {
				if b != 0 {
					nonZero++
				}
			}
			t.Logf("Non-zero bytes: %d / %d (%.1f%%)", nonZero, len(seg.Data), float64(nonZero)*100/float64(len(seg.Data)))

			// Show first 64 bytes
			t.Logf("First 64 bytes:")
			for j := 0; j < 64 && j < len(seg.Data); j += 16 {
				end := j + 16
				if end > len(seg.Data) {
					end = len(seg.Data)
				}
				t.Logf("0x%04x: % x", j, seg.Data[j:end])
			}

			// Check if this matches the .rodata section in the ELF
			for _, sec := range sections {
				if sec.Name == ".rodata" {
					t.Logf("\n.rodata section: offset=0x%x, size=%d", sec.Offset, sec.Size)

					// .rodata should start at offset 0x100 (256 bytes) into DROM segment
					// DROM starts at 0x3c000020, .rodata starts at 0x3c000120
					rodataOffsetInDROM := 0x100
					if len(seg.Data) > rodataOffsetInDROM+256 {
						t.Logf("DROM data at offset 0x100 (should be .rodata start): % x", seg.Data[rodataOffsetInDROM:rodataOffsetInDROM+32])
					}

					// Compare section data with segment data
					if rodataOffsetInDROM+len(sec.Data) <= len(seg.Data) {
						segData := seg.Data[rodataOffsetInDROM : rodataOffsetInDROM+len(sec.Data)]
						match := true
						for j := range sec.Data {
							if sec.Data[j] != segData[j] {
								match = false
								t.Logf("Mismatch at %d: section=0x%02x, segment=0x%02x", j, sec.Data[j], segData[j])
								break
							}
						}
						if match {
							t.Log(".rodata section data matches DROM segment data")
						} else {
							t.Error(".rodata section data does NOT match DROM segment data")
						}
					}
					break
				}
			}
		}
	}
}

// TestELFFileAccess checks if we can read data directly from ELF file
func TestELFFileAccess(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	t.Logf("ELF file size: %d bytes", len(data))

	// Check data at specific offsets
	checkOffset := func(offset int, size int, desc string) {
		if offset+size > len(data) {
			t.Logf("Offset 0x%x (%d): BEYOND FILE", offset, offset)
			return
		}
		chunk := data[offset : offset+size]
		nonZero := 0
		for _, b := range chunk {
			if b != 0 {
				nonZero++
			}
		}
		t.Logf("Offset 0x%x (%d): %d/%d non-zero - %s", offset, offset, nonZero, size, desc)
		t.Logf("  Data: % x", chunk)
	}

	checkOffset(0x1020, 32, ".flash.appdesc")
	checkOffset(0x1120, 32, ".rodata start")
	checkOffset(0xD700, 32, ".rodata end / .eh_frame_hdr area")
	checkOffset(0xD724, 32, ".eh_frame_hdr")

	// Check if .rodata section (0x1120 to 0xD724) is contiguous with data
	rodataStart := 0x1120
	rodataEnd := 0xD724
	rodataSize := rodataEnd - rodataStart
	t.Logf("\n.rodata range: 0x%x to 0x%x (size: %d bytes)", rodataStart, rodataEnd, rodataSize)

	rodataData := data[rodataStart:rodataEnd]
	nonZero := 0
	for _, b := range rodataData {
		if b != 0 {
			nonZero++
		}
	}
	t.Logf("Non-zero bytes in .rodata range: %d / %d (%.1f%%)", nonZero, rodataSize, float64(nonZero)*100/float64(rodataSize))
}
