package flash

import (
	"fmt"
	"os"
	"testing"

	"github.com/georgik/espbrew-go/internal/chips"
)

// TestP4Compare writes espbrew-go's app image for the P4 ELF so it can be
// diffed against an espflash-generated reference image.
func TestP4Compare(t *testing.T) {
	elfData := readFixtureOrSkip(t, espflashReferenceELF)

	out, err := ConvertELFToESPImage(elfData, chips.ChipESP32P4, 301)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	parts, err := ParseMultiPartImage(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, p := range parts {
		t.Logf("part offset=0x%x len=%d", p.Offset, len(p.Data))
	}

	// Find the app part (factory partition offset 0x10000).
	var app []byte
	for _, p := range parts {
		if p.Offset == 0x10000 {
			app = p.Data
		}
	}
	if app == nil {
		t.Fatalf("no app part found")
	}
	if err := os.WriteFile("/tmp/p4_espbrew_app.bin", app, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("app image len=%d", len(app))

	// Print segment headers for both images.
	fmt.Println("=== espbrew-go app segment headers ===")
	printSegs(app)
}

func printSegs(data []byte) {
	// Segments start right after the 24-byte image header.
	off := 24
	n := 0
	for off+8 <= len(data) && n < 12 {
		addr := uint32(data[off]) | uint32(data[off+1])<<8 | uint32(data[off+2])<<16 | uint32(data[off+3])<<24
		length := uint32(data[off+4]) | uint32(data[off+5])<<8 | uint32(data[off+6])<<16 | uint32(data[off+7])<<24
		fmt.Printf("  seg %d: hdr@0x%x addr=0x%x length=0x%x (data@0x%x)\n", n, off, addr, length, off+8)
		off += 8 + int(length)
		n++
	}
}
