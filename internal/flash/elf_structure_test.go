package flash

import (
	"encoding/binary"
	"os"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"codeberg.org/georgik/espbrew-go/internal/flash/testutil"
)

// TestImageFormatDebug checks the generated image format
func TestImageFormatDebug(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	// Parse ELF sections for debug
	sections, _ := ParseELFSections(data)
	t.Logf("=== ELF SECTIONS (with data) ===")
	sectCount := 0
	for _, s := range sections {
		if len(s.Data) > 0 { // All sections with data
			memType := "UNKNOWN"
			if s.Addr >= 0x40370000 && s.Addr < 0x403E0000 {
				memType = "IRAM"
			} else if s.Addr >= 0x3FC88000 && s.Addr < 0x3FD00000 {
				memType = "DRAM"
			} else if s.Addr >= 0x42000000 && s.Addr < 0x42800000 {
				memType = "IROM"
			} else if s.Addr >= 0x3C000000 && s.Addr < 0x3E000000 {
				memType = "DROM"
			} else if s.Addr == 0 {
				memType = "ZERO"
			}
			t.Logf("  Section: %-25s addr=0x%08x size=%6d type=%2d %s", s.Name, s.Addr, len(s.Data), s.Type, memType)
			sectCount++
			if sectCount > 40 { // Show all
				break
			}
		}
	}

	// Parse ELF segments
	romSegs, ramSegs, err := ParseELF(data, chips.ChipESP32S3)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("\n=== ROM SEGMENTS (merged for IROM alignment) ===")
	for i, s := range romSegs {
		memType := "IROM"
		if s.Addr >= 0x3C000000 && s.Addr < 0x3E000000 {
			memType = "DROM"
		}
		t.Logf("  ROM[%d]: addr=0x%08x len=%d %s", i, s.Addr, len(s.Data), memType)
	}

	t.Logf("\n=== RAM SEGMENTS (before merge) ===")
	for i, s := range ramSegs {
		memType := "IRAM"
		if s.Addr >= 0x3FC88000 && s.Addr < 0x3FD00000 {
			memType = "DRAM"
		}
		t.Logf("  RAM[%d]: addr=0x%08x len=%d %s", i, s.Addr, len(s.Data), memType)
	}

	imageData, err := ConvertELFToESPImage(data, chips.ChipESP32S3)
	if err != nil {
		t.Fatal(err)
	}

	parts, err := ParseMultiPartImage(imageData)
	if err != nil {
		t.Fatal(err)
	}

	for i, part := range parts {
		preview := len(part.Data)
		if preview > 32 {
			preview = 32
		}
		t.Logf("Part[%d]: offset=0x%08x, len=%d, first_bytes=% x",
			i, part.Offset, len(part.Data), part.Data[:preview])

		// Check if ESP image header is valid
		if len(part.Data) >= 4 && part.Data[0] == 0xE9 {
			t.Logf("  ESP image: magic=0x%02x, segments=%d, mode=0x%02x",
				part.Data[0], part.Data[1], part.Data[2])
			if len(part.Data) >= 8 {
				entry := binary.LittleEndian.Uint32(part.Data[4:8])
				t.Logf("  Entry: 0x%08x", entry)
			}

			// Parse segment headers
			if len(part.Data) >= 24 {
				t.Logf("  Segment headers at offset 24+:")
				offset := 24
				for j := 0; j < int(part.Data[1]) && offset+8 <= len(part.Data); j++ {
					addr := binary.LittleEndian.Uint32(part.Data[offset : offset+4])
					length := binary.LittleEndian.Uint32(part.Data[offset+4 : offset+8])
					t.Logf("    Seg[%d]: addr=0x%08x, len=%d (0x%x)", j, addr, length, length)
					offset += 8 + int(length)
				}
			}
		}
	}
}
