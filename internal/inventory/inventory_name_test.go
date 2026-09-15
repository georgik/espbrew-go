package inventory

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInventoryWithDBPath builds an Inventory backed by the given file.
func newInventoryWithDBPath(t *testing.T, dbPath string) *Inventory {
	t.Helper()
	inv := &Inventory{dbPath: dbPath, devices: map[string]*DeviceInventory{}}
	return inv
}

// TestUpdateNamePersistence verifies a name set via UpdateName is loaded back
// by a fresh Inventory instance (i.e. it survives "restarts").
func TestUpdateNamePersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), DevicesFileName)

	// Seed a record and set a name.
	inv := newInventoryWithDBPath(t, dbPath)
	inv.devices["esp-aa:bb:cc:dd:ee:ff"] = &DeviceInventory{
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		Tags:     []string{},
		Aliases:  []string{},
	}
	require.NoError(t, inv.UpdateName("esp-aa:bb:cc:dd:ee:ff", "living-room-brick"))

	// Fresh Inventory loaded from disk should carry the name.
	fresh := newInventoryWithDBPath(t, dbPath)
	require.NoError(t, fresh.load())

	dev, err := fresh.Get("esp-aa:bb:cc:dd:ee:ff")
	require.NoError(t, err)
	assert.Equal(t, "living-room-brick", dev.Name)
}

// TestUpdateNameClears verifies an empty name clears the stored name.
func TestUpdateNameClears(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), DevicesFileName)
	inv := newInventoryWithDBPath(t, dbPath)
	inv.devices["esp-aa:bb:cc:dd:ee:ff"] = &DeviceInventory{
		DeviceID: "esp-aa:bb:cc:dd:ee:ff",
		Name:     "living-room-brick",
		Tags:     []string{},
		Aliases:  []string{},
	}
	require.NoError(t, inv.UpdateName("esp-aa:bb:cc:dd:ee:ff", ""))

	dev, err := inv.Get("esp-aa:bb:cc:dd:ee:ff")
	require.NoError(t, err)
	assert.Equal(t, "", dev.Name)
}

// TestUpdateNameUnknownDevice verifies an error on an unknown device ID.
func TestUpdateNameUnknownDevice(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), DevicesFileName)
	inv := newInventoryWithDBPath(t, dbPath)
	err := inv.UpdateName("esp-nonexistent", "somename")
	assert.Error(t, err)
}
