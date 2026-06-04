package snap

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/jpeg"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"github.com/rs/zerolog/log"
)

// Capturer handles camera capture for snapshots
type Capturer struct {
	cameraID   string
	timeout    time.Duration
	discoverer *camera.Discoverer
}

// NewCapturer creates a new camera capturer with the specified parameters
func NewCapturer(cameraID string, timeout time.Duration) *Capturer {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &Capturer{
		cameraID:   cameraID,
		timeout:    timeout,
		discoverer: camera.NewDiscoverer(),
	}
}

// Capture captures a JPEG image from the configured camera
func (c *Capturer) Capture(ctx context.Context) ([]byte, error) {
	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Resolve camera ID
	cameraID, err := c.resolveCameraID(ctx)
	if err != nil {
		return nil, fmt.Errorf("camera resolution failed: %w", err)
	}

	log.Info().
		Str("camera_id", cameraID).
		Str("original_request", c.cameraID).
		Msg("Capturing image")

	// Create capture request
	req := &camera.CaptureRequest{
		CameraID: cameraID,
		Format:   "jpg",
		Quality:  85,
		Timeout:  c.timeout,
	}

	// Create capturer and capture
	camCapturer := camera.NewCapturer(nil)
	result, err := camCapturer.Capture(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	// Validate JPEG format
	if err := c.validateJPEG(result.Data); err != nil {
		return nil, fmt.Errorf("image validation failed: %w", err)
	}

	log.Info().
		Int("size", len(result.Data)).
		Int("width", result.Width).
		Int("height", result.Height).
		Msg("Image captured successfully")

	return result.Data, nil
}

// CaptureBase64 captures a JPEG image and returns it as a base64-encoded string
func (c *Capturer) CaptureBase64(ctx context.Context) (string, error) {
	data, err := c.Capture(ctx)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// resolveCameraID finds the appropriate camera ID to use
func (c *Capturer) resolveCameraID(ctx context.Context) (string, error) {
	// If camera ID is explicitly set, validate it exists
	if c.cameraID != "" {
		if err := c.validateCameraID(ctx, c.cameraID); err != nil {
			// Log warning but continue - camera might be accessible
			log.Warn().Err(err).Str("camera_id", c.cameraID).Msg("Camera validation failed, attempting capture anyway")
		}
		return c.cameraID, nil
	}

	// No camera ID specified - use discovery to find first available
	cameras, err := c.discoverer.Discover()
	if err != nil {
		log.Warn().Err(err).Msg("Camera discovery failed, falling back to platform default")
		return "default", nil
	}

	if len(cameras) == 0 {
		return "", fmt.Errorf("no cameras available")
	}

	selected := cameras[0]
	log.Info().
		Str("camera_id", selected.ID).
		Str("camera_name", selected.Name).
		Msg("Auto-selected first available camera")

	return selected.ID, nil
}

// validateCameraID checks if a camera ID is valid and accessible
func (c *Capturer) validateCameraID(ctx context.Context, cameraID string) error {
	cameras, err := c.discoverer.Discover()
	if err != nil {
		// Discovery failed - can't validate, proceed optimistically
		return nil
	}

	for _, cam := range cameras {
		if cam.ID == cameraID {
			if !cam.IsAvailable() {
				return fmt.Errorf("camera '%s' (%s) is not available", cameraID, cam.Name)
			}
			log.Debug().Str("camera_id", cameraID).Msg("Camera validated")
			return nil
		}
	}

	// Camera not found in discovery - may still be accessible via platform tool
	log.Debug().Str("camera_id", cameraID).Msg("Camera not found in discovery, may still be accessible")
	return nil
}

// validateJPEG verifies that the data is a valid JPEG image
func (c *Capturer) validateJPEG(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("image data too small (%d bytes)", len(data))
	}

	// Check JPEG magic bytes (FF D8)
	if data[0] != 0xFF || data[1] != 0xD8 {
		return fmt.Errorf("invalid JPEG format (missing magic bytes)")
	}

	// Try to decode to verify it's a valid JPEG
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("JPEG decode failed: %w", err)
	}

	// Verify we got a valid image
	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		return fmt.Errorf("decoded image has invalid dimensions")
	}

	return nil
}

// encodeJPEG converts image data to JPEG format
func (c *Capturer) encodeJPEG(img []byte, quality int) ([]byte, error) {
	// If already JPEG, just validate and return
	if c.isJPEG(img) {
		if err := c.validateJPEG(img); err != nil {
			return nil, err
		}
		return img, nil
	}

	// For non-JPEG input, we'd need image decoding - return error
	return nil, fmt.Errorf("non-JPEG input not supported")
}

// isJPEG checks if data appears to be JPEG format
func (c *Capturer) isJPEG(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	return data[0] == 0xFF && data[1] == 0xD8
}

// findCameraByID searches for a camera by its ID in the discovered list
func (c *Capturer) findCameraByID(cameraID string) (*camera.CameraInfo, error) {
	cameras, err := c.discoverer.Discover()
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	for _, cam := range cameras {
		if cam.ID == cameraID {
			return cam, nil
		}
	}

	return nil, fmt.Errorf("camera '%s' not found", cameraID)
}

// GetAvailableCameras returns a list of available cameras
func (c *Capturer) GetAvailableCameras(ctx context.Context) ([]*camera.CameraInfo, error) {
	cameras, err := c.discoverer.Discover()
	if err != nil {
		return nil, fmt.Errorf("camera discovery failed: %w", err)
	}
	return cameras, nil
}
