//go:build !darwin
// +build !darwin

package camera

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"os"
	"os/exec"
	"time"

	"github.com/rs/zerolog/log"
)

// captureMacOS is a stub for non-darwin builds
func (c *Capturer) captureMacOS(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	return nil, 0, 0, fmt.Errorf("macOS capture called on non-darwin platform")
}

// captureLinux captures using fswebcam
func (c *Capturer) captureLinux(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	// Check if fswebcam is available
	if _, err := exec.LookPath("fswebcam"); err != nil {
		return nil, 0, 0, fmt.Errorf("fswebcam not found: install with 'sudo apt install fswebcam'")
	}

	// Create temp file for capture with unique name
	tmpFile := fmt.Sprintf("/tmp/espbrew-capture-%d.jpg", time.Now().UnixNano())

	// Build command - specify device if provided
	args := []string{
		"-r", fmt.Sprintf("%dx%d", width, height),
		"--jpeg", fmt.Sprintf("%d", quality),
		"-q",       // Skip banner
		"-S", "10", // Skip frames for stability
	}

	// Add device argument if cameraID is specified and not "default"
	if cameraID != "" && cameraID != "default" {
		// Convert pion camera ID to V4L2 path if needed
		v4l2Path := extractV4L2Path(cameraID)
		args = append([]string{"-d", v4l2Path}, args...)
	}
	args = append(args, tmpFile)

	log.Info().
		Str("camera_id", cameraID).
		Str("device", extractV4L2Path(cameraID)).
		Msg("captureLinux: Executing fswebcam")

	cmd := exec.CommandContext(ctx, "fswebcam", args...)
	if err := cmd.Run(); err != nil {
		return nil, 0, 0, fmt.Errorf("fswebcam failed: %w", err)
	}

	// Read captured file
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read capture file: %w", err)
	}

	// Clean up temp file
	_ = os.Remove(tmpFile)

	// Decode to get dimensions
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return data, int(width), int(height), nil
	}
	bounds := img.Bounds()
	return data, bounds.Dx(), bounds.Dy(), nil
}

// captureWindows captures using ffmpeg
func (c *Capturer) captureWindows(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, 0, 0, fmt.Errorf("ffmpeg not found: install ffmpeg")
	}

	// Create temp file for capture with unique name
	tmpFile := fmt.Sprintf("/tmp/espbrew-capture-%d.jpg", time.Now().UnixNano())

	// Build ffmpeg command
	args := []string{
		"-f", "dshow",
		"-i", cameraID, // DirectShow device name
		"-vframes", "1",
		"-q:v", fmt.Sprintf("%d", quality/10), // ffmpeg uses different scale
		"-y", // Overwrite output file
		tmpFile,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		return nil, 0, 0, fmt.Errorf("ffmpeg failed: %w", err)
	}

	// Read captured file
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read capture file: %w", err)
	}

	// Clean up temp file
	_ = os.Remove(tmpFile)

	// Decode to get dimensions
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return data, int(width), int(height), nil
	}
	bounds := img.Bounds()
	return data, bounds.Dx(), bounds.Dy(), nil
}
