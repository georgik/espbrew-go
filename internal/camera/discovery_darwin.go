//go:build darwin
// +build darwin

package camera

import (
	"sync"

	"github.com/pion/mediadevices/pkg/avfoundation"
)

var (
	// Cache mapping device UID to friendly name
	macOSCameraNameCache   map[string]string
	macOSCameraNameCacheMu sync.RWMutex
)

func init() {
	macOSCameraNameCache = make(map[string]string)
}

// getMacOSCameraName returns friendly name for a device UID
func getMacOSCameraName(deviceUID string) string {
	macOSCameraNameCacheMu.RLock()
	if name, ok := macOSCameraNameCache[deviceUID]; ok {
		macOSCameraNameCacheMu.RUnlock()
		return name
	}
	macOSCameraNameCacheMu.RUnlock()

	// Query AVFoundation for all devices
	devices, err := avfoundation.Devices(avfoundation.Video)
	if err != nil {
		return ""
	}

	macOSCameraNameCacheMu.Lock()
	defer macOSCameraNameCacheMu.Unlock()

	// Build cache and find our device
	for _, device := range devices {
		macOSCameraNameCache[device.UID] = device.Name
		if device.UID == deviceUID {
			return device.Name
		}
	}

	return ""
}

// refreshMacOSCameraNameCache rebuilds the name cache
func refreshMacOSCameraNameCache() {
	macOSCameraNameCacheMu.Lock()
	defer macOSCameraNameCacheMu.Unlock()

	devices, err := avfoundation.Devices(avfoundation.Video)
	if err != nil {
		return
	}

	// Rebuild cache
	macOSCameraNameCache = make(map[string]string)
	for _, device := range devices {
		macOSCameraNameCache[device.UID] = device.Name
	}
}
