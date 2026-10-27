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

// requireHardware skips t unless both the opt-in env var is set and the target
// device node exists. It centralizes the guard so every hardware test fails
// closed (skip) instead of hanging when no board is attached.
func requireHardware(t *testing.T, path string) {
	t.Helper()
	if !hardwareTestEnabled() {
		t.Skipf("hardware tests disabled (set EB_HARDWARE_TEST=1)")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no %s attached: %v", path, err)
	}
}
