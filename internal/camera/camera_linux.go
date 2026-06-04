//go:build linux
// +build linux

package camera

import (
	linuxcam "codeberg.org/georgik/espbrew-go/internal/camera/linux"
)

// newLinuxController creates a Linux V4L2 camera controller instance
func newLinuxController(devicePath string) (Controller, error) {
	return linuxcam.NewCamera(devicePath)
}

// newStubController is not used on Linux
func newStubController(devicePath string) (Controller, error) {
	panic("stub controller should not be used on linux")
}
