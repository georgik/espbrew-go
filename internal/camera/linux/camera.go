//go:build linux
// +build linux

package linux

import (
	"fmt"
	"log"

	"github.com/vladimirvivien/go4vl/device"
	v4l2 "github.com/vladimirvivien/go4vl/v4l2"
)

// Camera wraps a V4L2 device with control methods
type Camera struct {
	dev  *device.Device
	path string
}

// NewCamera opens a V4L2 camera device
func NewCamera(devicePath string) (*Camera, error) {
	dev, err := device.Open(devicePath)
	if err != nil {
		return nil, fmt.Errorf("open camera: %w", err)
	}

	return &Camera{
		dev:  dev,
		path: devicePath,
	}, nil
}

// Close releases the camera device
func (c *Camera) Close() error {
	if c.dev != nil {
		return c.dev.Close()
	}
	return nil
}

// Path returns the device path
func (c *Camera) Path() string {
	return c.path
}

// Device returns the underlying V4L2 device
func (c *Camera) Device() *device.Device {
	return c.dev
}

// SetDisplayPreset configures the camera for optimal display photography
// Settings optimized for glowing/backlit displays:
// - Lower brightness to avoid overexposure
// - Higher contrast for text readability
// - Higher sharpness for clear text
// - Manual exposure for consistent results
func (c *Camera) SetDisplayPreset() error {
	settings := map[string]struct {
		name string
		set  func(int32) error
		val  int32
	}{
		"Brightness": {name: "Brightness", set: c.dev.SetBrightness, val: 80},
		"Contrast":   {name: "Contrast", set: c.dev.SetContrast, val: 140},
		"Sharpness": {name: "Sharpness", set: func(v int32) error {
			return c.dev.SetControlValue(v4l2.CtrlSharpness, v)
		}, val: 150},
		"Saturation": {name: "Saturation", set: func(v int32) error {
			return c.dev.SetControlSaturation(v)
		}, val: 90},
	}

	for _, s := range settings {
		if err := s.set(s.val); err != nil {
			log.Printf("Warning: failed to set %s to %d: %v", s.name, s.val, err)
			// Continue with other settings
		}
	}

	// Set manual exposure for consistent lighting
	return c.setManualExposure(300)
}

// setManualExposure configures manual exposure mode
func (c *Camera) setManualExposure(exposureValue int32) error {
	ctrls := v4l2.NewExtControls()
	ctrls.AddValue(v4l2.CtrlCameraExposureAuto, 1) // Manual mode
	ctrls.AddValue(v4l2.CtrlCameraExposureAbsolute, exposureValue)

	if err := c.dev.SetExtControls(ctrls); err != nil {
		return fmt.Errorf("set manual exposure: %w", err)
	}

	return nil
}

// SetFocus configures focus for a specific distance
// distance: 0-255 range (typical: 85 for ~1m display distance)
func (c *Camera) SetFocus(distance int32) error {
	// First disable continuous auto focus
	ctrls := v4l2.NewExtControls()

	// Try to disable continuous autofocus if supported
	ctrls.AddValue(v4l2.CtrlCameraFocusAuto, 0)
	if err := c.dev.SetExtControls(ctrls); err != nil {
		log.Printf("Warning: could not disable auto focus: %v", err)
		// Continue anyway - some cameras don't support this
	}

	// Set absolute focus
	ctrls = v4l2.NewExtControls()
	ctrls.AddValue(v4l2.CtrlCameraFocusAbsolute, distance)

	if err := c.dev.SetExtControls(ctrls); err != nil {
		return fmt.Errorf("set focus: %w", err)
	}

	return nil
}

// SetBrightness adjusts the camera brightness (0-255)
func (c *Camera) SetBrightness(value int32) error {
	return c.dev.SetBrightness(value)
}

// SetContrast adjusts the camera contrast (0-255)
func (c *Camera) SetContrast(value int32) error {
	return c.dev.SetContrast(value)
}

// SetSharpness adjusts the camera sharpness (0-255)
func (c *Camera) SetSharpness(value int32) error {
	return c.dev.SetControlValue(v4l2.CtrlSharpness, value)
}

// SetSaturation adjusts the camera saturation (0-255)
func (c *Camera) SetSaturation(value int32) error {
	return c.dev.SetControlSaturation(value)
}

// GetBrightness retrieves current brightness setting
func (c *Camera) GetBrightness() (int32, error) {
	return c.dev.GetBrightness()
}

// GetContrast retrieves current contrast setting
func (c *Camera) GetContrast() (int32, error) {
	return c.dev.GetContrast()
}

// GetSettings returns current camera settings as a map
func (c *Camera) GetSettings() (map[string]int32, error) {
	settings := make(map[string]int32)

	// Helper to safely add a setting if value is valid
	addSetting := func(key string, value int32, min, max int32) {
		if value >= min && value <= max {
			settings[key] = value
		} else {
			// Clamp invalid values to valid range
			if value < min {
				settings[key] = min
			} else if value > max {
				settings[key] = max
			}
			log.Printf("Warning: %s value %d out of range [%d, %d], clamped", key, value, min, max)
		}
	}

	// Get basic controls with reasonable defaults
	if brightness, err := c.dev.GetBrightness(); err == nil {
		addSetting("brightness", brightness, 0, 255)
	} else {
		log.Printf("Failed to get brightness: %v", err)
	}
	if contrast, err := c.dev.GetContrast(); err == nil {
		addSetting("contrast", contrast, 0, 255)
	} else {
		log.Printf("Failed to get contrast: %v", err)
	}

	// Get extended controls
	if saturation, err := c.dev.GetControl(v4l2.CtrlSaturation); err == nil {
		addSetting("saturation", saturation.Value, 0, 255)
	} else {
		log.Printf("Failed to get saturation: %v", err)
	}
	if sharpness, err := c.dev.GetControl(v4l2.CtrlSharpness); err == nil {
		addSetting("sharpness", sharpness.Value, 0, 255)
	} else {
		log.Printf("Failed to get sharpness: %v", err)
	}

	// Try to get gain (may not be supported on all cameras)
	if gain, err := c.dev.GetControl(v4l2.CtrlGain); err == nil {
		addSetting("gain", gain.Value, 0, 255)
	}

	// Try to get focus (may not be supported on all cameras)
	if focus, err := c.dev.GetControl(v4l2.CtrlCameraFocusAbsolute); err == nil {
		addSetting("focus", focus.Value, 0, 255)
	}

	// Try to get exposure (may not be supported on all cameras)
	if exposure, err := c.dev.GetControl(v4l2.CtrlCameraExposureAbsolute); err == nil {
		addSetting("exposure", exposure.Value, 0, 2047)
	}

	return settings, nil
}

// QueryControls returns all available controls for the device
func (c *Camera) QueryControls() ([]v4l2.Control, error) {
	return c.dev.QueryAllControls()
}

// GetControlRange returns the min/max range for a specific control
func (c *Camera) GetControlRange(controlID v4l2.CtrlID) (min, max int32, err error) {
	ctrl, err := v4l2.QueryControlInfo(c.dev.Fd(), controlID)
	if err != nil {
		return 0, 0, err
	}
	return ctrl.Minimum, ctrl.Maximum, nil
}

// GetControlInfo returns detailed information about a control
func (c *Camera) GetControlInfo(controlID v4l2.CtrlID) (*v4l2.Control, error) {
	ctrl, err := v4l2.QueryControlInfo(c.dev.Fd(), controlID)
	if err != nil {
		return nil, err
	}
	return &ctrl, nil
}

// GetControlRangeByName returns min/max for a control by name
func (c *Camera) GetControlRangeByName(controlName string) (min, max int32, err error) {
	var ctrlID v4l2.CtrlID
	switch controlName {
	case "brightness":
		ctrlID = v4l2.CtrlBrightness
	case "contrast":
		ctrlID = v4l2.CtrlContrast
	case "saturation":
		ctrlID = v4l2.CtrlSaturation
	case "sharpness":
		ctrlID = v4l2.CtrlSharpness
	case "gain":
		ctrlID = v4l2.CtrlGain
	case "focus_absolute":
		ctrlID = v4l2.CtrlCameraFocusAbsolute
	case "exposure_absolute":
		ctrlID = v4l2.CtrlCameraExposureAbsolute
	default:
		return 0, 0, fmt.Errorf("unknown control: %s", controlName)
	}
	return c.GetControlRange(ctrlID)
}

// GetControlInfoByName returns full control info including current value
func (c *Camera) GetControlInfoByName(controlName string) (min, max, current int32, err error) {
	var ctrlID v4l2.CtrlID
	switch controlName {
	case "brightness":
		ctrlID = v4l2.CtrlBrightness
	case "contrast":
		ctrlID = v4l2.CtrlContrast
	case "saturation":
		ctrlID = v4l2.CtrlSaturation
	case "sharpness":
		ctrlID = v4l2.CtrlSharpness
	case "gain":
		ctrlID = v4l2.CtrlGain
	case "focus_absolute":
		ctrlID = v4l2.CtrlCameraFocusAbsolute
	case "exposure_absolute":
		ctrlID = v4l2.CtrlCameraExposureAbsolute
	default:
		return 0, 0, 0, fmt.Errorf("unknown control: %s", controlName)
	}
	ctrl, err := v4l2.QueryControlInfo(c.dev.Fd(), ctrlID)
	if err != nil {
		return 0, 0, 0, err
	}
	// QueryControlInfo doesn't include current value, need to query it separately
	currentCtrl, err := v4l2.GetControl(c.dev.Fd(), ctrlID)
	if err != nil {
		// If we can't get current value, use the default
		return ctrl.Minimum, ctrl.Maximum, ctrl.Default, nil
	}
	return ctrl.Minimum, ctrl.Maximum, currentCtrl.Value, nil
}
