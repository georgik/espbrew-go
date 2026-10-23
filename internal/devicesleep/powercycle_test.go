package devicesleep

import (
	"testing"
	"time"
)

// newCycleManager returns a Manager with a fast CycleDelay so PowerCycle tests
// never wait on the real 500ms power-cycle gap.
func newCycleManager(t *testing.T) (*Manager, *MockController) {
	t.Helper()
	ctrl := NewMockController().AddHub("1-6", 4)
	cfg := DefaultConfig()
	cfg.WakeDelay = time.Millisecond
	cfg.SweepPeriod = time.Millisecond
	cfg.CycleDelay = time.Millisecond
	m := New(ctrl, cfg)
	t.Cleanup(func() { m.Stop() })
	return m, ctrl
}

// TestPowerCyclePowersOffThenOn verifies a full power cycle drives the hub port
// off, then on, and leaves the board awake — the "reliable cold boot" a flash
// relies on before the DTR/RTS reset.
func TestPowerCyclePowersOffThenOn(t *testing.T) {
	m, ctrl := newCycleManager(t)
	reg(m, "/dev/ttyACM5", "1-6", 1)

	if err := m.PowerCycle("/dev/ttyACM5"); err != nil {
		t.Fatalf("PowerCycle: %v", err)
	}

	// The cycle must be a genuine off -> on: two calls, first off then on.
	// PowerCall.On records the requested power state, so the first call is
	// On:false (off) and the second is On:true (on).
	calls := ctrl.PowerCalls
	t.Logf("power calls: %+v", calls)
	if len(calls) != 2 {
		t.Fatalf("expected 2 power calls for a cycle, got %d: %+v", len(calls), calls)
	}
	if calls[0].On {
		t.Errorf("first PowerCycle call should power the port OFF, got ON")
	}
	if !calls[1].On {
		t.Errorf("second PowerCycle call should power the port ON, got OFF")
	}
	if !ctrl.IsPoweredOn("1-6", 1) {
		t.Error("port should be powered on after a PowerCycle")
	}
	if m.IsSleeping("/dev/ttyACM5") {
		t.Error("device should be awake after a PowerCycle")
	}
}

// TestPowerCycleAlwaysCyclesEvenWhenAwake confirms PowerCycle is not a no-op
// for a board that is already awake — unlike Wake, it always reboots.
func TestPowerCycleAlwaysCyclesEvenWhenAwake(t *testing.T) {
	m, ctrl := newCycleManager(t)
	reg(m, "/dev/ttyACM5", "1-6", 1)

	// Board is awake and powered on.
	if err := m.Wake("/dev/ttyACM5"); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	ctrl.PowerCalls = nil // reset the log

	if err := m.PowerCycle("/dev/ttyACM5"); err != nil {
		t.Fatalf("PowerCycle: %v", err)
	}
	if len(ctrl.PowerCalls) != 2 {
		t.Fatalf("PowerCycle must cycle even when awake, got %d calls", len(ctrl.PowerCalls))
	}
}

// TestPowerCycleUnknownDevice confirms an unregistered board is rejected.
func TestPowerCycleUnknownDevice(t *testing.T) {
	m, _ := newCycleManager(t)
	if err := m.PowerCycle("/dev/ttyACM9"); err != ErrUnknownDevice {
		t.Fatalf("expected ErrUnknownDevice, got %v", err)
	}
}

// TestPowerCycleBusyIsRejected confirms a board mid-operation cannot be
// power-cycled (that would corrupt a flash).
func TestPowerCycleBusyIsRejected(t *testing.T) {
	m, _ := newCycleManager(t)
	reg(m, "/dev/ttyACM5", "1-6", 1)
	m.SetBusy("/dev/ttyACM5", true)

	if err := m.PowerCycle("/dev/ttyACM5"); err != ErrBusy {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
}
