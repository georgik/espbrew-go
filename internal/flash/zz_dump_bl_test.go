package flash

import (
	"fmt"
	"github.com/georgik/espbrew-go/internal/chips"
	"os"
	"testing"
)

func TestDumpBootloaderHeader(t *testing.T) {
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
		if p.Offset == 0x2000 {
			fmt.Printf("bootloader size=%d\n", len(p.Data))
			fmt.Printf("  header bytes 0..24: ")
			for i := 0; i < 24; i++ {
				fmt.Printf("%02x ", p.Data[i])
			}
			fmt.Println()
			fmt.Printf("  byte3 flash_config = 0x%02x -> sizeIdx=%d freqIdx=0x%x\n", p.Data[3], p.Data[3]>>4, p.Data[3]&0x0f)
			os.WriteFile("/tmp/espbrew_bl.bin", p.Data, 0644)
		}
	}
}
