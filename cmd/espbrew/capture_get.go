package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var captureGetCmd = &cobra.Command{
	Use:   "get <capture-id>",
	Short: "Get device capture image",
	Long: `Retrieve a device-specific capture image.

The capture-id can be:
  - Full capture filename (e.g., "2026-06-01/cam-devic-120000.jpg")
  - Full capture directory name (e.g., "2026-06-01/cam-devic-120000")

Examples:
  espbrew capture get 2026-06-01/cam-devic-120000.jpg --device-id esp-aa:bb:cc:dd:ee:ff
  espbrew capture get 2026-06-01/cam-devic-120000 --device-id esp-aa:bb:cc:dd:ee:ff --output device.jpg`,
	RunE: runCaptureGet,
}

var captureGetOpts struct {
	deviceID string
	output   string
}

func init() {
	captureCmd.AddCommand(captureGetCmd)

	captureGetCmd.Flags().StringVar(&captureGetOpts.deviceID, "device-id", "", "Device ID (required)")
	captureGetCmd.Flags().StringVar(&captureGetOpts.output, "output", "", "Output path (default: print subimage path)")

	captureGetCmd.MarkFlagRequired("device-id")
}

func runCaptureGet(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("capture ID required")
	}

	captureID := args[0]

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	capturesDir := filepath.Join(homeDir, ".espbrew", "captures")

	// Build full path to capture or its directory
	var fullPath string

	if strings.HasSuffix(captureID, ".jpg") || strings.HasSuffix(captureID, ".jpeg") || strings.HasSuffix(captureID, ".png") {
		// Full path to image file
		fullPath = filepath.Join(capturesDir, captureID)
	} else {
		// Directory name, find the image file
		dirPath := filepath.Join(capturesDir, captureID)
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return fmt.Errorf("read capture directory: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
					fullPath = filepath.Join(dirPath, entry.Name())
					break
				}
			}
		}

		if fullPath == "" {
			return fmt.Errorf("no image file found in capture directory")
		}
	}

	// Verify file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("capture not found: %s", captureID)
	}

	// Load device captures metadata
	store, err := camera.NewStore(capturesDir)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	deviceCaptures, err := store.LoadDeviceCaptures(fullPath)
	if err != nil {
		return fmt.Errorf("load device captures: %w", err)
	}

	if len(deviceCaptures) == 0 {
		return fmt.Errorf("no device captures found for %s", captureID)
	}

	// Find capture for the requested device
	var targetCapture *camera.DeviceCaptureInfo
	for i := range deviceCaptures {
		if deviceCaptures[i].DeviceID == captureGetOpts.deviceID {
			targetCapture = &deviceCaptures[i]
			break
		}
	}

	if targetCapture == nil {
		return fmt.Errorf("no capture found for device %s", captureGetOpts.deviceID)
	}

	// Build full path to subimage
	// Subimage path is relative to capture directory
	captureDir := filepath.Dir(fullPath)
	subimagePath := filepath.Join(captureDir, targetCapture.Subimage)

	// Verify subimage exists
	if _, err := os.Stat(subimagePath); os.IsNotExist(err) {
		return fmt.Errorf("device subimage not found: %s", targetCapture.Subimage)
	}

	// Output or copy
	if captureGetOpts.output != "" {
		// Copy to output file
		data, err := os.ReadFile(subimagePath)
		if err != nil {
			return fmt.Errorf("read subimage: %w", err)
		}

		if err := os.WriteFile(captureGetOpts.output, data, 0644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}

		log.Info().
			Str("from", subimagePath).
			Str("to", captureGetOpts.output).
			Msg("Device capture copied")
	} else {
		// Print path
		relPath, _ := filepath.Rel(capturesDir, subimagePath)
		fmt.Println(filepath.Join(capturesDir, relPath))
	}

	return nil
}
