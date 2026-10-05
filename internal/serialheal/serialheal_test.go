package serialheal

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// mountPointInProcMounts reports whether path is listed as a mount point in
// /proc/mounts. The test uses it to decide whether the /etc/hosts assertion
// below applies to the current environment: /etc/hosts is a bind-mounted file
// only inside containers such as Docker, and a plain file elsewhere.
func mountPointInProcMounts(path string) bool {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[1] == path {
			return true
		}
	}
	return false
}

func TestIsMountPoint(t *testing.T) {
	// These paths are always mount points on Linux.
	for _, p := range []string{"/", "/proc", "/sys"} {
		if !IsMountPoint(p) {
			t.Errorf("IsMountPoint(%q) = false, want true", p)
		}
	}

	// These paths are never mount points.
	for _, p := range []string{"/nonexistent-device-node-xyz", "/dev/ttyACM0-does-not-exist"} {
		if IsMountPoint(p) {
			t.Errorf("IsMountPoint(%q) = true, want false", p)
		}
	}

	// /etc/hosts is a bind-mounted file only inside containers (e.g. Docker).
	// When the current environment does not bind-mount it, the assertion does
	// not apply, so skip rather than fail.
	if mountPointInProcMounts("/etc/hosts") {
		if !IsMountPoint("/etc/hosts") {
			t.Errorf("IsMountPoint(/etc/hosts) = false, want true")
		}
	} else {
		t.Skip("/etc/hosts is not a bind mount in this environment")
	}
}

func TestEnabledDefault(t *testing.T) {
	// With the env var unset, the heal must be on by default.
	t.Setenv(envDisable, "")
	if !Enabled() {
		t.Fatal("Enabled() = false with env unset; want true")
	}
}

func TestEnabledDisabled(t *testing.T) {
	t.Setenv(envDisable, "0")
	if Enabled() {
		t.Fatal("Enabled() = true with env=0; want false")
	}
	t.Setenv(envDisable, "off")
	if Enabled() {
		t.Fatal("Enabled() = true with env=off; want false")
	}
}

// TestOpenNonMountNoHeal verifies that for a path that is not a mount point the
// wrapper is a transparent pass-through: it returns the raw serial.Open error
// and never attempts a heal (no sudo/mount involved).
func TestOpenNonMountNoHeal(t *testing.T) {
	if !Enabled() {
		t.Fatal("expected heal enabled for this test")
	}
	// A clearly-absent path under /proc is not a mount point.
	path := "/proc/__serialheal_test__"
	if IsMountPoint(path) {
		t.Fatalf("%q should not be a mount point", path)
	}
	_, err := Open(path, nil)
	if err == nil {
		t.Fatal("Open on a non-existent non-mount path: got nil error, want open failure")
	}
	// The error must be the raw open error, not a "serialheal: ... heal failed"
	// wrapper — proving no heal was attempted.
	if strings.HasPrefix(err.Error(), "serialheal:") {
		t.Fatalf("Open attempted a heal on a non-mount path: %v", err)
	}
}

// TestOpenDisabledNoHeal verifies that with the heal disabled the wrapper still
// returns the raw open error for a non-mount path.
func TestOpenDisabledNoHeal(t *testing.T) {
	t.Setenv(envDisable, "0")
	path := "/proc/__serialheal_disabled__"
	_, err := Open(path, nil)
	if err == nil {
		t.Fatal("Open on a non-existent path: got nil error, want open failure")
	}
}
