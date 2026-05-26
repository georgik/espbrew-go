package flash

import (
	"context"
	"fmt"
	"io"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"codeberg.org/georgik/espbrew-go/internal/flash/virtual"
	"github.com/rs/zerolog/log"
	"tinygo.org/x/espflasher/pkg/espflasher"
)

type Flasher struct {
	opts *FlasherOptions
}

type FlasherOptions struct {
	BaudRate      int
	FlashBaudRate int
	Compress      bool
}

type FlashResult struct {
	Success bool
	Error   error
	Bytes   int
}

func NewFlasher(opts *FlasherOptions) *Flasher {
	if opts == nil {
		opts = &FlasherOptions{
			BaudRate:      115200,
			FlashBaudRate: 460800,
			Compress:      true,
		}
	}
	return &Flasher{opts: opts}
}

type FlashRequest struct {
	Port     string
	Firmware []byte
	Offset   int
	Progress chan int
	Chip     chips.Chip // Optional: specify chip for ELF conversion
}

// Flash writes firmware to the device at port
func (f *Flasher) Flash(ctx context.Context, req *FlashRequest) *FlashResult {
	// Check if using virtual device
	if virtual.IsVirtualPath(req.Port) {
		return f.flashVirtual(ctx, req)
	}

	firmware := req.Firmware

	// Convert ELF to ESP-IDF binary if needed
	if DetectFileType(firmware) == FileTypeELF {
		log.Info().Msg("ELF file detected, converting to ESP-IDF binary format")

		// Detect chip from request or default
		chip := req.Chip
		if chip == 0 {
			// Try to detect from ELF sections or default to ESP32S3
			chip = chips.ChipESP32S3
			log.Info().Msg("Chip not specified, defaulting to ESP32-S3")
		}

		bin, err := ConvertELFToESPImage(firmware, chip)
		if err != nil {
			log.Error().Err(err).Msg("Failed to convert ELF to ESP-IDF image")
			return &FlashResult{Success: false, Error: fmt.Errorf("ELF conversion: %w", err)}
		}
		log.Info().Int("original_bytes", len(firmware)).Int("converted_bytes", len(bin)).Msg("ELF converted to ESP-IDF format")

		// Parse multi-part format
		parts, err := ParseMultiPartImage(bin)
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse multi-part image")
			return &FlashResult{Success: false, Error: fmt.Errorf("parse multipart: %w", err)}
		}

		logger := &flashLogger{port: req.Port}

		espOpts := espflasher.DefaultOptions()
		espOpts.BaudRate = f.opts.BaudRate
		espOpts.FlashBaudRate = f.opts.FlashBaudRate
		espOpts.Compress = false // Disable compression to isolate issue
		espOpts.Logger = logger

		flasher, err := espflasher.New(req.Port, espOpts)
		if err != nil {
			log.Error().Err(err).Str("port", req.Port).Msg("Failed to create flasher")
			return &FlashResult{Success: false, Error: err}
		}
		defer flasher.Close()

		log.Info().Str("port", req.Port).Msg("Chip detected")

		// Log detected chip for visibility
		chipName := flasher.ChipName()
		log.Info().Str("chip", chipName).Msg("Detected chip")

		// Flash each part
		// Bootloader and partition table are raw binaries
		// App is ESP image format with embedded segments
		totalBytes := 0
		for i, part := range parts {
			// Count non-zero bytes for logging
			nonZero := 0
			for _, b := range part.Data {
				if b != 0 {
					nonZero++
				}
			}
			var preview []byte
			if len(part.Data) > 32 {
				preview = part.Data[:32]
			} else {
				preview = part.Data
			}
			log.Info().Int("part", i+1).Int("part_size", len(part.Data)).Int("non_zero", nonZero).
				Uint32("offset", part.Offset).Bytes("preview", preview).Msg("Flashing part")

			// Use FlashImage for app part (ESP image format), FlashImages for raw parts
			if part.Offset == 0x10000 && len(part.Data) > 100 && part.Data[0] == 0xE9 {
				// App part - ESP image format with segments
				if err := flasher.FlashImage(part.Data, part.Offset, nil); err != nil {
					log.Error().Err(err).Msg("Flash app failed")
					return &FlashResult{Success: false, Error: err}
				}
			} else {
				// Raw binary part (bootloader, partition table)
				imageParts := []espflasher.ImagePart{
					{Data: part.Data, Offset: part.Offset},
				}
				if err := flasher.FlashImages(imageParts, nil); err != nil {
					log.Error().Err(err).Msg("Flash part failed")
					return &FlashResult{Success: false, Error: err}
				}
			}
			totalBytes += len(part.Data)
		}

		flasher.Reset()
		log.Info().Msg("Flash complete")
		return &FlashResult{
			Success: true,
			Bytes:   totalBytes,
		}
	}

	logger := &flashLogger{port: req.Port}

	espOpts := espflasher.DefaultOptions()
	espOpts.BaudRate = f.opts.BaudRate
	espOpts.FlashBaudRate = f.opts.FlashBaudRate
	espOpts.Compress = f.opts.Compress
	espOpts.Logger = logger

	flasher, err := espflasher.New(req.Port, espOpts)
	if err != nil {
		log.Error().Err(err).Str("port", req.Port).Msg("Failed to create flasher")
		return &FlashResult{Success: false, Error: err}
	}
	defer flasher.Close()

	log.Info().Str("port", req.Port).Msg("Chip detected")

	// Log detected chip for visibility
	chipName := flasher.ChipName()
	log.Info().Str("chip", chipName).Msg("Detected chip")

	log.Info().Int("bytes", len(firmware)).Msg("Starting flash")

	var progressFunc espflasher.ProgressFunc
	if req.Progress != nil {
		progressFunc = func(current, total int) {
			pct := 0
			if total > 0 {
				pct = current * 100 / total
			}
			select {
			case req.Progress <- pct:
			default:
			}
		}
	}

	if err := flasher.FlashImage(firmware, uint32(req.Offset), progressFunc); err != nil {
		log.Error().Err(err).Msg("Flash write failed")
		return &FlashResult{Success: false, Error: err}
	}

	flasher.Reset()

	log.Info().Msg("Flash complete")
	return &FlashResult{
		Success: true,
		Bytes:   len(firmware),
	}
}

// flashVirtual flashes to a virtual device (file backend)
func (f *Flasher) flashVirtual(ctx context.Context, req *FlashRequest) *FlashResult {
	chip := virtual.ChipFromVirtualPath(req.Port)
	deviceID := req.Port
	if deviceID == ":virtual:" {
		deviceID = "default"
	}

	log.Info().Str("device", req.Port).Str("chip", chip).Msg("Using virtual flash device")

	device, err := virtual.OpenDevice(deviceID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to open virtual device")
		return &FlashResult{Success: false, Error: err}
	}
	defer device.Close()

	firmware := req.Firmware
	var parts []ImagePart

	// Convert ELF to ESP-IDF binary if needed
	if DetectFileType(firmware) == FileTypeELF {
		log.Info().Msg("ELF file detected, converting to ESP-IDF binary format")

		// Detect chip from request or virtual path
		chipType := req.Chip
		if chipType == 0 {
			// Map chip name from virtual path
			switch chip {
			case "esp32s3":
				chipType = chips.ChipESP32S3
			case "esp32":
				chipType = chips.ChipESP32
			case "esp32c3":
				chipType = chips.ChipESP32C3
			default:
				chipType = chips.ChipESP32S3
			}
		}

		bin, err := ConvertELFToESPImage(firmware, chipType)
		if err != nil {
			log.Error().Err(err).Msg("Failed to convert ELF to ESP-IDF image")
			return &FlashResult{Success: false, Error: fmt.Errorf("ELF conversion: %w", err)}
		}
		log.Info().Int("original_bytes", len(firmware)).Int("converted_bytes", len(bin)).Msg("ELF converted to ESP-IDF format")

		parts, err = ParseMultiPartImage(bin)
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse multi-part image")
			return &FlashResult{Success: false, Error: fmt.Errorf("parse multipart: %w", err)}
		}
	} else {
		// Single binary part at specified offset
		parts = []ImagePart{
			{Offset: uint32(req.Offset), Data: firmware},
		}
	}

	// Write each part to virtual device
	totalBytes := 0
	for i, part := range parts {
		nonZero := 0
		for _, b := range part.Data {
			if b != 0 {
				nonZero++
			}
		}
		var preview []byte
		if len(part.Data) > 32 {
			preview = part.Data[:32]
		} else {
			preview = part.Data
		}
		log.Info().Int("part", i+1).Int("part_size", len(part.Data)).Int("non_zero", nonZero).
			Uint32("offset", part.Offset).Bytes("preview", preview).Msg("Writing to virtual device")

		if err := device.Write(part.Offset, part.Data); err != nil {
			log.Error().Err(err).Msg("Virtual device write failed")
			return &FlashResult{Success: false, Error: err}
		}
		totalBytes += len(part.Data)

		// Report progress
		if req.Progress != nil {
			pct := (i + 1) * 100 / len(parts)
			select {
			case req.Progress <- pct:
			default:
			}
		}
	}

	log.Info().Str("path", device.Path()).Int("bytes", totalBytes).Msg("Virtual flash complete")
	return &FlashResult{
		Success: true,
		Bytes:   totalBytes,
	}
}

// flashLogger adapts espflasher logging to zerolog
type flashLogger struct {
	port string
}

func (l *flashLogger) Logf(format string, args ...interface{}) {
	log.Debug().Str("port", l.port).Msgf(format, args...)
}

// Monitor opens a serial monitor for the device
func (f *Flasher) Monitor(ctx context.Context, port string) (io.ReadCloser, error) {
	// TODO: Implement serial monitor using go.bug.st/serial directly
	// The espflasher library doesn't expose a monitor API
	return nil, fmt.Errorf("monitor not yet implemented - use external terminal")
}

// ReadFlashRequest contains parameters for reading flash memory
type ReadFlashRequest struct {
	Port    string
	Address uint32
	Size    uint32
	Chip    chips.Chip
}

// ReadFlashResult contains the result of a flash read operation
type ReadFlashResult struct {
	Success bool
	Data    []byte
	Error   error
}

// ReadFlash reads data from the device's flash memory at the specified address
func (f *Flasher) ReadFlash(ctx context.Context, req *ReadFlashRequest) *ReadFlashResult {
	log.Info().Str("port", req.Port).Uint32("address", req.Address).Uint32("size", req.Size).Msg("Reading flash")

	// Build flasher options
	espOpts := espflasher.DefaultOptions()
	espOpts.BaudRate = f.opts.BaudRate
	espOpts.FlashBaudRate = f.opts.FlashBaudRate
	espOpts.Compress = f.opts.Compress
	espOpts.Logger = &flashLogger{port: req.Port}

	// Open connection
	flasher, err := espflasher.New(req.Port, espOpts)
	if err != nil {
		return &ReadFlashResult{Success: false, Error: fmt.Errorf("connect: %w", err)}
	}
	defer flasher.Close()

	log.Info().Str("port", req.Port).Msg("Chip detected")
	log.Info().Str("chip", flasher.ChipName()).Msg("Detected chip")

	// Read flash data
	data, err := flasher.ReadFlash(req.Address, req.Size)
	if err != nil {
		return &ReadFlashResult{Success: false, Error: fmt.Errorf("read flash: %w", err)}
	}

	log.Info().Int("bytes_read", len(data)).Msg("Flash read completed")
	return &ReadFlashResult{Success: true, Data: data}
}
