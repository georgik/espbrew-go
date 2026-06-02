package camera

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractor_ExtractDevices(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	cameraStore, err := NewStore(t.TempDir() + "/captures")
	require.NoError(t, err)

	extractor := NewExtractor(cameraStore, store)

	// Create test image
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// Fill with a pattern
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 2), uint8(y * 2), 128, 255})
		}
	}

	// Create device mappings
	mappings := []*persistence.DeviceBoundingBoxMapping{
		{
			ID:       "bbox-1",
			DeviceID: "device-1",
			CameraID: "cam-001",
			Bounds:   persistence.BoundingBox{X: 0, Y: 0, Width: 0.5, Height: 0.5},
		},
		{
			ID:       "bbox-2",
			DeviceID: "device-2",
			CameraID: "cam-001",
			Bounds:   persistence.BoundingBox{X: 0.5, Y: 0.5, Width: 0.5, Height: 0.5},
		},
	}

	for _, m := range mappings {
		err = store.SaveBoundingBox(m)
		require.NoError(t, err)
	}

	// Create temporary capture path
	capturePath := filepath.Join(t.TempDir(), "test-capture.jpg")

	// Extract devices
	results, err := extractor.ExtractDevices(img, "cam-001", capturePath)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify first device
	assert.Equal(t, "device-1", results[0].DeviceID)
	assert.NotEmpty(t, results[0].Subimage)
	assert.Equal(t, persistence.BoundingBox{X: 0, Y: 0, Width: 0.5, Height: 0.5}, results[0].Bounds)

	// Verify second device
	assert.Equal(t, "device-2", results[1].DeviceID)
	assert.NotEmpty(t, results[1].Subimage)
	assert.Equal(t, persistence.BoundingBox{X: 0.5, Y: 0.5, Width: 0.5, Height: 0.5}, results[1].Bounds)

	// Verify subimage files exist (convert relative path back to absolute)
	for _, result := range results {
		absPath := filepath.Join(cameraStore.GetBaseDir(), result.Subimage)
		_, err := os.Stat(absPath)
		assert.NoError(t, err, "Subimage file should exist")
	}
}

func TestExtractor_ExtractDevicesWithAdjustment(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	cameraStore, err := NewStore(t.TempDir() + "/captures")
	require.NoError(t, err)

	extractor := NewExtractor(cameraStore, store)

	// Create test image (gray)
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, gray)
		}
	}

	// Create mapping with brightness adjustment
	mapping := &persistence.DeviceBoundingBoxMapping{
		ID:       "bbox-1",
		DeviceID: "device-1",
		CameraID: "cam-001",
		Bounds:   persistence.BoundingBox{X: 0.1, Y: 0.1, Width: 0.3, Height: 0.3},
		Adjustment: persistence.ImageAdjustment{
			Brightness: 50,
			Contrast:   10,
			Saturation: 0,
		},
	}

	err = store.SaveBoundingBox(mapping)
	require.NoError(t, err)

	capturePath := filepath.Join(t.TempDir(), "test-capture.jpg")

	// Extract devices
	results, err := extractor.ExtractDevices(img, "cam-001", capturePath)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Verify adjustment was applied
	assert.Equal(t, 50, results[0].Adjustment.Brightness)
	assert.Equal(t, 10, results[0].Adjustment.Contrast)
	assert.NotEmpty(t, results[0].Subimage)
}

func TestExtractor_NoMappings(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	cameraStore, err := NewStore(t.TempDir() + "/captures")
	require.NoError(t, err)

	extractor := NewExtractor(cameraStore, store)

	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	capturePath := filepath.Join(t.TempDir(), "test-capture.jpg")

	// Extract with no mappings
	results, err := extractor.ExtractDevices(img, "cam-001", capturePath)
	require.NoError(t, err)
	assert.Nil(t, results) // No mappings means nil results
}

func TestExtractor_InvalidBounds(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	cameraStore, err := NewStore(t.TempDir() + "/captures")
	require.NoError(t, err)

	extractor := NewExtractor(cameraStore, store)

	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Create mapping with bounds that extend beyond image (should still work with clipping)
	mapping := &persistence.DeviceBoundingBoxMapping{
		ID:       "bbox-1",
		DeviceID: "device-1",
		CameraID: "cam-001",
		Bounds:   persistence.BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
	}

	err = store.SaveBoundingBox(mapping)
	require.NoError(t, err)

	capturePath := filepath.Join(t.TempDir(), "test-capture.jpg")

	// Should handle full-image bounds
	results, err := extractor.ExtractDevices(img, "cam-001", capturePath)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}
