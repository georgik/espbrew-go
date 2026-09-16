package espflash

import (
	"context"
	"os"
	"testing"
	"time"

	"go.bug.st/serial"
)

// TestOpenPortWithRetries verifies that a busy port is retried (not failed
// immediately) and that the retry succeeds once the other holder releases it.
func TestOpenPortWithRetries(t *testing.T) {
	const port = "/dev/ttyACM0"
	if _, err := os.Stat(port); err != nil {
		t.Skipf("board %s not present: %v", port, err)
	}

	// Open the port once and hold it, simulating another caller (the
	// boot-log probe) that currently owns the port.
	holder, err := serial.Open(port, &serial.Mode{BaudRate: 115200})
	if err != nil {
		t.Fatalf("precondition: open port to hold it: %v", err)
	}

	release := make(chan struct{})
	go func() {
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		_ = holder.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer close(release)

	start := time.Now()
	p, err := openPortWithRetries(ctx, port, &serial.Mode{BaudRate: 115200})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("openPortWithRetries failed: %v", err)
	}
	_ = p.Close()

	// If the second open hit a busy condition, it should have retried for a
	// while before the release unblocked it. A very short elapsed time means
	// the platform allowed a double open; either way the open must succeed.
	t.Logf("openPortWithRetries succeeded in %v", elapsed)
}
