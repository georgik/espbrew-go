//go:build linux

package flash

import (
	"path/filepath"
	"testing"
)

// The detectBridgeBoard* helpers are Linux-only: detectBridgeBoardIn reads a
// /dev/serial/by-id directory (a udev path that does not exist on macOS or
// Windows) and carries an explicit `runtime.GOOS != "linux"` guard that makes
// it return false on non-Linux hosts. These tests exercise that Linux behavior,
// so they are feature-gated to linux and skipped everywhere else.

// TestDetectBridgeBoardIn creates a fake /dev/serial/by-id tree and confirms the
// port is matched to the right by-id name and the vendor is recognised.
func TestDetectBridgeBoardIn(t *testing.T) {
	dir := t.TempDir()

	// Two boards: a CH9102 bridge on ttyACM5 and an Espressif native board on
	// ttyACM0. The symlink name (not target) carries the vendor id.
	mustSymlink(t, dir, "usb-1a86_USB_Single_Serial_5971084553-if00", "ttyACM5")
	mustSymlink(t, dir, "usb-303a_Espressif_USB_JTAG_serial_ABC-if00", "ttyACM0")

	if !detectBridgeBoardIn("/dev/ttyACM5", dir) {
		t.Error("ttyACM5 (CH9102) should be detected as a bridge")
	}
	if detectBridgeBoardIn("/dev/ttyACM0", dir) {
		t.Error("ttyACM0 (Espressif native) must NOT be detected as a bridge")
	}
	if detectBridgeBoardIn("/dev/ttyACM9", dir) {
		t.Error("an unknown port must not be detected as a bridge")
	}
}

// TestDetectBridgeBoardMissingDir confirms the safe default: when the by-id
// directory is absent the function returns false rather than erroring.
func TestDetectBridgeBoardMissingDir(t *testing.T) {
	if detectBridgeBoardIn("/dev/ttyACM5", filepath.Join(t.TempDir(), "nope")) {
		t.Error("missing by-id dir must return false")
	}
}
