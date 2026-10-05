//go:build linux
// +build linux

package powercontrol

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readArgv(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "argv.txt"))
	if err != nil {
		return ""
	}
	return string(b)
}

func TestUhub_FindHubByLocation(t *testing.T) {
	u := NewUhubController()
	hub, err := u.FindHubByLocation("3-10")
	if err != nil {
		t.Fatalf("FindHubByLocation(3-10) error: %v", err)
	}
	if hub.Location != "3-10" {
		t.Errorf("location = %q, want 3-10", hub.Location)
	}
	// An empty location must be rejected.
	if _, err := u.FindHubByLocation(""); err != ErrHubNotFound {
		t.Errorf("FindHubByLocation(\"\") error = %v, want ErrHubNotFound", err)
	}
}

func TestUhub_SetPortPower_On(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "uhubctl")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s' \"$*\" > "+filepath.Join(dir, "argv.txt")+"\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	u := NewUhubController()
	hub, err := u.FindHubByLocation("3-10")
	if err != nil {
		t.Fatalf("FindHubByLocation: %v", err)
	}
	if err := u.SetPortPowerDual(hub, 1, true); err != nil {
		t.Fatalf("SetPortPowerDual(on): %v", err)
	}
	argv := readArgv(t, dir)
	for _, want := range []string{"-l", "3-10", "-p", "1", "-a", "on"} {
		if !strings.Contains(argv, want) {
			t.Errorf("uhubctl argv missing %q (got: %q)", want, argv)
		}
	}
}

func TestUhub_SetPortPower_Off(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "uhubctl")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s' \"$*\" > "+filepath.Join(dir, "argv.txt")+"\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	u := NewUhubController()
	hub, _ := u.FindHubByLocation("3-10")
	if err := u.SetPortPowerDual(hub, 1, false); err != nil {
		t.Fatalf("SetPortPowerDual(off): %v", err)
	}
	argv := readArgv(t, dir)
	for _, want := range []string{"-l", "3-10", "-p", "1", "-a", "off"} {
		if !strings.Contains(argv, want) {
			t.Errorf("uhubctl argv missing %q (got: %q)", want, argv)
		}
	}
}

// TestUhub_BareCrashTreatedAsSuccess mirrors the real-world quirk: uhubctl
// segfaults (exit 139) AFTER issuing the transfer. A non-zero exit with no
// "Failed"/"Permission" text must be treated as success.
func TestUhub_BareCrashTreatedAsSuccess(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "uhubctl")
	// Exit 139 (SIGSEGV) but print nothing.
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s' \"$*\" > "+filepath.Join(dir, "argv.txt")+"\nexit 139\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	u := NewUhubController()
	hub, _ := u.FindHubByLocation("3-10")
	if err := u.SetPortPowerDual(hub, 1, false); err != nil {
		t.Errorf("bare crash should be treated as success, got: %v", err)
	}
}

// TestUhub_RejectedRequestSurfacedError: uhubctl prints "Failed" -> real error.
func TestUhub_RejectedRequestSurfacedError(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "uhubctl")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s' \"$*\" > "+filepath.Join(dir, "argv.txt")+"\necho \"Failed to set port status\" >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	u := NewUhubController()
	hub, _ := u.FindHubByLocation("3-10")
	if err := u.SetPortPowerDual(hub, 1, false); err == nil {
		t.Errorf("rejected request (Failed) should return an error")
	}
}

func TestUhub_PortBelowOne(t *testing.T) {
	u := NewUhubController()
	hub, _ := u.FindHubByLocation("3-10")
	if err := u.SetPortPowerDual(hub, 0, true); err != ErrPortNotFound {
		t.Errorf("SetPortPowerDual(port=0) error = %v, want ErrPortNotFound", err)
	}
}

func TestUhub_Enabled(t *testing.T) {
	t.Setenv("ESPBREW_POWER_UHUBCTL", "")
	if Enabled() {
		t.Error("Enabled() = true with empty env, want false")
	}
	t.Setenv("ESPBREW_POWER_UHUBCTL", "1")
	if !Enabled() {
		t.Error("Enabled() = false with env=1, want true")
	}
}
