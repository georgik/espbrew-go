package cluster

import (
	"testing"
	"time"
)

func TestDeviceLock_ReserveAvailable(t *testing.T) {
	lock := &DeviceLock{state: DeviceAvailable}

	if !lock.Reserve("job1") {
		t.Fatal("Failed to reserve available device")
	}

	if lock.State() != DeviceReserved {
		t.Errorf("Expected Reserved, got %s", lock.State())
	}

	if lock.Owner() != "job1" {
		t.Errorf("Expected owner job1, got %s", lock.Owner())
	}
}

func TestDeviceLock_ReserveBusy(t *testing.T) {
	lock := &DeviceLock{state: DeviceBusy, owner: "job1"}

	if lock.Reserve("job2") {
		t.Fatal("Should not reserve busy device")
	}
}

func TestDeviceLock_Release(t *testing.T) {
	lock := &DeviceLock{state: DeviceReserved, owner: "job1"}

	if !lock.Release("job1") {
		t.Fatal("Owner failed to release")
	}

	if lock.State() != DeviceAvailable {
		t.Errorf("Expected Available, got %s", lock.State())
	}
}

func TestDeviceLock_ReleaseWrongOwner(t *testing.T) {
	lock := &DeviceLock{state: DeviceReserved, owner: "job1"}

	if lock.Release("job2") {
		t.Fatal("Non-owner should not release")
	}
}

func TestDeviceLock_Acquire(t *testing.T) {
	lock := &DeviceLock{state: DeviceReserved, owner: "job1"}

	if !lock.Acquire("job1") {
		t.Fatal("Owner failed to acquire")
	}

	if lock.State() != DeviceBusy {
		t.Errorf("Expected Busy, got %s", lock.State())
	}
}

func TestDeviceRegistry_Register(t *testing.T) {
	reg := NewDeviceRegistry()
	reg.Register("/dev/ttyUSB0")

	states := reg.ListDevices()
	if len(states) != 1 {
		t.Fatalf("Expected 1 device, got %d", len(states))
	}

	if states["/dev/ttyUSB0"] != DeviceAvailable {
		t.Errorf("Expected Available, got %s", states["/dev/ttyUSB0"])
	}
}

func TestDeviceRegistry_Reserve(t *testing.T) {
	reg := NewDeviceRegistry()
	reg.Register("/dev/ttyUSB0")

	if !reg.Reserve("/dev/ttyUSB0", "job1") {
		t.Fatal("Failed to reserve device")
	}

	if !reg.Reserve("/dev/ttyUSB0", "job2") {
		// Should fail - already reserved
	}

	if reg.GetState("/dev/ttyUSB0") != DeviceReserved {
		t.Errorf("Expected Reserved, got %s", reg.GetState("/dev/ttyUSB0"))
	}
}

func TestDeviceRegistry_AvailableDevices(t *testing.T) {
	reg := NewDeviceRegistry()
	reg.Register("/dev/ttyUSB0")
	reg.Register("/dev/ttyUSB1")

	reg.Reserve("/dev/ttyUSB0", "job1")

	avail := reg.AvailableDevices()
	if len(avail) != 1 {
		t.Fatalf("Expected 1 available, got %d", len(avail))
	}

	if avail[0] != "/dev/ttyUSB1" {
		t.Errorf("Expected /dev/ttyUSB1, got %s", avail[0])
	}
}

func TestDeviceLock_ForceRelease(t *testing.T) {
	lock := &DeviceLock{state: DeviceBusy, owner: "job1"}

	prevOwner := lock.ForceRelease()
	if prevOwner != "job1" {
		t.Errorf("Expected job1, got %s", prevOwner)
	}

	if lock.State() != DeviceAvailable {
		t.Errorf("Expected Available, got %s", lock.State())
	}

	if lock.Owner() != "" {
		t.Errorf("Expected empty owner, got %s", lock.Owner())
	}
}

func TestDeviceLock_ConcurrentAccess(t *testing.T) {
	lock := &DeviceLock{state: DeviceAvailable}
	done := make(chan bool)

	// Try to reserve from multiple goroutines
	for i := 0; i < 10; i++ {
		go func(id int) {
			if lock.Reserve(string(rune('a' + id))) {
				time.Sleep(10 * time.Millisecond)
				lock.Release(string(rune('a' + id)))
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Final state should be available
	if lock.State() != DeviceAvailable {
		t.Errorf("Expected Available after concurrent ops, got %s", lock.State())
	}
}
