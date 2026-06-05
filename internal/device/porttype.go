package device

import (
	"strings"
)

// PortType represents the type of serial port connection
type PortType string

const (
	PortTypeUART          PortType = "uart"
	PortTypeUSBSerialJTAG PortType = "usb_serial_jtag"
)

// USJProductNames contains product name substrings that indicate USB Serial/JTAG
var USJProductNames = []string{
	"jtag/serial",
	"jtag_serial",
	"usb jtag",
	"usb serial/jtag",
}

// NativeUSBPathPrefixes are path prefixes that suggest native USB connection
var NativeUSBPathPrefixes = []string{
	"/dev/tty.usbmodem",
	"/dev/cu.usbmodem",
}

// DetectPortType determines if a port is USB Serial/JTAG or external UART
// based on product name and USB path characteristics
func DetectPortType(vid, pid uint16, product, path string) PortType {
	productLower := strings.ToLower(product)
	pathLower := strings.ToLower(path)

	// Check product name for JTAG indicators
	for _, usjName := range USJProductNames {
		if strings.Contains(productLower, usjName) {
			return PortTypeUSBSerialJTAG
		}
	}

	// For Espressif devices with native USB Serial/JTAG,
	// check if path suggests native USB connection
	if vid == ESP_VID {
		switch pid {
		case ESP_PID_S3, ESP_PID_C3, ESP_PID_C6:
			// These chips have native USB Serial/JTAG
			for _, prefix := range NativeUSBPathPrefixes {
				if strings.HasPrefix(pathLower, prefix) {
					return PortTypeUSBSerialJTAG
				}
			}
		}
	}

	return PortTypeUART
}

// IsNativeUSBPath returns true if the path suggests a native USB connection
func IsNativeUSBPath(path string) bool {
	pathLower := strings.ToLower(path)
	for _, prefix := range NativeUSBPathPrefixes {
		if strings.HasPrefix(pathLower, prefix) {
			return true
		}
	}
	return false
}
