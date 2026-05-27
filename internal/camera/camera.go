package camera

import (
	"fmt"
	"strings"
)

// Backend type for camera implementations
type Backend string

const (
	BackendV4L2         Backend = "v4l2"
	BackendAVFoundation Backend = "avfoundation"
	BackendDirectShow   Backend = "directshow"
	BackendUnknown      Backend = "unknown"
)

// VideoFormat represents a supported video format
type VideoFormat struct {
	Width       uint32
	Height      uint32
	PixelFormat string // "MJPG", "YUYV", "RGB24", etc
}

// String returns a string representation of the format
func (v VideoFormat) String() string {
	return fmt.Sprintf("%dx%d/%s", v.Width, v.Height, v.PixelFormat)
}

// CameraInfo represents a discovered camera
type CameraInfo struct {
	ID      string        // Unique identifier
	Name    string        // Human-readable name
	Path    string        // Platform-specific device path
	Backend Backend       // Platform backend
	Formats []VideoFormat // Supported formats
	NodeID  string        // Cluster node owning this camera (empty for local)
}

// IsAvailable checks if camera is available for capture
func (c *CameraInfo) IsAvailable() bool {
	return c.ID != "" && c.Name != ""
}

// GetBestFormat finds the closest matching format for requested dimensions
func (c *CameraInfo) GetBestFormat(width, height uint32) *VideoFormat {
	if len(c.Formats) == 0 {
		return nil
	}

	// Exact match first
	for _, f := range c.Formats {
		if f.Width == width && f.Height == height {
			return &f
		}
	}

	// Find closest match
	var best *VideoFormat
	bestScore := int64(-1)

	for i := range c.Formats {
		f := &c.Formats[i]
		score := int64(f.Width) * int64(f.Height)
		if score > bestScore {
			bestScore = score
			best = f
		}
	}

	return best
}

// DetectBackend determines the backend from a device path or ID
func DetectBackend(deviceID string) Backend {
	id := strings.ToLower(deviceID)

	// Windows DirectShow - check first (has specific prefixes)
	if strings.Contains(id, "\\?\\") || strings.Contains(id, "@device:") || strings.Contains(id, "usb#vid_") || strings.Contains(id, "usb\\") {
		return BackendDirectShow
	}

	// Linux V4L2 devices
	if strings.Contains(id, "/dev/video") {
		return BackendV4L2
	}

	// macOS AVFoundation - typically has UUID-like IDs or specific names
	if strings.HasPrefix(id, "0x") || strings.Contains(id, "facetime") || strings.Contains(id, "facetimehd") {
		return BackendAVFoundation
	}

	return BackendUnknown
}
