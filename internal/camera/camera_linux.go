//go:build linux
// +build linux

package camera

import (
	linuxcam "codeberg.org/georgik/espbrew-go/internal/camera/linux"
)

// linuxController wraps the Linux camera to implement Controller interface
type linuxController struct {
	*linuxcam.Camera
}

// newLinuxController creates a Linux V4L2 camera controller instance
func newLinuxController(devicePath string) (Controller, error) {
	cam, err := linuxcam.NewCamera(devicePath)
	if err != nil {
		return nil, err
	}
	return &linuxController{Camera: cam}, nil
}

// GetControlRange returns min/max for a control by name
func (l *linuxController) GetControlRange(controlName string) (min, max int32, err error) {
	return l.Camera.GetControlRangeByName(controlName)
}

// GetControlInfo returns full info for a control by name
func (l *linuxController) GetControlInfo(controlName string) (min, max, current int32, err error) {
	return l.Camera.GetControlInfoByName(controlName)
}

// newStubController is not used on Linux
func newStubController(devicePath string) (Controller, error) {
	panic("stub controller should not be used on linux")
}
