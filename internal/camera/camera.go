package camera

import (
	"fmt"
	"runtime"
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
	Width       uint32 `json:"width"`
	Height      uint32 `json:"height"`
	PixelFormat string `json:"pixel_format"` // "MJPG", "YUYV", "RGB24", etc
}

// String returns a string representation of the format
func (v VideoFormat) String() string {
	return fmt.Sprintf("%dx%d/%s", v.Width, v.Height, v.PixelFormat)
}

// CameraInfo represents a discovered camera
type CameraInfo struct {
	ID      string        `json:"id"`      // Unique identifier
	Name    string        `json:"name"`    // Human-readable name
	Path    string        `json:"path"`    // Platform-specific device path
	Backend Backend       `json:"backend"` // Platform backend
	Formats []VideoFormat `json:"formats"` // Supported formats
	NodeID  string        `json:"node_id"` // Cluster node owning this camera (empty for local)
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

// Platform returns the camera backend/platform name
func Platform() string {
	switch runtime.GOOS {
	case "linux":
		return "v4l2"
	case "darwin":
		return "avfoundation"
	case "windows":
		return "directshow"
	default:
		return "unknown"
	}
}

// Controller interface provides cross-platform camera control methods
type Controller interface {
	// Close releases the camera device
	Close() error

	// Path returns the device path/identifier
	Path() string

	// SetDisplayPreset configures camera for display photography
	// On Linux: Applies optimized settings for glowing/backlit displays
	// On other platforms: No-op (uses camera defaults)
	SetDisplayPreset() error

	// SetFocus configures focus distance (0-255)
	SetFocus(distance int32) error

	// SetBrightness adjusts brightness (0-255)
	SetBrightness(value int32) error

	// SetContrast adjusts contrast (0-255)
	SetContrast(value int32) error

	// SetSharpness adjusts sharpness (0-255)
	SetSharpness(value int32) error

	// SetSaturation adjusts saturation (0-255)
	SetSaturation(value int32) error

	// GetBrightness retrieves current brightness
	GetBrightness() (int32, error)

	// GetContrast retrieves current contrast
	GetContrast() (int32, error)

	// GetSettings retrieves all current settings
	GetSettings() (map[string]int32, error)
}

// NewController creates a platform-appropriate camera controller
func NewController(devicePath string) (Controller, error) {
	if runtime.GOOS != "linux" {
		return newStubController(devicePath)
	}
	return newLinuxController(devicePath)
}

// ControllerAvailable returns true if camera controls are available on this platform
func ControllerAvailable() bool {
	return runtime.GOOS == "linux"
}

// DisplayPresetSettings defines optimal values for display photography
var DisplayPresetSettings = struct {
	Brightness int32
	Contrast   int32
	Sharpness  int32
	Saturation int32
	Exposure   int32
}{
	Brightness: 80,  // Lowered from 128 to avoid overexposure
	Contrast:   140, // Increased from 32 for text readability
	Sharpness:  150, // Increased from 22 for clear text
	Saturation: 90,  // Slightly reduced from 32
	Exposure:   300, // Manual exposure value
}

// FocusPresets defines focus values for common distances
var FocusPresets = struct {
	Close   int32 // ~0.3m (macro)
	Display int32 // ~1m (typical display distance)
	Far     int32 // ~3m+ (distant objects)
}{
	Close:   200,
	Display: 85,
	Far:     30,
}
