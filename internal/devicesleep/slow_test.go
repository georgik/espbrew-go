//go:build slow

// This file holds tests that use real wall-clock timing. They are excluded from
// the normal build so CI stays fast; run them explicitly with:
//
//	go test -tags slow ./internal/devicesleep/
package devicesleep

import (
	"testing"
	"time"
)

// TestSweepRealIdleTimeout verifies the idle timeout works with real time, not
// just injected clock values. It proves a board stays powered on inside the
// timeout window and is powered down once the real timeout elapses.
func TestSweepRealIdleTimeout(t *testing.T) {
	ctrl := NewMockController().AddHub("1-2", 10)
	cfg := DefaultConfig()
	cfg.IdleTimeout = 200 * time.Millisecond
	cfg.SweepPeriod = 20 * time.Millisecond
	cfg.WakeDelay = time.Millisecond
	m := New(ctrl, cfg)
	m.Start()
	defer m.Stop()

	const path = "/dev/ttyACM0"
	m.Register(path, "1-2", 4)
	m.Wake(path)
	m.Touch(path) // operation starts -> idle timer begins

	// Inside the timeout window the board must stay powered on.
	time.Sleep(100 * time.Millisecond)
	if !ctrl.IsPoweredOn("1-2", 4) {
		t.Fatal("board powered off too early, before idle timeout")
	}

	// After the timeout the sweeper powers it down.
	deadline := time.Now().Add(2 * time.Second)
	for ctrl.IsPoweredOn("1-2", 4) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctrl.IsPoweredOn("1-2", 4) {
		t.Fatal("board was not powered down after the idle timeout")
	}
}
