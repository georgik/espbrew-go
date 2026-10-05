package wokwi

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/georgik/espbrew-go/internal/backend/wokwi/api"
	"github.com/georgik/espbrew-go/internal/chips"
)

// buildMinimalELF constructs a valid 32-bit little-endian ELF with a single
// PROGBITS section placed in the ESP32-S3 IROM region. It is only large enough
// for flash.ParseELF to extract the one ROM segment, which is all
// assembleSections needs to exercise the ELF conversion path.
func buildMinimalELF() []byte {
	const (
		segAddr  = 0x42000000 // ESP32-S3 IROM
		segData  = 8
		ehSize   = 64
		shSize   = 40
		shNum    = 3 // null, .text, .shstrtab
		shStrNdx = 2
	)

	// Layout:
	//   [0, 64)     ELF header
	//   [64, 72)    segment data (shOffset of .text)
	//   [72,192)    3 section headers (shOff)
	//   [192,208)   .shstrtab
	shOff := uint32(ehSize + segData)        // 72
	shStrOff := uint32(shOff + shSize*shNum) // 192

	buf := make([]byte, shStrOff+16)

	// ELF header
	buf[0] = 0x7F
	buf[1] = 'E'
	buf[2] = 'L'
	buf[3] = 'F'
	buf[4] = 1                                          // 32-bit
	buf[5] = 1                                          // little-endian
	binary.LittleEndian.PutUint32(buf[24:28], segAddr)  // entry point
	binary.LittleEndian.PutUint32(buf[32:36], shOff)    // section header offset
	binary.LittleEndian.PutUint16(buf[40:42], ehSize)   // ELF header size
	binary.LittleEndian.PutUint16(buf[46:48], shSize)   // section header entry size
	binary.LittleEndian.PutUint16(buf[48:50], shNum)    // section header count
	binary.LittleEndian.PutUint16(buf[50:52], shStrNdx) // string table index

	// Segment data at file offset 64
	for i := 0; i < segData; i++ {
		buf[ehSize+i] = byte(i + 1)
	}

	// .text section header (index 1) at shOff + shSize
	text := shOff + shSize
	binary.LittleEndian.PutUint32(buf[text+12:text+16], segAddr)        // sh_addr
	binary.LittleEndian.PutUint32(buf[text+16:text+20], uint32(ehSize)) // sh_offset
	binary.LittleEndian.PutUint32(buf[text+20:text+24], segData)        // sh_size
	binary.LittleEndian.PutUint32(buf[text+4:text+8], 1)                // sh_type = PROGBITS

	// .shstrtab section header (index 2)
	strtab := shOff + 2*shSize
	binary.LittleEndian.PutUint32(buf[strtab+16:strtab+20], shStrOff) // sh_offset
	binary.LittleEndian.PutUint32(buf[strtab+20:strtab+24], 16)       // sh_size
	binary.LittleEndian.PutUint32(buf[strtab+4:strtab+8], 3)          // sh_type = STRTAB

	// String table: ".text\0" then ".shstrtab\0"
	copy(buf[shStrOff:], []byte(".text\x00.shstrtab\x00"))

	return buf
}

// buildMultiPart builds an espbrew-format multi-part image: for each part, an
// 8-byte [offset, length] header followed by the part data.
func buildMultiPart(parts ...[2]interface{}) []byte {
	var out []byte
	for _, p := range parts {
		data := p[1].([]byte)
		hdr := make([]byte, 8)
		binary.BigEndian.PutUint32(hdr[0:4], p[0].(uint32))
		binary.BigEndian.PutUint32(hdr[4:8], uint32(len(data)))
		out = append(out, hdr...)
		out = append(out, data...)
	}
	return out
}

func sectionOffsets(sections []api.FlashSection) []int {
	out := make([]int, 0, len(sections))
	for _, s := range sections {
		out = append(out, s.Offset)
	}
	return out
}

func TestAssembleSectionsELF(t *testing.T) {
	elf := buildMinimalELF()
	sections, err := assembleSections(chips.ChipESP32S3, elf)
	if err != nil {
		t.Fatalf("assembleSections: %v", err)
	}

	// An assembled image must contain the bootloader, partition table and app.
	offs := sectionOffsets(sections)
	if got, want := len(offs), 3; got != want {
		t.Fatalf("expected %d sections, got %d (%v)", want, got, offs)
	}
	if !slices.Contains(offs, 0x8000) {
		t.Errorf("expected partition table section at 0x8000, got %v", offs)
	}
	if !slices.Contains(offs, 0x10000) {
		t.Errorf("expected app section at 0x10000, got %v", offs)
	}
	// The app section must be a real ESP image (magic 0xE9).
	var app *api.FlashSection
	for i := range sections {
		if sections[i].Offset == 0x10000 {
			app = &sections[i]
		}
	}
	if app == nil {
		t.Fatal("no app section found")
	}
	if len(app.Data) == 0 || app.Data[0] != 0xE9 {
		t.Errorf("app section missing ESP image magic 0xE9: %v", app.Data[:min(1, len(app.Data))])
	}
}

func TestAssembleSectionsMultiPart(t *testing.T) {
	app := []byte{0xE9, 0x00, 0x11, 0x22, 0x33, 0x44}
	boot := []byte{0x01, 0x02, 0x03}
	part := []byte{0xAA, 0xBB}
	image := buildMultiPart(
		[2]interface{}{uint32(0x1000), boot},
		[2]interface{}{uint32(0x8000), part},
		[2]interface{}{uint32(0x10000), app},
	)

	sections, err := assembleSections(chips.ChipESP32S3, image)
	if err != nil {
		t.Fatalf("assembleSections: %v", err)
	}

	offs := sectionOffsets(sections)
	want := []int{0x1000, 0x8000, 0x10000}
	if len(offs) != len(want) {
		t.Fatalf("expected %d sections, got %d (%v)", len(want), len(offs), offs)
	}
	for i, off := range offs {
		if off != want[i] {
			t.Errorf("section[%d] = 0x%X, want 0x%X", i, off, want[i])
		}
	}
}

func TestAssembleSectionsRawBinary(t *testing.T) {
	firmware := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	sections, err := assembleSections(chips.ChipESP32S3, firmware)
	if err != nil {
		t.Fatalf("assembleSections: %v", err)
	}
	offs := sectionOffsets(sections)
	if got, want := len(offs), 1; got != want {
		t.Fatalf("expected 1 section for raw binary, got %d (%v)", got, offs)
	}
	if offs[0] != 0 {
		t.Errorf("raw binary should sit at offset 0, got 0x%X", offs[0])
	}
	if len(sections[0].Data) != len(firmware) {
		t.Errorf("raw binary data mismatch: got %d bytes, want %d", len(sections[0].Data), len(firmware))
	}
}

func TestAssembleSectionsLoneESPImage(t *testing.T) {
	// A 0xE9 image that is NOT a valid multi-part image (huge length field):
	// must fall back to a single section at the app offset.
	firmware := []byte{0xE9, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0x01, 0x02, 0x03, 0x04}
	sections, err := assembleSections(chips.ChipESP32S3, firmware)
	if err != nil {
		t.Fatalf("assembleSections: %v", err)
	}
	offs := sectionOffsets(sections)
	if got, want := len(offs), 1; got != want {
		t.Fatalf("expected 1 section for lone ESP image, got %d (%v)", got, offs)
	}
	if offs[0] != int(appFlashOffset) {
		t.Errorf("lone ESP image should sit at app offset 0x%X, got 0x%X", appFlashOffset, offs[0])
	}
}
