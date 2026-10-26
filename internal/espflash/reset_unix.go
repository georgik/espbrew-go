//go:build darwin || linux

package espflash

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"syscall"
	"time"
	"unsafe"

	"go.bug.st/serial"
	"golang.org/x/sys/unix"
)

var errNoHandle = errors.New("port does not have accessible file descriptor")

// setDTRandRTSAtomic performs an atomic TIOCMSET ioctl on Unix systems.
// It attempts to use the underlying file descriptor to set DTR and RTS
// simultaneously, which is important for precise timing on CH340 boards.
// Returns errNoHandle if the port doesn't have an accessible fd (signals to try separate calls).
func setDTRandRTSAtomic(port interface{}, dtr, rts bool) error {
	// Try to get the underlying file descriptor via type assertion.
	// go.bug.st/serial's unixPort has a handle field (int), but it's unexported.
	// We'll use reflect to access it.
	const ptrKind = reflect.Ptr
	v := reflect.ValueOf(port)
	if v.Kind() == ptrKind {
		v = v.Elem()
	}

	// Look for a field named "handle" (fd)
	handleField := v.FieldByName("handle")
	if !handleField.IsValid() || handleField.Kind() != reflect.Int {
		// Field doesn't exist or wrong type; caller will fall back to separate calls
		return errNoHandle
	}

	fd := int(handleField.Int())

	// Build the TIOCMSET bitmask
	var status int
	if dtr {
		status |= unix.TIOCM_DTR
	}
	if rts {
		status |= unix.TIOCM_RTS
	}

	// Perform the atomic ioctl
	return unix.IoctlSetPointerInt(fd, unix.TIOCMSET, status)
}

// tioCMGet reads back the current modem line status (DTR/RTS bits) via
// TIOCMGET so callers can verify the lines were actually toggled. Returns 0 if
// the port handle cannot be resolved.
func tioCMGet(port serial.Port) int {
	const ptrKind = reflect.Ptr
	v := reflect.ValueOf(port)
	if v.Kind() == ptrKind {
		v = v.Elem()
	}
	handleField := v.FieldByName("handle")
	if !handleField.IsValid() || handleField.Kind() != reflect.Int {
		return -1
	}
	fd := int(handleField.Int())
	var status int
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), unix.TIOCMGET, uintptr(unsafe.Pointer(&status))); err != 0 {
		return -2
	}
	return status
}

// forceP4Download enters the ESP32-P4 serial bootloader via the DTR/RTS control
// lines, using the exact polarity verified on the Conway board (see
// /tmp/p4probe/main.go forceDownload). The P4 native USB-Serial-JTAG peripheral
// does not need a USB-JTAG re-enumeration to accept the serial bootloader: holding
// IO0 (DTR) LOW across the EN reset pulse (RTS) latches the chip into download
// mode. The polarity on this board is:
//
//	DTR=true  -> IO0 LOW  -> download mode
//	DTR=false -> IO0 HIGH -> run application
//	RTS=false -> EN LOW   -> chip held in reset (active low)
//	RTS=true  -> EN HIGH  -> reset released
//
// The critical transition is releasing EN (RTS false->true) WHILE IO0 is still
// LOW (DTR=true); the chip then boots into the serial bootloader. Only after the
// boot does DTR go low/high so the bootloader flag stays latched.
func forceP4Download(port serial.Port) {
	step := func(name string, dtr, rts bool) {
		setDTRandRTS(port, dtr, rts) //nolint:errcheck
		if os.Getenv("EB_DEBUG") != "" {
			got := tioCMGet(port)
			fmt.Printf("[DBG reset] %s -> set(DTR=%v RTS=%v) readback(DTR=%v RTS=%v)\n",
				name, dtr, rts, got&unix.TIOCM_DTR != 0, got&unix.TIOCM_RTS != 0)
		}
	}
	// IO0 LOW (download), EN released.
	step("idle", true, true)
	time.Sleep(150 * time.Millisecond)
	// Keep IO0 LOW, assert EN (reset active).
	step("reset-pulse", true, false)
	time.Sleep(200 * time.Millisecond)
	// Release EN while IO0 still LOW -> chip latches into download mode.
	step("release-en", true, true)
	time.Sleep(200 * time.Millisecond)
	// Release IO0 (keep EN released) so the port is idle for the sync loop.
	step("release-io0", false, true)
	time.Sleep(200 * time.Millisecond)
}
