package flash

import (
	"encoding/binary"
	"fmt"
	"github.com/georgik/espbrew-go/internal/chips"
	"os"
	"testing"
)

func TestDumpEspbrewAppHeader(t *testing.T) {
	elfPath := "/home/juraj/research/espbrew/repos/esp32-conways-game-of-life-rs/esp32-p4-function-ev-board/target/riscv32imafc-unknown-none-elf/release/esp32-p4-function-ev-board-conways-game-of-life"
	elfData, err := os.ReadFile(elfPath)
	if err != nil {
		t.Fatal(err)
	}
	bin, err := ConvertELFToESPImage(elfData, chips.ChipESP32P4, 301)
	if err != nil {
		t.Fatal(err)
	}
	parts, err := ParseMultiPartImage(bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range parts {
		fmt.Printf("PART offset=0x%08x size=%d\n", p.Offset, len(p.Data))
		if p.Offset == 0x10000 {
			h := p.Data
			fmt.Printf("  app image size=%d\n", len(h))
			fmt.Printf("  header bytes 0..24: ")
			for i := 0; i < 24; i++ {
				fmt.Printf("%02x ", h[i])
			}
			fmt.Println()
			fmt.Printf("  byte3 flash_config = 0x%02x  -> sizeIdx=%d freqIdx=%d\n", h[3], h[3]>>4, h[3]&0x0f)
			fmt.Printf("  minChipRevFull = 0x%04x\n", binary.LittleEndian.Uint16(h[15:17]))
			fmt.Printf("  maxChipRevFull = 0x%04x\n", binary.LittleEndian.Uint16(h[17:19]))
			os.WriteFile("/tmp/espbrew_app.bin", h, 0644)
		}
		if p.Offset == 0x8000 {
			fmt.Printf("  partition_table size=%d\n", len(p.Data))
			// print factory partition entry
			// entries start at offset 8 in the part (after 0x50AA magic? no, part.Data starts at 0x8000 content)
			for i := 0; i+32 <= len(p.Data); i += 32 {
				e := p.Data[i:]
				if binary.LittleEndian.Uint16(e[0:2]) == 0x50AA {
					pt := e[2]
					pst := e[3]
					off := binary.LittleEndian.Uint32(e[4:8])
					sz := binary.LittleEndian.Uint32(e[8:12])
					label := string(e[12:28])
					label = trimNull(label)
					fmt.Printf("   part label=%-14s type=0x%02x sub=0x%02x off=0x%08x size=0x%08x\n", label, pt, pst, off, sz)
				}
				if e[0:2][0] == 0xEB {
					break
				}
			}
		}
	}
}

func trimNull(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return s[:i]
		}
	}
	return s
}
