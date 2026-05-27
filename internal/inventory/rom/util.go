package rom

import (
	"fmt"
)

// formatMAC formats 6 bytes as MAC address string "xx:xx:xx:xx:xx:xx"
func formatMAC(mac []byte) string {
	if len(mac) < 6 {
		return ""
	}
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

// DeviceID formats a MAC address as a device ID "esp-xx:xx:xx:xx:xx:xx"
func DeviceID(mac string) string {
	return "esp-" + mac
}
