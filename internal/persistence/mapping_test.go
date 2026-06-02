package persistence

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndGetBoundingBox(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	mapping := &DeviceBoundingBoxMapping{
		ID:       "bbox-1",
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		CameraID: "cam-001",
		Bounds: BoundingBox{
			X:      0.1,
			Y:      0.2,
			Width:  0.3,
			Height: 0.4,
		},
		CalibrationVersion: 1,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	err = store.SaveBoundingBox(mapping)
	require.NoError(t, err)

	retrieved, err := store.GetBoundingBox("bbox-1")
	require.NoError(t, err)

	assert.Equal(t, "bbox-1", retrieved.ID)
	assert.Equal(t, "esp-aa:bb:cc:dd:ee:ff", retrieved.DeviceID)
	assert.Equal(t, "cam-001", retrieved.CameraID)
	assert.Equal(t, 0.1, retrieved.Bounds.X)
	assert.Equal(t, 0.2, retrieved.Bounds.Y)
	assert.Equal(t, 0.3, retrieved.Bounds.Width)
	assert.Equal(t, 0.4, retrieved.Bounds.Height)
	assert.Equal(t, 1, retrieved.CalibrationVersion)
}

func TestSaveBoundingBoxUpdatesExisting(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	mapping := &DeviceBoundingBoxMapping{
		ID:       "bbox-1",
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		CameraID: "cam-001",
		Bounds: BoundingBox{
			X:      0.1,
			Y:      0.2,
			Width:  0.3,
			Height: 0.4,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.SaveBoundingBox(mapping)
	require.NoError(t, err)

	// Update the bounds
	mapping.Bounds = BoundingBox{
		X:      0.5,
		Y:      0.6,
		Width:  0.2,
		Height: 0.2,
	}
	mapping.UpdatedAt = time.Now()

	err = store.SaveBoundingBox(mapping)
	require.NoError(t, err)

	retrieved, err := store.GetBoundingBox("bbox-1")
	require.NoError(t, err)

	assert.Equal(t, 0.5, retrieved.Bounds.X)
	assert.Equal(t, 0.6, retrieved.Bounds.Y)
	assert.Equal(t, 0.2, retrieved.Bounds.Width)
	assert.Equal(t, 0.2, retrieved.Bounds.Height)
}

func TestListBoundingBoxesForCamera(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	// Create multiple mappings for same camera
	mappings := []*DeviceBoundingBoxMapping{
		{
			ID:       "bbox-1",
			DeviceID: "esp-11:11:11:11:11:11",
			CameraID: "cam-001",
			Bounds:   BoundingBox{X: 0, Y: 0, Width: 0.5, Height: 0.5},
		},
		{
			ID:       "bbox-2",
			DeviceID: "esp-22:22:22:22:22:22",
			CameraID: "cam-001",
			Bounds:   BoundingBox{X: 0.5, Y: 0, Width: 0.5, Height: 0.5},
		},
		{
			ID:       "bbox-3",
			DeviceID: "esp-33:33:33:33:33:33",
			CameraID: "cam-002", // Different camera
			Bounds:   BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
		},
	}

	for _, m := range mappings {
		m.CreatedAt = time.Now()
		m.UpdatedAt = time.Now()
		err = store.SaveBoundingBox(m)
		require.NoError(t, err)
	}

	// List for cam-001
	results, err := store.ListBoundingBoxesForCamera("cam-001")
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// List for cam-002
	results, err = store.ListBoundingBoxesForCamera("cam-002")
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// List for non-existent camera
	results, err = store.ListBoundingBoxesForCamera("cam-999")
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestListBoundingBoxesForDevice(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	// Create mappings for same device in different cameras
	mappings := []*DeviceBoundingBoxMapping{
		{
			ID:       "bbox-1",
			DeviceID: "esp-aa:bb:cc:dd:ee:ff",
			CameraID: "cam-001",
			Bounds:   BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
		},
		{
			ID:       "bbox-2",
			DeviceID: "esp-aa:bb:cc:dd:ee:ff",
			CameraID: "cam-002",
			Bounds:   BoundingBox{X: 0.2, Y: 0.2, Width: 0.6, Height: 0.6},
		},
		{
			ID:       "bbox-3",
			DeviceID: "esp-11:11:11:11:11:11", // Different device
			CameraID: "cam-001",
			Bounds:   BoundingBox{X: 0, Y: 0, Width: 0.5, Height: 0.5},
		},
	}

	for _, m := range mappings {
		m.CreatedAt = time.Now()
		m.UpdatedAt = time.Now()
		err = store.SaveBoundingBox(m)
		require.NoError(t, err)
	}

	// List for device
	results, err := store.ListBoundingBoxesForDevice("esp-aa:bb:cc:dd:ee:ff")
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// List for different device
	results, err = store.ListBoundingBoxesForDevice("esp-11:11:11:11:11:11")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestDeleteBoundingBox(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	mapping := &DeviceBoundingBoxMapping{
		ID:        "bbox-1",
		DeviceID:  "esp-aa:bb:cc:dd:ee:ff",
		CameraID:  "cam-001",
		Bounds:    BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.SaveBoundingBox(mapping)
	require.NoError(t, err)

	// Verify it exists
	_, err = store.GetBoundingBox("bbox-1")
	require.NoError(t, err)

	// Delete it
	err = store.DeleteBoundingBox("bbox-1")
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.GetBoundingBox("bbox-1")
	assert.Error(t, err)

	// Verify it's not in camera index
	results, err := store.ListBoundingBoxesForCamera("cam-001")
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestCameraCalibration(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	// Create initial calibration
	calib := &CameraCalibration{
		ID:          "calib-cam-001",
		CameraID:    "cam-001",
		Version:     1,
		Description: "Initial position",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err = store.SaveCalibration(calib)
	require.NoError(t, err)

	// Retrieve it
	retrieved, err := store.GetCalibration("cam-001")
	require.NoError(t, err)

	assert.Equal(t, 1, retrieved.Version)
	assert.Equal(t, "Initial position", retrieved.Description)

	// Increment version
	err = store.IncrementCalibrationVersion("cam-001", "Position 2")
	require.NoError(t, err)

	// Verify the incremented version is stored
	retrieved, err = store.GetCalibration("cam-001")
	require.NoError(t, err)

	assert.Equal(t, 2, retrieved.Version)
	assert.Equal(t, "Position 2", retrieved.Description)
}

func TestBoundingBoxCalibrationVersion(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	// Create calibration version 1
	calib := &CameraCalibration{
		ID:        "calib-cam-001",
		CameraID:  "cam-001",
		Version:   1,
		CreatedAt: time.Now(),
	}
	err = store.SaveCalibration(calib)
	require.NoError(t, err)

	// Create mapping with calibration version 1
	mapping := &DeviceBoundingBoxMapping{
		ID:                 "bbox-1",
		DeviceID:           "esp-aa:bb:cc:dd:ee:ff",
		CameraID:           "cam-001",
		Bounds:             BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
		CalibrationVersion: 1,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	err = store.SaveBoundingBox(mapping)
	require.NoError(t, err)

	// Increment calibration to version 2
	err = store.IncrementCalibrationVersion("cam-001", "New position")
	require.NoError(t, err)

	// Old mapping should still exist with version 1
	retrieved, err := store.GetBoundingBox("bbox-1")
	require.NoError(t, err)
	assert.Equal(t, 1, retrieved.CalibrationVersion)

	// Create new mapping with version 2
	mapping2 := &DeviceBoundingBoxMapping{
		ID:                 "bbox-2",
		DeviceID:           "esp-aa:bb:cc:dd:ee:ff",
		CameraID:           "cam-001",
		Bounds:             BoundingBox{X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8},
		CalibrationVersion: 2,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	err = store.SaveBoundingBox(mapping2)
	require.NoError(t, err)

	// List all for camera should return both
	results, err := store.ListBoundingBoxesForCamera("cam-001")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestBoundingBoxPersistenceAcrossReopen(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	// Create and save a mapping
	store1, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)

	mapping := &DeviceBoundingBoxMapping{
		ID:       "bbox-persist-1",
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		CameraID: "cam-001",
		Bounds: BoundingBox{
			X:      0.1,
			Y:      0.2,
			Width:  0.3,
			Height: 0.4,
		},
		CalibrationVersion: 1,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	err = store1.SaveBoundingBox(mapping)
	require.NoError(t, err)
	err = store1.Close()
	require.NoError(t, err)

	// Reopen database and verify mapping persists
	store2, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store2.Close()

	retrieved, err := store2.GetBoundingBox("bbox-persist-1")
	require.NoError(t, err)

	assert.Equal(t, "bbox-persist-1", retrieved.ID)
	assert.Equal(t, "esp-aa:bb:cc:dd:ee:ff", retrieved.DeviceID)
	assert.Equal(t, "cam-001", retrieved.CameraID)
	assert.Equal(t, 0.1, retrieved.Bounds.X)
	assert.Equal(t, 0.2, retrieved.Bounds.Y)
	assert.Equal(t, 0.3, retrieved.Bounds.Width)
	assert.Equal(t, 0.4, retrieved.Bounds.Height)

	// Verify camera index also persists
	results, err := store2.ListBoundingBoxesForCamera("cam-001")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestGetBoundingBoxForDeviceAndCamera(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	// Create multiple mappings
	mappings := []*DeviceBoundingBoxMapping{
		{
			ID:       "bbox-1",
			DeviceID: "esp-aa:bb:cc:dd:ee:ff",
			CameraID: "cam-001",
			Bounds:   BoundingBox{X: 0, Y: 0, Width: 0.5, Height: 0.5},
		},
		{
			ID:       "bbox-2",
			DeviceID: "esp-11:11:11:11:11:11",
			CameraID: "cam-001",
			Bounds:   BoundingBox{X: 0.5, Y: 0, Width: 0.5, Height: 0.5},
		},
		{
			ID:       "bbox-3",
			DeviceID: "esp-aa:bb:cc:dd:ee:ff",
			CameraID: "cam-002",
			Bounds:   BoundingBox{X: 0.2, Y: 0.2, Width: 0.6, Height: 0.6},
		},
	}

	for _, m := range mappings {
		m.CreatedAt = time.Now()
		m.UpdatedAt = time.Now()
		err = store.SaveBoundingBox(m)
		require.NoError(t, err)
	}

	// Find existing mapping for device+camera
	result, err := store.GetBoundingBoxForDeviceAndCamera("esp-aa:bb:cc:dd:ee:ff", "cam-001")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "bbox-1", result.ID)
	assert.Equal(t, "esp-aa:bb:cc:dd:ee:ff", result.DeviceID)
	assert.Equal(t, "cam-001", result.CameraID)

	// Find different device
	result, err = store.GetBoundingBoxForDeviceAndCamera("esp-11:11:11:11:11:11", "cam-001")
	require.NoError(t, err)
	assert.Equal(t, "bbox-2", result.ID)

	// Find same device on different camera
	result, err = store.GetBoundingBoxForDeviceAndCamera("esp-aa:bb:cc:dd:ee:ff", "cam-002")
	require.NoError(t, err)
	assert.Equal(t, "bbox-3", result.ID)

	// Non-existent combination should return nil
	result, err = store.GetBoundingBoxForDeviceAndCamera("nonexistent", "cam-001")
	require.NoError(t, err)
	assert.Nil(t, result)

	result, err = store.GetBoundingBoxForDeviceAndCamera("esp-aa:bb:cc:dd:ee:ff", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestUniqueDeviceCameraMapping(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	// Create initial mapping
	mapping1 := &DeviceBoundingBoxMapping{
		ID:        "bbox-1",
		DeviceID:  "esp-aa:bb:cc:dd:ee:ff",
		CameraID:  "cam-001",
		Bounds:    BoundingBox{X: 0.1, Y: 0.1, Width: 0.3, Height: 0.3},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.SaveBoundingBox(mapping1)
	require.NoError(t, err)

	// Verify only one mapping exists for this device+camera
	results, err := store.ListBoundingBoxesForCamera("cam-001")
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Create mapping with same device but different camera - should work
	mapping2 := &DeviceBoundingBoxMapping{
		ID:        "bbox-2",
		DeviceID:  "esp-aa:bb:cc:dd:ee:ff",
		CameraID:  "cam-002",
		Bounds:    BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.SaveBoundingBox(mapping2)
	require.NoError(t, err)

	// Verify both cameras have mappings
	results, err = store.ListBoundingBoxesForCamera("cam-001")
	require.NoError(t, err)
	assert.Len(t, results, 1)

	results, err = store.ListBoundingBoxesForCamera("cam-002")
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Verify device has two mappings (one per camera)
	results, err = store.ListBoundingBoxesForDevice("esp-aa:bb:cc:dd:ee:ff")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}
