package cluster

import (
	"testing"
	"time"

	"github.com/georgik/espbrew-go/internal/config"
	"github.com/georgik/espbrew-go/internal/device"
	"github.com/georgik/espbrew-go/internal/persistence"
)

// TestHandleDeviceEvent_MultiAlias verifies that a single physical port
// registered under several aliases in espbrew.toml (same path, different
// alias) is stamped with ALL of its aliases onto the persisted record, so the
// board is addressable by any of them. Regression cover for the waveshare
// board which is exposed both as "waveshare-...-1.28" and "...-1_28".
func TestHandleDeviceEvent_MultiAlias(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	path := "/dev/serial/by-id/usb-1a86_USB_Single_Serial_5971084553-if00"

	leader := NewLeaderNode("test-leader", &LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		Devices: []config.DeviceConfig{
			{Path: path, ID: "esp-5971084553-if00", Alias: "waveshare-esp32-s3-touch-lcd-1.28", ChipType: "esp32-s3"},
			{Path: path, ID: "esp-5971084553-underscore-if00", Alias: "waveshare-esp32-s3-touch-lcd-1_28", ChipType: "esp32-s3"},
		},
	}, store)

	if err := leader.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		leader.Stop()
	}()

	// Fire a DeviceAdded event for the shared path.
	leader.handleDeviceEvent(device.DeviceEvent{
		Type:     device.DeviceAdded,
		Path:     path,
		RealPath: path,
		VID:      0x1a86,
		PID:      0x55d3,
	})

	// The record must be addressable by BOTH aliases.
	for _, alias := range []string{
		"waveshare-esp32-s3-touch-lcd-1.28",
		"waveshare-esp32-s3-touch-lcd-1_28",
	} {
		rec, err := store.GetDeviceByAlias(alias)
		if err != nil {
			t.Errorf("expected device addressable by alias %q, got error: %v", alias, err)
			continue
		}
		if rec == nil {
			t.Errorf("expected device addressable by alias %q, got nil record", alias)
		}
	}

	// Both aliases must be present on the record, in file order, deduplicated.
	rec, err := store.GetDeviceByAlias("waveshare-esp32-s3-touch-lcd-1.28")
	if err != nil {
		t.Fatalf("failed to load record: %v", err)
	}
	got := rec.Aliases
	want := []string{
		"waveshare-esp32-s3-touch-lcd-1.28",
		"waveshare-esp32-s3-touch-lcd-1_28",
	}
	if len(got) != len(want) {
		t.Fatalf("aliases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("aliases = %v, want %v", got, want)
		}
	}

	// Identity from the first (authoritative) config entry is applied.
	if rec.ChipType != "esp32-s3" {
		t.Errorf("chip = %q, want esp32-s3", rec.ChipType)
	}
}
