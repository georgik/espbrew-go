package persistence

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// TestCameraSettingsPersistsAcrossRestart verifies that camera settings
// survive closing and reopening the database, simulating server restart.
func TestCameraSettingsPersistsAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Phase 1: Save settings
	t.Run("save phase", func(t *testing.T) {
		store, err := Open(DefaultConfig(dbPath))
		if err != nil {
			t.Fatalf("Failed to open store: %v", err)
		}

		settings := &CameraSettings{
			CameraID:         "/dev/video0",
			Name:             "Test Camera Name",
			Brightness:       150,
			Contrast:         50,
			Saturation:       75,
			Sharpness:        80,
			Gain:             20,
			Focus:            100,
			Exposure:         250,
			WhiteBalance:     4500,
			AutoExposure:     true,
			AutoFocus:        false,
			AutoWhiteBalance: true,
		}

		if err := store.StoreCameraSettings(settings); err != nil {
			t.Fatalf("Failed to store settings: %v", err)
		}

		// Verify timestamps were set
		if settings.CreatedAt.IsZero() {
			t.Fatal("CreatedAt should be set")
		}
		if settings.UpdatedAt.IsZero() {
			t.Fatal("UpdatedAt should be set")
		}

		savedCreatedAt := settings.CreatedAt
		savedUpdatedAt := settings.UpdatedAt

		// Explicitly close to flush data
		if err := store.Close(); err != nil {
			t.Fatalf("Failed to close store: %v", err)
		}

		// Phase 2: Reopen and verify (simulates server restart)
		t.Run("load phase after restart", func(t *testing.T) {
			reopenedStore, err := Open(DefaultConfig(dbPath))
			if err != nil {
				t.Fatalf("Failed to reopen store: %v", err)
			}
			defer reopenedStore.Close()

			retrieved, err := reopenedStore.GetCameraSettings("/dev/video0")
			if err != nil {
				t.Fatalf("Failed to retrieve settings after restart: %v", err)
			}

			// Verify all fields persisted
			if retrieved.CameraID != settings.CameraID {
				t.Errorf("CameraID: got %s, want %s", retrieved.CameraID, settings.CameraID)
			}
			if retrieved.Name != settings.Name {
				t.Errorf("Name: got %s, want %s", retrieved.Name, settings.Name)
			}
			if retrieved.Brightness != settings.Brightness {
				t.Errorf("Brightness: got %d, want %d", retrieved.Brightness, settings.Brightness)
			}
			if retrieved.Contrast != settings.Contrast {
				t.Errorf("Contrast: got %d, want %d", retrieved.Contrast, settings.Contrast)
			}
			if retrieved.Saturation != settings.Saturation {
				t.Errorf("Saturation: got %d, want %d", retrieved.Saturation, settings.Saturation)
			}
			if retrieved.Sharpness != settings.Sharpness {
				t.Errorf("Sharpness: got %d, want %d", retrieved.Sharpness, settings.Sharpness)
			}
			if retrieved.Gain != settings.Gain {
				t.Errorf("Gain: got %d, want %d", retrieved.Gain, settings.Gain)
			}
			if retrieved.Focus != settings.Focus {
				t.Errorf("Focus: got %d, want %d", retrieved.Focus, settings.Focus)
			}
			if retrieved.Exposure != settings.Exposure {
				t.Errorf("Exposure: got %d, want %d", retrieved.Exposure, settings.Exposure)
			}
			if retrieved.WhiteBalance != settings.WhiteBalance {
				t.Errorf("WhiteBalance: got %d, want %d", retrieved.WhiteBalance, settings.WhiteBalance)
			}
			if retrieved.AutoExposure != settings.AutoExposure {
				t.Errorf("AutoExposure: got %v, want %v", retrieved.AutoExposure, settings.AutoExposure)
			}
			if retrieved.AutoFocus != settings.AutoFocus {
				t.Errorf("AutoFocus: got %v, want %v", retrieved.AutoFocus, settings.AutoFocus)
			}
			if retrieved.AutoWhiteBalance != settings.AutoWhiteBalance {
				t.Errorf("AutoWhiteBalance: got %v, want %v", retrieved.AutoWhiteBalance, settings.AutoWhiteBalance)
			}

			// Verify timestamps persisted correctly (not zero)
			if retrieved.CreatedAt.IsZero() {
				t.Error("CreatedAt should persist, got zero")
			}
			if retrieved.UpdatedAt.IsZero() {
				t.Error("UpdatedAt should persist, got zero")
			}

			// Verify timestamps match original values
			if !retrieved.CreatedAt.Equal(savedCreatedAt) {
				t.Errorf("CreatedAt mismatch: got %s, want %s", retrieved.CreatedAt, savedCreatedAt)
			}
			if !retrieved.UpdatedAt.Equal(savedUpdatedAt) {
				t.Errorf("UpdatedAt mismatch: got %s, want %s", retrieved.UpdatedAt, savedUpdatedAt)
			}
		})
	})
}

// TestDevicePersistsAcrossRestart verifies that device records
// survive closing and reopening the database.
func TestDevicePersistsAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Save device
	store, err := Open(DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}

	dev := &DeviceRecord{
		DeviceID:    "esp-restart-test",
		MACAddress:  "aa:bb:cc:dd:ee:ff",
		ChipType:    "ESP32-S3",
		ChipRev:     "0.1",
		FlashSize:   8 * 1024 * 1024,
		PSRAMSize:   8 * 1024 * 1024,
		BoardModel:  "ESP32-S3-DevKitC-1",
		Aliases:     []string{"devkit1", "test-board"},
		Tags:        []string{"dev", "test"},
		Description: "Test board for persistence",
	}

	if err := store.SaveDevice(dev); err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	savedFirstSeen := dev.FirstSeen
	savedLastSeen := dev.LastSeen

	// Close store
	if err := store.Close(); err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Reopen and verify
	reopenedStore, err := Open(DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer reopenedStore.Close()

	retrieved, err := reopenedStore.GetDevice("esp-restart-test")
	if err != nil {
		t.Fatalf("Failed to retrieve device after restart: %v", err)
	}

	// Verify all fields
	if retrieved.DeviceID != dev.DeviceID {
		t.Errorf("DeviceID: got %s, want %s", retrieved.DeviceID, dev.DeviceID)
	}
	if retrieved.MACAddress != dev.MACAddress {
		t.Errorf("MACAddress: got %s, want %s", retrieved.MACAddress, dev.MACAddress)
	}
	if retrieved.ChipType != dev.ChipType {
		t.Errorf("ChipType: got %s, want %s", retrieved.ChipType, dev.ChipType)
	}
	if retrieved.ChipRev != dev.ChipRev {
		t.Errorf("ChipRev: got %s, want %s", retrieved.ChipRev, dev.ChipRev)
	}
	if retrieved.FlashSize != dev.FlashSize {
		t.Errorf("FlashSize: got %d, want %d", retrieved.FlashSize, dev.FlashSize)
	}
	if retrieved.PSRAMSize != dev.PSRAMSize {
		t.Errorf("PSRAMSize: got %d, want %d", retrieved.PSRAMSize, dev.PSRAMSize)
	}
	if retrieved.BoardModel != dev.BoardModel {
		t.Errorf("BoardModel: got %s, want %s", retrieved.BoardModel, dev.BoardModel)
	}
	if retrieved.Description != dev.Description {
		t.Errorf("Description: got %s, want %s", retrieved.Description, dev.Description)
	}
	if len(retrieved.Aliases) != len(dev.Aliases) {
		t.Errorf("Aliases count: got %d, want %d", len(retrieved.Aliases), len(dev.Aliases))
	}
	if len(retrieved.Tags) != len(dev.Tags) {
		t.Errorf("Tags count: got %d, want %d", len(retrieved.Tags), len(dev.Tags))
	}

	// Verify timestamps persisted
	if retrieved.FirstSeen.IsZero() {
		t.Error("FirstSeen should persist, got zero")
	}
	if retrieved.LastSeen.IsZero() {
		t.Error("LastSeen should persist, got zero")
	}
	if !retrieved.FirstSeen.Equal(savedFirstSeen) {
		t.Errorf("FirstSeen mismatch: got %s, want %s", retrieved.FirstSeen, savedFirstSeen)
	}
	if !retrieved.LastSeen.Equal(savedLastSeen) {
		t.Errorf("LastSeen mismatch: got %s, want %s", retrieved.LastSeen, savedLastSeen)
	}

	// Verify indexes still work
	byMAC, err := reopenedStore.GetDeviceByMAC("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Errorf("GetDeviceByMAC failed after restart: %v", err)
	}
	if byMAC.DeviceID != dev.DeviceID {
		t.Errorf("MAC index returned wrong device: got %s, want %s", byMAC.DeviceID, dev.DeviceID)
	}

	byAlias, err := reopenedStore.GetDeviceByAlias("devkit1")
	if err != nil {
		t.Errorf("GetDeviceByAlias failed after restart: %v", err)
	}
	if byAlias.DeviceID != dev.DeviceID {
		t.Errorf("Alias index returned wrong device: got %s, want %s", byAlias.DeviceID, dev.DeviceID)
	}
}

// TestMultipleCloseReopenCycles verifies data survives multiple restart cycles.
func TestMultipleCloseReopenCycles(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cameraID := "/dev/video0"
	expectedName := "Cycle Test Camera"

	// Cycle 1: Initial save
	for cycle := 0; cycle < 3; cycle++ {
		t.Run(fmt.Sprintf("cycle %d", cycle), func(t *testing.T) {
			store, err := Open(DefaultConfig(dbPath))
			if err != nil {
				t.Fatalf("Cycle %d: Failed to open store: %v", cycle, err)
			}

			// On first cycle, save data. On subsequent cycles, verify it exists.
			if cycle == 0 {
				settings := &CameraSettings{
					CameraID:   cameraID,
					Name:       expectedName,
					Brightness: int32(100 + cycle*10),
				}
				if err := store.StoreCameraSettings(settings); err != nil {
					t.Fatalf("Cycle %d: Failed to save: %v", cycle, err)
				}
			} else {
				retrieved, err := store.GetCameraSettings(cameraID)
				if err != nil {
					t.Fatalf("Cycle %d: Failed to retrieve: %v", cycle, err)
				}
				if retrieved.Name != expectedName {
					t.Errorf("Cycle %d: Name got %s, want %s", cycle, retrieved.Name, expectedName)
				}
				if retrieved.CreatedAt.IsZero() {
					t.Errorf("Cycle %d: CreatedAt is zero", cycle)
				}
			}

			if err := store.Close(); err != nil {
				t.Fatalf("Cycle %d: Failed to close: %v", cycle, err)
			}

			// Add small delay to ensure filesystem sync
			time.Sleep(10 * time.Millisecond)
		})
	}
}
