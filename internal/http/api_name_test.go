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

// createFullTestRouter creates a router with the full set of device routes,
// including POST /devices, so the name round-trip can be exercised end-to-end.
func createFullTestRouter(handler *APIHandler) *mux.Router {
	router := mux.NewRouter()
	router.SkipClean(true)
	api := router.PathPrefix("/api/v1").Subrouter()
	api.SkipClean(true)
	api.HandleFunc("/devices", handler.handleDevices).Methods("GET")
	api.HandleFunc("/devices", handler.handleAddDevice).Methods("POST")
	api.HandleFunc("/devices/{id:.*}", handler.handleDeviceDetail).Methods("GET")
	api.HandleFunc("/devices/{id:.*}", handler.handleUpdateDevice).Methods("PUT", "PATCH")
	api.HandleFunc("/devices/{id:.*}", handler.handleDeleteDevice).Methods("DELETE")
	return router
}

func doReq(t *testing.T, router *mux.Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeDetail(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), "body: %s", w.Body.String())
	return m
}

// TestAPI_NameSetThroughPatch verifies PATCH persists the name and the detail
// returns it, mirroring exactly what the WASM client sends/receives.
func TestAPI_NameSetThroughPatch(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/name.db"))
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
	router := createFullTestRouter(handler)

	// Seed a device in the store.
	require.NoError(t, store.SaveDevice(&persistence.DeviceRecord{
		DeviceID:   "esp-84:f7:03:12:34:56",
		MACAddress: "84:f7:03:12:34:56",
		ChipType:   "ESP32-S3",
		Tags:       []string{},
		Aliases:    []string{},
	}))

	w := doReq(t, router, "PATCH", "/api/v1/devices/esp-84:f7:03:12:34:56", `{"name":"living-room-brick"}`)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	// The detail endpoint must return the name.
	w2 := doReq(t, router, "GET", "/api/v1/devices/esp-84:f7:03:12:34:56", "")
	require.Equal(t, http.StatusOK, w2.Code)
	detail := decodeDetail(t, w2)
	assert.Equal(t, "living-room-brick", detail["name"])

	// Persistence layer confirms it.
	rec, err := store.GetDevice("esp-84:f7:03:12:34:56")
	require.NoError(t, err)
	assert.Equal(t, "living-room-brick", rec.Name)
}

// TestAPI_NameClearThroughPatch verifies PATCH with an empty name clears it.
func TestAPI_NameClearThroughPatch(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/name-clear.db"))
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
	router := createFullTestRouter(handler)

	require.NoError(t, store.SaveDevice(&persistence.DeviceRecord{
		DeviceID:   "esp-84:f7:03:12:34:56",
		MACAddress: "84:f7:03:12:34:56",
		ChipType:   "ESP32-S3",
		Name:       "clear-me",
		Tags:       []string{},
		Aliases:    []string{},
	}))

	// Empty string should clear (not be ignored) because Name is a pointer.
	w := doReq(t, router, "PATCH", "/api/v1/devices/esp-84:f7:03:12:34:56", `{"name":""}`)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	detail := decodeDetail(t, doReq(t, router, "GET", "/api/v1/devices/esp-84:f7:03:12:34:56", ""))
	assert.Equal(t, "", detail["name"])

	rec, err := store.GetDevice("esp-84:f7:03:12:34:56")
	require.NoError(t, err)
	assert.Equal(t, "", rec.Name)
}

// TestAPI_OmittedNameUnchanged verifies PATCH without a name leaves it untouched.
func TestAPI_OmittedNameUnchanged(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/name-omit.db"))
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
	router := createFullTestRouter(handler)

	require.NoError(t, store.SaveDevice(&persistence.DeviceRecord{
		DeviceID:   "esp-84:f7:03:12:34:56",
		MACAddress: "84:f7:03:12:34:56",
		ChipType:   "ESP32-S3",
		Name:       "keep-me",
		Tags:       []string{},
		Aliases:    []string{},
	}))

	// Patch only the description; name must be preserved.
	w := doReq(t, router, "PATCH", "/api/v1/devices/esp-84:f7:03:12:34:56", `{"description":"changed"}`)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	detail := decodeDetail(t, doReq(t, router, "GET", "/api/v1/devices/esp-84:f7:03:12:34:56", ""))
	assert.Equal(t, "keep-me", detail["name"])
	assert.Equal(t, "changed", detail["description"])
}

// TestAPI_NameThroughCreate verifies POST /devices persists the name.
func TestAPI_NameThroughCreate(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/name-create.db"))
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
	router := createFullTestRouter(handler)

	w := doReq(t, router, "POST", "/api/v1/devices",
		`{"chip_type":"ESP32-S3","mac_address":"84:f7:03:12:34:56","board_model":"ESP32-S3-BOX-3","name":"living-room-brick"}`)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	rec, err := store.GetDevice("esp-84:f7:03:12:34:56")
	require.NoError(t, err)
	assert.Equal(t, "living-room-brick", rec.Name)
}

// TestAPI_UnprobedDeviceUpdateName verifies updating an in-memory-only device
// with a name creates a store record carrying the name.
func TestAPI_UnprobedDeviceUpdateName(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/name-unprobed.db"))
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
	router := createFullTestRouter(handler)

	// Register a memory-only device with no DeviceID (truly unprobed).
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:         "/dev/ttyUSB0",
		RealPath:     "/dev/serial/by-id/usb-1a86_USB_Single_Serial_1234-if00",
		VID:          0x4348,
		PID:          0x0027,
		SerialNumber: "",
		Status:       "available",
	})

	// Verify it exists in memory but not yet in the store.
	state := leader.State()
	require.Contains(t, state.Devices, "/dev/ttyUSB0")
	require.Empty(t, state.Devices["/dev/ttyUSB0"].DeviceID, "unprobed device has no DeviceID")
	_, err = store.GetDevice("/dev/ttyUSB0")
	require.Error(t, err, "unprobed device should not be in store yet")

	// PATCH the memory-only device with a name (and a MAC so a stable ID can be derived).
	w := doReq(t, router, "PATCH", "/api/v1/devices/%2Fdev%2FttyUSB0",
		`{"mac_address":"aa:bb:cc:dd:ee:ff","name":"unprobed-name"}`)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	// The record must be persisted with the name and the MAC-derived ID.
	rec, err := store.GetDevice("esp-aa:bb:cc:dd:ee:ff")
	require.NoError(t, err, "patch should have created a store record")
	assert.Equal(t, "unprobed-name", rec.Name, "name should be persisted")
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", rec.MACAddress, "MAC should be persisted")

	// In-memory state should now carry the derived DeviceID.
	state = leader.State()
	assert.Equal(t, "esp-aa:bb:cc:dd:ee:ff", state.Devices["/dev/ttyUSB0"].DeviceID,
		"in-memory DeviceID should be updated")
}

// TestAPI_DeviceListReturnsName ensures the list endpoint carries the name.
func TestAPI_DeviceListReturnsName(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/name-list.db"))
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
	router := createFullTestRouter(handler)

	require.NoError(t, store.SaveDevice(&persistence.DeviceRecord{
		DeviceID:   "esp-84:f7:03:12:34:56",
		MACAddress: "84:f7:03:12:34:56",
		ChipType:   "ESP32-S3",
		Name:       "living-room-brick",
		BoardModel: "ESP32-S3-BOX-3",
		Tags:       []string{},
		Aliases:    []string{},
	}))

	var w *httptest.ResponseRecorder
	// List devices (the list endpoint returns an array in {"devices": ...}).
	{
		req := httptest.NewRequest("GET", "/api/v1/devices", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
	require.Equal(t, http.StatusOK, w.Code)

	var devices []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &devices), "body: %s", w.Body.String())
	found := false
	for _, d := range devices {
		if d["device_id"] == "esp-84:f7:03:12:34:56" {
			found = true
			assert.Equal(t, "living-room-brick", d["name"])
		}
	}
	assert.True(t, found, "expected device in list")
}
