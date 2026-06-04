package camera

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/pion/mediadevices"
	// Platform-specific camera drivers - must be imported for side effects
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	"github.com/rs/zerolog/log"
)

// Discoverer handles camera discovery
type Discoverer struct {
	mu      sync.RWMutex
	cameras map[string]*CameraInfo
}

// NewDiscoverer creates a new camera discoverer
func NewDiscoverer() *Discoverer {
	return &Discoverer{
		cameras: make(map[string]*CameraInfo),
	}
}

// Discover scans for available cameras
func (d *Discoverer) Discover() ([]*CameraInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	log.Debug().Msg("Scanning for cameras...")

	// Enumerate all media devices
	dms := mediadevices.EnumerateDevices()

	cameras := make([]*CameraInfo, 0, len(dms))

	for _, dm := range dms {
		// Only include video input devices
		if dm.Kind != mediadevices.VideoInput {
			continue
		}

		info, err := deviceToCameraInfo(dm)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get camera info")
			continue
		}
		if info != nil {
			// Skip non-primary video devices (video1, video3, etc.)
			// V4L2 creates multiple nodes per camera; we want the primary one
			if isPrimaryVideoDevice(info.ID) {
				d.cameras[info.ID] = info
				cameras = append(cameras, info)
			} else {
				log.Debug().Str("device_id", info.ID).Msg("Skipping non-primary video device")
			}
		}
	}

	log.Info().
		Int("count", len(cameras)).
		Str("platform", runtime.GOOS).
		Msg("Camera discovery completed")

	return cameras, nil
}

// GetByID retrieves a camera by ID
func (d *Discoverer) GetByID(id string) (*CameraInfo, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cam, ok := d.cameras[id]
	return cam, ok
}

// List returns all discovered cameras
func (d *Discoverer) List() []*CameraInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cameras := make([]*CameraInfo, 0, len(d.cameras))
	for _, cam := range d.cameras {
		cameras = append(cameras, cam)
	}
	return cameras
}

// Refresh re-scans for cameras
func (d *Discoverer) Refresh() ([]*CameraInfo, error) {
	return d.Discover()
}

// deviceToCameraInfo converts a pion media device to CameraInfo
func deviceToCameraInfo(dm mediadevices.MediaDeviceInfo) (*CameraInfo, error) {
	if dm.Label == "" {
		return nil, fmt.Errorf("device has no label")
	}

	// For V4L2, extract actual device path from pion's device ID
	// pion returns: "usb-046d_HD_Webcam_C615_C574F460-video-index0"
	// We need: "/dev/video0" as the ID for API calls
	cameraID := dm.DeviceID
	devicePath := dm.DeviceID
	if runtime.GOOS == "linux" {
		devicePath = extractV4L2Path(dm.DeviceID)
		cameraID = devicePath // Use /dev/video0 as the primary ID
	}

	info := &CameraInfo{
		ID:      cameraID,
		Name:    dm.Label,
		Path:    devicePath,
		Backend: DetectBackend(dm.DeviceID),
	}

	// Try to get supported formats
	// Note: pion/mediadevices doesn't expose format enumeration directly
	// We'll populate common formats that most cameras support
	info.Formats = []VideoFormat{
		{Width: 640, Height: 480, PixelFormat: "YUYV"},
		{Width: 1280, Height: 720, PixelFormat: "YUYV"},
		{Width: 1920, Height: 1080, PixelFormat: "YUYV"},
		{Width: 640, Height: 480, PixelFormat: "MJPG"},
		{Width: 1280, Height: 720, PixelFormat: "MJPG"},
		{Width: 1920, Height: 1080, PixelFormat: "MJPG"},
	}

	return info, nil
}

// isPrimaryVideoDevice checks if the device ID represents the primary video device
// V4L2 creates multiple nodes per camera (video0=primary, video1=metadata, etc.)
// We want only the primary device (even-indexed video devices)
func isPrimaryVideoDevice(deviceID string) bool {
	// Check for pion's video-index pattern (e.g., "video-index0", "video-index1")
	idx := strings.LastIndex(deviceID, "-video-index")
	if idx != -1 && idx+12 < len(deviceID) {
		numStr := deviceID[idx+12:]
		var videoNum int
		if _, err := fmt.Sscanf(numStr, "%d", &videoNum); err == nil {
			// Primary devices have even numbers (0, 2, 4, ...)
			return videoNum%2 == 0
		}
	}
	// For non-V4L2 devices (macOS, Windows), include all
	return true
}

// extractV4L2Path converts a pion device ID to a V4L2 device path
// Input: "usb-046d_HD_Webcam_C615_C574F460-video-index0"
// Output: "/dev/video0"
func extractV4L2Path(deviceID string) string {
	// Extract the video index from the device ID
	// Format: ...-video-indexN
	idx := strings.LastIndex(deviceID, "-video-index")
	if idx != -1 && idx+12 < len(deviceID) {
		numStr := deviceID[idx+12:]
		var videoNum int
		if _, err := fmt.Sscanf(numStr, "%d", &videoNum); err == nil {
			return fmt.Sprintf("/dev/video%d", videoNum)
		}
	}
	// Fallback: try to find any number in the string
	return deviceID
}

// Discover is a convenience function that discovers cameras without a discoverer instance
func Discover() ([]*CameraInfo, error) {
	d := NewDiscoverer()
	return d.Discover()
}
