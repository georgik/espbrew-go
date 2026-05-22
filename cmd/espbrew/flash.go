package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/rs/zerolog/log"
	"github.com/georgik/esp-ci-cluster/internal/flash"
)

var flashCmd = &cobra.Command{
	Use:   "flash <firmware.bin>",
	Short: "Flash firmware to ESP device",
	RunE:  runFlash,
}

var flashOpts struct {
	port          string
	baud          int
	chip          string
	flashMode     string
	flashFreq     string
	flashSize     string
	eraseAll      bool
	noCompress    bool
	resetMode     string
	monitorAfter  bool
	monitorBaud   int
}

func init() {
	flashCmd.Flags().StringVarP(&flashOpts.port, "port", "p", "", "Serial port (required)")
	flashCmd.Flags().IntVar(&flashOpts.baud, "baud", 460800, "Flash baud rate")
	flashCmd.Flags().StringVar(&flashOpts.chip, "chip", "auto", "Chip type (auto, esp8266, esp32, esp32s2, esp32s3, esp32c3, esp32c6, esp32h2)")
	flashCmd.Flags().StringVar(&flashOpts.flashMode, "fm", "keep", "Flash mode (keep, qio, qout, dio, dout)")
	flashCmd.Flags().StringVar(&flashOpts.flashFreq, "ff", "keep", "Flash frequency (keep, 80m, 40m, 26m, 20m)")
	flashCmd.Flags().StringVar(&flashOpts.flashSize, "fs", "keep", "Flash size (keep, 1MB, 2MB, 4MB, 8MB, 16MB)")
	flashCmd.Flags().BoolVar(&flashOpts.eraseAll, "erase-all", false, "Erase entire flash before writing")
	flashCmd.Flags().BoolVar(&flashOpts.noCompress, "no-compress", false, "Disable compression")
	flashCmd.Flags().StringVar(&flashOpts.resetMode, "reset", "default", "Reset mode (default, no-reset, usb-jtag, auto)")
	flashCmd.Flags().BoolVarP(&flashOpts.monitorAfter, "monitor", "m", false, "Enter monitor mode after flashing")
	flashCmd.Flags().IntVar(&flashOpts.monitorBaud, "monitor-baud", 115200, "Monitor baud rate")

	rootCmd.AddCommand(flashCmd)
}

func runFlash(cmd *cobra.Command, args []string) error {
	if flashOpts.port == "" {
		return fmt.Errorf("--port is required")
	}

	if len(args) == 0 {
		return fmt.Errorf("firmware.bin path required")
	}

	firmwarePath := args[0]

	data, err := os.ReadFile(firmwarePath)
	if err != nil {
		return fmt.Errorf("read firmware: %w", err)
	}

	log.Info().Str("port", flashOpts.port).Str("chip", flashOpts.chip).Msg("Creating flasher")

	// TODO: Add support for chip, flashMode, flashFreq, flashSize, eraseAll, resetMode
	// These options require extending internal/flash wrapper

	opts := &flash.FlasherOptions{
		BaudRate:      115200,
		FlashBaudRate: flashOpts.baud,
		Compress:      !flashOpts.noCompress,
	}

	flasher := flash.NewFlasher(opts)

	progress := make(chan int, 10)
	go func() {
		for pct := range progress {
			log.Info().Int("progress", pct).Msg("Flashing")
		}
	}()

	req := &flash.FlashRequest{
		Port:     flashOpts.port,
		Firmware: data,
		Progress: progress,
	}

	log.Info().Int("bytes", len(data)).Msg("Flashing...")

	start := time.Now()
	result := flasher.Flash(cmd.Context(), req)
	close(progress)
	duration := time.Since(start)

	if !result.Success {
		return fmt.Errorf("flash failed: %w", result.Error)
	}

	log.Info().Str("duration", duration.String()).Msg("Flash complete")

	if flashOpts.monitorAfter {
		// For monitor, we need to open raw serial port
		// The internal/flash.Monitor() is not yet implemented
		log.Warn().Msg("Monitor after flash not yet implemented - use 'espbrew monitor' separately")
	}

	return nil
}
