//go:build windows
// +build windows

package camera

// getMacOSCameraName returns empty string on Windows
func getMacOSCameraName(deviceUID string) string {
	return ""
}

// refreshMacOSCameraNameCache is a no-op on Windows
func refreshMacOSCameraNameCache() {
}
