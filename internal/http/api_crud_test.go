package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestRouter creates a router with the API handler for testing
func createTestRouter(handler *APIHandler) *mux.Router {
	router := mux.NewRouter()
	api := router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/devices", handler.handleDevices).Methods("GET")
	api.HandleFunc("/devices", handler.handleAddDevice).Methods("POST")
	api.HandleFunc("/devices/{id}", handler.handleDeviceDetail).Methods("GET")
	api.HandleFunc("/devices/{id}", handler.handleUpdateDevice).Methods("PUT", "PATCH")
	api.HandleFunc("/devices/{id}", handler.handleDeleteDevice).Methods("DELETE")
	return router
}

// TestAPI_MultipleDevicesUpdate tests that updating one device doesn't affect others
func TestAPI_MultipleDevicesUpdate(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
	}, store)

	ctx := context.Background()
	require.NoError(t, leader.Start(ctx))
	defer leader.Stop()

	handler := NewAPIHandler(leader, store)
	router := createTestRouter(handler)

	// Register multiple devices
	devices := []struct {
		path     string
		deviceID string
		chipType string
	}{
		{"/dev/ttyUSB0", "esp-device-1", "ESP32"},
		{"/dev/ttyUSB1", "esp-device-2", "ESP32-S2"},
		{"/dev/ttyUSB2", "esp-device-3", "ESP32-S3"},
	}

	for _, d := range devices {
		leader.RegisterDevice(&protocol.DeviceInfo{
			Path:   d.path,
			VID:    0x4348,
			PID:    0x0027,
			Status: "available",
		})
		leader.UpdateDeviceInfo(d.path, d.deviceID, d.chipType, "aa:bb:cc:dd:ee:0"+string(d.path[len(d.path)-1:]))
	}

	// Verify devices are in store before update
	allDevices, _ := store.ListDevices()
	assert.Equal(t, 3, len(allDevices), "Should have 3 devices in store")

	// Update only device-2
	req := httptest.NewRequest("PUT", "/api/v1/devices/esp-device-2",
		strings.NewReader(`{"chip_type":"ESP32-C3"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Update should succeed: %s", w.Body.String())

	// Verify: device-2 should be updated, others unchanged
	dev1, _ := store.GetDevice("esp-device-1")
	assert.Equal(t, "ESP32", dev1.ChipType, "Device 1 should not be affected")

	dev2, _ := store.GetDevice("esp-device-2")
	assert.Equal(t, "ESP32-C3", dev2.ChipType, "Device 2 should be updated")

	dev3, _ := store.GetDevice("esp-device-3")
	assert.Equal(t, "ESP32-S3", dev3.ChipType, "Device 3 should not be affected")

	// Verify in-memory state
	state := leader.State()
	assert.Equal(t, "ESP32", state.Devices["/dev/ttyUSB0"].ChipType)
	assert.Equal(t, "ESP32-C3", state.Devices["/dev/ttyUSB1"].ChipType)
	assert.Equal(t, "ESP32-S3", state.Devices["/dev/ttyUSB2"].ChipType)
}

// TestAPI_DeviceDelete tests deletion of a device
func TestAPI_DeviceDelete(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
	}, store)

	ctx := context.Background()
	require.NoError(t, leader.Start(ctx))
	defer leader.Stop()

	handler := NewAPIHandler(leader, store)
	router := createTestRouter(handler)

	// Register device
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0027,
		Status: "available",
	})
	leader.UpdateDeviceInfo("/dev/ttyUSB0", "esp-delete-test", "ESP32", "aa:bb:cc:dd:ee:ff")

	// Verify device exists
	_, err = store.GetDevice("esp-delete-test")
	require.NoError(t, err, "Device should exist before delete")

	// Verify in-memory
	_, exists := leader.State().Devices["/dev/ttyUSB0"]
	assert.True(t, exists, "Device should be in memory before delete")

	// Delete device
	req := httptest.NewRequest("DELETE", "/api/v1/devices/esp-delete-test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify deleted from store
	_, err = store.GetDevice("esp-delete-test")
	assert.Error(t, err, "Device should be deleted from store")

	// Verify deleted from memory
	_, exists = leader.State().Devices["/dev/ttyUSB0"]
	assert.False(t, exists, "Device should be removed from memory")
}
