//go:build linux

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

// TestIsMountPoint verifies IsMountPoint against known mount points. It is
// Linux-only: IsMountPoint reads /proc/mounts (a Linux procfs file), and the
// paths below ("/", "/proc", "/sys") are mount points only on Linux. On macOS
// and Windows the wrapper is a no-op, so these assertions do not apply.
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
