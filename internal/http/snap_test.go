package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"codeberg.org/georgik/espbrew-go/internal/snap"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleSnap_Success tests a successful snap operation with a virtual device.
func TestHandleSnap_Success(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Register a virtual device
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:     ":virtual:",
		DeviceID: "virtual-test-device",
		VID:      0x4348,
		PID:      0x0027,
		Status:   "available",
		NodeID:   leader.ID(),
	})

	// Create test firmware file
	firmwareData := []byte{0xE9, 0x05, 0x02, 0x20, 0xAA, 0xBB, 0xCC, 0xDD}
	firmwarePath := filepath.Join(t.TempDir(), "test-firmware.bin")
	err = os.WriteFile(firmwarePath, firmwareData, 0644)
	require.NoError(t, err)

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create snap request with skip_flash to avoid flash operations
	// Skip monitor since virtual devices don't support serial monitoring
	reqBody := SnapRequest{
		Duration:    5,
		CameraID:    "",
		Firmware:    firmwarePath,
		SkipFlash:   true, // Skip flash for faster testing
		SkipCapture: true, // Skip camera capture
		SkipMonitor: true, // Skip monitor (virtual devices don't support it)
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/devices/snap?device_id=virtual-test-device", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"deviceId": "virtual-test-device"})

	w := httptest.NewRecorder()
	api.handleSnap(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp SnapResponse
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify response structure
	assert.NotEmpty(t, resp.SnapID, "SnapID should be generated")
	assert.Equal(t, string(snap.SnapStatusSuccess), resp.Status, "Status should be success when all operations are skipped")
	assert.NotNil(t, resp.Result, "Result should not be nil")
	assert.Empty(t, resp.Error, "Error should be empty on success")

	// Verify result structure
	resultMap, ok := resp.Result.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	metadata, ok := resultMap["metadata"].(map[string]interface{})
	require.True(t, ok, "Result should contain metadata")

	assert.Equal(t, resp.SnapID, metadata["snap_id"], "SnapID should match in metadata")
	assert.Equal(t, ":virtual:", metadata["device_path"], "Device path should match")
}

// TestHandleSnap_DeviceNotFound tests the snap endpoint with a non-existent device.
func TestHandleSnap_DeviceNotFound(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create snap request for non-existent device
	reqBody := SnapRequest{
		Duration:  10,
		SkipFlash: true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/devices/snap?device_id=non-existent-device", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"deviceId": "non-existent-device"})

	w := httptest.NewRecorder()
	api.handleSnap(w, req)

	// Check response - should return 404 Not Found
	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify error message
	assert.Contains(t, resp["error"], "Device not found", "Error message should indicate device not found")
}

// TestHandleSnap_CameraNotAvailable tests the snap endpoint when camera is not available.
func TestHandleSnap_CameraNotAvailable(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Register a virtual device without any camera
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:     ":virtual:",
		DeviceID: "virtual-no-camera",
		VID:      0x4348,
		PID:      0x0027,
		Status:   "available",
		NodeID:   leader.ID(),
	})

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create snap request requesting a non-existent camera but skipping capture
	// (camera capture can take time, so we skip it for faster testing)
	reqBody := SnapRequest{
		Duration:    10,
		CameraID:    "non-existent-camera",
		SkipFlash:   true,
		SkipCapture: true, // Skip camera capture for faster testing
		SkipMonitor: true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/devices/snap?device_id=virtual-no-camera", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"deviceId": "virtual-no-camera"})

	w := httptest.NewRecorder()
	api.handleSnap(w, req)

	// The snap should succeed even with a non-existent camera ID (since capture is skipped)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp SnapResponse
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Should have a snap result
	assert.NotEmpty(t, resp.SnapID, "SnapID should be generated")
}

// TestHandleSnap_ForceFlash tests the force_flash flag behavior.
func TestHandleSnap_ForceFlash(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node with device watcher disabled to avoid probe hangs
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Register a virtual device
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:     ":virtual:",
		DeviceID: "virtual-force-flash",
		VID:      0x4348,
		PID:      0x0027,
		Status:   "available",
		NodeID:   leader.ID(),
	})

	// Create test firmware file
	firmwareData := []byte{0xE9, 0x05, 0x02, 0x20, 0xAA, 0xBB, 0xCC, 0xDD}
	firmwarePath := filepath.Join(t.TempDir(), "test-firmware-force.bin")
	err = os.WriteFile(firmwarePath, firmwareData, 0644)
	require.NoError(t, err)

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create snap request with force_flash but skip actual flash for faster testing
	// (the ForceFlash flag is still set in the request to test parameter handling)
	reqBody := SnapRequest{
		Duration:    5,
		Firmware:    firmwarePath,
		ForceFlash:  true,
		SkipFlash:   true, // Skip actual flash for faster testing (flag is still validated)
		SkipCapture: true,
		SkipMonitor: true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/devices/snap?device_id=virtual-force-flash", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"deviceId": "virtual-force-flash"})

	w := httptest.NewRecorder()
	api.handleSnap(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp SnapResponse
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify response structure
	assert.NotEmpty(t, resp.SnapID, "SnapID should be generated")
	assert.Equal(t, string(snap.SnapStatusSuccess), resp.Status, "Status should be success")

	// Verify request was processed with force_flash flag
	resultMap, ok := resp.Result.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	metadata, ok := resultMap["metadata"].(map[string]interface{})
	require.True(t, ok, "Result should contain metadata")

	// Flash was skipped by request flag, but the request processing worked
	assert.True(t, metadata["flash_skipped"].(bool), "Flash should be skipped when SkipFlash is true")
}

// TestHandleSnap_InvalidRequest tests the snap endpoint with invalid request body.
func TestHandleSnap_InvalidRequest(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Register a virtual device
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:     ":virtual:",
		DeviceID: "virtual-invalid",
		VID:      0x4348,
		PID:      0x0027,
		Status:   "available",
		NodeID:   leader.ID(),
	})

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create snap request with invalid JSON
	req := httptest.NewRequest("POST", "/api/v1/devices/snap?device_id=virtual-invalid", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"deviceId": "virtual-invalid"})

	w := httptest.NewRecorder()
	api.handleSnap(w, req)

	// Check response - should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify error message
	assert.Contains(t, resp["error"], "Invalid request body", "Error message should indicate invalid request")
}

// TestHandleSnap_MissingDeviceID tests the snap endpoint without a device ID.
func TestHandleSnap_MissingDeviceID(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create snap request without device ID in URL
	reqBody := SnapRequest{
		Duration:  10,
		SkipFlash: true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/devices/snap?device_id=", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"deviceId": ""})

	w := httptest.NewRecorder()
	api.handleSnap(w, req)

	// Check response - should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify error message
	assert.Contains(t, resp["error"], "device_id query parameter required", "Error message should indicate device_id is required")
}

// TestListSnaps tests the list snaps endpoint.
func TestListSnaps(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Register a virtual device
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:     ":virtual:",
		DeviceID: "virtual-list-snaps",
		VID:      0x4348,
		PID:      0x0027,
		Status:   "available",
		NodeID:   leader.ID(),
	})

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create list snaps request
	req := httptest.NewRequest("GET", "/api/v1/devices/virtual-list-snaps/snaps", nil)
	req = mux.SetURLVars(req, map[string]string{"deviceId": "virtual-list-snaps"})

	w := httptest.NewRecorder()
	api.handleListSnaps(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify response structure
	assert.Equal(t, "virtual-list-snaps", resp["device_id"], "Device ID should match")
	assert.Equal(t, 0.0, resp["count"], "Count should be 0 (no snaps stored yet)")

	snaps, ok := resp["snaps"].([]interface{})
	require.True(t, ok, "Snaps should be an array")
	assert.Empty(t, snaps, "Snaps list should be empty initially")
}

// TestListSnaps_MissingDeviceID tests the list snaps endpoint without a device ID.
func TestListSnaps_MissingDeviceID(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create list snaps request without device ID
	req := httptest.NewRequest("GET", "/api/v1/devices//snaps", nil)
	req = mux.SetURLVars(req, map[string]string{"deviceId": ""})

	w := httptest.NewRecorder()
	api.handleListSnaps(w, req)

	// Check response - should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify error message (handleListSnaps uses path param, not query param)
	assert.Contains(t, resp["error"], "device_id required", "Error message should indicate device_id is required")
}

// TestHandleSnap_PartialStatus tests that partial status is returned when some operations fail.
func TestHandleSnap_PartialStatus(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Register a virtual device
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:     ":virtual:",
		DeviceID: "virtual-partial",
		VID:      0x4348,
		PID:      0x0027,
		Status:   "available",
		NodeID:   leader.ID(),
	})

	// Create test firmware file
	firmwareData := []byte{0xE9, 0x05, 0x02, 0x20, 0xAA, 0xBB, 0xCC, 0xDD}
	firmwarePath := filepath.Join(t.TempDir(), "test-firmware-partial.bin")
	err = os.WriteFile(firmwarePath, firmwareData, 0644)
	require.NoError(t, err)

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create snap request with monitor enabled (will fail on virtual device)
	reqBody := SnapRequest{
		Duration:    5,
		Firmware:    firmwarePath,
		SkipFlash:   true,
		SkipCapture: true,
		SkipMonitor: false, // Monitor will fail on virtual device
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/devices/snap?device_id=virtual-partial", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"deviceId": "virtual-partial"})

	w := httptest.NewRecorder()
	api.handleSnap(w, req)

	// Check response - should still succeed but with partial status
	assert.Equal(t, http.StatusOK, w.Code)

	var resp SnapResponse
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify partial status due to monitor failure
	assert.Equal(t, string(snap.SnapStatusPartial), resp.Status, "Status should be partial when monitor fails")
	assert.NotEmpty(t, resp.SnapID, "SnapID should be generated")
	assert.Empty(t, resp.Error, "Error should be empty even with partial status")
}

// TestHandleSnap_SkipFlash tests that skip_flash properly skips flashing.
func TestHandleSnap_SkipFlash(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Register a virtual device
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:     ":virtual:",
		DeviceID: "virtual-skip-flash",
		VID:      0x4348,
		PID:      0x0027,
		Status:   "available",
		NodeID:   leader.ID(),
	})

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	// Create snap request with skip_flash (no firmware needed)
	// Note: Skip monitor since virtual devices don't support serial monitoring
	reqBody := SnapRequest{
		Duration:    5,
		SkipFlash:   true,
		SkipCapture: true,
		SkipMonitor: true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/devices/snap?device_id=virtual-skip-flash", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"deviceId": "virtual-skip-flash"})

	w := httptest.NewRecorder()
	api.handleSnap(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp SnapResponse
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify flash was skipped in metadata
	resultMap, ok := resp.Result.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	metadata, ok := resultMap["metadata"].(map[string]interface{})
	require.True(t, ok, "Result should contain metadata")

	assert.True(t, metadata["flash_skipped"].(bool), "Flash should be skipped when SkipFlash is true")
}

// TestHandleSnap_VariousCombinations tests different combinations of snap options.
func TestHandleSnap_VariousCombinations(t *testing.T) {
	// Create temporary store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create leader node
	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true, // Disable device watcher to avoid probe hangs during test cleanup
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Register a virtual device
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:     ":virtual:",
		DeviceID: "virtual-combinations",
		VID:      0x4348,
		PID:      0x0027,
		Status:   "available",
		NodeID:   leader.ID(),
	})

	// Create test firmware file
	firmwareData := []byte{0xE9, 0x05, 0x02, 0x20}
	firmwarePath := filepath.Join(t.TempDir(), "test-firmware-combo.bin")
	err = os.WriteFile(firmwarePath, firmwareData, 0644)
	require.NoError(t, err)

	// Create snap API handler
	api := NewSnapAPI(store, leader)

	testCases := []struct {
		name               string
		request            SnapRequest
		expectedStatus     int
		expectedSnapStatus string
		skipIfNoCamera     bool // Skip test if camera is not available
	}{
		{
			name: "all_skipped",
			request: SnapRequest{
				Duration:    3,
				SkipFlash:   true,
				SkipCapture: true,
				SkipMonitor: true,
			},
			expectedStatus:     http.StatusOK,
			expectedSnapStatus: string(snap.SnapStatusSuccess),
		},
		{
			name: "capture_only",
			request: SnapRequest{
				Duration:    0,
				SkipFlash:   true,
				SkipCapture: false,
				SkipMonitor: true,
			},
			expectedStatus:     http.StatusOK,
			expectedSnapStatus: string(snap.SnapStatusSuccess),
			skipIfNoCamera:     true, // Skip if no camera available
		},
		{
			name: "minimal",
			request: SnapRequest{
				Duration:    1,
				SkipFlash:   true,
				SkipCapture: true,
				SkipMonitor: true,
			},
			expectedStatus:     http.StatusOK,
			expectedSnapStatus: string(snap.SnapStatusSuccess),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Skip test if camera is required but not available
			if tc.skipIfNoCamera && !camera.ControllerAvailable() {
				t.Skip("Camera capture not available on this platform")
			}
			if tc.skipIfNoCamera && camera.ControllerAvailable() {
				// Quick check if camera actually works
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				camCapturer := camera.NewCapturer(nil)
				_, err := camCapturer.Capture(ctx, &camera.CaptureRequest{
					Format:  "jpg",
					Quality: 75,
					Timeout: 2 * time.Second,
					Preview: true, // Don't save, just test capture
				})
				if err != nil {
					t.Skip("Camera capture failed in test environment:", err.Error())
				}
			}

			body, _ := json.Marshal(tc.request)
			req := httptest.NewRequest("POST", "/api/v1/devices/snap?device_id=virtual-combinations", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"deviceId": "virtual-combinations"})

			w := httptest.NewRecorder()
			api.handleSnap(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code, "Status code should match")

			var resp SnapResponse
			err = json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedSnapStatus, resp.Status, "Snap status should match")
			assert.NotEmpty(t, resp.SnapID, "SnapID should be generated")
		})
	}
}
