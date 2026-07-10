package http

import (
	"context"
	"encoding/json"
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
	router.SkipClean(true) // Don't clean paths, allow encoded slashes
	api := router.PathPrefix("/api/v1").Subrouter()
	api.SkipClean(true)
	api.HandleFunc("/devices", handler.handleDevices).Methods("GET")
	api.HandleFunc("/devices", handler.handleAddDevice).Methods("POST")
	// Use {id:.*} to match paths with slashes (e.g., /dev/ttyUSB0)
	api.HandleFunc("/devices/{id:.*}", handler.handleDeviceDetail).Methods("GET")
	api.HandleFunc("/devices/{id:.*}", handler.handleUpdateDevice).Methods("PUT", "PATCH")
	api.HandleFunc("/devices/{id:.*}", handler.handleDeleteDevice).Methods("DELETE")
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
		DisableVirtual:     true,
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
		DisableVirtual:     true,
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
	assert.True(t, leader.DeviceExists("/dev/ttyUSB0"), "Device should be in memory before delete")

	// Delete device
	req := httptest.NewRequest("DELETE", "/api/v1/devices/esp-delete-test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify deleted from store
	_, err = store.GetDevice("esp-delete-test")
	assert.Error(t, err, "Device should be deleted from store")

	// Verify deleted from memory
	assert.False(t, leader.DeviceExists("/dev/ttyUSB0"), "Device should be removed from memory")
}

// TestAPI_DeviceForget tests forgetting an unidentified device by path
func TestAPI_DeviceForget(t *testing.T) {
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
		DisableVirtual:     true,
	}, store)

	ctx := context.Background()
	require.NoError(t, leader.Start(ctx))
	defer leader.Stop()

	handler := NewAPIHandler(leader, store)
	router := mux.NewRouter()
	api := router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/devices/forgot/{path:.*}", handler.handleForgetDevice).Methods("DELETE")

	// Register unidentified device (no device_id)
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyACM0",
		VID:    0x4348,
		PID:    0x0028,
		Status: "available",
	})

	// Verify device exists in memory
	assert.True(t, leader.DeviceExists("/dev/ttyACM0"), "Device should be in memory before forget")

	// Forget device
	req := httptest.NewRequest("DELETE", "/api/v1/devices/forgot/dev/ttyACM0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify removed from memory
	assert.False(t, leader.DeviceExists("/dev/ttyACM0"), "Device should be removed from memory")

	// Verify response contains status
	assert.Contains(t, w.Body.String(), "forgotten")
}

// TestAPI_DeviceForget_NotFound tests forgetting a non-existent device
func TestAPI_DeviceForget_NotFound(t *testing.T) {
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
		DisableVirtual:     true,
	}, store)

	ctx := context.Background()
	require.NoError(t, leader.Start(ctx))
	defer leader.Stop()

	handler := NewAPIHandler(leader, store)
	router := mux.NewRouter()
	api := router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/devices/forgot/{path:.*}", handler.handleForgetDevice).Methods("DELETE")

	// Try to forget non-existent device
	req := httptest.NewRequest("DELETE", "/api/v1/devices/forgot/dev/ttyUSB999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestAPI_DeviceHardwareIdentification tests that VID/PID/SerialNumber are persisted and returned
func TestAPI_DeviceHardwareIdentification(t *testing.T) {
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
		DisableVirtual:     true,
	}, store)

	ctx := context.Background()
	require.NoError(t, leader.Start(ctx))
	defer leader.Stop()

	handler := NewAPIHandler(leader, store)
	router := createTestRouter(handler)

	// Simulate device discovery with hardware IDs (from /dev/by-id)
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:         "/dev/ttyACM0",
		VID:          0x303a,
		PID:          0x1001,
		SerialNumber: "30:30:f9:5a:a3:a0",
		Status:       "available",
	})

	// Simulate successful probe - this updates device with MAC and chip type
	leader.UpdateDeviceInfo("/dev/ttyACM0", "esp-30:30:f9:5a:a3:a0", "ESP32-S3", "30:30:f9:5a:a3:a0")

	// Verify device in persistence has all hardware identification fields
	dev, err := store.GetDevice("esp-30:30:f9:5a:a3:a0")
	require.NoError(t, err, "Device should exist in persistence")

	assert.Equal(t, uint16(0x303a), dev.VID, "VID should be persisted")
	assert.Equal(t, uint16(0x1001), dev.PID, "PID should be persisted")
	assert.Equal(t, "30:30:f9:5a:a3:a0", dev.SerialNumber, "SerialNumber should be persisted")
	assert.Equal(t, "ESP32-S3", dev.ChipType, "ChipType should be set")
	assert.Equal(t, "/dev/ttyACM0", dev.LastPath, "LastPath should be set")

	// Verify API returns hardware identification fields
	req := httptest.NewRequest("GET", "/api/v1/devices", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Response should be valid JSON")
	require.Equal(t, 1, len(response), "Should have 1 device")

	deviceResp := response[0]
	assert.Equal(t, "esp-30:30:f9:5a:a3:a0", deviceResp["device_id"], "Device ID should match")
	assert.Equal(t, "0x303a", deviceResp["vid"], "VID should be returned in hex format")
	assert.Equal(t, "0x1001", deviceResp["pid"], "PID should be returned in hex format")
	assert.Equal(t, "30:30:f9:5a:a3:a0", deviceResp["serial"], "Serial should be returned")
	assert.Equal(t, "ESP32-S3", deviceResp["chip_type"], "Chip type should be returned")
}

// TestAPI_DeviceUpdatePreservesHardwareID tests that updating a device doesn't lose VID/PID/SerialNumber
func TestAPI_DeviceUpdatePreservesHardwareID(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create device with hardware identification
	err = store.SaveDevice(&persistence.DeviceRecord{
		DeviceID:     "esp-test-hardware",
		MACAddress:   "aa:bb:cc:dd:ee:ff",
		ChipType:     "ESP32",
		VID:          0x303a,
		PID:          0x1001,
		SerialNumber: "30:30:F9:5A:A3:A0",
		Manufacturer: "Espressif",
		Product:      "ESP32-S3",
		FirstSeen:    time.Now(),
		LastSeen:     time.Now(),
		LastPath:     "/dev/ttyUSB0",
		NodeID:       "test-node",
	})
	require.NoError(t, err)

	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		DisableVirtual:     true,
	}, store)

	ctx := context.Background()
	require.NoError(t, leader.Start(ctx))
	defer leader.Stop()

	handler := NewAPIHandler(leader, store)
	router := createTestRouter(handler)

	// Update device (change chip type)
	req := httptest.NewRequest("PATCH", "/api/v1/devices/esp-test-hardware",
		strings.NewReader(`{"chip_type":"ESP32-S3","description":"Updated description"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Update should succeed")

	// Verify hardware identification fields are preserved
	dev, err := store.GetDevice("esp-test-hardware")
	require.NoError(t, err)

	assert.Equal(t, uint16(0x303a), dev.VID, "VID should be preserved after update")
	assert.Equal(t, uint16(0x1001), dev.PID, "PID should be preserved after update")
	assert.Equal(t, "30:30:F9:5A:A3:A0", dev.SerialNumber, "SerialNumber should be preserved after update")
	assert.Equal(t, "Espressif", dev.Manufacturer, "Manufacturer should be preserved after update")
	assert.Equal(t, "ESP32-S3", dev.ChipType, "ChipType should be updated")
	assert.Equal(t, "Updated description", dev.Description, "Description should be updated")
}

// TestAPI_UnprobedDeviceUpdate tests updating a device that exists only in memory (unprobed)
// This verifies the fix for the duplicate device bug
func TestAPI_UnprobedDeviceUpdate(t *testing.T) {
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
		DisableVirtual:     true,
	}, store)

	ctx := context.Background()
	require.NoError(t, leader.Start(ctx))
	defer leader.Stop()

	handler := NewAPIHandler(leader, store)
	router := createTestRouter(handler)

	// Register an unprobed device (exists in memory only, no DeviceID)
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:         "/dev/ttyUSB0",
		RealPath:     "/dev/serial/by-id/usb-1a86_USB_Single_Serial_5971084553-if00",
		VID:          0x1a86,
		PID:          0x7523,
		SerialNumber: "",
		Status:       "available",
	})

	// Verify device exists in memory but not in store
	state := leader.State()
	require.Contains(t, state.Devices, "/dev/ttyUSB0", "Device should be in memory")
	assert.Empty(t, state.Devices["/dev/ttyUSB0"].DeviceID, "Unprobed device has no DeviceID")

	// Try to get from store - should not exist
	_, err = store.GetDevice("esp-aa:bb:cc:dd:ee:ff")
	assert.Error(t, err, "Unprobed device should not be in store")

	// Update the unprobed device with chip type and MAC
	// URL-encode the path (%2F = /)
	req := httptest.NewRequest("PATCH", "/api/v1/devices/%2Fdev%2FttyUSB0",
		strings.NewReader(`{"chip_type":"ESP32-S3","mac_address":"aa:bb:cc:dd:ee:ff","aliases":["test-esp"]}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Update should succeed: %s", w.Body.String())

	// Parse response
	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Verify DeviceID was generated from MAC
	deviceID, ok := resp["device_id"].(string)
	require.True(t, ok, "Response should contain device_id")
	assert.Equal(t, "esp-aa:bb:cc:dd:ee:ff", deviceID, "DeviceID should be generated from MAC")

	// Verify device is now in store
	dev, err := store.GetDevice(deviceID)
	require.NoError(t, err, "Device should now exist in store")
	assert.Equal(t, "ESP32-S3", dev.ChipType, "ChipType should be saved")
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", dev.MACAddress, "MAC should be saved")
	assert.Equal(t, []string{"test-esp"}, dev.Aliases, "Aliases should be saved")

	// Verify in-memory state was updated with the DeviceID
	state = leader.State()
	assert.Equal(t, deviceID, state.Devices["/dev/ttyUSB0"].DeviceID, "In-memory DeviceID should be updated")
	assert.Equal(t, "ESP32-S3", state.Devices["/dev/ttyUSB0"].ChipType, "In-memory ChipType should be updated")

	// Verify no duplicate devices - only one device should exist
	allDevices, _ := store.ListDevices()
	assert.Equal(t, 1, len(allDevices), "Should have exactly 1 device in store (no duplicates)")
}
