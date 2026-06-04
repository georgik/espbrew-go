// +build !linux

package stub

import (
	"fmt"
)

// Camera is a stub implementation for non-Linux platforms
// On macOS/Windows, camera controls are limited or unavailable
// The pion/mediadevices library handles basic capture
type Camera struct {
	path string
}

// NewCamera creates a stub camera for non-Linux platforms
func NewCamera(devicePath string) (*Camera, error) {
	return &Camera{
		path: devicePath,
	}, nil
}

// Close releases the camera device (no-op on stub)
func (c *Camera) Close() error {
	return nil
}

// Path returns the device path
func (c *Camera) Path() string {
	return c.path
}

// SetDisplayPreset is a no-op on non-Linux platforms
// V4L2 camera controls are only available on Linux
func (c *Camera) SetDisplayPreset() error {
	// Silently ignore - pion/mediadevices will use default camera settings
	return nil
}

// SetFocus is a no-op on non-Linux platforms
func (c *Camera) SetFocus(distance int32) error {
	return nil
}

// SetBrightness is a no-op on non-Linux platforms
func (c *Camera) SetBrightness(value int32) error {
	return nil
}

// SetContrast is a no-op on non-Linux platforms
func (c *Camera) SetContrast(value int32) error {
	return nil
}

// SetSharpness is a no-op on non-Linux platforms
func (c *Camera) SetSharpness(value int32) error {
	return nil
}

// SetSaturation is a no-op on non-Linux platforms
func (c *Camera) SetSaturation(value int32) error {
	return nil
}

// GetBrightness returns default value on non-Linux platforms
func (c *Camera) GetBrightness() (int32, error) {
	return 128, nil // Default brightness
}

// GetContrast returns default value on non-Linux platforms
func (c *Camera) GetContrast() (int32, error) {
	return 32, nil // Default contrast
}

// GetSettings returns default settings on non-Linux platforms
func (c *Camera) GetSettings() (map[string]int32, error) {
	return map[string]int32{
		"brightness": 128,
		"contrast":   32,
		"saturation": 32,
		"sharpness":  22,
	}, nil
}

// QueryControls returns empty list on non-Linux platforms
func (c *Camera) QueryControls() ([]interface{}, error) {
	return nil, fmt.Errorf("camera controls not available on this platform")
}
