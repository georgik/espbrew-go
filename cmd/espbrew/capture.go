package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/cluster"
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
	clusterURL string
	cameraID   string
	width      uint32
	height     uint32
	format     string
	quality    int
	timeout    time.Duration
	list       bool
}

func init() {
	captureCmd.Flags().StringVar(&captureOpts.clusterURL, "cluster", os.Getenv("ESPBREW_CLUSTER"), "Cluster URL for remote capture")
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
	if captureOpts.clusterURL != "" {
		return runCaptureRemote(args)
	}
	return runCaptureLocal(args)
}

func runCaptureLocal(args []string) error {
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

func runCaptureRemote(args []string) error {
	client := cluster.NewClient(captureOpts.clusterURL)

	// List cameras if requested
	if captureOpts.list {
		if err := listRemoteCameras(client); err != nil {
			return err
		}
		return nil
	}

	// Build capture request
	req := &cluster.CameraCaptureRequest{
		CameraID: captureOpts.cameraID,
		Width:    captureOpts.width,
		Height:   captureOpts.height,
		Format:   captureOpts.format,
		Quality:  captureOpts.quality,
	}

	log.Info().
		Str("camera", req.CameraID).
		Uint32("width", req.Width).
		Uint32("height", req.Height).
		Str("format", req.Format).
		Str("cluster", captureOpts.clusterURL).
		Msg("Capturing image via cluster")

	// Capture via cluster
	result, err := client.CaptureImage(*req)
	if err != nil {
		return fmt.Errorf("cluster capture failed: %w", err)
	}

	// Save image data if available
	if len(result.Data) > 0 {
		var outputPath string
		if len(args) > 0 {
			outputPath = args[0]
		} else {
			// Generate default filename
			timestamp := result.Timestamp.Format("20060102-150405")
			outputPath = fmt.Sprintf("capture-%s.%s", timestamp, result.Format)
		}

		if err := os.WriteFile(outputPath, result.Data, 0644); err != nil {
			return fmt.Errorf("save image: %w", err)
		}
		log.Info().
			Str("path", outputPath).
			Int("width", result.Width).
			Int("height", result.Height).
			Int("size", result.Size).
			Msg("Image saved")
	} else if result.Path != "" {
		// Download file from cluster
		log.Info().
			Str("path", result.Path).
			Msg("Downloading image from cluster")

		data, err := client.DownloadCapture(result.Path)
		if err != nil {
			return fmt.Errorf("download capture: %w", err)
		}

		// Save downloaded data
		var outputPath string
		if len(args) > 0 {
			outputPath = args[0]
		} else {
			// Extract filename from path or generate default
			timestamp := result.Timestamp.Format("20060102-150405")
			outputPath = fmt.Sprintf("capture-%s.%s", timestamp, result.Format)
		}

		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			return fmt.Errorf("save downloaded image: %w", err)
		}
		log.Info().
			Str("path", outputPath).
			Int("size", len(data)).
			Msg("Image downloaded and saved")
	} else {
		return fmt.Errorf("no image data returned from cluster")
	}

	return nil
}

func listRemoteCameras(client *cluster.Client) error {
	cameras, err := client.ListCameras()
	if err != nil {
		return fmt.Errorf("list cameras: %w", err)
	}

	if len(cameras) == 0 {
		log.Info().Msg("No cameras found on cluster")
		return nil
	}

	log.Info().Msg("Available cameras on cluster:")
	for i, cam := range cameras {
		log.Info().Msgf("  %d. %s", i+1, cam.Name)
		log.Info().Msgf("     ID:     %s", cam.ID)
		log.Info().Msgf("     Backend: %s", cam.Backend)
		log.Info().Msgf("     Status: %s", cam.Status)
		if cam.DevicePath != "" {
			log.Info().Msgf("     Path:   %s", cam.DevicePath)
		}
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
