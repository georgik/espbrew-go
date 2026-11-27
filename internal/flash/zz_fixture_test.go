package flash

import (
	"os"
	"testing"
)

// investigationFixture is the path to a prebuilt firmware ELF used by the
// P4 investigation tests (bootloader/app header dumps, reset sweep, ...).
//
// It lives in a separate example project under repos/ and is only present on
// the developer's machine, not in CI. The tests below read it through
// readFixtureOrSkip, which skips (instead of failing) when the file is
// absent, so the investigation suite stays green in CI while still running
// locally where the fixture exists.
// investigationFixture resolves the P4 investigation ELF from the
// EB_INVESTIGATION_ELF environment variable. It lives in a sibling example
// project and is only present on the developer's machine, not in CI.
func investigationFixture() string { return fixtureFromEnv("EB_INVESTIGATION_ELF") }

// espflashReferenceELF resolves the reference P4 ELF from the
// EB_ESPFLASH_REFERENCE_ELF environment variable. It ships with espflash in a
// sibling repo, so it is likewise absent from the espbrew-go CI checkout.
func espflashReferenceELF() string { return fixtureFromEnv("EB_ESPFLASH_REFERENCE_ELF") }

// fixtureFromEnv resolves a developer-only fixture path from the named
// environment variable, returning "" when it is unset. Test files use it to
// locate prebuilt ELFs that live in sibling example projects (and are therefore
// absent from the espbrew-go checkout and from CI). An empty result makes
// readFixtureOrSkip skip the test instead of failing, so the suite stays green
// in CI while still running locally where the developer exports the variable.
// No developer's absolute path is hardcoded anywhere.
func fixtureFromEnv(name string) string { return os.Getenv(name) }

// readFixtureOrSkip reads an ELF/binary fixture for the investigation tests.
// When the file is missing (e.g. in CI, where the full example project is not
// built) the test is skipped instead of failing, preserving the diagnostic on
// the developer's machine where the fixture is present.
func readFixtureOrSkip(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("investigation fixture %q not available, skipping: %v", path, err)
	}
	return data
}

// hasHardwarePort reports whether the target serial port device node exists.
// The hardware e2e investigation tests skip when it is absent so they never
// block on a real timeout or fail in CI, where no board is attached.
func hasHardwarePort(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// hardwareTestEnabled reports whether hardware-backed tests are permitted to
// run. They require an explicit opt-in (EB_HARDWARE_TEST set) so they never
// block on a stale or absent board node by default. A device node existing is
// not enough: a node alone does not mean a responding board is attached, and
// the tests must not hang waiting on it. Set EB_HARDWARE_TEST=1 to run them
// against a real board.
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
