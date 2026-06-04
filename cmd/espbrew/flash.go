package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/device"
	flashlib "codeberg.org/georgik/espbrew-go/internal/flash"
	"codeberg.org/georgik/espbrew-go/internal/inventory"
	"codeberg.org/georgik/espbrew-go/internal/project"
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
	deviceID        string // Device selection by ID, alias, or MAC
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
	// Hash-based flash optimization
	skipHashCheck bool // Skip hash check (disable optimization)
	// Multi-image mode
	bootloader string
	partitions string
	app        string
	// ESP-IDF integration
	buildDir string
	// Project detection
	noDetect bool
	// Device filtering
	filterBoardModel string   // Filter by board model (e.g., "ESP32-S3-BOX")
	filterTags       []string // Filter by tags (all must match)
	filterChip       string   // Filter by chip type (e.g., "ESP32-S3")
}

func init() {
	flashCmd.Flags().StringVar(&flashOpts.clusterURL, "cluster", "", "Cluster URL for remote flashing")
	flashCmd.Flags().StringVar(&flashOpts.deviceID, "device", "", "Device selection by ID, alias, or MAC (from inventory)")
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
	// Hash-based flash optimization
	flashCmd.Flags().BoolVar(&flashOpts.skipHashCheck, "skip-hash-check", false, "Skip hash-based flash detection optimization (enabled by default)")
	// Multi-image mode flags
	flashCmd.Flags().StringVar(&flashOpts.bootloader, "bootloader", "", "Bootloader .bin file (multi-image mode)")
	flashCmd.Flags().StringVar(&flashOpts.partitions, "partitions", "", "Partition table .bin file (multi-image mode)")
	flashCmd.Flags().StringVar(&flashOpts.app, "app", "", "Application .bin file (multi-image mode)")
	// ESP-IDF integration
	flashCmd.Flags().StringVar(&flashOpts.buildDir, "build-dir", "", "ESP-IDF build directory (reads flash_args)")
	// Project detection
	flashCmd.Flags().BoolVar(&flashOpts.noDetect, "no-detect", false, "Disable automatic project detection")
	// Device filtering
	flashCmd.Flags().StringVar(&flashOpts.filterBoardModel, "filter-board", "", "Filter devices by board model (e.g., ESP32-S3-BOX)")
	flashCmd.Flags().StringSliceVar(&flashOpts.filterTags, "filter-tag", []string{}, "Filter devices by tags (can be specified multiple times, all must match)")
	flashCmd.Flags().StringVar(&flashOpts.filterChip, "filter-chip", "", "Filter devices by chip type (e.g., ESP32-S3)")

	rootCmd.AddCommand(flashCmd)
}

var projectRegistry = func() *project.Registry {
	r := project.NewRegistry()
	r.Register(&project.ESPIDFDetector{})
	r.Register(&project.RustESPDetector{})
	r.Register(&project.TinyGoDetector{})
	return r
}()

func runFlash(cmd *cobra.Command, args []string) error {
	if flashOpts.clusterURL != "" {
		return runFlashRemote(args)
	}
	return runFlashLocal(args)
}

func runFlashRemote(args []string) error {
	// Auto-detect project if no paths specified
	if !flashOpts.noDetect && len(args) == 0 &&
		flashOpts.bootloader == "" && flashOpts.partitions == "" && flashOpts.app == "" && flashOpts.buildDir == "" {

		cwd, err := os.Getwd()
		if err == nil {
			projType, detector := projectRegistry.Detect(cwd)
			if projType != project.ProjectTypeNone {
				log.Info().Str("type", string(projType)).Str("dir", cwd).Msg("Detected project")

				buildDir, err := detector.FindBuildDir(cwd)
				if err == nil {
					log.Info().Str("build_dir", buildDir).Msg("Found build directory")

					artifacts, err := detector.GetArtifacts(buildDir)
					if err == nil {
						// Populate flashOpts from detected artifacts (only if not explicitly set)
						if flashOpts.bootloader == "" && artifacts.Bootloader != "" {
							flashOpts.bootloader = artifacts.Bootloader
						}
						if flashOpts.partitions == "" && artifacts.Partitions != "" {
							flashOpts.partitions = artifacts.Partitions
						}
						if flashOpts.app == "" && artifacts.App != "" {
							flashOpts.app = artifacts.App
						}

						log.Info().
							Str("bootloader", flashOpts.bootloader).
							Str("partitions", flashOpts.partitions).
							Str("app", flashOpts.app).
							Msg("Auto-populated flash paths")
					}
				}
			}
		}
	}

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

	// Resolve device from inventory if --device specified
	if flashOpts.deviceID != "" {
		port, err := resolveDevice()
		if err != nil {
			return err
		}
		flashOpts.port = port
	}

	client := cluster.NewClient(flashOpts.clusterURL)

	// Get available devices if port not specified
	var devicePath string
	if flashOpts.port == "" {
		devices, err := client.ListDevices()
		if err != nil {
			return fmt.Errorf("list devices: %w", err)
		}

		// Filter devices based on criteria
		filtered, err := filterDevices(devices)
		if err != nil {
			return err
		}

		// Find first available device from filtered list
		for _, d := range filtered {
			if d.State == "available" {
				devicePath = d.Path
				break
			}
		}

		if devicePath == "" {
			return fmt.Errorf("no matching devices on cluster (use --filter-board/--filter-tag/--filter-chip to specify criteria)")
		}

		log.Info().Str("device", devicePath).Msg("Auto-selected available device")
	} else {
		devicePath = flashOpts.port
	}

	log.Info().Str("cluster", flashOpts.clusterURL).Str("device", devicePath).Msg("Uploading firmware to cluster")

	// Hash-based flash detection (skip if disabled)
	if !flashOpts.skipHashCheck {
		if err := checkFlashStatusOptimization(client, devicePath, firmwarePath); err != nil {
			log.Warn().Err(err).Msg("Hash-based optimization failed, proceeding with full flash")
		}
	}

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

// resolveDevice resolves device identifier to port path using inventory
func resolveDevice() (string, error) {
	if flashOpts.deviceID == "" {
		return "", fmt.Errorf("no device identifier specified")
	}

	inv, err := inventory.NewInventory()
	if err != nil {
		return "", fmt.Errorf("load inventory: %w", err)
	}

	dev, err := findDevice(inv, flashOpts.deviceID)
	if err != nil {
		return "", err
	}

	// Get last known path
	if dev.LastPath == "" {
		return "", fmt.Errorf("device %s has no recorded path (probe device first)", dev.DeviceID)
	}

	log.Info().Str("device_id", dev.DeviceID).Str("path", dev.LastPath).Msg("Resolved device from inventory")
	return dev.LastPath, nil
}

func runFlashLocal(args []string) error {
	// Resolve device from inventory if --device specified
	if flashOpts.deviceID != "" {
		port, err := resolveDevice()
		if err != nil {
			return err
		}
		flashOpts.port = port
	}

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

	opts := &flashlib.FlasherOptions{
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

func runMultiImage(opts *flashlib.FlasherOptions) error {
	var images []imageToFlash

	// Collect images to flash
	if flashOpts.bootloader != "" {
		// Default to 0x0 (ESP32-S3 and most newer chips)
		// For ESP32/ESP32-S2, user must specify --chip explicitly
		offset := 0x0
		if flashOpts.chip != "auto" && flashOpts.chip != "" {
			if off, ok := flashlib.BootloaderOffset(flashOpts.chip); ok {
				offset = int(off)
			}
		}
		images = append(images, imageToFlash{name: "bootloader", path: flashOpts.bootloader, offset: offset})
	}

	if flashOpts.partitions != "" {
		images = append(images, imageToFlash{name: "partitions", path: flashOpts.partitions, offset: flashlib.PresetOffsetPartitions})
	}

	if flashOpts.app != "" {
		images = append(images, imageToFlash{name: "app", path: flashOpts.app, offset: flashlib.PresetOffsetApp})
	}

	log.Info().Int("images", len(images)).Msg("Multi-image flash mode")

	totalSize := 0
	for _, img := range images {
		data, err := os.ReadFile(img.path)
		if err != nil {
			return fmt.Errorf("read %s: %w", img.name, err)
		}
		totalSize += len(data)
	}

	log.Info().Int("total_bytes", totalSize).Msg("Multi-image flash")

	flasher := flashlib.NewFlasher(opts)

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

		req := &flashlib.FlashRequest{
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

func runSingleImage(opts *flashlib.FlasherOptions, firmwarePath string) error {
	data, err := os.ReadFile(firmwarePath)
	if err != nil {
		return fmt.Errorf("read firmware: %w", err)
	}

	// Detect file type
	fileType := flashlib.DetectFileType(data)
	log.Info().Str("type", fileType.String()).Msg("Detected file type")

	// Resolve offset from preset
	offset := flashOpts.offset
	if flashOpts.preset != "" {
		switch flashOpts.preset {
		case "bootloader":
			// Default to 0x0 (ESP32-S3 and most newer chips)
			offset = 0x0
			if flashOpts.chip != "auto" && flashOpts.chip != "" {
				if off, ok := flashlib.BootloaderOffset(flashOpts.chip); ok {
					offset = int(off)
				}
			}
			log.Info().Str("chip", flashOpts.chip).Int("offset", offset).Msg("Using bootloader preset")
		case "partitions":
			offset = flashlib.PresetOffsetPartitions
		case "app":
			offset = flashlib.PresetOffsetApp
		default:
			return fmt.Errorf("unknown preset: %s (use: bootloader, partitions, app)", flashOpts.preset)
		}
		log.Info().Str("preset", flashOpts.preset).Int("offset", offset).Msg("Resolved preset to offset")
	}

	log.Info().Str("port", flashOpts.port).Str("chip", flashOpts.chip).Int("offset", offset).Msg("Creating flasher")

	flasher := flashlib.NewFlasher(opts)

	progress := make(chan int, 10)
	go func() {
		for pct := range progress {
			log.Info().Int("progress", pct).Msg("Flashing")
		}
	}()

	req := &flashlib.FlashRequest{
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

	flashArgsPath, err := flashlib.FindFlashArgs(flashOpts.buildDir)
	if err != nil {
		return fmt.Errorf("flash_args not found in %s: %w", flashOpts.buildDir, err)
	}

	log.Info().Str("path", flashArgsPath).Msg("Found flash_args")

	data, err := os.ReadFile(flashArgsPath)
	if err != nil {
		return fmt.Errorf("read flash_args: %w", err)
	}

	parsed, err := flashlib.ParseFlashArgs(data)
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

	opts := &flashlib.FlasherOptions{
		BaudRate:      115200,
		FlashBaudRate: flashOpts.baud,
		Compress:      !flashOpts.noCompress,
	}

	flasher := flashlib.NewFlasher(opts)

	for i, file := range parsed.Files {
		resolvedPath := flashlib.ResolveBuildPath(flashOpts.buildDir, file.Path)
		if _, err := os.Stat(resolvedPath); err != nil {
			log.Warn().Str("path", resolvedPath).Msg("File not found, skipping")
			continue
		}

		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", resolvedPath, err)
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

		req := &flashlib.FlashRequest{
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
	// Resolve device from inventory if --device specified
	if flashOpts.deviceID != "" {
		port, err := resolveDevice()
		if err != nil {
			return err
		}
		flashOpts.port = port
	}

	client := cluster.NewClient(flashOpts.clusterURL)

	// Get available devices if port not specified
	var devicePath string
	if flashOpts.port == "" {
		devices, err := client.ListDevices()
		if err != nil {
			return fmt.Errorf("list devices: %w", err)
		}

		// Filter devices based on criteria
		filtered, err := filterDevices(devices)
		if err != nil {
			return err
		}

		for _, d := range filtered {
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
		if off, ok := flashlib.BootloaderOffset(flashOpts.chip); ok {
			bootloaderOffset = int(off)
		}
	}

	if flashOpts.bootloader != "" {
		images = append(images, remoteImage{name: "bootloader", path: flashOpts.bootloader, offset: bootloaderOffset})
	}
	if flashOpts.partitions != "" {
		images = append(images, remoteImage{name: "partitions", path: flashOpts.partitions, offset: flashlib.PresetOffsetPartitions})
	}
	if flashOpts.app != "" {
		images = append(images, remoteImage{name: "app", path: flashOpts.app, offset: flashlib.PresetOffsetApp})
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

// filterDevices filters devices based on CLI filter criteria
func filterDevices(devices []cluster.DeviceInfo) ([]cluster.DeviceInfo, error) {
	// If no filters specified, return all devices
	if flashOpts.filterBoardModel == "" && len(flashOpts.filterTags) == 0 && flashOpts.filterChip == "" {
		return devices, nil
	}

	log.Info().
		Str("board", flashOpts.filterBoardModel).
		Strs("tags", flashOpts.filterTags).
		Str("chip", flashOpts.filterChip).
		Msg("Filtering devices")

	// Filter devices by matching against API-provided metadata
	var filtered []cluster.DeviceInfo
	for _, d := range devices {
		// Check if device matches all filter criteria
		if matchesFilters(d) {
			filtered = append(filtered, d)
			log.Info().Str("device", d.Path).
				Str("board", d.BoardModel).
				Strs("tags", d.Tags).
				Msg("Device matches filter")
		}
	}

	log.Info().Int("total", len(devices)).Int("matched", len(filtered)).Msg("Device filter results")

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no devices match the specified filters")
	}

	return filtered, nil
}

// matchesFilters checks if a device from the API matches the filter criteria
func matchesFilters(d cluster.DeviceInfo) bool {
	// Check board model filter
	if flashOpts.filterBoardModel != "" && d.BoardModel != flashOpts.filterBoardModel {
		return false
	}

	// Check chip type filter
	if flashOpts.filterChip != "" && d.ChipType != flashOpts.filterChip {
		return false
	}

	// Check tags filter (all specified tags must be present)
	if len(flashOpts.filterTags) > 0 {
		for _, requiredTag := range flashOpts.filterTags {
			found := false
			for _, deviceTag := range d.Tags {
				if deviceTag == requiredTag {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}
