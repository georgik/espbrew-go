package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/device"
	"codeberg.org/georgik/espbrew-go/internal/inventory"
	"codeberg.org/georgik/espbrew-go/internal/project"
	"codeberg.org/georgik/espbrew-go/internal/snap"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var snapCmd = &cobra.Command{
	Use:   "snap [flags]",
	Short: "Capture snapshot from camera-enabled ESP device",
	RunE:  runSnapCmd,
}

var snapOpts struct {
	clusterURL    string
	deviceID      string
	port          string
	firmware      string
	duration      int
	baud          int
	camera        string
	output        string
	forceFlash    bool
	skipFlash     bool
	noCapture     bool
	noMonitor     bool
	saveDir       string
	leader        string
	jobID         string
	displayPreset bool
}

func init() {
	snapCmd.Flags().StringVar(&snapOpts.clusterURL, "cluster", os.Getenv("ESPBREW_CLUSTER"), "Cluster URL for remote capture")
	snapCmd.Flags().StringVar(&snapOpts.deviceID, "device", "", "Device selection by ID, alias, or MAC (from inventory)")
	snapCmd.Flags().StringVarP(&snapOpts.port, "port", "p", "", "Serial port (auto-detect if empty)")
	snapCmd.Flags().StringVarP(&snapOpts.firmware, "firmware", "f", "", "Firmware .bin file to flash before capture")
	snapCmd.Flags().IntVar(&snapOpts.duration, "duration", 10, "Capture duration in seconds")
	snapCmd.Flags().IntVar(&snapOpts.baud, "baud-rate", 115200, "Serial baud rate")
	snapCmd.Flags().StringVar(&snapOpts.camera, "camera", "", "Camera ID (empty for auto-select first available)")
	snapCmd.Flags().StringVarP(&snapOpts.output, "output", "o", "", "Output file path for captured image")
	snapCmd.Flags().BoolVar(&snapOpts.forceFlash, "force-flash", false, "Force flash even if firmware hash matches")
	snapCmd.Flags().BoolVar(&snapOpts.skipFlash, "skip-flash", false, "Skip flashing step")
	snapCmd.Flags().BoolVar(&snapOpts.noCapture, "no-capture", false, "Skip image capture (flash only)")
	snapCmd.Flags().BoolVar(&snapOpts.noMonitor, "no-monitor", false, "Skip serial monitor after flash")
	snapCmd.Flags().StringVar(&snapOpts.saveDir, "save-dir", "", "Directory to save captured images")
	snapCmd.Flags().StringVar(&snapOpts.leader, "leader", os.Getenv("ESPBREW_LEADER"), "Leader address for cluster mode")
	snapCmd.Flags().StringVar(&snapOpts.jobID, "job-id", "", "Job ID for resuming operations")
	snapCmd.Flags().BoolVar(&snapOpts.displayPreset, "display-preset", false, "Apply display photography preset (Linux only)")

	rootCmd.AddCommand(snapCmd)
}

func runSnapCmd(cmd *cobra.Command, args []string) error {
	if snapOpts.clusterURL != "" {
		return runClusterSnap()
	}
	return runLocalSnap()
}

func runLocalSnap() error {
	// Resolve device from inventory if --device specified
	if snapOpts.deviceID != "" {
		port, err := resolveSnapDevice()
		if err != nil {
			return err
		}
		snapOpts.port = port
	}

	if snapOpts.port == "" {
		scanner := device.NewScanner()
		espPorts, err := scanner.ScanESP()
		if err != nil || len(espPorts) == 0 {
			return fmt.Errorf("--port required or no ESP devices found")
		}
		snapOpts.port = espPorts[0].Path
		log.Info().Str("auto_port", snapOpts.port).Msg("Auto-detected ESP device")
	}

	// Detect firmware if not specified
	if snapOpts.firmware == "" && !snapOpts.skipFlash {
		fw, err := detectFirmware()
		if err != nil {
			log.Warn().Err(err).Msg("Could not auto-detect firmware")
			return fmt.Errorf("please specify --firmware or --skip-flash")
		}
		snapOpts.firmware = fw
		log.Info().Str("firmware", snapOpts.firmware).Msg("Auto-detected firmware")
	}

	// Create snap executor with resolved parameters
	duration := time.Duration(snapOpts.duration) * time.Second
	executor := snap.NewExecutor(snapOpts.port, duration)

	// Configure executor with flag options
	executor.SetBaudRate(snapOpts.baud)
	executor.SetCameraID(snapOpts.camera)
	executor.SetNoCapture(snapOpts.noCapture)
	executor.SetNoMonitor(snapOpts.noMonitor)
	executor.SetDisplayPreset(snapOpts.displayPreset)

	// Run the snapshot workflow
	result, err := executor.Run(context.Background())
	if err != nil {
		return fmt.Errorf("snapshot failed: %w", err)
	}

	// Handle nil result case (error during execution)
	if result == nil {
		log.Error().Msg("Snapshot returned nil result")
		return fmt.Errorf("internal error: nil result from executor")
	}

	// Create output handler based on --output flag
	handler, outputErr := createSnapOutputHandler()
	if outputErr != nil {
		return fmt.Errorf("failed to create output handler: %w", outputErr)
	}

	// Write result using handler
	if writeErr := handler.Write(result); writeErr != nil {
		log.Warn().Err(writeErr).Msg("Failed to write output")
		return writeErr
	}

	// Log result summary
	log.Info().
		Str("snap_id", result.Metadata.SnapID).
		Str("status", string(result.Metadata.Status)).
		Int64("duration_ms", result.Metadata.Duration).
		Int("log_count", len(result.Logs)).
		Int("image_size", len(result.ImageData)).
		Msg("Snapshot completed")

	// Print saved file paths if save-dir was used
	if snapOpts.saveDir != "" && len(result.ImageData) > 0 {
		imagePath := fmt.Sprintf("%s/snap-%s.jpg", snapOpts.saveDir, result.Metadata.SnapID)
		metaPath := fmt.Sprintf("%s/snap-%s.json", snapOpts.saveDir, result.Metadata.SnapID)
		fmt.Printf("Saved image: %s\n", imagePath)
		fmt.Printf("Saved metadata: %s\n", metaPath)
	}

	// Return appropriate exit code based on result status
	if result.Metadata.Status == snap.SnapStatusFailed {
		return fmt.Errorf("snapshot failed: %s", result.Metadata.Error)
	}

	// Return early if no-capture flag is set (after executor runs)
	if snapOpts.noCapture {
		log.Info().Msg("Skipping capture (--no-capture)")
		return nil
	}

	return nil
}

func runClusterSnap() error {
	client := cluster.NewClient(snapOpts.clusterURL)

	// Set longer timeout for snap operations (duration + capture + overhead)
	snapTimeout := time.Duration(snapOpts.duration)*time.Second + 30*time.Second
	client.SetTimeout(snapTimeout)

	// Resolve device from inventory if --device specified
	var devicePath string
	if snapOpts.deviceID != "" {
		port, err := resolveSnapDevice()
		if err != nil {
			return err
		}
		devicePath = port
	}

	// Get available devices if device not specified
	if devicePath == "" && snapOpts.port == "" {
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
	} else if devicePath == "" {
		devicePath = snapOpts.port
	}

	// Detect firmware if not specified
	if snapOpts.firmware == "" && !snapOpts.skipFlash {
		fw, err := detectFirmware()
		if err != nil {
			log.Warn().Err(err).Msg("Could not auto-detect firmware")
			return fmt.Errorf("please specify --firmware or --skip-flash")
		}
		snapOpts.firmware = fw
		log.Info().Str("firmware", snapOpts.firmware).Msg("Auto-detected firmware")
	}

	// Step 1: Check flash hash if firmware is specified and not skipping flash
	flashNeeded := false
	if !snapOpts.skipFlash && snapOpts.firmware != "" {
		// Resolve device ID for hash check
		var resolvedDeviceID string
		if snapOpts.deviceID != "" {
			inv, err := inventory.NewInventory()
			if err != nil {
				return fmt.Errorf("load inventory: %w", err)
			}
			dev, err := findDevice(inv, snapOpts.deviceID)
			if err != nil {
				return fmt.Errorf("resolve device ID: %w", err)
			}
			resolvedDeviceID = dev.DeviceID
		} else {
			// Use device path as ID if no inventory device specified
			resolvedDeviceID = devicePath
		}

		log.Info().
			Str("device_id", resolvedDeviceID).
			Str("firmware", snapOpts.firmware).
			Msg("Checking flash hash")

		hashCheckReq := cluster.FlashHashCheckRequest{
			Firmware: snapOpts.firmware,
			Chip:     "esp32s3", // Default chip type
		}
		hashResp, err := client.CheckFlashHash(resolvedDeviceID, hashCheckReq)
		if err != nil {
			log.Warn().Err(err).Msg("Hash check failed, will flash")
			flashNeeded = true
		} else {
			if hashResp.Match && !snapOpts.forceFlash {
				log.Info().
					Str("device_hash", hashResp.DeviceHash).
					Str("firmware_hash", hashResp.FirmwareHash).
					Msg("Firmware hash matches, skipping flash")
				flashNeeded = false
			} else {
				log.Info().
					Bool("match", hashResp.Match).
					Bool("force_flash", snapOpts.forceFlash).
					Msg("Flash needed")
				flashNeeded = true
			}
		}
	}

	// Step 2: Flash if needed
	if !snapOpts.skipFlash && snapOpts.firmware != "" && (flashNeeded || snapOpts.forceFlash) {
		log.Info().Str("cluster", snapOpts.clusterURL).Str("device", devicePath).Msg("Uploading firmware to cluster")

		uploadResp, err := client.UploadFirmware(snapOpts.firmware)
		if err != nil {
			return fmt.Errorf("upload firmware: %w", err)
		}

		log.Info().Str("file_id", uploadResp.FileID).Int64("size", uploadResp.Size).Msg("Firmware uploaded")

		// Submit flash job
		submitReq := cluster.FlashSubmitRequest{
			DevicePath: devicePath,
			FileID:     uploadResp.FileID,
			ClientID:   "espbrew-snap",
		}

		flashResp, err := client.SubmitFlash(submitReq)
		if err != nil {
			return fmt.Errorf("submit flash: %w", err)
		}

		log.Info().Str("job_id", flashResp.JobID).Msg("Flash job submitted")

		// Wait for flash completion
		progressClient, err := client.ConnectProgress(flashResp.JobID)
		if err != nil {
			return fmt.Errorf("connect progress: %w", err)
		}
		defer progressClient.Close()

		completed := false
		err = progressClient.Stream(func(msg cluster.ProgressMessage) {
			switch msg.Type {
			case "progress":
				displaySnapProgressBar(msg.Progress, msg.Status)
			case "complete":
				completed = true
				if msg.Status == "completed" {
					log.Info().Msg("Flash completed successfully")
				} else {
					log.Error().Str("error", msg.Error).Msg("Flash failed")
				}
			}
		})

		if err != nil || !completed {
			return fmt.Errorf("flash did not complete successfully")
		}
	}

	// Skip capture if requested
	if snapOpts.noCapture {
		log.Info().Msg("Skipping capture (--no-capture)")
		return nil
	}

	// Resolve device ID for snap request
	var resolvedDeviceID string
	if snapOpts.deviceID != "" {
		inv, err := inventory.NewInventory()
		if err != nil {
			return fmt.Errorf("load inventory: %w", err)
		}
		dev, err := findDevice(inv, snapOpts.deviceID)
		if err != nil {
			return fmt.Errorf("resolve device ID: %w", err)
		}
		resolvedDeviceID = dev.DeviceID
	} else {
		// Use device path as ID if no inventory device specified
		resolvedDeviceID = devicePath
	}

	// Step 3: Snap (monitor+capture only)
	snapReq := cluster.SnapRequest{
		DeviceID:  resolvedDeviceID,
		Duration:  snapOpts.duration,
		CameraID:  snapOpts.camera,
		SkipFlash: true, // Snap only does monitor+capture, flash is done separately
	}

	log.Info().
		Str("device_id", resolvedDeviceID).
		Int("duration", snapReq.Duration).
		Str("camera_id", snapReq.CameraID).
		Msg("Executing snap (monitor+capture) via cluster API")

	// Execute snap request
	snapResp, err := client.ExecuteSnap(snapReq)
	if err != nil {
		return fmt.Errorf("execute snap: %w", err)
	}

	// Handle snap response
	fmt.Printf("Snap ID:  %s\n", snapResp.SnapID)
	fmt.Printf("Status:   %s\n", snapResp.Status)

	if snapResp.Error != "" {
		log.Error().Str("error", snapResp.Error).Msg("Snap completed with errors")
		return fmt.Errorf("snap failed: %s", snapResp.Error)
	}

	// Save image if --save-dir specified and image data is available
	if snapOpts.saveDir != "" && snapResp.ImageData != nil && len(snapResp.ImageData) > 0 {
		imagePath := fmt.Sprintf("%s/snap-%s.jpg", snapOpts.saveDir, snapResp.SnapID)
		if err := os.WriteFile(imagePath, snapResp.ImageData, 0644); err != nil {
			log.Warn().Err(err).Msg("Failed to save image")
		} else {
			fmt.Printf("Saved image: %s\n", imagePath)
		}

		// Save metadata
		metaPath := fmt.Sprintf("%s/snap-%s.json", snapOpts.saveDir, snapResp.SnapID)
		metaData, err := json.MarshalIndent(snapResp.Metadata, "", "  ")
		if err != nil {
			log.Warn().Err(err).Msg("Failed to marshal metadata")
		} else {
			if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
				log.Warn().Err(err).Msg("Failed to save metadata")
			} else {
				fmt.Printf("Saved metadata: %s\n", metaPath)
			}
		}
	}

	// Print log summary if available
	if snapResp.Metadata != nil && snapResp.Metadata.LogEntryCount > 0 {
		fmt.Printf("Log entries: %d\n", snapResp.Metadata.LogEntryCount)
	}

	log.Info().
		Str("snap_id", snapResp.SnapID).
		Str("status", snapResp.Status).
		Msg("Cluster snap completed")

	return nil
}

// detectFirmware auto-detects firmware path from current project
func detectFirmware() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	projType, detector := projectRegistry.Detect(cwd)
	if projType == project.ProjectTypeNone {
		return "", fmt.Errorf("no supported project detected")
	}

	log.Info().Str("type", string(projType)).Str("dir", cwd).Msg("Detected project")

	buildDir, err := detector.FindBuildDir(cwd)
	if err != nil {
		return "", fmt.Errorf("find build directory: %w", err)
	}

	artifacts, err := detector.GetArtifacts(buildDir)
	if err != nil {
		return "", fmt.Errorf("get artifacts: %w", err)
	}

	if artifacts.App == "" {
		return "", fmt.Errorf("no application binary found")
	}

	return artifacts.App, nil
}

// resolveSnapDevice resolves device identifier to port path using inventory for snap command
func resolveSnapDevice() (string, error) {
	if snapOpts.deviceID == "" {
		return "", fmt.Errorf("no device identifier specified")
	}

	inv, err := inventory.NewInventory()
	if err != nil {
		return "", fmt.Errorf("load inventory: %w", err)
	}

	dev, err := findDevice(inv, snapOpts.deviceID)
	if err != nil {
		return "", err
	}

	if dev.LastPath == "" {
		return "", fmt.Errorf("device %s has no recorded path (probe device first)", dev.DeviceID)
	}

	log.Info().Str("device_id", dev.DeviceID).Str("path", dev.LastPath).Msg("Resolved device from inventory")
	return dev.LastPath, nil
}

func displaySnapProgressBar(progress int, status string) {
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

// createSnapOutputHandler creates an output handler based on --output flag.
// It parses the format from the flag and determines the output destination.
func createSnapOutputHandler() (*snap.Handler, error) {
	// Parse format from --output flag
	format := parseSnapOutputFormat()

	// Determine output writer (stdout or file)
	var output io.Writer = os.Stdout

	// If --output specifies a file (not just a format), open it for writing
	if snapOpts.output != "" && !isFormatOnly(snapOpts.output) {
		f, err := os.Create(snapOpts.output)
		if err != nil {
			return nil, fmt.Errorf("create output file: %w", err)
		}
		output = f
	}

	// Create handler with format, output, and save-dir
	return snap.NewHandler(format, output, snapOpts.saveDir), nil
}

// parseSnapOutputFormat parses the output format from the --output flag.
// Rules:
// - If --output ends with .json, use JSON format
// - If --output is "json", use JSON format
// - If --output is "text", use text format
// - If --output is "compact", use compact format
// - Default: text format to stdout
func parseSnapOutputFormat() snap.OutputFormat {
	output := snapOpts.output

	// If output is empty, default to text
	if output == "" {
		return snap.OutputFormatText
	}

	// If output ends with .json, use JSON format
	if strings.HasSuffix(output, ".json") {
		return snap.OutputFormatJSON
	}

	// Check for explicit format names
	switch strings.ToLower(output) {
	case "json":
		return snap.OutputFormatJSON
	case "text":
		return snap.OutputFormatText
	case "compact":
		return snap.OutputFormatCompact
	default:
		// Default to text format for unrecognized values
		return snap.OutputFormatText
	}
}

// isFormatOnly returns true if the output string is just a format specifier
// (like "json", "text", "compact") rather than a file path.
func isFormatOnly(output string) bool {
	switch strings.ToLower(output) {
	case "json", "text", "compact":
		return true
	default:
		return false
	}
}
