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
	preset          string
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
	// Multi-image mode
	bootloader string
	partitions string
	app        string
	// ESP-IDF integration
	buildDir string
}

func init() {
	flashCmd.Flags().StringVar(&flashOpts.clusterURL, "cluster", "", "Cluster URL for remote flashing")
	flashCmd.Flags().StringVarP(&flashOpts.port, "port", "p", "", "Serial port (auto-detect if empty)")
	flashCmd.Flags().IntVar(&flashOpts.baud, "baud", 460800, "Flash baud rate")
	flashCmd.Flags().IntVar(&flashOpts.offset, "offset", 0, "Flash offset (default 0x0, ignored with --preset)")
	flashCmd.Flags().StringVar(&flashOpts.preset, "preset", "app", "Offset preset: bootloader, partitions, app")
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
	// Multi-image mode flags
	flashCmd.Flags().StringVar(&flashOpts.bootloader, "bootloader", "", "Bootloader .bin file (multi-image mode)")
	flashCmd.Flags().StringVar(&flashOpts.partitions, "partitions", "", "Partition table .bin file (multi-image mode)")
	flashCmd.Flags().StringVar(&flashOpts.app, "app", "", "Application .bin file (multi-image mode)")
	// ESP-IDF integration
	flashCmd.Flags().StringVar(&flashOpts.buildDir, "build-dir", "", "ESP-IDF build directory (reads flash_args)")

	rootCmd.AddCommand(flashCmd)
}

func runFlash(cmd *cobra.Command, args []string) error {
	if flashOpts.clusterURL != "" {
		return runFlashRemote(args)
	}
	return runFlashLocal(args)
}

func runFlashRemote(args []string) error {
	// Check for multi-image mode
	multiImage := flashOpts.bootloader != "" || flashOpts.partitions != "" || flashOpts.app != ""
	singleImage := !multiImage && len(args) > 0

	if multiImage && len(args) > 0 {
		return fmt.Errorf("cannot use both multi-image flags and positional firmware argument")
	}

	if !multiImage && !singleImage {
		return fmt.Errorf("provide firmware.bin or use --bootloader/--partitions/--app flags or --build-dir")
	}

	if multiImage {
		return runFlashRemoteMultiImage()
	}

	// Single image mode
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
	if flashOpts.port == "" {
		scanner := device.NewScanner()
		espPorts, err := scanner.ScanESP()
		if err != nil || len(espPorts) == 0 {
			return fmt.Errorf("--port required or no ESP devices found")
		}
		flashOpts.port = espPorts[0].Path
		log.Info().Str("auto_port", flashOpts.port).Msg("Auto-detected ESP device")
	}

	// ESP-IDF build directory mode
	if flashOpts.buildDir != "" {
		return runBuildDir()
	}

	// Multi-image mode
	multiImage := flashOpts.bootloader != "" || flashOpts.partitions != "" || flashOpts.app != ""
	singleImage := !multiImage && len(args) > 0

	if multiImage && len(args) > 0 {
		return fmt.Errorf("cannot use both multi-image flags and positional firmware argument")
	}

	if !multiImage && !singleImage {
		return fmt.Errorf("provide firmware.bin or use --bootloader/--partitions/--app flags or --build-dir")
	}

	opts := &flash.FlasherOptions{
		BaudRate:      115200,
		FlashBaudRate: flashOpts.baud,
		Compress:      !flashOpts.noCompress,
	}

	if multiImage {
		return runMultiImage(opts)
	}

	return runSingleImage(opts, args[0])
}

type imageToFlash struct {
	name   string
	path   string
	offset int
}

func runMultiImage(opts *flash.FlasherOptions) error {
	var images []imageToFlash

	// Collect images to flash
	if flashOpts.bootloader != "" {
		// Default to 0x0 (ESP32-S3 and most newer chips)
		// For ESP32/ESP32-S2, user must specify --chip explicitly
		offset := 0x0
		if flashOpts.chip != "auto" && flashOpts.chip != "" {
			if off, ok := flash.BootloaderOffset(flashOpts.chip); ok {
				offset = int(off)
			}
		}
		images = append(images, imageToFlash{name: "bootloader", path: flashOpts.bootloader, offset: offset})
	}

	if flashOpts.partitions != "" {
		images = append(images, imageToFlash{name: "partitions", path: flashOpts.partitions, offset: flash.PresetOffsetPartitions})
	}

	if flashOpts.app != "" {
		images = append(images, imageToFlash{name: "app", path: flashOpts.app, offset: flash.PresetOffsetApp})
	}

	log.Info().Int("images", len(images)).Msg("Multi-image flash mode")

	totalSize := 0
	for _, img := range images {
		data, err := os.ReadFile(img.path)
		if err != nil {
			return fmt.Errorf("read %s: %w", img.name, err)
		}
		totalSize += len(data)

		fileType := flash.DetectFileType(data)
		if fileType == flash.FileTypeELF {
			return fmt.Errorf("%s: %w", img.name, flash.ErrELFNotSupported)
		}
	}

	log.Info().Int("total_bytes", totalSize).Msg("Multi-image flash")

	flasher := flash.NewFlasher(opts)

	for _, img := range images {
		data, err := os.ReadFile(img.path)
		if err != nil {
			return fmt.Errorf("read %s: %w", img.name, err)
		}

		log.Info().Str("image", img.name).Str("path", img.path).Int("offset", img.offset).Int("bytes", len(data)).Msg("Flashing")

		progress := make(chan int, 10)
		go func() {
			for pct := range progress {
				log.Info().Str("image", img.name).Int("progress", pct).Msg("Flashing")
			}
		}()

		req := &flash.FlashRequest{
			Port:     flashOpts.port,
			Firmware: data,
			Offset:   img.offset,
			Progress: progress,
		}

		result := flasher.Flash(context.Background(), req)
		close(progress)

		if !result.Success {
			return fmt.Errorf("%s flash failed: %w", img.name, result.Error)
		}

		log.Info().Str("image", img.name).Msg("Flashed successfully")
	}

	log.Info().Msg("All images flashed successfully")

	if flashOpts.monitorAfter {
		log.Info().Msg("Starting monitor...")
		monitorOpts.noRaw = flashOpts.monitorNoRaw
		monitorOpts.duration = flashOpts.monitorDuration
		monitorOpts.resetFirst = flashOpts.monitorReset
		return runMonitor(flashOpts.port, flashOpts.monitorBaud)
	}

	return nil
}

func runSingleImage(opts *flash.FlasherOptions, firmwarePath string) error {
	data, err := os.ReadFile(firmwarePath)
	if err != nil {
		return fmt.Errorf("read firmware: %w", err)
	}

	// Detect file type
	fileType := flash.DetectFileType(data)
	log.Info().Str("type", fileType.String()).Msg("Detected file type")

	if fileType == flash.FileTypeELF {
		return fmt.Errorf("%w", flash.ErrELFNotSupported)
	}

	// Resolve offset from preset
	offset := flashOpts.offset
	if flashOpts.preset != "" {
		switch flashOpts.preset {
		case "bootloader":
			// Default to 0x0 (ESP32-S3 and most newer chips)
			offset = 0x0
			if flashOpts.chip != "auto" && flashOpts.chip != "" {
				if off, ok := flash.BootloaderOffset(flashOpts.chip); ok {
					offset = int(off)
				}
			}
			log.Info().Str("chip", flashOpts.chip).Int("offset", offset).Msg("Using bootloader preset")
		case "partitions":
			offset = flash.PresetOffsetPartitions
		case "app":
			offset = flash.PresetOffsetApp
		default:
			return fmt.Errorf("unknown preset: %s (use: bootloader, partitions, app)", flashOpts.preset)
		}
		log.Info().Str("preset", flashOpts.preset).Int("offset", offset).Msg("Resolved preset to offset")
	}

	log.Info().Str("port", flashOpts.port).Str("chip", flashOpts.chip).Int("offset", offset).Msg("Creating flasher")

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
		Offset:   offset,
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

func runBuildDir() error {
	log.Info().Str("build_dir", flashOpts.buildDir).Msg("ESP-IDF build directory mode")

	flashArgsPath, err := flash.FindFlashArgs(flashOpts.buildDir)
	if err != nil {
		return fmt.Errorf("flash_args not found in %s: %w", flashOpts.buildDir, err)
	}

	log.Info().Str("path", flashArgsPath).Msg("Found flash_args")

	data, err := os.ReadFile(flashArgsPath)
	if err != nil {
		return fmt.Errorf("read flash_args: %w", err)
	}

	parsed, err := flash.ParseFlashArgs(data)
	if err != nil {
		return fmt.Errorf("parse flash_args: %w", err)
	}

	log.Info().
		Str("mode", parsed.FlashMode).
		Str("freq", parsed.FlashFreq).
		Str("size", parsed.FlashSize).
		Int("files", len(parsed.Files)).
		Msg("Parsed flash_args")

	if len(parsed.Files) == 0 {
		return fmt.Errorf("no files to flash in flash_args")
	}

	if flashOpts.flashMode == "keep" && parsed.FlashMode != "" {
		flashOpts.flashMode = parsed.FlashMode
	}
	if flashOpts.flashFreq == "keep" && parsed.FlashFreq != "" {
		flashOpts.flashFreq = parsed.FlashFreq
	}
	if flashOpts.flashSize == "keep" && parsed.FlashSize != "" {
		flashOpts.flashSize = parsed.FlashSize
	}

	opts := &flash.FlasherOptions{
		BaudRate:      115200,
		FlashBaudRate: flashOpts.baud,
		Compress:      !flashOpts.noCompress,
	}

	flasher := flash.NewFlasher(opts)

	for i, file := range parsed.Files {
		resolvedPath := flash.ResolveBuildPath(flashOpts.buildDir, file.Path)
		if _, err := os.Stat(resolvedPath); err != nil {
			log.Warn().Str("path", resolvedPath).Msg("File not found, skipping")
			continue
		}

		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", resolvedPath, err)
		}

		fileType := flash.DetectFileType(data)
		if fileType == flash.FileTypeELF {
			return fmt.Errorf("%s: %w", resolvedPath, flash.ErrELFNotSupported)
		}

		log.Info().
			Str("file", file.Path).
			Str("resolved", resolvedPath).
			Int("index", i+1).
			Int("total", len(parsed.Files)).
			Int("offset", int(file.Offset)).
			Int("bytes", len(data)).
			Msg("Flashing")

		progress := make(chan int, 10)
		go func() {
			for pct := range progress {
				log.Info().Str("file", file.Path).Int("progress", pct).Msg("Flashing")
			}
		}()

		req := &flash.FlashRequest{
			Port:     flashOpts.port,
			Firmware: data,
			Offset:   int(file.Offset),
			Progress: progress,
		}

		result := flasher.Flash(context.Background(), req)
		close(progress)

		if !result.Success {
			return fmt.Errorf("%s flash failed: %w", file.Path, result.Error)
		}

		log.Info().Str("file", file.Path).Msg("Flashed successfully")
	}

	log.Info().Msg("All files from flash_args flashed successfully")

	if flashOpts.monitorAfter {
		log.Info().Msg("Starting monitor...")
		monitorOpts.noRaw = flashOpts.monitorNoRaw
		monitorOpts.duration = flashOpts.monitorDuration
		monitorOpts.resetFirst = flashOpts.monitorReset
		return runMonitor(flashOpts.port, flashOpts.monitorBaud)
	}

	return nil
}

func runFlashRemoteMultiImage() error {
	client := cluster.NewClient(flashOpts.clusterURL)

	// Get available devices if port not specified
	var devicePath string
	if flashOpts.port == "" {
		devices, err := client.ListDevices()
		if err != nil {
			return fmt.Errorf("list devices: %w", err)
		}

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

	type remoteImage struct {
		name   string
		path   string
		offset int
	}

	var images []remoteImage

	// Determine bootloader offset based on chip
	// Default to 0x0 (ESP32-S3 and most newer chips)
	// For ESP32/ESP32-S2, user must specify --chip explicitly
	bootloaderOffset := 0x0
	if flashOpts.chip != "auto" && flashOpts.chip != "" {
		if off, ok := flash.BootloaderOffset(flashOpts.chip); ok {
			bootloaderOffset = int(off)
		}
	}

	if flashOpts.bootloader != "" {
		images = append(images, remoteImage{name: "bootloader", path: flashOpts.bootloader, offset: bootloaderOffset})
	}
	if flashOpts.partitions != "" {
		images = append(images, remoteImage{name: "partitions", path: flashOpts.partitions, offset: flash.PresetOffsetPartitions})
	}
	if flashOpts.app != "" {
		images = append(images, remoteImage{name: "app", path: flashOpts.app, offset: flash.PresetOffsetApp})
	}

	log.Info().Int("images", len(images)).Str("device", devicePath).Msg("Multi-image flash via cluster")

	// Flash each image sequentially
	for i, img := range images {
		log.Info().Str("image", img.name).Str("path", img.path).Int("offset", img.offset).
			Int("index", i+1).Int("total", len(images)).Msg("Uploading to cluster")

		uploadResp, err := client.UploadFirmware(img.path)
		if err != nil {
			return fmt.Errorf("upload %s: %w", img.name, err)
		}

		// Submit flash job with offset
		submitReq := cluster.FlashSubmitRequest{
			DevicePath: devicePath,
			FileID:     uploadResp.FileID,
			ClientID:   "espbrew-cli",
			Offset:     img.offset,
		}

		flashResp, err := client.SubmitFlash(submitReq)
		if err != nil {
			return fmt.Errorf("submit flash %s: %w", img.name, err)
		}

		log.Info().Str("image", img.name).Str("job_id", flashResp.JobID).Msg("Flash job submitted")

		// Wait for completion
		progressClient, err := client.ConnectProgress(flashResp.JobID)
		if err != nil {
			log.Warn().Err(err).Msg("Could not connect to progress WebSocket")
			return fmt.Errorf("monitoring %s: %w", img.name, err)
		}

		completed := false
		err = progressClient.Stream(func(msg cluster.ProgressMessage) {
			switch msg.Type {
			case "progress":
				displayProgressBar(msg.Progress, fmt.Sprintf("%s: %s", img.name, msg.Status))
			case "complete":
				completed = true
				if msg.Status == "completed" {
					fmt.Printf("\n✓ %s flashed successfully\n", img.name)
				} else {
					fmt.Printf("\n✗ %s flash failed: %s\n", img.name, msg.Error)
				}
			}
		})
		progressClient.Close()

		if err != nil {
			return fmt.Errorf("flash %s: %w", img.name, err)
		}

		if !completed {
			return fmt.Errorf("%s flash did not complete", img.name)
		}
	}

	log.Info().Msg("All images flashed successfully")

	if flashOpts.monitorAfter {
		log.Info().Msg("Starting monitor...")
		// Set monitor options and call existing monitor logic
		monitorOpts.clusterURL = flashOpts.clusterURL
		monitorOpts.port = devicePath
		monitorOpts.baud = flashOpts.monitorBaud
		monitorOpts.resetFirst = flashOpts.monitorReset
		monitorOpts.duration = flashOpts.monitorDuration
		monitorOpts.noRaw = flashOpts.monitorNoRaw
		return runMonitorCmd(nil, nil)
	}

	return nil
}
