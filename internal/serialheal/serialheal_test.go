package serialheal

import (
	"strings"
	"testing"
)

func TestIsMountPoint(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/proc", true},
		{"/sys", true},
		{"/etc/hosts", true},
		{"/nonexistent-device-node-xyz", false},
		{"/dev/ttyACM0-does-not-exist", false},
	}
	for _, tc := range cases {
		if got := IsMountPoint(tc.path); got != tc.want {
			t.Errorf("IsMountPoint(%q) = %v, want %v", tc.path, got, tc.want)
		}
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
