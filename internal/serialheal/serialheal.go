// Package serialheal transparently recovers a serial device node whose mount
// point has gone stale after a USB power cycle or re-enumeration.
//
// # Background
//
// Inside the espbrew cluster container, /dev/ttyACM0 (and similar per-device
// nodes) can be a separately-mounted bind mount of the host device inode. When
// the board is power-cycled over the USB hub it disconnects and re-enumerates,
// which the container's bind mount does NOT follow: the node either points at a
// dead-but-still-0666 inode (ENXIO/EIO on open) or is re-minted by the host with
// a restrictive mode (EACCES). In both cases serial.Open fails.
//
// The fix is host-side but can be driven from espbrew itself: if the path is a
// mount point, umount it and re-bind it in place to pick up the fresh inode.
// This requires CAP_SYS_ADMIN + seccomp allowing mount() (see the container
// launch flags) and NOPASSWD sudo for umount/mount/chmod.
//
// # Design
//
// Open is a drop-in replacement for go.bug.st/serial.Open. It only attempts a
// heal when the open fails AND the path is a mount point AND the failure is not
// a transient "busy" condition (which the caller retries on its own). For plain
// devtmpfs nodes (not a mount point) it is exactly serial.Open — a no-op.
package serialheal

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"go.bug.st/serial"
)

// envDisable gates the heal off entirely (used for debugging / CI).
const envDisable = "ESPBREW_SERIAL_HEAL"

var mu sync.Mutex

// Enabled reports whether the self-heal is active. It is on by default; set
// ESPBREW_SERIAL_HEAL=0 to disable.
func Enabled() bool {
	return os.Getenv(envDisable) != "0" && !strings.EqualFold(os.Getenv(envDisable), "off")
}

// Open opens a serial port, healing a stale bind mount on open failure.
//
// It is a drop-in replacement for serial.Open. On success it returns the port
// immediately. On failure it attempts a single heal (umount + mount --bind +
// chmod 0666) when the path is a mount point and the failure is not a transient
// busy condition, then retries the open once.
func Open(portName string, mode *serial.Mode) (serial.Port, error) {
	port, err := serial.Open(portName, mode)
	if err == nil {
		return port, nil
	}
	// A busy port is handled by the caller's own retry loop; do not umount a
	// mount that is in use.
	if isBusyError(err) {
		return nil, err
	}
	if !Enabled() {
		return nil, err
	}
	if !IsMountPoint(portName) {
		// Not a bind mount (e.g. a plain devtmpfs node): nothing to heal.
		return nil, err
	}
	if healErr := Heal(portName); healErr == nil {
		port, err = serial.Open(portName, mode)
	} else {
		// Heal could not help (e.g. the mount is busy, or sudo is missing).
		// Surface the original open error so the caller can retry as before.
		err = fmt.Errorf("serialheal: %w (heal failed: %v)", err, healErr)
	}
	return port, err
}

// Heal rebinds a mount-point device node to its current host inode and clears
// any restrictive mode the host re-minted on re-enumeration.
//
// Steps: sudo umount <path>, sudo mount --bind <path> <path>, sudo chmod 0666
// <path>. chmod is best-effort (a node that is not restrictive needs no chmod).
func Heal(path string) error {
	mu.Lock()
	defer mu.Unlock()

	if out, err := runSudo("umount", path); err != nil {
		return fmt.Errorf("umount %s: %w: %s", path, err, strings.TrimSpace(string(out)))
	}
	if out, err := runSudo("mount", "--bind", path, path); err != nil {
		return fmt.Errorf("mount --bind %s: %w: %s", path, err, strings.TrimSpace(string(out)))
	}
	// Best-effort: clear a restrictive mode the host may have re-minted.
	// A plain 0666 node is left untouched (non-zero exit is ignored).
	_, _ = runSudo("chmod", "0666", path)
	return nil
}

// runSudo runs a command via NOPASSWD sudo, returning combined output.
func runSudo(args ...string) ([]byte, error) {
	argv := append([]string{"-n"}, args...)
	return exec.Command("sudo", argv...).CombinedOutput()
}

// IsMountPoint reports whether path is listed as a mount point in /proc/mounts.
// It returns false (and does not attempt a heal) for plain devtmpfs nodes, so
// the wrapper is a safe no-op everywhere.
func IsMountPoint(path string) bool {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		// /proc/mounts columns: device mountpoint fstype options freq passno
		if len(fields) >= 2 && fields[1] == path {
			return true
		}
	}
	return false
}

// isBusyError reports whether err is the transient "resource busy" condition
// that go.bug.st/serial surfaces and that a retry should wait out (rather than
// a heal). Mirrors the flasher's own busy detection.
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	var portErr *serial.PortError
	if errors.As(err, &portErr) {
		return portErr.Code() == serial.PortBusy
	}
	if errors.Is(err, syscall.EBUSY) {
		return true
	}
	return strings.Contains(err.Error(), "Serial port busy")
}
