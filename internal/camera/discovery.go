package camera

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/pion/mediadevices"
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
			d.cameras[info.ID] = info
			cameras = append(cameras, info)
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

	info := &CameraInfo{
		ID:      dm.DeviceID,
		Name:    dm.Label,
		Path:    dm.DeviceID,
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

// Discover is a convenience function that discovers cameras without a discoverer instance
func Discover() ([]*CameraInfo, error) {
	d := NewDiscoverer()
	return d.Discover()
}
