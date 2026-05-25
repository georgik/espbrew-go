package flash

import (
	"context"
	"fmt"
	"io"

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
}

// Flash writes firmware to the device at port
func (f *Flasher) Flash(ctx context.Context, req *FlashRequest) *FlashResult {
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

	log.Info().Int("bytes", len(req.Firmware)).Msg("Starting flash")

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

	if err := flasher.FlashImage(req.Firmware, uint32(req.Offset), progressFunc); err != nil {
		log.Error().Err(err).Msg("Flash write failed")
		return &FlashResult{Success: false, Error: err}
	}

	flasher.Reset()

	log.Info().Msg("Flash complete")
	return &FlashResult{
		Success: true,
		Bytes:   len(req.Firmware),
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
