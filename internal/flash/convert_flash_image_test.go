package flash

import (
	"testing"

	"github.com/georgik/espbrew-go/internal/chips"
)

// c3FlashFixture resolves the prebuilt ESP32-C3 firmware ELF from the
// ESPBREW_C3_FLASH_ELF environment variable. It lives in a sibling example
// project and is only present on the developer's machine, not in CI. The test
// reads it through readFixtureOrSkip, which skips (instead of failing) when the
// variable is unset or the file is absent, so the suite stays green in CI.
func c3FlashFixture() string { return fixtureFromEnv("ESPBREW_C3_FLASH_ELF") }

// TestConvertELFToFlashImage verifies that ConvertELFToFlashImage shrinks a
// debug-heavy release ELF (several MiB) down to the compact, contiguous flash
// blob the spi-slave S31 relays to the C3, and that the blob is laid out with
// each part at its real flash offset.
func TestConvertELFToFlashImage(t *testing.T) {
	elf := readFixtureOrSkip(t, c3FlashFixture())
	if len(elf) == 0 {
		t.Skip("C3 ELF fixture not available")
	}

	img, err := ConvertELFToFlashImage(elf, chips.ChipESP32C3, 0)
	if err != nil {
		t.Fatalf("ConvertELFToFlashImage: %v", err)
	}

	// The whole point of the conversion is to avoid shipping the raw ELF
	// (which blows past the server's upload-size guard). It must be dramatically
	// smaller than the source.
	if len(img) >= len(elf)/2 {
		t.Fatalf("flash image not compact: %d bytes vs %d-byte ELF", len(img), len(elf))
	}

	if len(img) == 0 {
		t.Fatal("produced an empty flash image")
	}

	// The blob starts with the bootloader esp-image (0xE9) header.
	if img[0] != 0xE9 {
		t.Fatalf("expected bootloader esp-image magic 0xE9 at offset 0, got 0x%02x", img[0])
	}

	// The app part must sit at its real flash offset (0x10000 for C3) and carry
	// its own 0xE9 header, proving the parts were flattened into place rather
	// than concatenated without padding.
	const appOffset = 0x10000
	if int(appOffset)+4 > len(img) {
		t.Fatalf("flash image too small to contain app at 0x%x: %d bytes", appOffset, len(img))
	}
	if img[appOffset] != 0xE9 {
		t.Fatalf("expected app esp-image magic 0xE9 at 0x%x, got 0x%02x", appOffset, img[appOffset])
	}

	t.Logf("converted %d-byte ELF to %d-byte flash image (x%.1f smaller)",
		len(elf), len(img), float64(len(elf))/float64(len(img)))
}
