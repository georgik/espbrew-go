//go:build !darwin && !linux

package espflash

import (
	"errors"

	"go.bug.st/serial"
)

var errNoHandle = errors.New("atomic TIOCMSET not available on this platform")

// setDTRandRTSAtomic is not available on non-Unix platforms.
// Non-Unix systems will always fall back to separate SetDTR/SetRTS calls.
func setDTRandRTSAtomic(port interface{}, dtr, rts bool) error {
	return errNoHandle
}

// forceP4Download is a no-op on non-Unix platforms (e.g. js/wasm). The P4
// download-mode latch sequence relies on the atomic TIOCMSET ioctl, which is
// unavailable off darwin/linux; the browser UI build therefore ships this stub
// so the package still links. The real implementation lives in reset_unix.go.
func forceP4Download(port serial.Port) {}
