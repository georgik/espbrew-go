package virtual

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog/log"
)

const (
	VirtualFlashSize = 16 * 1024 * 1024 // 16MB default flash size
)

// NormalizeVirtualPath converts old-style virtual paths to URI-style format
// wokwi-esp32s3 -> wokwi:esp32-s3
// wokwi-esp32c3 -> wokwi:esp32-c3
// wokwi-esp32 -> wokwi:esp32
func NormalizeVirtualPath(port string) string {
	if len(port) > 6 && port[:6] == "wokwi-" {
		chip := port[6:]
		// Convert chip name to proper format with hyphens
		switch chip {
		case "esp32s3":
			return "wokwi:esp32-s3"
		case "esp32c3":
			return "wokwi:esp32-c3"
		case "esp32c6":
			return "wokwi:esp32-c6"
		case "esp32":
			return "wokwi:esp32"
		default:
			// For unknown chips, try to add hyphen after esp32
			if len(chip) > 5 && chip[:5] == "esp32" {
				return "wokwi:esp32-" + chip[5:]
			}
			return "wokwi:" + chip
		}
	}
	return port
}

// Device represents a virtual flash device backed by a file
type Device struct {
	mu     sync.Mutex
	path   string
	mem    []byte
	chip   string
	active bool
}

// IsVirtualPath checks if a device path is a virtual device
func IsVirtualPath(port string) bool {
	normalized := NormalizeVirtualPath(port)
	return normalized == ":virtual:" || (len(normalized) > 6 && normalized[:6] == "wokwi:")
}

// ChipFromVirtualPath extracts chip type from virtual path
// Supports both old format (wokwi-esp32s3) and new format (wokwi:esp32-s3)
func ChipFromVirtualPath(port string) string {
	normalized := NormalizeVirtualPath(port)
	if len(normalized) > 6 && normalized[:6] == "wokwi:" {
		// Convert wokwi:esp32-s3 back to esp32s3 format
		chip := normalized[6:]
		// Remove hyphens and convert to lowercase
		switch chip {
		case "esp32-s3":
			return "esp32s3"
		case "esp32-c3":
			return "esp32c3"
		case "esp32-c6":
			return "esp32c6"
		case "esp32":
			return "esp32"
		default:
			// Remove hyphens for other chips
			result := make([]byte, 0, len(chip))
			for i := 0; i < len(chip); i++ {
				if chip[i] != '-' {
					result = append(result, chip[i])
				}
			}
			return string(result)
		}
	}
	return "esp32s3" // default
}

// OpenDevice opens or creates a virtual flash device
func OpenDevice(id string) (*Device, error) {
	dir := filepath.Join(os.Getenv("HOME"), ".espbrew", "virtual")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create virtual directory: %w", err)
	}

	// Normalize virtual path for consistent file naming
	normalizedID := NormalizeVirtualPath(id)
	path := filepath.Join(dir, normalizedID+".bin")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Initialize with 0xFF (erased flash state)
			data = make([]byte, VirtualFlashSize)
			for i := range data {
				data[i] = 0xFF
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				return nil, fmt.Errorf("initialize virtual device: %w", err)
			}
			log.Info().Str("path", path).Msg("Created new virtual flash device")
		} else {
			return nil, fmt.Errorf("read virtual device: %w", err)
		}
	}

	if len(data) != VirtualFlashSize {
		log.Warn().Int("size", len(data)).Int("expected", VirtualFlashSize).
			Msg("Virtual device size mismatch, resizing")
		newData := make([]byte, VirtualFlashSize)
		copy(newData, data)
		for i := len(data); i < VirtualFlashSize; i++ {
			newData[i] = 0xFF
		}
		data = newData
	}

	return &Device{
		path:   path,
		mem:    data,
		active: true,
	}, nil
}

// Read reads from flash at address
func (d *Device) Read(addr uint32, size uint32) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.active {
		return nil, fmt.Errorf("device not active")
	}

	if addr+size > VirtualFlashSize {
		return nil, fmt.Errorf("read beyond flash size: 0x%x + 0x%x > 0x%x", addr, size, VirtualFlashSize)
	}

	return d.mem[addr : addr+size], nil
}

// Write writes to flash at address
func (d *Device) Write(addr uint32, data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.active {
		return fmt.Errorf("device not active")
	}

	if addr+uint32(len(data)) > VirtualFlashSize {
		return fmt.Errorf("write beyond flash size: 0x%x + 0x%x > 0x%x", addr, len(data), VirtualFlashSize)
	}

	copy(d.mem[addr:addr+uint32(len(data))], data)

	// Persist to file
	if err := os.WriteFile(d.path, d.mem, 0644); err != nil {
		return fmt.Errorf("persist virtual device: %w", err)
	}

	return nil
}

// EraseRegion erases flash region to 0xFF
func (d *Device) EraseRegion(addr uint32, size uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.active {
		return fmt.Errorf("device not active")
	}

	end := addr + size
	if end > VirtualFlashSize {
		end = VirtualFlashSize
	}

	for i := addr; i < end; i++ {
		d.mem[i] = 0xFF
	}

	// Persist to file
	return os.WriteFile(d.path, d.mem, 0644)
}

// Dump returns the entire flash contents
func (d *Device) Dump() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]byte, len(d.mem))
	copy(result, d.mem)
	return result, nil
}

// Close closes the virtual device
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.active = false
	return nil
}

// Path returns the file path of the virtual device
func (d *Device) Path() string {
	return d.path
}

// Size returns the flash size
func (d *Device) Size() uint32 {
	return VirtualFlashSize
}
