package cluster

import (
	"path/filepath"
	"testing"

	"github.com/georgik/espbrew-go/internal/config"
	"github.com/georgik/espbrew-go/internal/persistence"
	"github.com/georgik/espbrew-go/pkg/protocol"
)

// newMappingTestLeader builds a LeaderNode wired to a fresh store, with the
// given devices registered as its espbrew.toml mapping. It does NOT Start the
// leader (no watcher/maintenance) - the mapping sync is exercised directly.
func newMappingTestLeader(t *testing.T, devices ...config.DeviceConfig) (*LeaderNode, *persistence.Store) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}

	leader := NewLeaderNode("test-leader", &LeaderConfig{
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		DisableVirtual:     true,
		Devices:            devices,
	}, store)

	return leader, store
}

// registerCamera puts a camera into leader state, mirroring what discoverCameras
// does at startup (so the config-driven mapping can resolve its camera id).
func registerCamera(leader *LeaderNode, cam *protocol.CameraInfo) {
	leader.mu.Lock()
	leader.state.Cameras[cam.ID] = cam
	leader.mu.Unlock()
}

// TestSyncConfiguredCameraMappings_PersistsMapping verifies that a device that
// declares Camera + a non-zero CameraBox gets a device→camera bounding-box
// mapping persisted to the store, keyed on the configured device id and the
// resolved camera id, with the configured bounds.
func TestSyncConfiguredCameraMappings_PersistsMapping(t *testing.T) {
	leader, store := newMappingTestLeader(t, config.DeviceConfig{
		Path:     "/dev/serial/by-id/usb-test-if00",
		ID:       "esp-test-device",
		ChipType: "ESP32-S3",
		Alias:    "test-box-3",
		Camera:   "cam-test-camera",
		CameraBox: config.CameraBoxConfig{
			X: 0.3671875, Y: 0.26634114583333335,
			Width: 0.2453125, Height: 0.2375,
		},
	})

	registerCamera(leader, &protocol.CameraInfo{
		ID: "cam-test-camera", Name: "usb-046d_Brio-video-index0;video0",
		Path: "/dev/video0", Backend: "v4l2", Status: "available", NodeID: "test-leader",
	})

	leader.syncConfiguredCameraMappings()

	mappings, err := store.ListBoundingBoxesForDevice("esp-test-device")
	if err != nil {
		t.Fatalf("ListBoundingBoxesForDevice: %v", err)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	m := mappings[0]
	if m.CameraID != "cam-test-camera" {
		t.Errorf("CameraID = %q, want cam-test-camera", m.CameraID)
	}
	b := m.Bounds
	if b.X != 0.3671875 || b.Y != 0.26634114583333335 || b.Width != 0.2453125 || b.Height != 0.2375 {
		t.Errorf("bounds = %+v, want the configured box", b)
	}
}

// TestSyncConfiguredCameraMappings_Idempotent verifies that re-running the sync
// (as it would be on a later device event) updates the existing mapping in place
// rather than creating duplicates.
func TestSyncConfiguredCameraMappings_Idempotent(t *testing.T) {
	leader, store := newMappingTestLeader(t, config.DeviceConfig{
		Path:   "/dev/serial/by-id/usb-test-if00",
		ID:     "esp-test-device",
		Camera: "cam-test-camera",
		CameraBox: config.CameraBoxConfig{
			X: 0.1, Y: 0.2, Width: 0.3, Height: 0.4,
		},
	})
	registerCamera(leader, &protocol.CameraInfo{
		ID: "cam-test-camera", Name: "cam", Path: "/dev/video0", Status: "available",
	})

	leader.syncConfiguredCameraMappings()
	first, err := store.ListBoundingBoxesForDevice("esp-test-device")
	if err != nil || len(first) != 1 {
		t.Fatalf("after first sync: len=%d err=%v", len(first), err)
	}
	id := first[0].ID

	// Re-run with a different box; the mapping must be updated, not duplicated.
	leader.config.Devices[0].CameraBox = config.CameraBoxConfig{
		X: 0.5, Y: 0.5, Width: 0.5, Height: 0.5,
	}
	leader.syncConfiguredCameraMappings()

	second, err := store.ListBoundingBoxesForDevice("esp-test-device")
	if err != nil {
		t.Fatalf("ListBoundingBoxesForDevice: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("expected 1 mapping after re-sync, got %d", len(second))
	}
	if second[0].ID != id {
		t.Errorf("mapping id changed across re-sync: %q -> %q", id, second[0].ID)
	}
	if second[0].Bounds.Width != 0.5 {
		t.Errorf("bounds not updated: Width = %v, want 0.5", second[0].Bounds.Width)
	}
}

// TestSyncConfiguredCameraMappings_SkipsUnconfigured verifies devices without a
// camera reference or without a bounded box produce no mapping.
func TestSyncConfiguredCameraMappings_SkipsUnconfigured(t *testing.T) {
	leader, store := newMappingTestLeader(t,
		config.DeviceConfig{
			ID:     "esp-without-box",
			Camera: "cam-test-camera",
			// CameraBox left all-zero => no box configured.
		},
		config.DeviceConfig{
			ID:        "esp-without-camera",
			CameraBox: config.CameraBoxConfig{X: 0.1, Y: 0.1, Width: 0.2, Height: 0.2},
			// Camera left empty.
		},
	)
	registerCamera(leader, &protocol.CameraInfo{
		ID: "cam-test-camera", Name: "cam", Path: "/dev/video0", Status: "available",
	})

	leader.syncConfiguredCameraMappings()

	for _, dev := range []string{"esp-without-box", "esp-without-camera"} {
		mappings, err := store.ListBoundingBoxesForDevice(dev)
		if err != nil {
			t.Fatalf("ListBoundingBoxesForDevice(%s): %v", dev, err)
		}
		if len(mappings) != 0 {
			t.Errorf("%s: expected 0 mappings, got %d", dev, len(mappings))
		}
	}
}

// TestSyncConfiguredCameraMappings_CameraNotFound verifies a device that
// references an unknown camera is skipped (no mapping, no panic) rather than
// silently creating a broken one.
func TestSyncConfiguredCameraMappings_CameraNotFound(t *testing.T) {
	leader, store := newMappingTestLeader(t, config.DeviceConfig{
		ID:        "esp-unknown-cam",
		Camera:    "cam-does-not-exist",
		CameraBox: config.CameraBoxConfig{X: 0.1, Y: 0.1, Width: 0.2, Height: 0.2},
	})
	// No camera registered.

	leader.syncConfiguredCameraMappings()

	mappings, err := store.ListBoundingBoxesForDevice("esp-unknown-cam")
	if err != nil {
		t.Fatalf("ListBoundingBoxesForDevice: %v", err)
	}
	if len(mappings) != 0 {
		t.Errorf("expected 0 mappings for unknown camera, got %d", len(mappings))
	}
}

// TestResolveCameraRefByNameOrID verifies resolveCameraRef matches the discovered
// camera by ID first, then falls back to the stable Name.
func TestResolveCameraRefByNameOrID(t *testing.T) {
	leader, _ := newMappingTestLeader(t)
	registerCamera(leader, &protocol.CameraInfo{
		ID: "cam-abc", Name: "usb-046d_Brio-video-index0;video0",
		Path: "/dev/video0", Status: "available",
	})

	if id, _, ok := leader.resolveCameraRef("cam-abc"); !ok || id != "cam-abc" {
		t.Errorf("resolve by ID: id=%q ok=%v, want cam-abc/true", id, ok)
	}
	if id, name, ok := leader.resolveCameraRef("USB-046d_Brio-video-index0;video0"); !ok || id != "cam-abc" || name != "usb-046d_Brio-video-index0;video0" {
		t.Errorf("resolve by Name: id=%q name=%q ok=%v, want cam-abc/<hardware name>/true", id, name, ok)
	}
	if _, _, ok := leader.resolveCameraRef("cam-missing"); ok {
		t.Error("resolve of unknown camera: ok=true, want false")
	}
}
