package wokwi

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

const (
	// FirmwareStorageDir is the base directory for storing simulator firmware
	FirmwareStorageDir = "/tmp/espbrew-wokwi-firmware"
)

// Flasher implements protocol.Flasher for Wokwi simulator
type Flasher struct {
	config      *protocol.WokwiConfig
	firmwareDir string
}

// NewFlasher creates a new Wokwi flasher from device info
func NewFlasher(device *protocol.DeviceInfo) (protocol.Flasher, error) {
	if device.Backend != protocol.BackendWokwi {
		return nil, fmt.Errorf("device backend is not wokwi: %s", device.Backend)
	}

	cfg, ok := device.BackendConfig.(*protocol.WokwiConfig)
	if !ok {
		return nil, fmt.Errorf("invalid backend config type for wokwi device")
	}

	// Create firmware storage directory
	if err := os.MkdirAll(FirmwareStorageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create firmware storage directory: %w", err)
	}

	return &Flasher{
		config:      cfg,
		firmwareDir: FirmwareStorageDir,
	}, nil
}

// Flash stores the firmware for Wokwi simulator use
// For simulators, this means copying the firmware to a known location
func (f *Flasher) Flash(ctx context.Context, firmwarePath string, progress chan<- int) error {
	log.Info().
		Str("firmware", firmwarePath).
		Str("chip", f.config.ChipType).
		Msg("Storing firmware for Wokwi simulator")

	// Verify source file exists
	if _, err := os.Stat(firmwarePath); os.IsNotExist(err) {
		return fmt.Errorf("firmware file does not exist: %s", firmwarePath)
	}

	// Create device-specific firmware path
	deviceFirmwarePath := f.getFirmwarePath(firmwarePath)

	// Copy firmware to storage location
	if err := f.copyFirmware(firmwarePath, deviceFirmwarePath, progress); err != nil {
		return fmt.Errorf("failed to copy firmware: %w", err)
	}

	log.Info().
		Str("stored_at", deviceFirmwarePath).
		Msg("Firmware stored for Wokwi simulator")

	// Send 100% progress
	if progress != nil {
		select {
		case progress <- 100:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// ReadFlash is not applicable for Wokwi simulator
func (f *Flasher) ReadFlash(ctx context.Context, address, size uint32) ([]byte, error) {
	return nil, fmt.Errorf("read flash not supported for Wokwi simulator")
}

// GetFirmwarePath returns the path where firmware is stored for a given source file
func (f *Flasher) GetFirmwarePath(sourcePath string) string {
	return f.getFirmwarePath(sourcePath)
}

// GetStoredFirmwares returns list of stored firmware files
func (f *Flasher) GetStoredFirmwares() ([]string, error) {
	entries, err := os.ReadDir(f.firmwareDir)
	if err != nil {
		return nil, err
	}

	var firmwares []string
	for _, entry := range entries {
		if !entry.IsDir() && isFirmwareFile(entry.Name()) {
			firmwares = append(firmwares, filepath.Join(f.firmwareDir, entry.Name()))
		}
	}

	return firmwares, nil
}

// Cleanup removes old firmware files from storage
func (f *Flasher) Cleanup(keepRecent int) error {
	entries, err := os.ReadDir(f.firmwareDir)
	if err != nil {
		return err
	}

	// Sort by modification time and remove old files
	// For now, just remove all files older than 24 hours
	cutoff := 24 * time.Hour

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > cutoff {
			path := filepath.Join(f.firmwareDir, entry.Name())
			if err := os.Remove(path); err != nil {
				log.Warn().Err(err).Str("path", path).Msg("Failed to remove old firmware")
			}
		}
	}

	return nil
}

func (f *Flasher) getFirmwarePath(sourcePath string) string {
	// Create a unique filename based on source path and timestamp
	ext := filepath.Ext(sourcePath)
	base := filepath.Base(sourcePath)
	base = strings.TrimSuffix(base, ext)

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s%s", base, timestamp, ext)

	return filepath.Join(f.firmwareDir, filename)
}

func (f *Flasher) copyFirmware(src, dst string, progress chan<- int) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Get file size for progress calculation
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}
	fileSize := sourceInfo.Size()

	// Copy with progress tracking
	buffer := make([]byte, 32*1024)
	var copied int64

	for {
		n, err := sourceFile.Read(buffer)
		if n > 0 {
			if _, writeErr := destFile.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
			copied += int64(n)

			// Update progress (0-99%)
			if progress != nil {
				percent := int((copied * 100) / fileSize)
				if percent > 99 {
					percent = 99
				}
				select {
				case progress <- percent:
				default:
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	return destFile.Sync()
}

func isFirmwareFile(filename string) bool {
	ext := filepath.Ext(filename)
	return ext == ".bin" || ext == ".elf" || ext == ".json"
}
