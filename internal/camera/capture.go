package camera

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/rs/zerolog/log"
)

// CaptureRequest specifies capture parameters
type CaptureRequest struct {
	CameraID string        // Camera ID (empty for first available)
	Width    uint32        // Desired width (0 for camera default)
	Height   uint32        // Desired height (0 for camera default)
	Format   string        // Output format: "jpg" (default)
	Quality  int           // JPEG quality 1-100 (default: 85)
	Timeout  time.Duration // Capture timeout (default: 5s)
}

// CaptureResult contains the captured image data
type CaptureResult struct {
	Path      string    // Path to saved file
	Data      []byte    // Image data
	Format    string    // Image format
	Width     int       // Actual width
	Height    int       // Actual height
	Size      int       // Size in bytes
	Timestamp time.Time // Capture timestamp
}

// Capturer handles image capture
type Capturer struct {
	store      *Store
	discoverer *Discoverer
}

// NewCapturer creates a new image capturer
func NewCapturer(store *Store) *Capturer {
	return &Capturer{
		store:      store,
		discoverer: NewDiscoverer(),
	}
}

// NewCapturerWithStore creates a capturer with the default store
func NewCapturerWithStore() (*Capturer, error) {
	store, err := DefaultStore()
	if err != nil {
		return nil, err
	}
	return NewCapturer(store), nil
}

// Capture captures an image from the specified camera
func (c *Capturer) Capture(ctx context.Context, req *CaptureRequest) (*CaptureResult, error) {
	if req.Timeout == 0 {
		req.Timeout = 5 * time.Second
	}
	if req.Quality == 0 {
		req.Quality = 85
	}
	if req.Format == "" {
		req.Format = "jpg"
	}

	// Set deadline
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Find camera ID
	var cameraID string
	if req.CameraID != "" {
		cameraID = req.CameraID
	} else {
		// Try to discover cameras first
		cameras, err := c.discoverer.Discover()
		if err == nil && len(cameras) > 0 {
			cameraID = cameras[0].ID
			log.Info().Str("camera", cameras[0].Name).Msg("Using discovered camera")
		} else {
			// Fallback: use default camera ID for platform tool
			cameraID = "default"
			log.Debug().Msg("No cameras discovered, using platform default")
		}
	}

	log.Info().
		Str("camera", cameraID).
		Uint32("width", req.Width).
		Uint32("height", req.Height).
		Msg("Capturing image")

	// Capture using platform-specific tool
	data, width, height, err := c.capturePlatformSpecific(ctx, cameraID, req.Width, req.Height, req.Quality)
	if err != nil {
		return nil, fmt.Errorf("capture: %w", err)
	}

	// Save to storage
	path, err := c.store.Save(cameraID, req.Format, data)
	if err != nil {
		return nil, fmt.Errorf("save image: %w", err)
	}

	result := &CaptureResult{
		Path:      path,
		Data:      data,
		Format:    req.Format,
		Width:     width,
		Height:    height,
		Size:      len(data),
		Timestamp: time.Now(),
	}

	log.Info().
		Str("path", path).
		Int("width", result.Width).
		Int("height", result.Height).
		Int("size", result.Size).
		Msg("Capture completed")

	return result, nil
}

// capturePlatformSpecific captures using platform-specific tools
func (c *Capturer) capturePlatformSpecific(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	switch runtime.GOOS {
	case "darwin":
		return c.captureMacOS(ctx, cameraID, width, height, quality)
	case "linux":
		return c.captureLinux(ctx, cameraID, width, height, quality)
	case "windows":
		return c.captureWindows(ctx, cameraID, width, height, quality)
	default:
		return nil, 0, 0, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// captureMacOS captures using imagesnap
func (c *Capturer) captureMacOS(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	// Check if imagesnap is available
	if _, err := exec.LookPath("imagesnap"); err != nil {
		return nil, 0, 0, fmt.Errorf("imagesnap not found: install with 'brew install imagesnap'")
	}

	// Create temp file for capture
	tmpFile := "/tmp/espbrew-capture.jpg"

	// Build command - imagesnap uses default camera when no device specified
	cmd := exec.CommandContext(ctx, "imagesnap", tmpFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, 0, 0, fmt.Errorf("imagesnap failed: %w, output: %s", err, output)
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
		return data, 0, 0, nil // Return data even if decode fails
	}
	bounds := img.Bounds()
	return data, bounds.Dx(), bounds.Dy(), nil
}

// captureLinux captures using fswebcam
func (c *Capturer) captureLinux(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	// Check if fswebcam is available
	if _, err := exec.LookPath("fswebcam"); err != nil {
		return nil, 0, 0, fmt.Errorf("fswebcam not found: install with 'sudo apt install fswebcam'")
	}

	// Create temp file for capture
	tmpFile := "/tmp/espbrew-capture.jpg"

	// Build command - specify device if provided
	args := []string{
		"-r", fmt.Sprintf("%dx%d", width, height),
		"--jpeg", fmt.Sprintf("%d", quality),
		"-q",       // Skip banner
		"-S", "10", // Skip frames for stability
	}

	// Add device argument if cameraID is specified and not "default"
	if cameraID != "" && cameraID != "default" {
		args = append([]string{"-d", cameraID}, args...)
	}
	args = append(args, tmpFile)

	cmd := exec.CommandContext(ctx, "fswebcam", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, 0, 0, fmt.Errorf("fswebcam failed: %w, output: %s", err, output)
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
		return data, 0, 0, nil // Return data even if decode fails
	}
	bounds := img.Bounds()
	return data, bounds.Dx(), bounds.Dy(), nil
}

// captureWindows captures using PowerShell
func (c *Capturer) captureWindows(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	// Windows capture using PowerShell and Windows.Media.Capture
	// This is a placeholder - actual implementation would use PowerShell script
	return nil, 0, 0, fmt.Errorf("Windows capture not yet implemented - consider using ffmpeg")
}

// frameToJPEG converts an image to JPEG bytes
func frameToJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode JPEG: %w", err)
	}
	return buf.Bytes(), nil
}

// Capture is a convenience function that captures with default settings
func Capture(ctx context.Context, cameraID string, width, height uint32) (*CaptureResult, error) {
	capturer, err := NewCapturerWithStore()
	if err != nil {
		return nil, err
	}

	req := &CaptureRequest{
		CameraID: cameraID,
		Width:    width,
		Height:   height,
		Format:   "jpg",
		Quality:  85,
		Timeout:  5 * time.Second,
	}

	return capturer.Capture(ctx, req)
}
