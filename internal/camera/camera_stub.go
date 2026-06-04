//go:build !linux
// +build !linux

package camera

import (
	stubcam "codeberg.org/georgik/espbrew-go/internal/camera/stub"
)

// newLinuxController is not used on non-Linux platforms
func newLinuxController(devicePath string) (Controller, error) {
	panic("linux controller should not be used on non-linux platforms")
}

// newStubController creates a stub camera controller instance
func newStubController(devicePath string) (Controller, error) {
	return stubcam.NewCamera(devicePath)
}
