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
	"github.com/stretchr/testify/assert"
)

func TestAPIHandler_CameraPreviewFlag(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	master.Start(ctx)
	defer master.Stop()

	// Register a test camera
	testCamera := &protocol.CameraInfo{
		ID:      "test-camera-uuid",
		Name:    "Test Camera",
		Path:    "/dev/video0",
		Backend: "v4l2",
		Status:  "available",
		NodeID:  "test-node",
	}
	master.RegisterCamera(testCamera)

	handler := NewAPIHandler(master, store)

	t.Run("preview=true returns image directly", func(t *testing.T) {
		body := `{
			"camera_id": "test-camera-uuid",
			"width": 640,
			"height": 480,
			"format": "jpg",
			"quality": 75,
			"preview": true
		}`
		req := httptest.NewRequest("POST", "/api/v1/cameras/capture", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.handleCameraCapture(w, req)

		// For preview, should return image data directly (JPEG)
		// Status should be OK (200) with image data
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "image/jpg", w.Header().Get("Content-Type"))

		// Verify we got JPEG data (JPEG starts with FF D8)
		bodyBytes := w.Body.Bytes()
		assert.True(t, len(bodyBytes) > 2, "Should have image data")
		assert.Equal(t, []byte{0xFF, 0xD8}, bodyBytes[:2], "Should start with JPEG magic bytes")

		// Verify no JSON response (image data returned directly)
		assert.NotContains(t, w.Body.String(), "camera_id")
	})

	t.Run("preview=false saves to gallery", func(t *testing.T) {
		body := `{
			"camera_id": "test-camera-uuid",
			"width": 640,
			"height": 480,
			"format": "jpg",
			"quality": 75,
			"preview": false
		}`
		req := httptest.NewRequest("POST", "/api/v1/cameras/capture", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.handleCameraCapture(w, req)

		// Should save and return JSON response with path
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var resp map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "success", resp["status"])
		assert.NotEmpty(t, resp["path"], "Should have file path")
		assert.Contains(t, resp["path"].(string), "/captures/", "Path should contain captures directory")
	})
}

func TestCameraCaptureRequest_PreviewFlag(t *testing.T) {
	t.Run("preview flag is properly parsed", func(t *testing.T) {
		body := `{
			"camera_id": "test-camera",
			"width": 640,
			"height": 480,
			"format": "jpg",
			"quality": 75,
			"preview": true
		}`
		var req CameraCaptureRequest
		err := json.NewDecoder(strings.NewReader(body)).Decode(&req)

		assert.NoError(t, err)
		assert.True(t, req.Preview)
		assert.Equal(t, "test-camera", req.CameraID)
		assert.Equal(t, uint32(640), req.Width)
		assert.Equal(t, uint32(480), req.Height)
	})

	t.Run("preview defaults to false when omitted", func(t *testing.T) {
		body := `{
			"camera_id": "test-camera",
			"width": 640,
			"height": 480
		}`
		var req CameraCaptureRequest
		err := json.NewDecoder(strings.NewReader(body)).Decode(&req)

		assert.NoError(t, err)
		assert.False(t, req.Preview)
	})
}

func TestAPIHandler_CameraNotFound(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	handler := NewAPIHandler(master, store)

	body := `{
		"camera_id": "non-existent-camera",
		"width": 640,
		"height": 480,
		"preview": true
	}`
	req := httptest.NewRequest("POST", "/api/v1/cameras/capture", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCameraCapture(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "Camera not found", resp["error"])
}
