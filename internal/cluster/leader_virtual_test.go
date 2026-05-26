package cluster

import (
	"context"
	"testing"
	"time"
)

func TestVirtualDevicesRegistered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &LeaderConfig{
		HeartbeatInterval: 1 * time.Second,
		NodeTimeout:       5 * time.Second,
		HTTPPort:          8081,
		DisablemDNS:       true,
		DisableWatcher:    true,
	}

	leader := NewLeaderNode("test-leader", cfg)
	if err := leader.Start(ctx); err != nil {
		t.Fatalf("Failed to start leader: %v", err)
	}
	defer leader.Stop()

	// Give time for virtual device registration
	time.Sleep(100 * time.Millisecond)

	state := leader.State()

	// Check that virtual devices are registered
	expectedVirtual := []string{
		"wokwi-esp32s3",
		"wokwi-esp32",
		"wokwi-esp32c3",
		"wokwi-esp32c6",
	}

	for _, path := range expectedVirtual {
		dev, exists := state.Devices[path]
		if !exists {
			t.Errorf("Virtual device %q not registered", path)
			continue
		}
		if dev.Status != "available" {
			t.Errorf("Virtual device %q has status %q, want 'available'", path, dev.Status)
		}
		if dev.NodeID != "test-leader" {
			t.Errorf("Virtual device %q has node_id %q, want 'test-leader'", path, dev.NodeID)
		}
	}

	t.Logf("Registered %d virtual devices", len(state.Devices))
}

func TestVirtualDeviceFlashJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &LeaderConfig{
		HeartbeatInterval: 1 * time.Second,
		NodeTimeout:       5 * time.Second,
		HTTPPort:          8082,
		DisablemDNS:       true,
		DisableWatcher:    true,
	}

	leader := NewLeaderNode("test-leader", cfg)
	leader.executor = NewJobExecutor(1)

	if err := leader.Start(ctx); err != nil {
		t.Fatalf("Failed to start leader: %v", err)
	}
	defer leader.Stop()

	// Wait for virtual device registration
	time.Sleep(100 * time.Millisecond)

	// Try to enqueue a job for virtual device
	job, err := leader.EnqueueJob("test-firmware.bin", "wokwi-esp32s3")
	if err != nil {
		t.Fatalf("Failed to enqueue job: %v", err)
	}

	if job.DevicePath != "wokwi-esp32s3" {
		t.Errorf("Job device path = %q, want 'wokwi-esp32s3'", job.DevicePath)
	}

	t.Logf("Job %s enqueued for virtual device", job.ID)
}
