package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var captureCmd = &cobra.Command{
	Use:   "capture [output-file]",
	Short: "Capture image from camera",
	Long: `Capture an image from a camera and save it to a file.

If no output file is specified, the image is saved to ~/.espbrew/captures/
with a timestamped filename.

If no camera ID is specified, the first available camera is used.

Examples:
  espbrew capture                    # Capture with defaults
  espbrew capture --list             # List cameras first
  espbrew capture --camera-id cam-001 --width 1920 --height 1080
  espbrew capture --format jpg --quality 90
  espbrew capture my-photo.jpg       # Save to specific file`,
	RunE: runCapture,
}

var captureOpts struct {
	cameraID string
	width    uint32
	height   uint32
	format   string
	quality  int
	timeout  time.Duration
	list     bool
}

func init() {
	rootCmd.AddCommand(captureCmd)

	captureCmd.Flags().StringVar(&captureOpts.cameraID, "camera-id", "", "Camera ID (default: first available)")
	captureCmd.Flags().Uint32Var(&captureOpts.width, "width", 1280, "Image width")
	captureCmd.Flags().Uint32Var(&captureOpts.height, "height", 720, "Image height")
	captureCmd.Flags().StringVar(&captureOpts.format, "format", "jpg", "Output format (jpg, png)")
	captureCmd.Flags().IntVar(&captureOpts.quality, "quality", 85, "JPEG quality (1-100)")
	captureCmd.Flags().DurationVar(&captureOpts.timeout, "timeout", 5*time.Second, "Capture timeout")
	captureCmd.Flags().BoolVar(&captureOpts.list, "list", false, "List cameras before capturing")
}

func runCapture(cmd *cobra.Command, args []string) error {
	// List cameras if requested
	if captureOpts.list {
		if err := listCameras(); err != nil {
			return err
		}
	}

	ctx := context.Background()

	// Create capturer
	capturer, err := camera.NewCapturerWithStore()
	if err != nil {
		return fmt.Errorf("create capturer: %w", err)
	}

	// Build capture request
	req := &camera.CaptureRequest{
		CameraID: captureOpts.cameraID,
		Width:    captureOpts.width,
		Height:   captureOpts.height,
		Format:   captureOpts.format,
		Quality:  captureOpts.quality,
		Timeout:  captureOpts.timeout,
	}

	log.Info().
		Str("camera", req.CameraID).
		Uint32("width", req.Width).
		Uint32("height", req.Height).
		Str("format", req.Format).
		Msg("Capturing image")

	// Capture
	result, err := capturer.Capture(ctx, req)
	if err != nil {
		return fmt.Errorf("capture failed: %w", err)
	}

	// Output file if specified
	if len(args) > 0 {
		outputPath := args[0]
		if err := copyFile(result.Path, outputPath); err != nil {
			return fmt.Errorf("copy to output file: %w", err)
		}
		log.Info().
			Str("from", result.Path).
			Str("to", outputPath).
			Msg("Image copied")
	} else {
		log.Info().
			Str("path", result.Path).
			Int("width", result.Width).
			Int("height", result.Height).
			Int("size", result.Size).
			Msg("Image captured")
	}

	return nil
}

func listCameras() error {
	discoverer := camera.NewDiscoverer()
	cameras, err := discoverer.Discover()
	if err != nil {
		return fmt.Errorf("discover cameras: %w", err)
	}

	if len(cameras) == 0 {
		log.Info().Msg("No cameras found")
		return nil
	}

	log.Info().Msg("Available cameras:")
	for i, cam := range cameras {
		log.Info().Msgf("  %d. %s", i+1, cam.Name)
		log.Info().Msgf("     ID:     %s", cam.ID)
		log.Info().Msgf("     Backend: %s", cam.Backend)
	}

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
