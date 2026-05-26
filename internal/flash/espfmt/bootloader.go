package espfmt

import (
	"fmt"
	"sync"

	"codeberg.org/georgik/espbrew-go/internal/flash/bootloaders"
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

// GetBootloader returns the bootloader for a chip and crystal frequency
// It uses the external bootloader manager which downloads from GitHub if not cached
func GetBootloader(chip Chip, xtalFreq XtalFrequency) ([]byte, error) {
	if err := initBootloaderManager(); err != nil {
		return nil, fmt.Errorf("init bootloader manager: %w", err)
	}

	// Chip is already chips.Chip via type alias
	data, _, err := defaultManager.GetBootloader(chip)
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
	return GetBootloader(chip, xtalFreq)
}
