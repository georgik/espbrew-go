package persistence

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCameraSettingsCRUD(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := Open(DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test settings
	settings := &CameraSettings{
		CameraID:         "/dev/video0",
		Name:             "Logitech C615",
		Brightness:       128,
		Contrast:         32,
		Saturation:       32,
		Sharpness:        22,
		Gain:             0,
		Focus:            85,
		Exposure:         300,
		WhiteBalance:     4000,
		AutoExposure:     false,
		AutoFocus:        false,
		AutoWhiteBalance: false,
	}

	// Test Store
	t.Run("StoreCameraSettings", func(t *testing.T) {
		if err := store.StoreCameraSettings(settings); err != nil {
			t.Errorf("StoreCameraSettings failed: %v", err)
		}

		// Verify timestamps were set
		if settings.CreatedAt.IsZero() {
			t.Error("CreatedAt should be set")
		}
		if settings.UpdatedAt.IsZero() {
			t.Error("UpdatedAt should be set")
		}
	})

	// Test Get
	t.Run("GetCameraSettings", func(t *testing.T) {
		retrieved, err := store.GetCameraSettings(settings.CameraID)
		if err != nil {
			t.Errorf("GetCameraSettings failed: %v", err)
		}

		if retrieved.CameraID != settings.CameraID {
			t.Errorf("CameraID mismatch: got %s, want %s", retrieved.CameraID, settings.CameraID)
		}
		if retrieved.Name != settings.Name {
			t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, settings.Name)
		}
		if retrieved.Brightness != settings.Brightness {
			t.Errorf("Brightness mismatch: got %d, want %d", retrieved.Brightness, settings.Brightness)
		}
		if retrieved.Focus != settings.Focus {
			t.Errorf("Focus mismatch: got %d, want %d", retrieved.Focus, settings.Focus)
		}
		if retrieved.AutoExposure != settings.AutoExposure {
			t.Errorf("AutoExposure mismatch: got %v, want %v", retrieved.AutoExposure, settings.AutoExposure)
		}
	})

	// Test Update
	t.Run("UpdateCameraSettings", func(t *testing.T) {
		settings.Brightness = 200
		settings.Focus = 120

		if err := store.StoreCameraSettings(settings); err != nil {
			t.Errorf("StoreCameraSettings (update) failed: %v", err)
		}

		retrieved, err := store.GetCameraSettings(settings.CameraID)
		if err != nil {
			t.Errorf("GetCameraSettings (after update) failed: %v", err)
		}

		if retrieved.Brightness != 200 {
			t.Errorf("Brightness not updated: got %d, want 200", retrieved.Brightness)
		}
		if retrieved.Focus != 120 {
			t.Errorf("Focus not updated: got %d, want 120", retrieved.Focus)
		}
	})

	// Test List
	t.Run("ListCameraSettings", func(t *testing.T) {
		// Add another camera
		settings2 := &CameraSettings{
			CameraID:   "/dev/video1",
			Name:       "Built-in Camera",
			Brightness: 100,
		}
		store.StoreCameraSettings(settings2)

		list, err := store.ListCameraSettings(nil)
		if err != nil {
			t.Errorf("ListCameraSettings failed: %v", err)
		}

		if len(list) != 2 {
			t.Errorf("ListCameraSettings count: got %d, want 2", len(list))
		}
	})

	// Test Filter
	t.Run("ListCameraSettings with filter", func(t *testing.T) {
		filter := &CameraSettingsFilter{
			CameraID: "/dev/video0",
		}

		list, err := store.ListCameraSettings(filter)
		if err != nil {
			t.Errorf("ListCameraSettings with filter failed: %v", err)
		}

		if len(list) != 1 {
			t.Errorf("Filtered list count: got %d, want 1", len(list))
		}
		if list[0].CameraID != "/dev/video0" {
			t.Errorf("Filtered wrong camera: got %s", list[0].CameraID)
		}
	})

	// Test Preset filter
	t.Run("ListCameraSettings preset filter", func(t *testing.T) {
		presetSettings := &CameraSettings{
			CameraID:   "preset:display",
			Name:       "Display Preset",
			PresetName: "display",
			Brightness: 80,
			Contrast:   140,
		}
		store.StoreCameraSettings(presetSettings)

		filter := &CameraSettingsFilter{
			PresetName: "display",
		}

		list, err := store.ListCameraSettings(filter)
		if err != nil {
			t.Errorf("ListCameraSettings preset filter failed: %v", err)
		}

		if len(list) != 1 {
			t.Errorf("Preset filter count: got %d, want 1", len(list))
		}
	})

	// Test Limit
	t.Run("ListCameraSettings with limit", func(t *testing.T) {
		filter := &CameraSettingsFilter{
			Limit: 1,
		}

		list, err := store.ListCameraSettings(filter)
		if err != nil {
			t.Errorf("ListCameraSettings with limit failed: %v", err)
		}

		if len(list) != 1 {
			t.Errorf("Limited list count: got %d, want 1", len(list))
		}
	})

	// Test Delete
	t.Run("DeleteCameraSettings", func(t *testing.T) {
		if err := store.DeleteCameraSettings(settings.CameraID); err != nil {
			t.Errorf("DeleteCameraSettings failed: %v", err)
		}

		_, err := store.GetCameraSettings(settings.CameraID)
		if err == nil {
			t.Error("Expected error when getting deleted settings")
		}
	})
}

func TestGetCameraPreset(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := Open(DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create preset
	preset := &CameraSettings{
		CameraID:   "preset:low-light",
		Name:       "Low Light Preset",
		PresetName: "low-light",
		Brightness: 200,
		Gain:       100,
	}
	store.StoreCameraSettings(preset)

	t.Run("Get existing preset", func(t *testing.T) {
		retrieved, err := store.GetCameraPreset("low-light")
		if err != nil {
			t.Errorf("GetCameraPreset failed: %v", err)
		}

		if retrieved.PresetName != "low-light" {
			t.Errorf("PresetName mismatch: got %s, want low-light", retrieved.PresetName)
		}
		if retrieved.Brightness != 200 {
			t.Errorf("Brightness mismatch: got %d, want 200", retrieved.Brightness)
		}
	})

	t.Run("Get non-existent preset", func(t *testing.T) {
		_, err := store.GetCameraPreset("nonexistent")
		if err == nil {
			t.Error("Expected error for non-existent preset")
		}
	})
}

func TestListCameraPresets(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := Open(DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create some settings (presets and regular)
	settings := []*CameraSettings{
		{CameraID: "preset:display", Name: "Display", PresetName: "display"},
		{CameraID: "preset:outdoor", Name: "Outdoor", PresetName: "outdoor"},
		{CameraID: "/dev/video0", Name: "Webcam"}, // No preset name
	}

	for _, s := range settings {
		store.StoreCameraSettings(s)
	}

	t.Run("List all presets", func(t *testing.T) {
		presets, err := store.ListCameraPresets()
		if err != nil {
			t.Errorf("ListCameraPresets failed: %v", err)
		}

		if len(presets) != 3 {
			t.Errorf("ListCameraPresets count: got %d, want 3", len(presets))
		}
	})
}

func TestCameraSettingsValidation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := Open(DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("Valid settings store successfully", func(t *testing.T) {
		settings := &CameraSettings{
			CameraID:   "/dev/video0",
			Brightness: 128,
			Contrast:   128,
			Focus:      85,
		}

		if err := store.StoreCameraSettings(settings); err != nil {
			t.Errorf("Failed to store valid settings: %v", err)
		}
	})

	// BoltDB allows any value, validation happens at HTTP layer
	// This test verifies storage handles edge cases
	t.Run("Edge case values", func(t *testing.T) {
		settings := &CameraSettings{
			CameraID:   "/dev/video1",
			Brightness: 0,
			Contrast:   255,
			Focus:      255,
		}

		if err := store.StoreCameraSettings(settings); err != nil {
			t.Errorf("Failed to store edge case settings: %v", err)
		}

		retrieved, _ := store.GetCameraSettings(settings.CameraID)
		if retrieved.Contrast != 255 {
			t.Errorf("Contrast not stored correctly: got %d, want 255", retrieved.Contrast)
		}
	})
}

func TestCameraSettingsTimestamps(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := Open(DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	settings := &CameraSettings{
		CameraID:   "/dev/video0",
		Name:       "Test Camera",
		Brightness: 128,
	}

	// First store
	beforeStore := time.Now()
	if err := store.StoreCameraSettings(settings); err != nil {
		t.Fatalf("First store failed: %v", err)
	}
	afterStore := time.Now()

	if settings.CreatedAt.Before(beforeStore) || settings.CreatedAt.After(afterStore) {
		t.Error("CreatedAt not set correctly")
	}
	if settings.UpdatedAt.Before(beforeStore) || settings.UpdatedAt.After(afterStore) {
		t.Error("UpdatedAt not set correctly")
	}

	// Wait to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)

	// Update
	originalCreatedAt := settings.CreatedAt
	if err := store.StoreCameraSettings(settings); err != nil {
		t.Fatalf("Update store failed: %v", err)
	}

	// CreatedAt should not change, UpdatedAt should
	if !settings.CreatedAt.Equal(originalCreatedAt) {
		t.Error("CreatedAt should not change on update")
	}
	if !settings.UpdatedAt.After(originalCreatedAt) {
		t.Error("UpdatedAt should advance on update")
	}
}
