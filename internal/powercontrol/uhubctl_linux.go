//go:build linux
// +build linux

package powercontrol

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// UhubController controls USB hub port power by shelling out to the uhubctl
// command-line tool.
//
// Why uhubctl (instead of the sysfs/ioctl controller in this package):
//
//   - The sysfs controller enumerates hubs with listHubsLinux(), which
//     deliberately SKIPS parent hubs (hubs that have downstream sub-hubs on
//     their ports). A board that sits directly on a parent hub (e.g. "3-10",
//     which carries several sub-devices) is therefore invisible to it and
//     FindHubByLocation returns ErrHubNotFound. uhubctl takes the hub
//     location string directly, so it can drive parent hubs.
//
//   - The ioctl path needs the raw /dev/bus/usb/<bus>/<dev> nodes, which are
//     not present in this container (kernel >= 6 dropped usbfs; only the udev
//     devtmpfs layout exists). uhubctl reaches the device through libusb.
//
// Requirement: the container must expose the host USB device to libusb, i.e.
// /dev/bus/usb/<bus>/<dev> must exist and be readable (usbfs, or a devtmpfs
// that includes the device). Without it libusb cannot open the device.
//
// Known quirk: uhubctl segfaults on some container kernels. That crash happens
// AFTER uhubctl has already issued the port-power control transfer, so the
// power state still changes. We treat a bare crash as success and only surface
// a real error when uhubctl prints "Failed"/"Permission" (the request was
// rejected, not just crashed).
type UhubController struct {
	// bin is the uhubctl executable. Overridable for testing.
	bin string
}

// NewUhubController returns a uhubctl-backed power controller.
func NewUhubController() *UhubController {
	return &UhubController{bin: "uhubctl"}
}

// Enabled reports whether the uhubctl controller should be used in place of the
// default sysfs/ioctl controller. Set ESPBREW_POWER_UHUBCTL to any non-empty
// value (e.g. "1") to opt in.
func Enabled() bool {
	return os.Getenv("ESPBREW_POWER_UHUBCTL") != ""
}

// FindHubByLocation returns a hub described purely by its location string
// (e.g. "3-10"). No enumeration is performed, so any location is accepted,
// including parent hubs that listHubs would skip. NumPorts is set high so the
// caller's port bounds check passes; uhubctl validates the port itself.
func (u *UhubController) FindHubByLocation(loc string) (*Hub, error) {
	if loc == "" {
		return nil, ErrHubNotFound
	}
	return &Hub{Location: loc, NumPorts: 24}, nil
}

// SetPortPowerDual switches port power via uhubctl. The hub on this board is a
// single physical interface (no USB 2.0+3.0 dual counterpart), so we drive the
// port once. uhubctl command:
//
//	uhubctl -l <location> -p <port> -a on|off
func (u *UhubController) SetPortPowerDual(hub *Hub, port int, on bool) error {
	if port < 1 {
		return ErrPortNotFound
	}
	return u.setPortPower(hub.Location, port, on)
}

// setPortPower runs uhubctl for a single location/port. A non-zero exit is
// accepted as long as uhubctl did not report a rejected request.
func (u *UhubController) setPortPower(location string, port int, on bool) error {
	args := []string{"-l", location, "-p", strconv.Itoa(port), "-a"}
	if on {
		args = append(args, "on")
	} else {
		args = append(args, "off")
	}

	out, err := exec.Command(u.bin, args...).CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		// A bare crash (signal) with no "Failed"/"Permission" text means the
		// request was issued before the crash; treat as success.
		if !strings.Contains(msg, "Failed") && !strings.Contains(msg, "Permission") {
			return nil
		}
		return fmt.Errorf("uhubctl -l %s -p %d: %v: %s", location, port, err, msg)
	}
	return nil
}
