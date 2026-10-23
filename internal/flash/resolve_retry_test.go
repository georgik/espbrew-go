package flash

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestResolveByIDLinkPresent verifies a link that already resolves returns
// immediately with the joined kernel-node path (relative symlink target).
func TestResolveByIDLinkPresent(t *testing.T) {
	dir := t.TempDir()
	mustSymlink(t, dir, "usb-1a86_USB_Single_Serial_5971084553-if00", "ttyACM5")

	start := time.Now()
	got, err := resolveByIDLink(context.Background(), dir, "usb-1a86_USB_Single_Serial_5971084553-if00", maxResolveTimeout)
	if err != nil {
		t.Fatalf("resolveByIDLink failed: %v", err)
	}
	want := filepath.Join(dir, "ttyACM5")
	if got != want {
		t.Errorf("resolveByIDLink = %q, want %q", got, want)
	}
	// A present link must resolve essentially instantly (no retry back-off).
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("resolved in %v, expected near-instant for an already-present link", elapsed)
	}
}

// TestResolveByIDLinkWaitsForReenumeration is the regression test for the flash
// failure: the by-id link is absent when the call starts (USB still
// re-enumerating after a power cycle) and appears a few hundred milliseconds
// later. The resolver must wait out the window and succeed, not fail on the
// first ENOENT.
func TestResolveByIDLinkWaitsForReenumeration(t *testing.T) {
	dir := t.TempDir()
	name := "usb-1a86_USB_Single_Serial_5971084553-if00"

	// The link appears after ~350ms (past one 250ms retry gap), simulating the
	// device finishing its USB enumeration mid-call.
	go func() {
		time.Sleep(350 * time.Millisecond)
		if err := os.Symlink("ttyACM7", filepath.Join(dir, name)); err != nil {
			t.Errorf("late symlink: %v", err)
		}
	}()

	start := time.Now()
	got, err := resolveByIDLink(context.Background(), dir, name, maxResolveTimeout)
	if err != nil {
		t.Fatalf("resolveByIDLink failed: %v", err)
	}
	want := filepath.Join(dir, "ttyACM7")
	if got != want {
		t.Errorf("resolveByIDLink = %q, want %q", got, want)
	}
	// It must have slept at least one back-off gap waiting for the link.
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Errorf("resolved in %v, expected the resolver to retry while the link was absent", elapsed)
	}
}

// TestResolveByIDLinkTimeout verifies the call fails (does not hang) once the
// timeout elapses while the link is still absent.
func TestResolveByIDLinkTimeout(t *testing.T) {
	dir := t.TempDir() // link never created

	start := time.Now()
	_, err := resolveByIDLink(context.Background(), dir, "usb-1a86_USB_Single_Serial_xxx-if00", 700*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error for a link that never appears")
	}
	if elapsed := time.Since(start); elapsed < 600*time.Millisecond {
		t.Errorf("returned in %v, expected to wait ~the 700ms timeout", elapsed)
	}
}

// TestResolveByIDLinkContextCancel verifies a cancelled context stops the retry
// loop promptly rather than waiting out the full timeout.
func TestResolveByIDLinkContextCancel(t *testing.T) {
	dir := t.TempDir() // link never created

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := resolveByIDLink(ctx, dir, "usb-1a86_USB_Single_Serial_xxx-if00", maxResolveTimeout)
	if err == nil {
		t.Fatal("expected a context-cancel error for a link that never appears")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v after cancel, expected a prompt return", elapsed)
	}
}

// TestResolveDevicePathWithRetryDirectPath verifies a non-by-id path is returned
// unchanged, with no attempt to read a symlink.
func TestResolveDevicePathWithRetryDirectPath(t *testing.T) {
	got, err := resolveDevicePathWithRetry(context.Background(), "/dev/ttyACM3", maxResolveTimeout)
	if err != nil {
		t.Fatalf("resolveDevicePathWithRetry failed: %v", err)
	}
	if got != "/dev/ttyACM3" {
		t.Errorf("resolveDevicePathWithRetry(%q) = %q", "/dev/ttyACM3", got)
	}
}
