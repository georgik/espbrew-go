package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/device"
	"codeberg.org/georgik/espbrew-go/internal/flash"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var flashCmd = &cobra.Command{
	Use:   "flash <firmware.bin>",
	Short: "Flash firmware to ESP device",
	RunE:  runFlash,
}

var flashOpts struct {
	clusterURL      string
	port            string
	baud            int
	offset          int
	chip            string
	flashMode       string
	flashFreq       string
	flashSize       string
	eraseAll        bool
	noCompress      bool
	resetMode       string
	monitorAfter    bool
	monitorBaud     int
	monitorNoRaw    bool
	monitorDuration int
	monitorReset    bool
}

func init() {
	flashCmd.Flags().StringVar(&flashOpts.clusterURL, "cluster", "", "Cluster URL for remote flashing")
	flashCmd.Flags().StringVarP(&flashOpts.port, "port", "p", "", "Serial port (auto-detect if empty)")
	flashCmd.Flags().IntVar(&flashOpts.baud, "baud", 460800, "Flash baud rate")
	flashCmd.Flags().IntVar(&flashOpts.offset, "offset", 0, "Flash offset (default 0x0)")
	flashCmd.Flags().StringVar(&flashOpts.chip, "chip", "auto", "Chip type (auto, esp8266, esp32, esp32s2, esp32s3, esp32c3, esp32c6, esp32h2)")
	flashCmd.Flags().StringVar(&flashOpts.flashMode, "fm", "keep", "Flash mode (keep, qio, qout, dio, dout)")
	flashCmd.Flags().StringVar(&flashOpts.flashFreq, "ff", "keep", "Flash frequency (keep, 80m, 40m, 26m, 20m)")
	flashCmd.Flags().StringVar(&flashOpts.flashSize, "fs", "keep", "Flash size (keep, 1MB, 2MB, 4MB, 8MB, 16MB)")
	flashCmd.Flags().BoolVar(&flashOpts.eraseAll, "erase-all", false, "Erase entire flash before writing")
	flashCmd.Flags().BoolVar(&flashOpts.noCompress, "no-compress", false, "Disable compression")
	flashCmd.Flags().StringVar(&flashOpts.resetMode, "reset", "default", "Reset mode (default, no-reset, usb-jtag, auto)")
	flashCmd.Flags().BoolVarP(&flashOpts.monitorAfter, "monitor", "m", false, "Enter monitor mode after flashing")
	flashCmd.Flags().IntVar(&flashOpts.monitorBaud, "monitor-baud", 115200, "Monitor baud rate")
	flashCmd.Flags().BoolVar(&flashOpts.monitorNoRaw, "monitor-no-raw", false, "Skip raw terminal in monitor (for testing)")
	flashCmd.Flags().IntVar(&flashOpts.monitorDuration, "monitor-duration", 0, "Monitor duration in seconds (0=no limit)")
	flashCmd.Flags().BoolVar(&flashOpts.monitorReset, "monitor-reset", false, "Reset device before monitoring")

	rootCmd.AddCommand(flashCmd)
}

func runFlash(cmd *cobra.Command, args []string) error {
	if flashOpts.clusterURL != "" {
		return runFlashRemote(args)
	}
	return runFlashLocal(args)
}

func runFlashRemote(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("firmware.bin path required")
	}

	firmwarePath := args[0]

	client := cluster.NewClient(flashOpts.clusterURL)

	// Get available devices if port not specified
	var devicePath string
	if flashOpts.port == "" {
		devices, err := client.ListDevices()
		if err != nil {
			return fmt.Errorf("list devices: %w", err)
		}

		// Find first available device
		for _, d := range devices {
			if d.State == "available" {
				devicePath = d.Path
				break
			}
		}

		if devicePath == "" {
			return fmt.Errorf("no available devices on cluster")
		}

		log.Info().Str("device", devicePath).Msg("Auto-selected available device")
	} else {
		devicePath = flashOpts.port
	}

	log.Info().Str("cluster", flashOpts.clusterURL).Str("device", devicePath).Msg("Uploading firmware to cluster")

	// Upload firmware
	uploadResp, err := client.UploadFirmware(firmwarePath)
	if err != nil {
		return fmt.Errorf("upload firmware: %w", err)
	}

	log.Info().Str("file_id", uploadResp.FileID).Int64("size", uploadResp.Size).Msg("Firmware uploaded")

	// Submit flash job
	submitReq := cluster.FlashSubmitRequest{
		DevicePath: devicePath,
		FileID:     uploadResp.FileID,
		ClientID:   "espbrew-cli",
	}

	flashResp, err := client.SubmitFlash(submitReq)
	if err != nil {
		return fmt.Errorf("submit flash: %w", err)
	}

	log.Info().Str("job_id", flashResp.JobID).Msg("Flash job submitted, streaming progress...")

	// Connect to progress WebSocket
	progressClient, err := client.ConnectProgress(flashResp.JobID)
	if err != nil {
		log.Warn().Err(err).Msg("Could not connect to progress WebSocket")
		fmt.Printf("Job ID: %s\n", flashResp.JobID)
		fmt.Printf("Status: %s\n", flashResp.Status)
		fmt.Printf("Device: %s\n", flashResp.DevicePath)
		return nil
	}
	defer progressClient.Close()

	// Stream progress
	lastProgress := -1
	err = progressClient.Stream(func(msg cluster.ProgressMessage) {
		switch msg.Type {
		case "init":
			fmt.Printf("\nJob ID: %s\n", msg.JobID)
			fmt.Printf("Device: %s\n\n", devicePath)
		case "progress":
			if msg.Progress > lastProgress {
				displayProgressBar(msg.Progress, msg.Status)
				lastProgress = msg.Progress
			}
		case "complete":
			if msg.Status == "completed" {
				fmt.Printf("\n✓ Flash completed successfully!\n")
			} else {
				fmt.Printf("\n✗ Flash failed: %s\n", msg.Error)
			}
		}
	})

	if err != nil {
		return fmt.Errorf("progress stream error: %w", err)
	}

	return nil
}

func displayProgressBar(progress int, status string) {
	const barWidth = 40
	filled := int(float64(progress) / 100.0 * float64(barWidth))
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "="
		} else {
			bar += " "
		}
	}
	fmt.Printf("\r[%s] %d%% %s", bar, progress, status)
}

func runFlashLocal(args []string) error {
	var err error
	if flashOpts.port == "" {
		scanner := device.NewScanner()
		espPorts, err := scanner.ScanESP()
		if err != nil || len(espPorts) == 0 {
			return fmt.Errorf("--port required or no ESP devices found")
		}
		flashOpts.port = espPorts[0].Path
		log.Info().Str("auto_port", flashOpts.port).Msg("Auto-detected ESP device")
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
		Offset:   flashOpts.offset,
		Progress: progress,
	}

	log.Info().Int("bytes", len(data)).Msg("Flashing...")

	start := time.Now()
	result := flasher.Flash(context.Background(), req)
	close(progress)
	duration := time.Since(start)

	if !result.Success {
		return fmt.Errorf("flash failed: %w", result.Error)
	}

	log.Info().Str("duration", duration.String()).Msg("Flash complete")

	if flashOpts.monitorAfter {
		log.Info().Msg("Starting monitor...")
		monitorOpts.noRaw = flashOpts.monitorNoRaw
		monitorOpts.duration = flashOpts.monitorDuration
		monitorOpts.resetFirst = flashOpts.monitorReset
		return runMonitor(flashOpts.port, flashOpts.monitorBaud)
	}

	return nil
}
