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
			// Check using device label which contains the video-index pattern
			if isPrimaryVideoDevice(dm.Label) {
				d.cameras[info.ID] = info
				cameras = append(cameras, info)
			} else {
				log.Debug().Str("device_id", info.ID).Str("label", dm.Label).Msg("Skipping non-primary video device")
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

	// For V4L2, extract actual device path from pion's device ID or label
	// pion DeviceID might be UUID, Label contains actual path
	// Label format: "usb-046d_HD_Webcam_C615_C574F460-video-index0" or "...;video4"
	cameraID := dm.DeviceID // Use UUID/DeviceID as unique identifier
	devicePath := dm.DeviceID
	if runtime.GOOS == "linux" {
		// Try DeviceID first (some pion versions return device path in ID)
		devicePath = extractV4L2Path(dm.DeviceID)
		log.Debug().
			Str("device_id", dm.DeviceID).
			Str("label", dm.Label).
			Bool("contains_video_keyword", containsVideoKeyword(dm.Label)).
			Str("path_after_device_id_extract", devicePath).
			Msg("Camera path extraction step 1")

		// If DeviceID didn't contain video pattern, try Label
		// But only if Label looks like it contains device info (has "video" keyword)
		if devicePath == dm.DeviceID && containsVideoKeyword(dm.Label) {
			devicePath = extractV4L2Path(dm.Label)
			log.Debug().
				Str("device_id", dm.DeviceID).
				Str("label", dm.Label).
				Str("path_after_label_extract", devicePath).
				Msg("Camera path extraction step 2")
		}
		// Keep cameraID as DeviceID (UUID), store device path in Path field
		log.Info().
			Str("camera_id", cameraID).
			Str("label", dm.Label).
			Str("final_path", devicePath).
			Msg("Camera registered with path")
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

// containsVideoKeyword checks if a string looks like it contains V4L2 device info
func containsVideoKeyword(s string) bool {
	return strings.Contains(s, "video") || strings.Contains(s, "-video-index")
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
// Input: "usb-046d_HD_Webcam_C615_C574F460-video-index0" or "...;video4"
// Output: "/dev/video0" or "/dev/video4"
// Priority: ";videoN" suffix (actual device) > "-video-indexN" (pion internal index)
func extractV4L2Path(deviceID string) string {
	// First priority: extract from ";videoN" suffix (e.g. "usb-...-video-index0;video4")
	// This is the actual V4L2 device number
	// ";video" is 6 characters, so we start slicing from position+6 to get the number
	if semicolonIdx := strings.LastIndex(deviceID, ";video"); semicolonIdx != -1 && semicolonIdx+6 < len(deviceID) {
		numStr := deviceID[semicolonIdx+6:] // Everything after ";video"
		var videoNum int
		if _, err := fmt.Sscanf(numStr, "%d", &videoNum); err == nil {
			return fmt.Sprintf("/dev/video%d", videoNum)
		}
	}

	// Second try: extract from "-video-indexN" pattern
	idx := strings.LastIndex(deviceID, "-video-index")
	if idx != -1 && idx+12 < len(deviceID) {
		numStr := deviceID[idx+12:]
		var videoNum int
		if _, err := fmt.Sscanf(numStr, "%d", &videoNum); err == nil {
			return fmt.Sprintf("/dev/video%d", videoNum)
		}
	}

	// Fallback: return original deviceID
	return deviceID
}

// Discover is a convenience function that discovers cameras without a discoverer instance
func Discover() ([]*CameraInfo, error) {
	d := NewDiscoverer()
	return d.Discover()
}
