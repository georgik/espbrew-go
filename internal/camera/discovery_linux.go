//go:build linux
// +build linux

package camera

// getMacOSCameraName returns empty string on Linux
func getMacOSCameraName(deviceUID string) string {
	return ""
}

// refreshMacOSCameraNameCache is a no-op on Linux
func refreshMacOSCameraNameCache() {
}
