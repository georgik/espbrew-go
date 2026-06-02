package persistence

import (
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/flashhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndGetFlashHashes(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	hashes := &flashhash.JobFlashHashes{
		JobID:    "job-123",
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		Regions: []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
			{Name: "application", Offset: 0x10000, Size: 0x100000, MD5: "22222222222222222222222222222222"},
		},
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	err = store.SaveFlashHashes(hashes)
	require.NoError(t, err)

	retrieved, err := store.GetFlashHashes("job-123")
	require.NoError(t, err)

	assert.Equal(t, "job-123", retrieved.JobID)
	assert.Equal(t, "esp-aa:bb:cc:dd:ee:ff", retrieved.DeviceID)
	assert.Len(t, retrieved.Regions, 2)
	assert.Equal(t, "bootloader", retrieved.Regions[0].Name)
	assert.Equal(t, "application", retrieved.Regions[1].Name)
	assert.Equal(t, "11111111111111111111111111111111", retrieved.Regions[0].MD5)
}

func TestGetFlashHashesNotFound(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	_, err = store.GetFlashHashes("nonexistent")
	assert.Error(t, err)
}

func TestDeleteFlashHashes(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	hashes := &flashhash.JobFlashHashes{
		JobID:    "job-delete",
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		Regions: []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
		},
	}

	err = store.SaveFlashHashes(hashes)
	require.NoError(t, err)

	// Verify it exists
	_, err = store.GetFlashHashes("job-delete")
	require.NoError(t, err)

	// Delete it
	err = store.DeleteFlashHashes("job-delete")
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.GetFlashHashes("job-delete")
	assert.Error(t, err)
}

func TestListFlashHashesForDevice(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	// Create multiple job hashes
	hashes1 := &flashhash.JobFlashHashes{
		JobID:    "job-1",
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		Regions: []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
		},
	}

	hashes2 := &flashhash.JobFlashHashes{
		JobID:    "job-2",
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		Regions: []flashhash.FlashRegionInfo{
			{Name: "application", Offset: 0x10000, Size: 0x100000, MD5: "22222222222222222222222222222222"},
		},
	}

	hashes3 := &flashhash.JobFlashHashes{
		JobID:    "job-3",
		DeviceID: "esp-11:11:11:11:11:11",
		Regions: []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "33333333333333333333333333333333"},
		},
	}

	err = store.SaveFlashHashes(hashes1)
	require.NoError(t, err)

	err = store.SaveFlashHashes(hashes2)
	require.NoError(t, err)

	err = store.SaveFlashHashes(hashes3)
	require.NoError(t, err)

	// List for specific device
	results, err := store.ListFlashHashesForDevice("esp-aa:bb:cc:dd:ee:ff")
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// List all devices
	allResults, err := store.ListFlashHashesForDevice("")
	require.NoError(t, err)
	assert.Len(t, allResults, 3)
}

func TestUpdateFlashHashes(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	hashes := &flashhash.JobFlashHashes{
		JobID:    "job-update",
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		Regions: []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
		},
	}

	err = store.SaveFlashHashes(hashes)
	require.NoError(t, err)

	// Update with new regions
	hashes.Regions = []flashhash.FlashRegionInfo{
		{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
		{Name: "application", Offset: 0x10000, Size: 0x100000, MD5: "22222222222222222222222222222222"},
	}

	err = store.SaveFlashHashes(hashes)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetFlashHashes("job-update")
	require.NoError(t, err)
	assert.Len(t, retrieved.Regions, 2)
}

func TestFlashHashesPersistence(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	// Create and save hashes
	store1, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)

	hashes := &flashhash.JobFlashHashes{
		JobID:    "job-persist",
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		Regions: []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
		},
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	err = store1.SaveFlashHashes(hashes)
	require.NoError(t, err)
	store1.Close()

	// Reopen and verify
	store2, err := Open(&Config{Path: dbPath})
	require.NoError(t, err)
	defer store2.Close()

	retrieved, err := store2.GetFlashHashes("job-persist")
	require.NoError(t, err)

	assert.Equal(t, "job-persist", retrieved.JobID)
	assert.Equal(t, "esp-aa:bb:cc:dd:ee:ff", retrieved.DeviceID)
	assert.Len(t, retrieved.Regions, 1)
}
