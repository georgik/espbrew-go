package flash

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/georgik/espbrew-go/internal/chips"
	"github.com/georgik/espbrew-go/internal/espflash"
	"github.com/georgik/espbrew-go/internal/flash/virtual"
	"github.com/rs/zerolog/log"
)

type Flasher struct {
	opts *FlasherOptions
}

type FlasherOptions struct {
	BaudRate      int
	FlashBaudRate int
	Compress      bool
	Erase         bool // Enable erase before flash (default: false)
	FastMode      bool // Enable fast connection mode (default: true)
	SkipUnchanged bool // Skip flashing unchanged segments (default: false)

	// ResetMode selects how the chip is reset into bootloader mode. It is
	// forwarded verbatim to the espflash layer. Default: espflash.ResetAuto
	// (try USB-JTAG, then classic DTR/RTS, then no-reset), which matches the
	// historical behaviour of the flash path when this field is left unset.
	ResetMode espflash.ResetMode
}

type FlashResult struct {
	Success bool
	Error   error
	Bytes   int
}

// Phase markers describe what the flasher is currently doing. They are
// forwarded to the client through FlashRequest.Phase / EraseRequest.Phase so the
// UI can display the flasher's state (connecting vs. flashing) instead of a
// bare 0% bar while the board is being brought up.
const (
	PhaseConnecting = "connecting"
	PhaseFlashing   = "flashing"
	PhaseErase      = "erasing"
)

// emitPhase forwards a phase marker to the client. It never blocks: a nil or
// full channel simply drops the marker.
func emitPhase(ch chan string, phase string) {
	if ch == nil {
		return
	}
	select {
	case ch <- phase:
	default:
	}
}

func NewFlasher(opts *FlasherOptions) *Flasher {
	if opts == nil {
		opts = &FlasherOptions{
			BaudRate:      115200,
			FlashBaudRate: 460800,
			Compress:      true,
			Erase:         false,
			FastMode:      true,
			SkipUnchanged: false,
			ResetMode:     espflash.ResetAuto,
		}
	}
	return &Flasher{opts: opts}
}

// SetErase updates the erase option for the flasher
func (f *Flasher) SetErase(erase bool) {
	f.opts.Erase = erase
}

// SetFlashBaud overrides the flash transfer baud rate at runtime. A value <= 0
// is ignored (the option's default is kept). This lets the cluster executor
// honor a client-supplied `--baud` (or a device param) without rebuilding the
// flasher. The chip's MaxUARTFlashBaud cap still applies inside flashBegin.
func (f *Flasher) SetFlashBaud(baud int) {
	if baud > 0 {
		f.opts.FlashBaudRate = baud
	}
}

type FlashRequest struct {
	Port     string
	Firmware []byte
	Offset   int
	Progress chan int
	Phase    chan string
	Chip     chips.Chip // Optional: specify chip for ELF conversion
}

// Flash writes firmware to the device at port
func (f *Flasher) Flash(ctx context.Context, req *FlashRequest) *FlashResult {
	// Check if using virtual device
	if virtual.IsVirtualPath(req.Port) {
		return f.flashVirtual(ctx, req)
	}

	// Resolve stable device path to actual port
	port, err := resolveDevicePathWithRetry(ctx, req.Port, maxResolveTimeout)
	if err != nil {
		return &FlashResult{Success: false, Error: fmt.Errorf("resolve device path: %w", err)}
	}
	log.Debug().Str("original", req.Port).Str("resolved", port).Msg("Device path resolved")

	firmware := req.Firmware

	// Convert ELF to ESP-IDF binary if needed
	if DetectFileType(firmware) == FileTypeELF {
		log.Info().Msg("ELF file detected, converting to ESP-IDF binary format")

		// Detect chip from request or default.
		chip := req.Chip
		if chip == 0 {
			// Try to detect from ELF sections or default to ESP32S3
			chip = chips.ChipESP32S3
			log.Info().Msg("Chip not specified, defaulting to ESP32-S3")
		}

		// Attach to the device first so we can detect the chip revision.
		// The revision is required to pick the right revision-aware
		// ESP32-P4 bootloader variant (v0 vs v3) when we build the image
		// below, mirroring espflash's flow where the revision is known
		// before the image is assembled.
		logger := &flashLogger{port: port}

		espOpts := espflash.DefaultOptions()
		espOpts.BaudRate = f.opts.BaudRate
		espOpts.FlashBaudRate = f.opts.FlashBaudRate
		espOpts.Compress = f.opts.Compress // use compression like esptool (reduces wire data, avoids RX-FIFO overflow)
		espOpts.Erase = f.opts.Erase
		espOpts.Logger = logger
		// Forward the caller-selected reset strategy. Default (ResetAuto)
		// preserves historical behaviour; the reset-reliability sweep sets this
		// explicitly to compare strategies.
		espOpts.ResetMode = f.opts.ResetMode
		// Disable FastMode for USB CDC ports - they re-enumerate after reset
		espOpts.FastMode = f.opts.FastMode && !isUSBPort(port)
		espOpts.SkipUnchanged = f.opts.SkipUnchanged
		// CH340/CH9102/CP210x boards are USB-UART bridges (no native
		// USB-JTAG): force the DTR/RTS bootloader reset and skip the dead
		// USB-JTAG path. Detected from the board's USB vendor id.
		espOpts.ResetBridge = detectBridgeBoard(port)
		// Force the espflash reset strategy to match the requested chip (see
		// the ELF branch). Without this, ChipType stays ChipAuto and the
		// ESP32-P4 never enters download mode under the classic reset.
		espOpts.ChipType = espflashChipType(req.Chip)
		// Force the espflash reset strategy to match the requested chip.
		// Without this, ChipType stays ChipAuto and enterBootloaderReset()
		// falls back to the classic DTR/RTS reset, which cannot latch the
		// ESP32-P4's native USB-Serial-JTAG peripheral into download mode.
		// forceP4Download() is only selected when ChipType == ChipESP32P4.
		espOpts.ChipType = espflashChipType(req.Chip)

		// ESP32-P4: the RAM-stub flash path stalls on this chip — the
		// compressed path (0x10/11/12) hangs at FLASH_DEFL_END, and the
		// uncompressed path (0x02/03/04) times out at FlashData because the
		// stub path skips the post-command Drain() that the P4's one-command
		// pipeline lag needs. espflash's known-working path for P4 is ROM
		// mode (--no-stub): no stub, uncompressed flash, Drain() applied.
		// Skip the stub so the ROM bootloader handles the flash directly,
		// mirroring espflash exactly. See wiki/esp32-p4-flash-solution.md.
		if req.Chip == chips.ChipESP32P4 {
			espOpts.SkipStub = true
		}

		emitPhase(req.Phase, PhaseConnecting)

		flasher, err := espflash.New(ctx, port, espOpts)
		if err != nil {
			log.Error().Err(err).Str("port", port).Str("original", req.Port).Msg("Failed to create flasher")
			return &FlashResult{Success: false, Error: err}
		}
		defer func() { _ = flasher.Close() }()

		log.Info().Str("port", port).Str("original", req.Port).Msg("Chip detected")

		// Log detected chip for visibility
		chipName := flasher.ChipName()
		log.Info().Str("chip", chipName).Msg("Detected chip")

		// The revision selects the revision-aware ESP32-P4 bootloader
		// variant (v0 for rev < 3.0, v3 for rev >= 3.0); it is ignored for
		// every other chip. 0 means "unknown" and falls back to v0.
		revision := flasher.Revision()
		log.Info().Int("revision", revision).Msg("Detected chip revision")

		bin, err := ConvertELFToESPImage(firmware, chip, revision)
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

		// Flash ALL parts (bootloader + partition table + app) in a SINGLE
		// compressed-flash session, matching esptool's write_flash flow:
		//
		//   one attach_flash, one change_baud, then per-part
		//   FLASH_DEFL_BEGIN / FLASH_DEFL_DATA / MD5, then FLASH_DEFL_END once.
		//
		// The previous code flashed the app via a separate FlashImage call,
		// which re-ran SPI_ATTACH (attachFlash) and CHANGE_BAUDRATE
		// (changeBaud) after the bootloader + partition table had already been
		// written. Per the ESP32-S3 serial protocol these are one-time
		// link/session-setup commands: re-issuing SPI_ATTACH while the flash
		// controller is mid-transaction can stall it, and a second
		// CHANGE_BAUDRATE with a stale old-baud argument makes the stub compute
		// the wrong UART divisor / reject the command, so the next
		// FLASH_DEFL_BEGIN is silently discarded and we get
		// "begin compressed flash download: timeout" at 0x10000. Flashing every
		// part in one call (as esptool does) avoids both.
		//
		// patchImageHeader is a no-op for the raw bootloader/partition parts
		// (magic != 0xE9) and correctly patches the app's ESP image header, so
		// a single FlashImages call handles every part.
		totalBytes := 0
		allParts := make([]espflash.ImagePart, 0, len(parts))
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

			allParts = append(allParts, espflash.ImagePart{Offset: part.Offset, Data: part.Data})
			totalBytes += len(part.Data)
		}

		// Flash the whole image in one session.
		emitPhase(req.Phase, PhaseFlashing)
		if err := flasher.FlashImages(allParts, nil); err != nil {
			log.Error().Err(err).Msg("Flash parts failed")
			return &FlashResult{Success: false, Error: err}
		}

		flasher.Reset()
		log.Info().Msg("Flash complete")
		return &FlashResult{
			Success: true,
			Bytes:   totalBytes,
		}
	}

	logger := &flashLogger{port: port}

	espOpts := espflash.DefaultOptions()
	espOpts.BaudRate = f.opts.BaudRate
	espOpts.FlashBaudRate = f.opts.FlashBaudRate
	espOpts.Compress = f.opts.Compress
	espOpts.Erase = f.opts.Erase
	espOpts.Logger = logger
	// Disable FastMode for USB CDC ports - they re-enumerate after reset
	espOpts.FastMode = f.opts.FastMode && !isUSBPort(port)
	espOpts.SkipUnchanged = f.opts.SkipUnchanged
	// CH340/CH9102/CP210x boards are USB-UART bridges (no native
	// USB-JTAG): force the DTR/RTS bootloader reset and skip the dead
	// USB-JTAG path. Detected from the board's USB vendor id.
	espOpts.ResetBridge = detectBridgeBoard(port)

	emitPhase(req.Phase, PhaseConnecting)

	flasher, err := espflash.New(ctx, port, espOpts)
	if err != nil {
		log.Error().Err(err).Str("port", port).Str("original", req.Port).Msg("Failed to create flasher")
		return &FlashResult{Success: false, Error: err}
	}
	defer func() { _ = flasher.Close() }()

	log.Info().Str("port", port).Str("original", req.Port).Msg("Chip detected")

	// Log detected chip for visibility
	chipName := flasher.ChipName()
	log.Info().Str("chip", chipName).Msg("Detected chip")

	log.Info().Int("bytes", len(firmware)).Msg("Starting flash")

	var progressFunc espflash.ProgressFunc
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

	// Begin the write phase so the client switches from "connecting" to
	// "flashing" the moment the chip accepts the flash.
	emitPhase(req.Phase, PhaseFlashing)

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

		bin, err := ConvertELFToESPImage(firmware, chipType, 0)
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
	log.Info().Str("port", l.port).Msgf(format, args...)
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
	// Resolve stable device path to actual port
	port, err := resolveDevicePathWithRetry(ctx, req.Port, maxResolveTimeout)
	if err != nil {
		return &ReadFlashResult{Success: false, Error: fmt.Errorf("resolve device path: %w", err)}
	}
	log.Info().Str("port", port).Str("original", req.Port).Uint32("address", req.Address).Uint32("size", req.Size).Msg("Reading flash")

	// Build flasher options
	espOpts := espflash.DefaultOptions()
	espOpts.BaudRate = f.opts.BaudRate
	espOpts.FlashBaudRate = f.opts.FlashBaudRate
	espOpts.Compress = f.opts.Compress
	espOpts.Erase = f.opts.Erase
	espOpts.Logger = &flashLogger{port: port}
	// Disable FastMode for USB CDC ports - they re-enumerate after reset
	espOpts.FastMode = f.opts.FastMode && !isUSBPort(port)
	espOpts.SkipUnchanged = f.opts.SkipUnchanged

	// Open connection
	flasher, err := espflash.New(ctx, port, espOpts)
	if err != nil {
		return &ReadFlashResult{Success: false, Error: fmt.Errorf("connect: %w", err)}
	}
	defer func() { _ = flasher.Close() }()

	log.Info().Str("port", port).Msg("Chip detected")
	log.Info().Str("chip", flasher.ChipName()).Msg("Detected chip")

	// Read flash data
	data, err := flasher.ReadFlash(req.Address, req.Size)
	if err != nil {
		return &ReadFlashResult{Success: false, Error: fmt.Errorf("read flash: %w", err)}
	}

	log.Info().Int("bytes_read", len(data)).Msg("Flash read completed")
	return &ReadFlashResult{Success: true, Data: data}
}

// EraseRequest contains parameters for erasing flash memory
type EraseRequest struct {
	Port     string
	Address  uint32
	Size     uint32
	EraseAll bool
	Progress chan int
	Phase    chan string
	Chip     chips.Chip
}

// EraseResult contains the result of a flash erase operation
type EraseResult struct {
	Success bool
	Error   error
	Bytes   int
}

// EraseFlash erases the device's flash memory
func (f *Flasher) EraseFlash(ctx context.Context, req *EraseRequest) *EraseResult {
	// Resolve stable device path to actual port
	port, err := resolveDevicePathWithRetry(ctx, req.Port, maxResolveTimeout)
	if err != nil {
		return &EraseResult{Success: false, Error: fmt.Errorf("resolve device path: %w", err)}
	}
	log.Info().Str("port", port).Str("original", req.Port).Uint32("address", req.Address).Uint32("size", req.Size).Bool("erase_all", req.EraseAll).Msg("Erasing flash")

	// Check if using virtual device
	if virtual.IsVirtualPath(req.Port) {
		return f.eraseVirtual(ctx, req)
	}

	// Build flasher options
	espOpts := espflash.DefaultOptions()
	espOpts.BaudRate = f.opts.BaudRate
	espOpts.FlashBaudRate = f.opts.FlashBaudRate
	espOpts.Compress = f.opts.Compress
	espOpts.Erase = f.opts.Erase
	espOpts.Logger = &flashLogger{port: port}
	// Disable FastMode for USB CDC ports - they re-enumerate after reset
	espOpts.FastMode = f.opts.FastMode && !isUSBPort(port)
	espOpts.SkipUnchanged = f.opts.SkipUnchanged

	// Force the espflash reset strategy to match the requested chip (see
	// Flash()). Without this, ChipType stays ChipAuto and the ESP32-P4
	// never enters download mode under the classic reset.
	espOpts.ChipType = espflashChipType(req.Chip)
	// Open connection
	emitPhase(req.Phase, PhaseConnecting)

	flasher, err := espflash.New(ctx, port, espOpts)
	if err != nil {
		return &EraseResult{Success: false, Error: fmt.Errorf("connect: %w", err)}
	}
	defer flasher.Close()

	log.Info().Str("port", port).Msg("Chip detected")
	log.Info().Str("chip", flasher.ChipName()).Msg("Detected chip")

	// Report initial progress
	if req.Progress != nil {
		select {
		case req.Progress <- 10:
		default:
		}
	}

	emitPhase(req.Phase, PhaseErase)

	var eraseSize int
	if req.EraseAll {
		// Erase entire flash
		if err := flasher.EraseFlash(); err != nil {
			return &EraseResult{Success: false, Error: fmt.Errorf("erase flash: %w", err)}
		}
		// Flash size varies by chip, use 4MB as default reporting value
		eraseSize = 4 * 1024 * 1024
		log.Info().Int("bytes", eraseSize).Msg("Entire flash erased")
	} else {
		// Erase specific region
		if err := flasher.EraseRegion(req.Address, req.Size); err != nil {
			return &EraseResult{Success: false, Error: fmt.Errorf("erase region: %w", err)}
		}
		eraseSize = int(req.Size)
		log.Info().Uint32("address", req.Address).Uint32("size", req.Size).Msg("Flash region erased")
	}

	// Report completion
	if req.Progress != nil {
		select {
		case req.Progress <- 100:
		default:
		}
	}

	log.Info().Msg("Flash erase completed")
	return &EraseResult{
		Success: true,
		Bytes:   eraseSize,
	}
}

// eraseVirtual erases a virtual device's flash memory
func (f *Flasher) eraseVirtual(ctx context.Context, req *EraseRequest) *EraseResult {
	chip := virtual.ChipFromVirtualPath(req.Port)
	deviceID := req.Port
	if deviceID == ":virtual:" {
		deviceID = "default"
	}

	log.Info().Str("device", req.Port).Str("chip", chip).Msg("Using virtual flash device for erase")

	device, err := virtual.OpenDevice(deviceID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to open virtual device")
		return &EraseResult{Success: false, Error: err}
	}
	defer device.Close()

	// Report initial progress
	if req.Progress != nil {
		select {
		case req.Progress <- 10:
		default:
		}
	}

	var eraseSize int
	if req.EraseAll {
		// For virtual device, get size and zero it out
		flashSize := device.Size()
		eraseSize = int(flashSize)

		// Zero out entire flash
		zeros := make([]byte, 4096)
		for offset := uint32(0); offset < flashSize; offset += 4096 {
			remaining := flashSize - offset
			if remaining < 4096 {
				zeros = zeros[:remaining]
			}
			if err := device.Write(offset, zeros); err != nil {
				return &EraseResult{Success: false, Error: fmt.Errorf("virtual erase: %w", err)}
			}
		}
		log.Info().Int("bytes", eraseSize).Msg("Entire virtual flash erased")
	} else {
		// Erase specific region
		eraseSize = int(req.Size)

		// Zero out the region
		zeros := make([]byte, req.Size)
		if err := device.Write(req.Address, zeros); err != nil {
			return &EraseResult{Success: false, Error: fmt.Errorf("virtual erase region: %w", err)}
		}
		log.Info().Uint32("address", req.Address).Uint32("size", req.Size).Msg("Virtual flash region erased")
	}

	// Report completion
	if req.Progress != nil {
		select {
		case req.Progress <- 100:
		default:
		}
	}

	log.Info().Msg("Virtual flash erase completed")
	return &EraseResult{
		Success: true,
		Bytes:   eraseSize,
	}
}

// espflashChipType maps a chips.Chip onto the espflash.ChipType used to
// select the bootloader reset strategy. It returns espflash.ChipAuto for
// unknown chips (chip == 0) and for chips espflash does not support, so the
// espflash layer keeps auto-detecting after connect. Chip==0 is treated as
// "unknown" rather than "ESP32" because resolveFlashChip returns 0 when it
// could not determine the chip.
func espflashChipType(c chips.Chip) espflash.ChipType {
	switch c {
	case chips.ChipESP32S2:
		return espflash.ChipESP32S2
	case chips.ChipESP32S3:
		return espflash.ChipESP32S3
	case chips.ChipESP32C3:
		return espflash.ChipESP32C3
	case chips.ChipESP32C6:
		return espflash.ChipESP32C6
	case chips.ChipESP32H2:
		return espflash.ChipESP32H2
	case chips.ChipESP32C2:
		return espflash.ChipESP32C2
	case chips.ChipESP32C5:
		return espflash.ChipESP32C5
	case chips.ChipESP32C61:
		return espflash.ChipESP32C6
	case chips.ChipESP32P4:
		return espflash.ChipESP32P4
	default:
		// 0 (unknown) and unsupported chips (ESP32-H4, ESP32-S31) -> auto.
		return espflash.ChipAuto
	}
}

// isUSBPort detects USB CDC ports that re-enumerate after reset.
// These include /dev/ttyACM* (Linux USB CDC) and /dev/cu.usb* (macOS).
// FastMode must be disabled for these ports to allow port reopen.
func isUSBPort(port string) bool {
	return len(port) > 11 && port[:11] == "/dev/ttyACM" || len(port) > 11 && port[:11] == "/dev/cu.usb"
}

// maxResolveTimeout bounds how long resolveDevicePathWithRetry waits for a
// power-cycled or freshly-reset USB device to re-enumerate and reappear under
// its /dev/serial/by-id link. A full hub power cycle drops the USB link and the
// kernel (plus udev) needs a moment to mint the symlink again; 15s is a
// comfortable upper bound that still fails fast enough to surface a genuinely
// dead or detached board.
const maxResolveTimeout = 15 * time.Second

// resolveDevicePathWithRetry resolves a /dev/serial/by-id path to its kernel
// node, retrying while the symlink is temporarily absent.
//
// A power-cycled or freshly-reset USB-UART board drops its by-id symlink for
// the whole re-enumeration window (udev has not yet created the link), so a
// single readlink fails with ENOENT even though the board is perfectly healthy.
// Retrying lets the resolution wait out that window instead of failing the
// whole operation. This is the common failure mode right after a hub power
// cycle (see devicesleep.PowerCycle) or a DTR/RTS bootloader reset, both of
// which disconnect the USB link.
//
// timeout bounds the total wall-clock wait; 0 defers only to ctx (retry until
// the context is cancelled). A non-by-id path or a non-Linux host is returned
// immediately, without any retrying.
func resolveDevicePathWithRetry(ctx context.Context, path string, timeout time.Duration) (string, error) {
	if runtime.GOOS != "linux" {
		return path, nil
	}

	// If it's already a direct device path, return as-is
	if !strings.HasPrefix(path, "/dev/serial/by-id/") {
		return path, nil
	}

	// path is <byIdDir>/<name>; retry reading that single link.
	return resolveByIDLink(ctx, filepath.Dir(path), filepath.Base(path), timeout)
}

// resolveByIDLink reads a single /dev/serial/by-id/<name> link inside byIdDir,
// retrying while the link is temporarily absent.
//
// A power-cycled or freshly-reset USB-UART board drops its by-id link for the
// whole re-enumeration window (udev has not yet created it), so a single
// readlink fails with ENOENT even though the board is healthy. Retrying lets
// this wait out that window instead of failing the whole operation. This is the
// common failure mode right after a hub power cycle (see devicesleep.PowerCycle)
// or a DTR/RTS bootloader reset, both of which disconnect the USB link.
//
// timeout bounds the total wall-clock wait; 0 defers only to ctx (retry until
// the context is cancelled).
func resolveByIDLink(ctx context.Context, byIdDir, name string, timeout time.Duration) (string, error) {
	const retryGap = 250 * time.Millisecond
	deadline := time.Now().Add(timeout)

	var lastErr error
	for {
		// Convert relative symlink target to absolute path.
		target, err := os.Readlink(filepath.Join(byIdDir, name))
		if err == nil {
			resolved := filepath.Join(byIdDir, target)
			log.Debug().Str("by-id", filepath.Join(byIdDir, name)).Str("resolved", resolved).Msg("Device path resolved")
			return resolved, nil
		}
		lastErr = fmt.Errorf("read symlink %s: %w", filepath.Join(byIdDir, name), err)

		// Decide whether to keep waiting out the re-enumeration window. We stop
		// when the wall-clock deadline (when set) is exceeded or the context is
		// cancelled.
		if timeout > 0 && !time.Now().Before(deadline) {
			break
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}

		timer := time.NewTimer(retryGap)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "", lastErr
}

// bridgeVendors lists the USB vendor ids of the UART-bridge chips whose boards
// are reset purely with DTR/RTS (no native USB-JTAG peripheral). A board is
// treated as a bridge when its /dev/serial/by-id name encodes one of these.
var bridgeVendors = []string{"1a86", "3171", "10c4", "0403"}

// detectBridgeBoard reports whether the given serial port sits behind a
// USB-UART bridge (CH9102/CH340/CH341, CP210x, FTDI) rather than a native
// USB-JTAG/Serial endpoint. Bridge boards have no USB-JTAG peripheral, so
// flashing must use the DTR/RTS reset (FlasherOptions.ResetBridge), not the
// USB-JTAG path.
//
// It matches the /dev/serial/by-id symlink whose target is `port`: the
// symlink name encodes the USB vendor id (e.g.
// "usb-1a86_USB_Single_Serial_…"), the reliable, driver-independent signal.
// It is a no-op (returns false) on non-Linux hosts or when the by-id node is
// unavailable, so callers can set the flag unconditionally.
func detectBridgeBoard(port string) bool {
	return detectBridgeBoardIn(port, "/dev/serial/by-id")
}

// detectBridgeBoardIn is the testable core of detectBridgeBoard: it looks the
// port up in the given /dev/serial/by-id directory. See detectBridgeBoard for
// the semantics.
func detectBridgeBoardIn(port string, byIDDir string) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	portBase := filepath.Base(port)
	entries, err := os.ReadDir(byIDDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name() // e.g. "usb-1a86_USB_Single_Serial_5971084553-if00"
		target, err := os.Readlink(filepath.Join(byIDDir, name))
		if err != nil {
			continue
		}
		// The symlink target is the kernel node name ("ttyACM5") or a full
		// path ("/dev/ttyACM5"); compare on the basename.
		if filepath.Base(target) != portBase {
			continue
		}
		return vendorInName(name)
	}
	return false
}

// vendorInName reports whether a /dev/serial/by-id name encodes a bridge
// vendor id. The name begins with "usb-<vid>"; the match is bounded so a 4-hex
// vendor id never matches a longer id (e.g. "1a86" must not match "1a860").
func vendorInName(name string) bool {
	lower := strings.ToLower(name)
	for _, vid := range bridgeVendors {
		prefix := "usb-" + vid
		if strings.HasPrefix(lower, prefix) {
			rest := lower[len(prefix):]
			if rest == "" || !isHexChar(rest[0]) {
				return true
			}
		}
	}
	return false
}

// isHexChar reports whether b is a lowercase hex digit (0-9, a-f).
func isHexChar(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f'
}
