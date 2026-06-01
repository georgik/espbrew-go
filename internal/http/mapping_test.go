package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMappingHandler_CreateBoundingBox(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	handler := NewMappingHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()

	// First create a device to reference
	device := &persistence.DeviceRecord{
		DeviceID:   "esp-aa:bb:cc:dd:ee:ff",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S3",
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}
	err = store.SaveDevice(device)
	require.NoError(t, err)

	// Create bounding box
	request := map[string]interface{}{
		"device_id": "esp-aa:bb:cc:dd:ee:ff",
		"camera_id": "cam-001",
		"bounds": map[string]float64{
			"x":      0.1,
			"y":      0.2,
			"width":  0.3,
			"height": 0.4,
		},
	}

	body, _ := json.Marshal(request)
	resp, err := http.Post(server.URL+"/api/v1/bounding_boxes", "application/json", bodyToReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	assert.NotEmpty(t, result["id"])
	assert.Equal(t, "esp-aa:bb:cc:dd:ee:ff", result["device_id"])
	assert.Equal(t, "cam-001", result["camera_id"])
}

func TestMappingHandler_CreateBoundingBoxInvalidBounds(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	handler := NewMappingHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()

	// Create device
	device := &persistence.DeviceRecord{
		DeviceID:   "esp-aa:bb:cc:dd:ee:ff",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}
	err = store.SaveDevice(device)
	require.NoError(t, err)

	// Invalid bounds (exceeds image)
	request := map[string]interface{}{
		"device_id": "esp-aa:bb:cc:dd:ee:ff",
		"camera_id": "cam-001",
		"bounds": map[string]float64{
			"x":      0.8,
			"y":      0.2,
			"width":  0.3, // 0.8 + 0.3 > 1.0
			"height": 0.4,
		},
	}

	body, _ := json.Marshal(request)
	resp, err := http.Post(server.URL+"/api/v1/bounding_boxes", "application/json", bodyToReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.NotEmpty(t, result["error"])
}

func TestMappingHandler_GetCameraBoxes(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	handler := NewMappingHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()

	// Create calibration
	calib := &persistence.CameraCalibration{
		ID:        "calib-cam-001",
		CameraID:  "cam-001",
		Version:   1,
		CreatedAt: time.Now(),
	}
	err = store.SaveCalibration(calib)
	require.NoError(t, err)

	// Create devices
	devices := []*persistence.DeviceRecord{
		{DeviceID: "esp-11:11:11:11:11:11", MACAddress: "11:11:11:11:11:11", ChipType: "ESP32-S3", FirstSeen: time.Now(), LastSeen: time.Now()},
		{DeviceID: "esp-22:22:22:22:22:22", MACAddress: "22:22:22:22:22:22", ChipType: "ESP32-S2", FirstSeen: time.Now(), LastSeen: time.Now()},
	}
	for _, d := range devices {
		err = store.SaveDevice(d)
		require.NoError(t, err)
	}

	// Create mappings
	mappings := []*persistence.DeviceBoundingBoxMapping{
		{ID: "bbox-1", DeviceID: "esp-11:11:11:11:11:11", CameraID: "cam-001", Bounds: persistence.BoundingBox{X: 0, Y: 0, Width: 0.5, Height: 0.5}, CalibrationVersion: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "bbox-2", DeviceID: "esp-22:22:22:22:22:22", CameraID: "cam-001", Bounds: persistence.BoundingBox{X: 0.5, Y: 0, Width: 0.5, Height: 0.5}, CalibrationVersion: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	for _, m := range mappings {
		err = store.SaveBoundingBox(m)
		require.NoError(t, err)
	}

	// Get camera boxes
	resp, err := http.Get(server.URL + "/api/v1/cameras/cam-001/boxes")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	assert.Equal(t, "cam-001", result["camera_id"])

	calibration := result["calibration"].(map[string]interface{})
	assert.Equal(t, float64(1), calibration["version"])

	mappingsResult := result["mappings"].([]interface{})
	assert.Len(t, mappingsResult, 2)
}

func TestMappingHandler_UpdateBoundingBox(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	handler := NewMappingHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()

	// Create device
	device := &persistence.DeviceRecord{
		DeviceID:   "esp-aa:bb:cc:dd:ee:ff",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}
	err = store.SaveDevice(device)
	require.NoError(t, err)

	// Create initial mapping
	mapping := &persistence.DeviceBoundingBoxMapping{
		ID:                 "bbox-1",
		DeviceID:           "esp-aa:bb:cc:dd:ee:ff",
		CameraID:           "cam-001",
		Bounds:             persistence.BoundingBox{X: 0.1, Y: 0.2, Width: 0.3, Height: 0.4},
		CalibrationVersion: 1,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	err = store.SaveBoundingBox(mapping)
	require.NoError(t, err)

	// Update with new bounds
	updateRequest := map[string]interface{}{
		"bounds": map[string]float64{
			"x":      0.5,
			"y":      0.6,
			"width":  0.2,
			"height": 0.2,
		},
	}

	body, _ := json.Marshal(updateRequest)
	req, _ := http.NewRequest("PUT", server.URL+"/api/v1/bounding_boxes/bbox-1", bodyToReader(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	bounds := result["bounds"].(map[string]interface{})
	assert.Equal(t, 0.5, bounds["x"])
	assert.Equal(t, 0.6, bounds["y"])
}

func TestMappingHandler_DeleteBoundingBox(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	handler := NewMappingHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()

	// Create device and mapping
	device := &persistence.DeviceRecord{
		DeviceID:   "esp-aa:bb:cc:dd:ee:ff",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}
	err = store.SaveDevice(device)
	require.NoError(t, err)

	mapping := &persistence.DeviceBoundingBoxMapping{
		ID:        "bbox-1",
		DeviceID:  "esp-aa:bb:cc:dd:ee:ff",
		CameraID:  "cam-001",
		Bounds:    persistence.BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.SaveBoundingBox(mapping)
	require.NoError(t, err)

	// Delete it
	req, _ := http.NewRequest("DELETE", server.URL+"/api/v1/bounding_boxes/bbox-1", nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify it's gone
	_, err = store.GetBoundingBox("bbox-1")
	assert.Error(t, err)
}

func TestMappingHandler_Calibration(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := persistence.Open(&persistence.Config{Path: dbPath})
	require.NoError(t, err)
	defer store.Close()

	handler := NewMappingHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()

	// Get calibration (doesn't exist yet)
	resp, err := http.Get(server.URL + "/api/v1/cameras/cam-001/calibration")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Create calibration
	createRequest := map[string]interface{}{
		"description": "Initial setup",
	}
	body, _ := json.Marshal(createRequest)
	resp, err = http.Post(server.URL+"/api/v1/cameras/cam-001/calibration", "application/json", bodyToReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(1), result["version"])
	assert.Equal(t, "Initial setup", result["description"])

	// Get calibration again
	resp, err = http.Get(server.URL + "/api/v1/cameras/cam-001/calibration")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(1), result["version"])

	// Create new version
	createRequest = map[string]interface{}{
		"description": "Camera moved",
	}
	body, _ = json.Marshal(createRequest)
	resp, err = http.Post(server.URL+"/api/v1/cameras/cam-001/calibration", "application/json", bodyToReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(2), result["version"])
	assert.Equal(t, "Camera moved", result["description"])
}

func bodyToReader(body []byte) *strings.Reader {
	return strings.NewReader(string(body))
}
