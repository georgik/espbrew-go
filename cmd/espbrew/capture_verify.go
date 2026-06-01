package main

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// exitCodeError wraps an error with an exit code
type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string {
	return e.err.Error()
}

func (e *exitCodeError) ExitCode() int {
	return e.code
}

var captureVerifyCmd = &cobra.Command{
	Use:   "verify --device-id <id> [output-path]",
	Short: "Capture and extract device region for verification",
	Long: `Capture an image from camera and extract the region specified by device's bounding box.

This command:
1. Gets the device's bounding box mapping
2. Captures an image from the specified camera
3. Extracts the region based on the bounding box
4. Saves to output path or auto-generated location
5. Prints the result path

Exit codes:
  0: Success
  1: No bounding box found for device
  2: Capture failed
  3: Extraction failed

Examples:
  espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff
  espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff --camera-id cam-001
  espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff --output /tmp/verify.jpg
  espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff --width 1920 --height 1080`,
	RunE: runCaptureVerify,
	// Handle exit codes from exitCodeError
	SilenceErrors: true,
	SilenceUsage:  true,
}

var captureVerifyOpts struct {
	deviceID string
	cameraID string
	output   string
	width    uint32
	height   uint32
}

func init() {
	// Store the original run function
	originalRun := runCaptureVerify

	// Wrap the run function to handle exit codes
	captureVerifyCmd.RunE = func(cmd *cobra.Command, args []string) error {
		err := originalRun(cmd, args)
		if exitErr, ok := err.(*exitCodeError); ok {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	captureCmd.AddCommand(captureVerifyCmd)

	captureVerifyCmd.Flags().StringVar(&captureVerifyOpts.deviceID, "device-id", "", "Device ID (required)")
	captureVerifyCmd.Flags().StringVar(&captureVerifyOpts.cameraID, "camera-id", "", "Camera ID (default: first available)")
	captureVerifyCmd.Flags().StringVar(&captureVerifyOpts.output, "output", "", "Output path (default: auto-generated)")
	captureVerifyCmd.Flags().Uint32Var(&captureVerifyOpts.width, "width", 1280, "Image width")
	captureVerifyCmd.Flags().Uint32Var(&captureVerifyOpts.height, "height", 720, "Image height")

	captureVerifyCmd.MarkFlagRequired("device-id")
}

func runCaptureVerify(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Open persistence store
	store, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	// Step 1: Get device bounding box mapping
	log.Info().Str("device_id", captureVerifyOpts.deviceID).Msg("Looking up bounding box")

	mappings, err := store.ListBoundingBoxesForDevice(captureVerifyOpts.deviceID)
	if err != nil {
		return fmt.Errorf("list bounding boxes: %w", err)
	}

	if len(mappings) == 0 {
		log.Error().Str("device_id", captureVerifyOpts.deviceID).Msg("No bounding box found for device")
		log.Info().Msg("Use 'espbrew mapping set --device-id <id> --camera <cam> --bounds <x,y,w,h>' to create one")
		return &exitCodeError{code: 1, err: fmt.Errorf("no bounding box found")}
	}

	// Find mapping for specified camera or use first available
	var mapping *persistence.DeviceBoundingBoxMapping
	if captureVerifyOpts.cameraID != "" {
		for _, m := range mappings {
			if m.CameraID == captureVerifyOpts.cameraID {
				mapping = m
				break
			}
		}
		if mapping == nil {
			log.Error().
				Str("device_id", captureVerifyOpts.deviceID).
				Str("camera_id", captureVerifyOpts.cameraID).
				Msg("No bounding box found for device on specified camera")
			return &exitCodeError{code: 1, err: fmt.Errorf("no bounding box found")}
		}
	} else {
		mapping = mappings[0]
		captureVerifyOpts.cameraID = mapping.CameraID
	}

	log.Info().
		Str("bbox_id", mapping.ID).
		Str("camera_id", mapping.CameraID).
		Msgf("Using bounding box: x=%.3f, y=%.3f, w=%.3f, h=%.3f",
			mapping.Bounds.X, mapping.Bounds.Y, mapping.Bounds.Width, mapping.Bounds.Height)

	// Step 2: Capture from camera
	log.Info().Str("camera", captureVerifyOpts.cameraID).Msg("Capturing image")

	capturer, err := camera.NewCapturerWithStore()
	if err != nil {
		return fmt.Errorf("create capturer: %w", err)
	}

	req := &camera.CaptureRequest{
		CameraID: captureVerifyOpts.cameraID,
		Width:    captureVerifyOpts.width,
		Height:   captureVerifyOpts.height,
		Format:   "jpg",
		Quality:  85,
		Timeout:  10 * time.Second,
	}

	result, err := capturer.Capture(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("Capture failed")
		return &exitCodeError{code: 2, err: fmt.Errorf("capture failed")}
	}

	log.Info().
		Str("path", result.Path).
		Int("width", result.Width).
		Int("height", result.Height).
		Msg("Image captured")

	// Step 3: Extract region based on bounds
	log.Info().Msg("Extracting device region")

	// Open captured image
	capturedFile, err := os.Open(result.Path)
	if err != nil {
		log.Error().Err(err).Msg("Failed to open captured image")
		return &exitCodeError{code: 3, err: fmt.Errorf("extraction failed")}
	}
	defer capturedFile.Close()

	img, err := jpeg.Decode(capturedFile)
	if err != nil {
		log.Error().Err(err).Msg("Failed to decode captured image")
		return &exitCodeError{code: 3, err: fmt.Errorf("extraction failed")}
	}

	// Convert bounds to pixel coordinates
	x, y, width, height := mapping.Bounds.ToPixels(result.Width, result.Height)

	// Validate bounds are within image
	bounds := img.Bounds()
	if x < 0 || y < 0 || x+width > bounds.Dx() || y+height > bounds.Dy() {
		log.Error().
			Int("img_width", bounds.Dx()).
			Int("img_height", bounds.Dy()).
			Int("box_x", x).
			Int("box_y", y).
			Int("box_width", width).
			Int("box_height", height).
			Msg("Bounding box extends beyond image bounds")
		return &exitCodeError{code: 3, err: fmt.Errorf("extraction failed")}
	}

	// Crop the image
	cropped := image.NewRGBA(image.Rect(0, 0, width, height))
	for py := y; py < y+height; py++ {
		for px := x; px < x+width; px++ {
			cropped.Set(px-x, py-y, img.At(px, py))
		}
	}

	// Step 4: Save to output or default location
	outputPath := captureVerifyOpts.output
	if outputPath == "" {
		// Auto-generate output path
		outputPath, err = generateVerifyPath(captureVerifyOpts.deviceID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to generate output path")
			return &exitCodeError{code: 3, err: fmt.Errorf("extraction failed")}
		}
	} else {
		// Ensure output directory exists
		outputDir := filepath.Dir(outputPath)
		if outputDir != "" && outputDir != "." {
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				log.Error().Err(err).Str("dir", outputDir).Msg("Failed to create output directory")
				return &exitCodeError{code: 3, err: fmt.Errorf("extraction failed")}
			}
		}
	}

	// Save cropped image
	outFile, err := os.Create(outputPath)
	if err != nil {
		log.Error().Err(err).Str("path", outputPath).Msg("Failed to create output file")
		return &exitCodeError{code: 3, err: fmt.Errorf("extraction failed")}
	}
	defer outFile.Close()

	if err := jpeg.Encode(outFile, cropped, &jpeg.Options{Quality: 90}); err != nil {
		log.Error().Err(err).Msg("Failed to encode cropped image")
		return &exitCodeError{code: 3, err: fmt.Errorf("extraction failed")}
	}

	// Step 5: Print result path
	log.Info().
		Str("path", outputPath).
		Int("width", width).
		Int("height", height).
		Msg("Device region extracted")
	fmt.Println(outputPath)

	return nil
}

// generateVerifyPath creates an auto-generated output path for verification images
func generateVerifyPath(deviceID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	verifyDir := filepath.Join(homeDir, ".espbrew", "verify")
	if err := os.MkdirAll(verifyDir, 0755); err != nil {
		return "", fmt.Errorf("create verify directory: %w", err)
	}

	// Sanitize device ID for filename
	safeDeviceID := deviceID
	if len(safeDeviceID) > 20 {
		safeDeviceID = safeDeviceID[:20]
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.jpg", safeDeviceID, timestamp)
	return filepath.Join(verifyDir, filename), nil
}
