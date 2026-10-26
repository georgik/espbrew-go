package espfmt

import (
	"fmt"
	"sync"

	"github.com/georgik/espbrew-go/internal/flash/bootloaders"
)

// XtalFrequency represents the crystal oscillator frequency
type XtalFrequency int

const (
	Xtal26MHz XtalFrequency = 26
	Xtal40MHz XtalFrequency = 40
	Xtal32MHz XtalFrequency = 32
	Xtal48MHz XtalFrequency = 48
)

var (
	defaultManager *bootloaders.Manager
	managerOnce    sync.Once
)

// initBootloaderManager initializes the default bootloader manager
func initBootloaderManager() error {
	var initErr error
	managerOnce.Do(func() {
		mgr, err := bootloaders.NewManager(bootloaders.ManagerConfig{})
		if err != nil {
			initErr = err
			return
		}
		defaultManager = mgr
	})
	return initErr
}

// GetBootloader returns the bootloader for a chip and crystal frequency.
// The revision selects the revision-aware ESP32-P4 variant (v0 vs v3); it is
// ignored for every other chip.
func GetBootloader(chip Chip, xtalFreq XtalFrequency, revision int) ([]byte, error) {
	if err := initBootloaderManager(); err != nil {
		return nil, fmt.Errorf("init bootloader manager: %w", err)
	}

	// Chip is already chips.Chip via type alias
	data, _, err := defaultManager.GetBootloader(chip, revision)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// SetCustomBootloaderPath sets a custom bootloader path for a chip
func SetCustomBootloaderPath(chip Chip, path string) {
	if err := initBootloaderManager(); err != nil {
		return
	}
	defaultManager.SetCustomPath(chip, path)
}

// GetBootloaderManager returns the default bootloader manager instance
func GetBootloaderManager() (*bootloaders.Manager, error) {
	if err := initBootloaderManager(); err != nil {
		return nil, err
	}
	return defaultManager, nil
}

// GetEmbeddedBootloader is deprecated; use GetBootloader instead
// Kept for backward compatibility
func GetEmbeddedBootloader(chip Chip, xtalFreq XtalFrequency) ([]byte, error) {
	return GetBootloader(chip, xtalFreq, 0)
}

// PatchBootloaderFlashConfig rewrites the 24-byte bootloader header's
// flash-config byte (offset 3) so the bootloader reads the SPI flash at the
// requested frequency.
//
// Why this is required: the ROM bootloader and the 2nd-stage bootloader read
// the flash using the flash-config byte embedded in the *bootloader's own*
// header (not the app image header). The bundled ESP32-P4 bootloaders ship
// with a generic 80 MHz config. If we flash the bootloader verbatim, the
// 2nd-stage bootloader will try to read the app image at 80 MHz even though
// the app header declares 40 MHz. On flash that cannot keep up with 80 MHz the
// app read is corrupted and the app crashes (Illegal instruction).
//
// espflash patches this byte before flashing (see image_format/idf.rs:
// write_flash_config + splice), defaulting to the chip's default flash
// frequency (40 MHz for ESP32-P4). We replicate that here so the produced
// image boots the same way espflash's does.
func PatchBootloaderFlashConfig(data []byte, flashSize FlashSize, freq FlashFreq) []byte {
	const bootloaderHeaderLen = 24
	if len(data) < bootloaderHeaderLen {
		return data
	}
	out := make([]byte, len(data))
	copy(out, data)
	out[3] = uint8(flashSize)<<4 | uint8(freq)
	return out
}
