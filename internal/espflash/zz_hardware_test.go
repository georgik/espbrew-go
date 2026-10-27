package espflash

import (
	"os"
	"testing"
)

// Hardware-backed investigation tests (connect/attach/flash writeback) require a
// real board on the target port. They gate on two conditions so they never block
// the default suite on a stale or absent board node:
//
//   - EB_HARDWARE_TEST must be set (explicit opt-in), and
//   - the target device node must exist.
//
// A device node existing is not enough: a node alone does not mean a responding
// board is attached, and these tests connect with multi-second context
// timeouts, so running them against a dead node would hang the suite. Set
// EB_HARDWARE_TEST=1 to run them against a real board.

// hardwareTestEnabled reports whether hardware-backed tests may run.
func hardwareTestEnabled() bool {
	return os.Getenv("EB_HARDWARE_TEST") != ""
}

// hardwarePort returns the board port for hardware-backed tests. It defaults to
// /dev/ttyACM0 but can be overridden with EB_HARDWARE_PORT so the same tests run
// on a different lab machine whose board is on e.g. /dev/ttyUSB0.
func hardwarePort() string {
	if p := os.Getenv("EB_HARDWARE_PORT"); p != "" {
		return p
	}
	return "/dev/ttyACM0"
}

// requireHardware skips t unless both the opt-in env var is set and the target
// device node exists. It centralizes the guard so every hardware test fails
// closed (skip) instead of hanging when no board is attached. The port comes
// from hardwarePort() so there is a single source of truth for the magic value.
func requireHardware(t *testing.T) {
	t.Helper()
	path := hardwarePort()
	if !hardwareTestEnabled() {
		t.Skipf("hardware tests disabled (set EB_HARDWARE_TEST=1)")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no %s attached: %v", path, err)
	}
}
