package cluster

import (
	"context"
	"sync"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/device"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

// TestDevicePersistenceOverRestart verifies that when a device is discovered
// after restart, it merges with persisted data instead of creating a duplicate.
func TestDevicePersistenceOverRestart(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	devicePath := "/dev/cu.usbmodem1401"

	// Phase 1: Simulate previous session - device was manually identified
	ctx1, cancel1 := context.WithCancel(context.Background())
	leader1 := NewLeaderNode("test-leader", &LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
	}, store)

	if err := leader1.Start(ctx1); err != nil {
		t.Fatal(err)
	}

	record := &persistence.DeviceRecord{
		DeviceID:   "manual-ESP32-S2-5",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S2",
		ChipRev:    "1.0",
		FlashSize:  4 * 1024 * 1024,
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
		LastPath:   devicePath,
		NodeID:     "test-leader",
	}
	if err := store.SaveDevice(record); err != nil {
		t.Fatal(err)
	}

	cancel1()
	leader1.Stop()

	// Phase 2: Restart - loadPersistedDevices should restore device
	ctx2, cancel2 := context.WithCancel(context.Background())
	leader2 := NewLeaderNode("test-leader", &LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
	}, store)

	if err := leader2.Start(ctx2); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel2()
		leader2.Stop()
	}()

	state := leader2.State()
	dev, exists := state.Devices[devicePath]
	if !exists {
		t.Fatal("Device not loaded from persistence")
	}

	if dev.DeviceID != "manual-ESP32-S2-5" {
		t.Errorf("Expected DeviceID manual-ESP32-S2-5, got %s", dev.DeviceID)
	}
	if dev.ChipType != "ESP32-S2" {
		t.Errorf("Expected ChipType ESP32-S2, got %s", dev.ChipType)
	}

	// Phase 3: Device hotplug event - should MERGE with existing, not replace
	event := device.DeviceEvent{
		Type: device.DeviceAdded,
		Path: devicePath,
		VID:  0x1234,
		PID:  0x5678,
	}
	leader2.handleDeviceEvent(event)

	// Verify device still has persisted identity
	state = leader2.State()
	dev = state.Devices[devicePath]

	if dev.DeviceID != "manual-ESP32-S2-5" {
		t.Errorf("After hotplug, DeviceID should be preserved. Expected manual-ESP32-S2-5, got %s", dev.DeviceID)
	}
	if dev.ChipType != "ESP32-S2" {
		t.Errorf("After hotplug, ChipType should be preserved. Expected ESP32-S2, got %s", dev.ChipType)
	}
	// VID/PID should be updated from hotplug event
	if dev.VID != 0x1234 {
		t.Errorf("Expected VID 0x1234, got 0x%x", dev.VID)
	}
	if dev.PID != 0x5678 {
		t.Errorf("Expected PID 0x5678, got 0x%x", dev.PID)
	}
}

// TestManualUpdateDeviceInfo verifies manual device info updates persist correctly.
func TestManualUpdateDeviceInfo(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	leader := NewLeaderNode("test-leader", &LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
	}, store)

	if err := leader.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		leader.Stop()
	}()

	devicePath := "/dev/cu.usbmodem1401"

	// Register device via hotplug
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:   devicePath,
		VID:    0x1234,
		PID:    0x5678,
		Status: "available",
	})

	// Manually update device info (via API call simulation)
	leader.UpdateDeviceInfo(devicePath, "manual-ESP32-S2-5", "ESP32-S2", "aa:bb:cc:dd:ee:ff")

	// Verify update reflected in state
	state := leader.State()
	dev := state.Devices[devicePath]

	if dev.DeviceID != "manual-ESP32-S2-5" {
		t.Errorf("Expected DeviceID manual-ESP32-S2-5, got %s", dev.DeviceID)
	}
	if dev.ChipType != "ESP32-S2" {
		t.Errorf("Expected ChipType ESP32-S2, got %s", dev.ChipType)
	}

	// Verify persisted to store
	record, err := store.GetDevice("manual-ESP32-S2-5")
	if err != nil {
		t.Fatalf("Failed to get device from store: %v", err)
	}

	if record.ChipType != "ESP32-S2" {
		t.Errorf("Persisted ChipType wrong. Expected ESP32-S2, got %s", record.ChipType)
	}
	if record.LastPath != devicePath {
		t.Errorf("Persisted LastPath wrong. Expected %s, got %s", devicePath, record.LastPath)
	}
}

// TestUpdateDeviceInfoPreservesFirstSeen verifies that updating device info
// doesn't overwrite FirstSeen timestamp.
func TestUpdateDeviceInfoPreservesFirstSeen(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	leader := NewLeaderNode("test-leader", &LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
	}, store)

	if err := leader.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		leader.Stop()
	}()

	devicePath := "/dev/cu.usbmodem1401"

	// Create initial device record
	firstSeen := time.Now().Add(-1 * time.Hour)
	record := &persistence.DeviceRecord{
		DeviceID:   "esp32-test-1",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32",
		ChipRev:    "1.0",
		FlashSize:  4 * 1024 * 1024,
		FirstSeen:  firstSeen,
		LastSeen:   firstSeen,
		LastPath:   devicePath,
		NodeID:     "test-leader",
	}
	if err := store.SaveDevice(record); err != nil {
		t.Fatal(err)
	}

	// Load the device
	leader.loadPersistedDevices()

	// Update device info (simulate user editing chip type)
	leader.UpdateDeviceInfo(devicePath, "esp32-test-1", "ESP32-S2", "aa:bb:cc:dd:ee:ff")

	// Verify FirstSeen was preserved
	updated, err := store.GetDevice("esp32-test-1")
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}

	if updated.FirstSeen.Equal(firstSeen) || updated.FirstSeen.After(firstSeen) {
		// FirstSeen should be the original value
		if !updated.FirstSeen.Equal(firstSeen) {
			t.Errorf("FirstSeead not preserved. Expected %v, got %v", firstSeen, updated.FirstSeen)
		}
	}
	if updated.ChipType != "ESP32-S2" {
		t.Errorf("ChipType not updated. Expected ESP32-S2, got %s", updated.ChipType)
	}
}

// TestConcurrentHotplugAndLoad tests concurrent access to device state
func TestConcurrentHotplugAndLoad(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	leader := NewLeaderNode("test-leader", &LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
	}, store)

	if err := leader.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		leader.Stop()
	}()

	// Save persisted device
	record := &persistence.DeviceRecord{
		DeviceID:   "manual-ESP32-1",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32",
		ChipRev:    "1.0",
		FlashSize:  4 * 1024 * 1024,
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
		LastPath:   "/dev/ttyUSB0",
		NodeID:     "test-leader",
	}
	if err := store.SaveDevice(record); err != nil {
		t.Fatal(err)
	}

	// Load it
	leader.loadPersistedDevices()

	var wg sync.WaitGroup
	// Simulate concurrent hotplug events
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event := device.DeviceEvent{
				Type: device.DeviceAdded,
				Path: "/dev/ttyUSB0",
				VID:  0x1234,
				PID:  0x5678,
			}
			leader.handleDeviceEvent(event)
		}()
	}
	wg.Wait()

	// Verify device still has persisted identity
	state := leader.State()
	dev := state.Devices["/dev/ttyUSB0"]

	if dev.DeviceID != "manual-ESP32-1" {
		t.Errorf("Concurrent hotplug should preserve DeviceID. Expected manual-ESP32-1, got %s", dev.DeviceID)
	}
}
