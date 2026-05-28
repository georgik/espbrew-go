package http

import (
	"bytes"
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

func TestDeviceDisableHandler_DisableDevice(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	master.Start(ctx)
	defer master.Stop()

	// Add a test device
	testDevice := &persistence.DeviceRecord{
		DeviceID:   "test-device-1",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32",
		LastPath:   "/dev/ttyUSB0",
		NodeID:     "test",
	}
	err = store.SaveDevice(testDevice)
	require.NoError(t, err)

	// Add device to cluster state manually
	master.State().Devices["/dev/ttyUSB0"] = &protocol.DeviceInfo{
		Path:         "/dev/ttyUSB0",
		DeviceID:     "test-device-1",
		ChipType:     "ESP32",
		SerialNumber: "aa:bb:cc:dd:ee:ff",
		NodeID:       "test",
		Status:       "available",
	}

	handler := NewDeviceDisableHandler(master, store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("disable device successfully", func(t *testing.T) {
		body := `{"reason": "maintenance", "client_id": "test-client"}`
		req := httptest.NewRequest("PUT", "/api/v1/devices/test-device-1/disable", strings.NewReader(body))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		assert.Equal(t, "disabled", resp["status"])
		assert.Equal(t, "test-device-1", resp["device_id"])

		// Verify device is disabled in persistence
		device, err := store.GetDevice("test-device-1")
		require.NoError(t, err)
		assert.True(t, device.Disabled)
		assert.Equal(t, "maintenance", device.DisabledReason)
		assert.Equal(t, "test-client", device.DisabledBy)

		// Verify device is disabled in cluster state
		state := master.State()
		dev := state.Devices["/dev/ttyUSB0"]
		assert.NotNil(t, dev)
		assert.True(t, dev.Disabled)
		assert.Equal(t, "disabled", dev.Status)
	})

	t.Run("cannot disable busy device", func(t *testing.T) {
		// Re-enable the device first
		store.SetDeviceDisabled("test-device-1", false, "", "")
		master.UpdateDeviceDisabled("test-device-1", false, "")

		// Set device to busy
		master.UpdateDeviceStatus("test-device-1", "busy")

		body := `{"reason": "test"}`
		req := httptest.NewRequest("PUT", "/api/v1/devices/test-device-1/disable", strings.NewReader(body))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		assert.Contains(t, resp["error"], "cannot disable device that is currently in use")
	})

	t.Run("disable with empty body", func(t *testing.T) {
		// Set device to available first
		master.UpdateDeviceStatus("test-device-1", "available")

		req := httptest.NewRequest("PUT", "/api/v1/devices/test-device-1/disable", strings.NewReader(""))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestDeviceDisableHandler_EnableDevice(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	master.Start(ctx)
	defer master.Stop()

	// Add a disabled test device
	testDevice := &persistence.DeviceRecord{
		DeviceID:       "test-device-2",
		MACAddress:     "11:22:33:44:55:66",
		ChipType:       "ESP32-S3",
		LastPath:       "/dev/ttyUSB1",
		NodeID:         "test",
		Disabled:       true,
		DisabledReason: "broken",
		DisabledBy:     "admin",
	}
	err = store.SaveDevice(testDevice)
	require.NoError(t, err)

	// Add disabled device to cluster state manually
	master.State().Devices["/dev/ttyUSB1"] = &protocol.DeviceInfo{
		Path:           "/dev/ttyUSB1",
		DeviceID:       "test-device-2",
		ChipType:       "ESP32-S3",
		SerialNumber:   "11:22:33:44:55:66",
		NodeID:         "test",
		Status:         "disabled",
		Disabled:       true,
		DisabledReason: "broken",
	}

	handler := NewDeviceDisableHandler(master, store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("enable disabled device", func(t *testing.T) {
		body := `{"client_id": "test-client"}`
		req := httptest.NewRequest("PUT", "/api/v1/devices/test-device-2/enable", strings.NewReader(body))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		assert.Equal(t, "enabled", resp["status"])
		assert.Equal(t, "test-device-2", resp["device_id"])

		// Verify device is enabled in persistence
		device, err := store.GetDevice("test-device-2")
		require.NoError(t, err)
		assert.False(t, device.Disabled)
		assert.Equal(t, "", device.DisabledReason)
		assert.Equal(t, "", device.DisabledBy)

		// Verify device is enabled in cluster state
		state := master.State()
		dev := state.Devices["/dev/ttyUSB1"]
		assert.NotNil(t, dev)
		assert.False(t, dev.Disabled)
		assert.Equal(t, "available", dev.Status)
	})

	t.Run("enable already enabled device", func(t *testing.T) {
		// Try to enable already enabled device
		body := `{}`
		req := httptest.NewRequest("PUT", "/api/v1/devices/test-device-2/enable", strings.NewReader(body))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		assert.Equal(t, "enabled", resp["status"])
	})
}

func TestDeviceDisableHandler_DisableByPath(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	master.Start(ctx)
	defer master.Stop()

	// Add test device with path
	testDevice := &persistence.DeviceRecord{
		DeviceID:   "test-device-3",
		MACAddress: "aa:aa:aa:aa:aa:aa",
		ChipType:   "ESP32-C3",
		LastPath:   "/dev/ttyUSB2",
		NodeID:     "test",
	}
	err = store.SaveDevice(testDevice)
	require.NoError(t, err)

	// Manually add to cluster state
	master.State().Devices["/dev/ttyUSB2"] = &protocol.DeviceInfo{
		Path:         "/dev/ttyUSB2",
		DeviceID:     "test-device-3",
		ChipType:     "ESP32-C3",
		SerialNumber: "aa:aa:aa:aa:aa:aa",
		NodeID:       "test",
		Status:       "available",
	}

	handler := NewDeviceDisableHandler(master, store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("disable by path", func(t *testing.T) {
		body := `{"reason": "test"}`
		req := httptest.NewRequest("PUT", "/api/v1/devices/test-device-3/disable", strings.NewReader(body))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		assert.Equal(t, "disabled", resp["status"])

		// Verify device is disabled
		device, err := store.GetDevice("test-device-3")
		require.NoError(t, err)
		assert.True(t, device.Disabled)
	})
}

func TestFlashAPI_DisabledDeviceBlocked(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	master.Start(ctx)
	defer master.Stop()

	// Add a disabled test device
	testDevice := &persistence.DeviceRecord{
		DeviceID:       "test-device-4",
		MACAddress:     "bb:bb:bb:bb:bb:bb",
		ChipType:       "ESP32",
		LastPath:       "/dev/ttyUSB3",
		NodeID:         "test",
		Disabled:       true,
		DisabledReason: "maintenance",
	}
	err = store.SaveDevice(testDevice)
	require.NoError(t, err)

	// Manually add disabled device to cluster state
	master.State().Devices["/dev/ttyUSB3"] = &protocol.DeviceInfo{
		Path:           "/dev/ttyUSB3",
		DeviceID:       "test-device-4",
		ChipType:       "ESP32",
		SerialNumber:   "bb:bb:bb:bb:bb:bb",
		NodeID:         "test",
		Status:         "disabled",
		Disabled:       true,
		DisabledReason: "maintenance",
	}

	progress := NewProgressHandler(master, nil)
	flashHandler := NewFlashHandler(master, t.TempDir(), progress)
	router := mux.NewRouter()
	flashHandler.RegisterRoutes(router)

	t.Run("flash rejected on disabled device", func(t *testing.T) {
		reqBody := FlashSubmitRequest{
			DevicePath: "/dev/ttyUSB3",
			FileID:     "test-file-id",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/flash", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var resp map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		assert.Contains(t, resp["error"], "device is disabled")
	})
}

func TestIsDeviceDisabled(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	master.Start(ctx)
	defer master.Stop()

	// Add test devices
	dev1 := &persistence.DeviceRecord{
		DeviceID:   "dev-1",
		MACAddress: "11:11:11:11:11:11",
		ChipType:   "ESP32",
		LastPath:   "/dev/ttyUSB0",
		NodeID:     "test",
	}
	dev2 := &persistence.DeviceRecord{
		DeviceID:       "dev-2",
		MACAddress:     "22:22:22:22:22:22",
		ChipType:       "ESP32",
		LastPath:       "/dev/ttyUSB1",
		NodeID:         "test",
		Disabled:       true,
		DisabledReason: "test",
	}
	store.SaveDevice(dev1)
	store.SaveDevice(dev2)

	// Add to cluster state
	master.State().Devices["/dev/ttyUSB0"] = &protocol.DeviceInfo{
		Path:     "/dev/ttyUSB0",
		DeviceID: "dev-1",
		Status:   "available",
		Disabled: false,
	}
	master.State().Devices["/dev/ttyUSB1"] = &protocol.DeviceInfo{
		Path:           "/dev/ttyUSB1",
		DeviceID:       "dev-2",
		Status:         "disabled",
		Disabled:       true,
		DisabledReason: "test",
	}

	state := master.State()

	t.Run("check enabled device", func(t *testing.T) {
		result := IsDeviceDisabled(state, "/dev/ttyUSB0")
		assert.False(t, result)

		result = IsDeviceDisabled(state, "dev-1")
		assert.False(t, result)
	})

	t.Run("check disabled device", func(t *testing.T) {
		result := IsDeviceDisabled(state, "/dev/ttyUSB1")
		assert.True(t, result)

		result = IsDeviceDisabled(state, "dev-2")
		assert.True(t, result)
	})

	t.Run("check non-existent device", func(t *testing.T) {
		result := IsDeviceDisabled(state, "/dev/ttyUSB99")
		assert.False(t, result)
	})
}

func TestAPIHandler_DeviceDetailReturnsDisabledFields(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	master.Start(ctx)
	defer master.Stop()

	// Create disabled device
	disabledDevice := &persistence.DeviceRecord{
		DeviceID:       "esp-11:22:33:44:55:66",
		MACAddress:     "11:22:33:44:55:66",
		ChipType:       "ESP32-S3",
		LastPath:       "/dev/ttyUSB0",
		NodeID:         "test",
		Disabled:       true,
		DisabledReason: "maintenance",
		DisabledBy:     "admin-user",
		DisabledAt:     time.Now().Add(-1 * time.Hour),
	}
	err = store.SaveDevice(disabledDevice)
	require.NoError(t, err)

	handler := NewAPIHandler(master, store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("disabled device detail returns disabled fields", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/devices/esp-11:22:33:44:55:66", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		// Verify disabled fields are present
		assert.True(t, resp["disabled"].(bool))
		assert.Equal(t, "maintenance", resp["disabled_reason"])
		assert.Equal(t, "admin-user", resp["disabled_by"])
		assert.NotNil(t, resp["disabled_at"])
	})

	t.Run("enabled device detail has no disabled fields", func(t *testing.T) {
		// Create enabled device
		enabledDevice := &persistence.DeviceRecord{
			DeviceID:   "esp-aa:bb:cc:dd:ee:ff",
			MACAddress: "aa:bb:cc:dd:ee:ff",
			ChipType:   "ESP32",
			LastPath:   "/dev/ttyUSB1",
			NodeID:     "test",
		}
		err = store.SaveDevice(enabledDevice)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/api/v1/devices/esp-aa:bb:cc:dd:ee:ff", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		// Verify disabled field is false or absent
		if disabled, ok := resp["disabled"]; ok {
			assert.False(t, disabled.(bool))
		}
		_, hasReason := resp["disabled_reason"]
		assert.False(t, hasReason, "enabled device should not have disabled_reason")
		_, hasBy := resp["disabled_by"]
		assert.False(t, hasBy, "enabled device should not have disabled_by")
	})
}
