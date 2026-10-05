package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/georgik/espbrew-go/internal/config"
	"github.com/georgik/espbrew-go/internal/devicesleep"
	"github.com/georgik/espbrew-go/internal/persistence"
)

// newSleepTestLeader builds a leader whose sleep manager is driven by an
// in-memory mock hub. The mock controller is returned so tests can assert on
// the simulated port power state. Config timings are fast so tests never wait
// on real wall-clock delays.
func newSleepTestLeader(t *testing.T, devices ...config.DeviceConfig) (*LeaderNode, *devicesleep.MockController) {
	t.Helper()

	mock := devicesleep.NewMockController().AddHub("1-2", 4)

	cfg := &LeaderConfig{
		HeartbeatInterval:  1 * time.Second,
		NodeTimeout:        5 * time.Second,
		HTTPPort:           8090,
		DisablemDNS:        true,
		DisableWatcher:     true, // no live USB tree in tests
		DisableMaintenance: true,
		InitialMode:        "operational",
		Devices:            devices,
		SleepConfig: &devicesleep.Config{
			WakeDelay:   0, // no artificial delay in tests
			IdleTimeout: 40 * time.Millisecond,
			SweepPeriod: 10 * time.Millisecond,
		},
	}

	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/sleep.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	leader := NewLeaderNode("test-leader", cfg, store)
	leader.powerCtrl = mock

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := leader.Start(ctx); err != nil {
		t.Fatalf("failed to start leader: %v", err)
	}
	t.Cleanup(func() { leader.Stop() })

	time.Sleep(50 * time.Millisecond)
	return leader, mock
}

func TestLeaderReconcileMarksAbsentBoardSleeping(t *testing.T) {
	leader, mock := newSleepTestLeader(t, config.DeviceConfig{
		Path:        "/dev/serial/by-id/usb-powerboard-if00",
		ID:          "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType:    "ESP32-C3",
		Alias:       "power-board",
		USBLocation: "1-2",
		USBPort:     2,
	})

	// The board is not in the live USB tree, so reconcile must have created a
	// record for it and marked it sleeping.
	dev, ok := leader.State().Devices["/dev/serial/by-id/usb-powerboard-if00"]
	if !ok {
		t.Fatal("expected a record for the power-managed board")
	}
	if !dev.Sleeping {
		t.Errorf("board should be marked sleeping, got Sleeping=false")
	}
	if dev.Status != "sleeping" {
		t.Errorf("board status = %q, want %q", dev.Status, "sleeping")
	}
	if !leader.sleeper.IsSleeping(dev.Path) {
		t.Errorf("sleep manager should report the board as sleeping")
	}
	// No power switch should have happened yet: the board is simply absent.
	if mock.IsPoweredOn("1-2", 2) {
		t.Errorf("port must not be powered before the board is woken")
	}
}

func TestLeaderWakesBoardBeforeJob(t *testing.T) {
	leader, mock := newSleepTestLeader(t, config.DeviceConfig{
		Path:        "/dev/serial/by-id/usb-powerboard-if00",
		ID:          "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType:    "ESP32-C3",
		Alias:       "power-board",
		USBLocation: "1-2",
		USBPort:     3,
	})

	path := "/dev/serial/by-id/usb-powerboard-if00"

	// Before the job the board is sleeping.
	if !leader.sleeper.IsSleeping(path) {
		t.Fatal("board should start sleeping")
	}

	if _, err := leader.EnqueueJob("firmware.bin", path); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	// Waking must have switched the hub port on...
	if !mock.IsPoweredOn("1-2", 3) {
		t.Errorf("expected hub port 3 powered on after wake")
	}
	// ...and the manager must no longer consider the board sleeping.
	dev := leader.State().Devices[path]
	if dev.Sleeping {
		t.Errorf("board should be awake after wake, got Sleeping=true")
	}
	if leader.sleeper.IsSleeping(path) {
		t.Errorf("sleep manager should report the board as awake after wake")
	}
}

func TestLeaderSweepPowersDownIdleBoard(t *testing.T) {
	leader, mock := newSleepTestLeader(t, config.DeviceConfig{
		Path:        "/dev/serial/by-id/usb-powerboard-if00",
		ID:          "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType:    "ESP32-C3",
		USBLocation: "1-2",
		USBPort:     1,
	})

	path := "/dev/serial/by-id/usb-powerboard-if00"

	// Simulate a board that has just been used (woken and touched).
	if err := leader.sleeper.Wake(path); err != nil {
		t.Fatalf("wake failed: %v", err)
	}
	leader.sleeper.Touch(path)

	// Advance well past the idle timeout and let the sweeper run.
	if down := leader.sleeper.Sweep(time.Now().Add(3 * time.Minute)); len(down) != 1 {
		t.Fatalf("expected 1 board swept, got %v", down)
	}

	if mock.IsPoweredOn("1-2", 1) {
		t.Errorf("idle board port should be powered off after sweep")
	}
	if !leader.sleeper.IsSleeping(path) {
		t.Errorf("sleep manager should report the board as sleeping after sweep")
	}
}

func TestLeaderLoopMirrorsSweepToRecord(t *testing.T) {
	leader, mock := newSleepTestLeader(t, config.DeviceConfig{
		Path:        "/dev/serial/by-id/usb-powerboard-if00",
		ID:          "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType:    "ESP32-C3",
		USBLocation: "1-2",
		USBPort:     4,
	})

	path := "/dev/serial/by-id/usb-powerboard-if00"
	// Wake the board (awake + powered on), then let it go idle.
	if err := leader.sleeper.Wake(path); err != nil {
		t.Fatalf("wake failed: %v", err)
	}
	leader.sleeper.Touch(path)

	// Let the leader's own sweeper loop fire on its own (fast) interval.
	deadline := time.Now().Add(2 * time.Second)
	for mock.IsPoweredOn("1-2", 4) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if mock.IsPoweredOn("1-2", 4) {
		t.Errorf("loop should have powered the idle board off")
	}
	dev := leader.State().Devices[path]
	if !dev.Sleeping {
		t.Errorf("record should be mirrored to Sleeping=true by the loop")
	}
	if dev.Status != "sleeping" {
		t.Errorf("record status = %q, want %q", dev.Status, "sleeping")
	}
}

func TestLeaderBusyBoardNotSwept(t *testing.T) {
	leader, mock := newSleepTestLeader(t, config.DeviceConfig{
		Path:        "/dev/serial/by-id/usb-powerboard-if00",
		ID:          "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType:    "ESP32-C3",
		USBLocation: "1-2",
		USBPort:     2,
	})

	path := "/dev/serial/by-id/usb-powerboard-if00"
	// The board was woken for an operation, then marked busy.
	if err := leader.sleeper.Wake(path); err != nil {
		t.Fatalf("wake failed: %v", err)
	}
	leader.sleeper.Touch(path)
	leader.sleeper.SetBusy(path, true) // an operation is running

	// Even far past the idle timeout a busy board must stay powered on.
	if down := leader.sleeper.Sweep(time.Now().Add(5 * time.Minute)); len(down) != 0 {
		t.Fatalf("busy board must not be swept, got %v", down)
	}
	if !mock.IsPoweredOn("1-2", 2) {
		t.Errorf("busy board port must remain powered on")
	}
}

func TestLeaderNonPowerManagedBoardUnaffected(t *testing.T) {
	leader, mock := newSleepTestLeader(t, config.DeviceConfig{
		Path:     "/dev/serial/by-id/usb-nopower-if00",
		ID:       "esp-11:22:33:44:55:66-if00",
		ChipType: "ESP32-S3",
		// No USBLocation => not power-managed.
	})

	path := "/dev/serial/by-id/usb-nopower-if00"

	// A non power-managed board that is absent from the live tree is simply
	// unconfigured: reconcile must not create a sleeping record for it.
	if _, ok := leader.State().Devices[path]; ok {
		t.Errorf("non power-managed absent board should not get a record")
	}

	// Touching/sweeping it must be a no-op: the sleep manager never tracked it.
	leader.sleeper.Touch(path)
	if down := leader.sleeper.Sweep(time.Now().Add(5 * time.Minute)); len(down) != 0 {
		t.Fatalf("non power-managed board must not be swept, got %v", down)
	}
	if mock.PowerCallCount() != 0 {
		t.Errorf("no power switches expected for a non power-managed board")
	}
}

func TestLeaderPowerDeviceWakesSleepingBoard(t *testing.T) {
	path := "/dev/serial/by-id/usb-powerboard-if00"
	leader, mock := newSleepTestLeader(t, config.DeviceConfig{
		Path:        path,
		ID:          "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType:    "ESP32-C3",
		Alias:       "power-board",
		USBLocation: "1-2",
		USBPort:     2,
	})

	// The board starts sleeping (reconcile created its record).
	if !leader.sleeper.IsSleeping(path) {
		t.Fatal("board should start sleeping")
	}

	if err := leader.PowerDevice(path, true); err != nil {
		t.Fatalf("PowerDevice(on) failed: %v", err)
	}

	// The hub port must now be powered on...
	if !mock.IsPoweredOn("1-2", 2) {
		t.Errorf("expected hub port 2 powered on after PowerDevice(on)")
	}
	// ...and the live record must report the board as awake.
	dev := leader.State().Devices[path]
	if dev.Sleeping {
		t.Errorf("board should be awake after PowerDevice(on), got Sleeping=true")
	}
	if dev.Status != "available" {
		t.Errorf("board status = %q, want available", dev.Status)
	}
}

func TestLeaderPowerDevicePowersOffAwakeBoard(t *testing.T) {
	path := "/dev/serial/by-id/usb-powerboard-if00"
	leader, mock := newSleepTestLeader(t, config.DeviceConfig{
		Path:        path,
		ID:          "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType:    "ESP32-C3",
		Alias:       "power-board",
		USBLocation: "1-2",
		USBPort:     4,
	})

	// Bring the board up first so we can then power it down.
	if err := leader.sleeper.Wake(path); err != nil {
		t.Fatalf("wake failed: %v", err)
	}
	if !mock.IsPoweredOn("1-2", 4) {
		t.Fatal("board should be powered on before test")
	}

	if err := leader.PowerDevice(path, false); err != nil {
		t.Fatalf("PowerDevice(off) failed: %v", err)
	}

	if mock.IsPoweredOn("1-2", 4) {
		t.Errorf("expected hub port 4 powered off after PowerDevice(off)")
	}
	dev := leader.State().Devices[path]
	if !dev.Sleeping {
		t.Errorf("board should be sleeping after PowerDevice(off), got Sleeping=false")
	}
	if dev.Status != "sleeping" {
		t.Errorf("board status = %q, want sleeping", dev.Status)
	}
}

func TestLeaderPowerDeviceRejectedForNonPowerManaged(t *testing.T) {
	path := "/dev/serial/by-id/usb-plainboard-if00"
	leader, _ := newSleepTestLeader(t, config.DeviceConfig{
		Path:     path,
		ID:       "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType: "ESP32-C3",
		Alias:    "plain-board",
	})

	if err := leader.PowerDevice(path, true); err == nil {
		t.Errorf("expected PowerDevice to reject a non-power-managed board")
	}
}

func TestLeaderPowerDeviceRefusedWhileBusy(t *testing.T) {
	path := "/dev/serial/by-id/usb-powerboard-if00"
	leader, mock := newSleepTestLeader(t, config.DeviceConfig{
		Path:        path,
		ID:          "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType:    "ESP32-C3",
		Alias:       "power-board",
		USBLocation: "1-2",
		USBPort:     1,
	})

	if err := leader.sleeper.Wake(path); err != nil {
		t.Fatalf("wake failed: %v", err)
	}
	leader.sleeper.SetBusy(path, true)
	t.Cleanup(func() { leader.sleeper.SetBusy(path, false) })

	if err := leader.PowerDevice(path, false); err == nil {
		t.Errorf("expected PowerDevice(off) to refuse while the board is busy")
	}
	// The refused request must not have toggled the port.
	if !mock.IsPoweredOn("1-2", 1) {
		t.Errorf("a busy board's port must stay powered on")
	}
}
