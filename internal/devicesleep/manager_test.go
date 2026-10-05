package devicesleep

import (
	"testing"
	"time"
)

// newTestManager returns a Manager wired to a mock hub controller with one hub
// at "1-2" offering 10 ports, and fast tunables so tests never sleep for real.
func newTestManager(t *testing.T) (*Manager, *MockController) {
	t.Helper()
	ctrl := NewMockController().AddHub("1-2", 10)
	cfg := DefaultConfig()
	cfg.WakeDelay = time.Millisecond
	cfg.IdleTimeout = 2 * time.Minute
	cfg.SweepPeriod = time.Millisecond
	m := New(ctrl, cfg)
	t.Cleanup(func() { m.Stop() })
	return m, ctrl
}

func reg(m *Manager, path, loc string, port int) { m.Register(path, loc, port) }

func TestWakePowersPortOn(t *testing.T) {
	m, ctrl := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)

	if err := m.Wake("/dev/ttyACM0"); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	if m.IsSleeping("/dev/ttyACM0") {
		t.Fatal("device should be awake after Wake")
	}
	if !ctrl.IsPoweredOn("1-2", 3) {
		t.Fatal("port 3 should be powered on after Wake")
	}
}

func TestWakeIdempotentEndState(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)

	// Waking repeatedly must leave the board powered on, not error.
	for i := 0; i < 3; i++ {
		if err := m.Wake("/dev/ttyACM0"); err != nil {
			t.Fatalf("Wake #%d: %v", i, err)
		}
	}
	if m.IsSleeping("/dev/ttyACM0") {
		t.Fatal("device should stay awake after repeated Wake")
	}
}

func TestSleepPowersPortOff(t *testing.T) {
	m, ctrl := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	if err := m.Wake("/dev/ttyACM0"); err != nil {
		t.Fatalf("Wake: %v", err)
	}

	if err := m.Sleep("/dev/ttyACM0"); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if !m.IsSleeping("/dev/ttyACM0") {
		t.Fatal("device should be sleeping after Sleep")
	}
	if ctrl.IsPoweredOn("1-2", 3) {
		t.Fatal("port 3 should be powered off after Sleep")
	}
}

func TestSleepIsIdempotent(t *testing.T) {
	m, ctrl := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Wake("/dev/ttyACM0")  // 1 call (power on)
	m.Sleep("/dev/ttyACM0") // 1 call (power off)
	before := ctrl.PowerCallCount()
	if err := m.Sleep("/dev/ttyACM0"); err != nil {
		t.Fatalf("Sleep again: %v", err)
	}
	after := ctrl.PowerCallCount()
	// The second Sleep is a no-op (guarded by the sleeping flag).
	if before != after {
		t.Fatalf("second Sleep should be a no-op (before=%d after=%d)", before, after)
	}
	if !m.IsSleeping("/dev/ttyACM0") {
		t.Fatal("device should be sleeping")
	}
}

func TestWakeUnknownDevice(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.Wake("/nope"); err != ErrUnknownDevice {
		t.Fatalf("expected ErrUnknownDevice, got %v", err)
	}
}

func TestSleepUnknownDevice(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.Sleep("/nope"); err != ErrUnknownDevice {
		t.Fatalf("expected ErrUnknownDevice, got %v", err)
	}
}

func TestWakeMissingHubErrors(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "9-9", 1)       // hub "9-9" was never registered
	m.devs["/dev/ttyACM0"].sleeping = true // simulate a powered-off board

	if err := m.Wake("/dev/ttyACM0"); err == nil {
		t.Fatal("expected error waking a board whose hub is absent")
	}
	if !m.IsSleeping("/dev/ttyACM0") {
		t.Fatal("device must stay sleeping when power-on fails")
	}
}

func TestSweepPowersDownIdleDevice(t *testing.T) {
	m, ctrl := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Wake("/dev/ttyACM0")
	// Simulate the board being used a while ago.
	m.devs["/dev/ttyACM0"].lastUsed = time.Now().Add(-3 * time.Minute)

	down := m.Sweep(time.Now())
	if len(down) != 1 || down[0] != "/dev/ttyACM0" {
		t.Fatalf("expected /dev/ttyACM0 powered down, got %v", down)
	}
	if ctrl.IsPoweredOn("1-2", 3) {
		t.Fatal("idle board should be powered off by Sweep")
	}
}

func TestSweepSkipsBusyDevice(t *testing.T) {
	m, ctrl := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Wake("/dev/ttyACM0")
	m.SetBusy("/dev/ttyACM0", true)
	m.devs["/dev/ttyACM0"].lastUsed = time.Now().Add(-3 * time.Minute)

	if down := m.Sweep(time.Now()); len(down) != 0 {
		t.Fatalf("busy device must not be swept, got %v", down)
	}
	if !ctrl.IsPoweredOn("1-2", 3) {
		t.Fatal("busy board must stay powered on")
	}
}

func TestSweepSkipsRecentlyUsed(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Wake("/dev/ttyACM0")
	// lastUsed well within the idle timeout.
	m.devs["/dev/ttyACM0"].lastUsed = time.Now().Add(-1 * time.Second)

	if down := m.Sweep(time.Now()); len(down) != 0 {
		t.Fatalf("recently used board must not be swept, got %v", down)
	}
}

func TestSweepSkipsNeverUsed(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	// Never touched -> lastUsed zero -> must be left alone.
	if down := m.Sweep(time.Now()); len(down) != 0 {
		t.Fatalf("never-used board must not be swept, got %v", down)
	}
}

func TestSweepSkipsAlreadySleeping(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Sleep("/dev/ttyACM0")
	m.devs["/dev/ttyACM0"].lastUsed = time.Now().Add(-3 * time.Minute)

	if down := m.Sweep(time.Now()); len(down) != 0 {
		t.Fatalf("already-sleeping board must not be swept again, got %v", down)
	}
}

func TestSweepReturnsOnlyPoweredDown(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 1)
	reg(m, "/dev/ttyACM1", "1-2", 2)
	m.Wake("/dev/ttyACM0")
	m.Wake("/dev/ttyACM1")
	m.Touch("/dev/ttyACM1") // ttyACM1 is actively used; only ttyACM0 is idle

	m.devs["/dev/ttyACM0"].lastUsed = time.Now().Add(-3 * time.Minute)

	down := m.Sweep(time.Now())
	if len(down) != 1 || down[0] != "/dev/ttyACM0" {
		t.Fatalf("expected only ttyACM0 swept, got %v", down)
	}
}

func TestTouchResetsIdleTimer(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Wake("/dev/ttyACM0")

	m.devs["/dev/ttyACM0"].lastUsed = time.Now().Add(-3 * time.Minute)
	m.Touch("/dev/ttyACM0")

	// Now recent again -> not swept.
	if down := m.Sweep(time.Now()); len(down) != 0 {
		t.Fatalf("touched board must not be swept, got %v", down)
	}
}

func TestWakeClearsSleeping(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Sleep("/dev/ttyACM0")
	if !m.IsSleeping("/dev/ttyACM0") {
		t.Fatal("expected sleeping")
	}
	// Touch after sleep should clear the sleeping state.
	m.Touch("/dev/ttyACM0")
	if m.IsSleeping("/dev/ttyACM0") {
		t.Fatal("Touch should clear sleeping state")
	}
}

func TestRegisterUpdatesLocation(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	reg(m, "/dev/ttyACM0", "1-4", 7)

	if got := m.devs["/dev/ttyACM0"].location; got != "1-4" {
		t.Fatalf("location not updated: %q", got)
	}
	if got := m.devs["/dev/ttyACM0"].port; got != 7 {
		t.Fatalf("port not updated: %d", got)
	}
}

func TestUnregister(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Unregister("/dev/ttyACM0")
	if m.IsSleeping("/dev/ttyACM0") {
		t.Fatal("unregistered device should report not-sleeping")
	}
	if err := m.Wake("/dev/ttyACM0"); err != ErrUnknownDevice {
		t.Fatalf("expected ErrUnknownDevice after unregister, got %v", err)
	}
}

func TestSnapshot(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Wake("/dev/ttyACM0")

	snap := m.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 snapshot entry, got %d", len(snap))
	}
	if snap[0].Path != "/dev/ttyACM0" || snap[0].Location != "1-2" || snap[0].Port != 3 {
		t.Fatalf("unexpected snapshot: %+v", snap[0])
	}
	if snap[0].Sleeping {
		t.Fatal("snapshot should report awake")
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.WakeDelay != time.Second {
		t.Fatalf("default WakeDelay = %v, want 1s", cfg.WakeDelay)
	}
	if cfg.IdleTimeout != 2*time.Minute {
		t.Fatalf("default IdleTimeout = %v, want 2m", cfg.IdleTimeout)
	}
	if cfg.SweepPeriod != 5*time.Second {
		t.Fatalf("default SweepPeriod = %v, want 5s", cfg.SweepPeriod)
	}
}

func TestStartStopSweeperRuns(t *testing.T) {
	m, ctrl := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 3)
	m.Wake("/dev/ttyACM0")
	m.devs["/dev/ttyACM0"].lastUsed = time.Now().Add(-3 * time.Minute)

	m.Start()
	// SweepPeriod is 1ms; wait for the sweeper to power the board off.
	deadline := time.Now().Add(2 * time.Second)
	for ctrl.IsPoweredOn("1-2", 3) && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	m.Stop()

	if ctrl.IsPoweredOn("1-2", 3) {
		t.Fatal("background sweeper should have powered the idle board off")
	}
}

func TestWakeThenSleepCycle(t *testing.T) {
	m, ctrl := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 5)

	// Sleep -> Wake -> Sleep, verifying the port tracks each transition.
	for _, wantOff := range []bool{true, false, true} {
		if wantOff {
			if err := m.Sleep("/dev/ttyACM0"); err != nil {
				t.Fatalf("Sleep: %v", err)
			}
		} else {
			if err := m.Wake("/dev/ttyACM0"); err != nil {
				t.Fatalf("Wake: %v", err)
			}
		}
		if got := ctrl.IsPoweredOn("1-2", 5); got != !wantOff {
			t.Fatalf("after wantOff=%v port powered=%v", wantOff, got)
		}
	}
}

func TestSetBusyTracking(t *testing.T) {
	m, _ := newTestManager(t)
	reg(m, "/dev/ttyACM0", "1-2", 1)
	if m.IsBusy("/dev/ttyACM0") {
		t.Fatal("should not be busy initially")
	}
	m.SetBusy("/dev/ttyACM0", true)
	if !m.IsBusy("/dev/ttyACM0") {
		t.Fatal("should be busy after SetBusy(true)")
	}
	m.SetBusy("/dev/ttyACM0", false)
	if m.IsBusy("/dev/ttyACM0") {
		t.Fatal("should not be busy after SetBusy(false)")
	}
}
