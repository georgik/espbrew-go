//go:build linux
// +build linux

package powercontrol

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// USB device file system ioctls
const (
	usbdevfsControl = 0xc0185500 // USBDEVFS_CONTROL
	usbdevfsHubInfo = 0x80115511 // USBDEVFS_HUB_INFO
)

// Helper macros for ioctl codes
const (
	iocRead     = 0x40000000 // _IOC_READ
	iocWrite    = 0x80000000 // _IOC_WRITE
	iocSizeMask = 0x1fff0000
)

// iowr builds IOWR ioctl code
func iowr(typ byte, nr, size uintptr) uintptr {
	return iocRead | iocWrite | (uintptr(typ) << 8) | nr | (size & 0x1fff)
}

// ior builds IOR ioctl code
func ior(typ byte, nr, size uintptr) uintptr {
	return iocRead | (uintptr(typ) << 8) | nr | (size & 0x1fff)
}

// USB hub class-specific requests
const (
	usbReqGetStatus    = 0x00
	usbReqClearFeature = 0x01
	usbReqSetFeature   = 0x03

	// Hub class features
	usbPortFeatConnection  = 0
	usbPortFeatEnable      = 1
	usbPortFeatSuspend     = 2
	usbPortFeatOvercurrent = 3
	usbPortFeatReset       = 4
	usbPortFeatPower       = 8
	usbPortFeatLowSpeed    = 9
	usbPortFeatHighSpeed   = 10
	usbPortFeatTest        = 11
	usbPortFeatIndicator   = 22

	// USB 3.0 specific
	usbPortFeatU1Timeout = 23
	usbPortFeatU2Timeout = 24
	usbPortFeatTestMode  = 25
)

// usbDevfsCtrltransfer represents a USB control transfer
type usbDevfsCtrltransfer struct {
	bRequestType uint8
	bRequest     uint8
	wValue       uint16
	wIndex       uint16
	wLength      uint16
	timeout      uint32 // in milliseconds
	data         uintptr
}

// usbDevfsHubInfo provides information about USB hub
type usbDevfsHubInfo struct {
	bHubCharacteristics uint16
	bPwrOn2PwrGood      uint8
	bHubContrCurrent    uint8
	deviceRemoveable    [32]uint8 // bit 0 means device is removable
	portRemoveable      [32]uint8 // bit 0 means device is removable
}

// usbHubPortStatus represents the status of a hub port
type usbHubPortStatus struct {
	wPortStatus uint16
	wPortChange uint16
}

// getHubDevFile opens the USB device file for the hub
func getHubDevFile(hub *Hub) (*os.File, error) {
	// Read bus number from sysfs
	busNum, _ := parseLocationToBusDevice(hub.Location)

	// Read actual device number from sysfs
	sysfsPath := fmt.Sprintf("/sys/bus/usb/devices/%s", hub.Location)
	devNumBytes, err := os.ReadFile(filepath.Join(sysfsPath, "devnum"))
	if err != nil {
		return nil, fmt.Errorf("read devnum: %w", err)
	}

	var devNum uint64
	devNumStr := strings.TrimSpace(string(devNumBytes))
	if devNumStr == "" {
		return nil, fmt.Errorf("empty devnum for %s", hub.Location)
	}

	if devNum, err = strconv.ParseUint(devNumStr, 10, 16); err != nil {
		return nil, fmt.Errorf("parse devnum: %w", err)
	}

	devPath := fmt.Sprintf("/dev/bus/usb/%03d/%03d", busNum, devNum)
	return os.OpenFile(devPath, os.O_RDWR, 0)
}

// parseLocationToBusDevice extracts bus and device numbers from location
// Location format: "4-2" or "1-5.3" -> bus, device
// For "1-5.3": bus=1, device=5 (the .3 is port number, ignored)
func parseLocationToBusDevice(location string) (bus, device uint16) {
	// Parse "X-Y" or "X-Y.Z" format
	parts := strings.Split(location, "-")
	if len(parts) < 2 {
		return
	}

	// Parse bus number
	if b, err := strconv.ParseUint(parts[0], 10, 16); err == nil {
		bus = uint16(b)
	}

	// Parse device number (stop before any dot)
	deviceStr := parts[1]
	if dotIdx := strings.Index(deviceStr, "."); dotIdx > 0 {
		deviceStr = deviceStr[:dotIdx]
	}

	if d, err := strconv.ParseUint(deviceStr, 10, 16); err == nil {
		device = uint16(d)
	}

	return
}

// getPortStatusIoctl gets port status via USB ioctl
func getPortStatusIoctl(hub *Hub, port int) (*PortStatus, error) {
	f, err := getHubDevFile(hub)
	if err != nil {
		return nil, fmt.Errorf("open hub device: %w", err)
	}
	defer f.Close()

	var status usbHubPortStatus

	// USB GET_PORT_STATUS request
	// Request type: 10100000b (recipient=port, type=class, direction=in)
	ctrl := usbDevfsCtrltransfer{
		bRequestType: 0xa3, // USB_TYPE_CLASS | USB_DIR_IN | USB_RECIP_OTHER
		bRequest:     usbReqGetStatus,
		wValue:       0,
		wIndex:       uint16(port),
		wLength:      4,
		timeout:      1000,
		data:         uintptr(unsafe.Pointer(&status)),
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		usbdevfsControl,
		uintptr(unsafe.Pointer(&ctrl)),
	)

	if errno != 0 {
		return nil, fmt.Errorf("ioctl get port status: %v", errno)
	}

	// Decode status
	ps := &PortStatus{
		Port: port,
	}

	// Power status depends on USB version
	if hub.SuperSpeed {
		ps.Power = (status.wPortStatus & usbSsPortStatPower) != 0
		ps.Enabled = (status.wPortStatus & usbPortStatEnable) != 0

		// USB 3.0: connection status from link state
		linkState := (status.wPortStatus & usbPortStatLinkState) >> 5
		ps.Connected = linkState != 0 // Link state 0 = disconnected/disabled

		// Decode speed/link state for USB 3.0
		switch linkState {
		case 0:
			ps.Speed = "disabled"
		case 1: // U0 - U3 active
			if (status.wPortStatus & usbPortStatSpeed5Gbps) != 0 {
				ps.Speed = "5gbps"
			} else {
				ps.Speed = "U0"
			}
		case 2:
			ps.Speed = "U1"
		case 3:
			ps.Speed = "U2"
		case 4:
			ps.Speed = "U3"
		case 5: // Rx.Detect
			ps.Speed = "5gbps"
		case 6: // Compliance Mode
			ps.Speed = "compliance"
		default:
			ps.Speed = "unknown"
		}
	} else {
		ps.Power = (status.wPortStatus & usbPortStatPower) != 0
		ps.Connected = (status.wPortStatus & usbPortStatConnection) != 0
		ps.Enabled = (status.wPortStatus & usbPortStatEnable) != 0

		// Decode speed for USB 2.0
		switch {
		case (status.wPortStatus & usbPortStatLowSpeed) != 0:
			ps.Speed = "low"
		case (status.wPortStatus & usbPortStatHighSpeed) != 0:
			ps.Speed = "high"
		case ps.Connected:
			ps.Speed = "full"
		}
	}

	return ps, nil
}

// setPortPowerIoctl sets port power via USB ioctl and verifies the change.
func setPortPowerIoctl(hub *Hub, port int, on bool) error {
	f, err := getHubDevFile(hub)
	if err != nil {
		return fmt.Errorf("open hub device: %w", err)
	}
	defer f.Close()

	// USB SET_FEATURE or CLEAR_FEATURE for port power
	// Request type: 00100000b (recipient=port, type=class, direction=out)
	feature := uint8(usbReqClearFeature)
	if on {
		feature = uint8(usbReqSetFeature)
	}

	ctrl := usbDevfsCtrltransfer{
		bRequestType: 0x23, // USB_TYPE_CLASS | USB_DIR_OUT | USB_RECIP_OTHER
		bRequest:     feature,
		wValue:       usbPortFeatPower,
		wIndex:       uint16(port),
		wLength:      0,
		timeout:      1000,
		data:         0,
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		usbdevfsControl,
		uintptr(unsafe.Pointer(&ctrl)),
	)

	if errno != 0 {
		if errno == syscall.EPERM {
			return ErrPermissionDenied
		}
		return fmt.Errorf("ioctl set port power: %v", errno)
	}

	// Verify the power state actually changed
	status, err := getPortStatusIoctl(hub, port)
	if err != nil {
		return fmt.Errorf("verify power state: %w", err)
	}

	// Check if power matches requested state
	if status.Power != on {
		return fmt.Errorf("power state verification failed: requested %v, actual %v", on, status.Power)
	}

	return nil
}
