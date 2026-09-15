package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSaveDeviceName verifies Name is persisted and read back.
func TestSaveDeviceName(t *testing.T) {
	store, err := Open(DefaultConfig(t.TempDir() + "/name.db"))
	require.NoError(t, err)
	defer store.Close()

	mac := "84:f7:03:12:34:56"
	require.NoError(t, store.SaveDevice(&DeviceRecord{
		DeviceID:   "esp-84:f7:03:12:34:56",
		MACAddress: mac,
		Name:       "living-room-brick",
		Tags:       []string{},
		Aliases:    []string{},
	}))

	dev, err := store.GetDevice("esp-84:f7:03:12:34:56")
	require.NoError(t, err)
	assert.Equal(t, "living-room-brick", dev.Name)
}

// TestUpdateDeviceName verifies SaveDevice with a new Name overwrites it.
func TestUpdateDeviceName(t *testing.T) {
	store, err := Open(DefaultConfig(t.TempDir() + "/name-update.db"))
	require.NoError(t, err)
	defer store.Close()

	rec := &DeviceRecord{
		DeviceID:   "esp-84:f7:03:12:34:56",
		MACAddress: "84:f7:03:12:34:56",
		Name:       "original",
		Tags:       []string{},
		Aliases:    []string{},
	}
	require.NoError(t, store.SaveDevice(rec))

	// Update only the name; other fields should persist.
	rec.Name = "renamed-device"
	require.NoError(t, store.SaveDevice(rec))

	got, err := store.GetDevice("esp-84:f7:03:12:34:56")
	require.NoError(t, err)
	assert.Equal(t, "renamed-device", got.Name)
}

// TestUpdateNameOnlyPersists verifies setting Name to empty clears it.
func TestUpdateDeviceNameCleared(t *testing.T) {
	store, err := Open(DefaultConfig(t.TempDir() + "/name-clear.db"))
	require.NoError(t, err)
	defer store.Close()

	rec := &DeviceRecord{
		DeviceID:   "esp-84:f7:03:12:34:56",
		MACAddress: "84:f7:03:12:34:56",
		Name:       "clear-me",
		Tags:       []string{},
		Aliases:    []string{},
	}
	require.NoError(t, store.SaveDevice(rec))

	rec.Name = ""
	require.NoError(t, store.SaveDevice(rec))

	got, err := store.GetDevice("esp-84:f7:03:12:34:56")
	require.NoError(t, err)
	assert.Equal(t, "", got.Name)
}
