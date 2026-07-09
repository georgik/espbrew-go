//go:build darwin
// +build darwin

package camera

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"time"

	"github.com/pion/mediadevices/pkg/avfoundation"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/rs/zerolog/log"
)

// captureMacOS captures using AVFoundation natively
func (c *Capturer) captureMacOS(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	log.Info().Str("camera_id", cameraID).Msg("captureMacOS: Starting macOS capture")

	// Find the device by UID
	devices, err := avfoundation.Devices(avfoundation.Video)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list devices: %w", err)
	}

	var selectedDevice *avfoundation.Device
	for i := range devices {
		if devices[i].UID == cameraID {
			selectedDevice = &devices[i]
			break
		}
	}

	// If not found by exact ID, try first device (for default/empty cameraID)
	if selectedDevice == nil && (cameraID == "" || cameraID == "default") {
		if len(devices) > 0 {
			selectedDevice = &devices[0]
			log.Info().Str("device", selectedDevice.Name).Msg("Using default camera")
		}
	}

	if selectedDevice == nil {
		return nil, 0, 0, fmt.Errorf("camera not found: %s", cameraID)
	}

	log.Info().
		Str("device_name", selectedDevice.Name).
		Str("device_uid", selectedDevice.UID).
		Msg("Found device")

	// Create capture session
	session, err := avfoundation.NewSession(*selectedDevice)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	// Build media properties
	media := prop.Media{
		Video: prop.Video{
			Width:       int(width),
			Height:      int(height),
			FrameRate:   30,
			FrameFormat: frame.FormatYUYV,
		},
	}

	// Try MJPEG if YUYV not available (some cameras prefer MJPEG)
	properties := session.Properties()
	for _, p := range properties {
		if p.FrameFormat == frame.FormatMJPEG {
			media.Video.FrameFormat = frame.FormatMJPEG
			break
		}
	}

	log.Info().
		Int("width", media.Video.Width).
		Int("height", media.Video.Height).
		Str("format", string(media.Video.FrameFormat)).
		Msg("Opening stream")

	// Open stream with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rc, err := session.Open(media)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("open stream: %w", err)
	}
	defer rc.Close()

	// Read first frame
	data, closeFn, err := rc.ReadContext(ctxTimeout)
	closeFn()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read frame: %w", err)
	}

	// Decode frame to image
	decoder, err := frame.NewDecoder(media.Video.FrameFormat)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("create decoder: %w", err)
	}

	img, cleanup, err := decoder.Decode(data, int(width), int(height))
	if cleanup != nil {
		cleanup()
	}
	if err != nil {
		// Return raw data if decode fails
		log.Warn().Err(err).Msg("Failed to decode frame, returning raw data")
		return data, int(width), int(height), nil
	}

	// Get actual image dimensions
	bounds := img.Bounds()
	actualWidth := bounds.Dx()
	actualHeight := bounds.Dy()

	// Encode to JPEG
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("encode jpeg: %w", err)
	}

	log.Info().
		Int("width", actualWidth).
		Int("height", actualHeight).
		Int("size", buf.Len()).
		Msg("Capture complete")

	return buf.Bytes(), actualWidth, actualHeight, nil
}

// captureLinux is a stub for macOS builds
func (c *Capturer) captureLinux(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	return nil, 0, 0, fmt.Errorf("linux capture called on darwin platform")
}

// captureWindows is a stub for macOS builds
func (c *Capturer) captureWindows(ctx context.Context, cameraID string, width, height uint32, quality int) ([]byte, int, int, error) {
	return nil, 0, 0, fmt.Errorf("windows capture called on darwin platform")
}
