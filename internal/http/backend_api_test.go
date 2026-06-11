package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/gorilla/mux"
)

func TestHandleGetBackendConfig(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create test device with wokwi backend
	testDevice := &persistence.DeviceRecord{
		DeviceID:    "wokwi:esp32-s3",
		ChipType:    "ESP32-S3",
		Description: "Test Wokwi device",
		Backend:     "wokwi",
		LastPath:    "wokwi:esp32-s3",
		BackendConfig: &persistence.BackendConfigData{
			Wokwi: &persistence.WokwiConfigData{
				ChipType:    "ESP32-S3",
				DiagramJSON: `{"version":1,"parts":[{"type":"esp32-s3-devkitc-1","id":"chip","position":{"x":0,"y":0}}]}`,
			},
		},
	}
	if err := store.SaveDevice(testDevice); err != nil {
		t.Fatalf("Failed to save test device: %v", err)
	}

	node := cluster.NewLeaderNode("test", &cluster.LeaderConfig{}, store)
	handler := NewAPIHandler(node, store)

	t.Run("get existing backend config", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/devices/wokwi:esp32-s3/backend", nil)
		w := httptest.NewRecorder()

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/devices/{id}/backend", handler.handleGetBackendConfig).Methods("GET")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Status = %d, body: %s", w.Code, w.Body.String())
		}

		var resp BackendConfigResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.Backend != "wokwi" {
			t.Errorf("Backend = %q, want 'wokwi'", resp.Backend)
		}

		if resp.DeviceID != "wokwi:esp32-s3" {
			t.Errorf("DeviceID = %q, want 'wokwi:esp32-s3'", resp.DeviceID)
		}

		// BackendConfig comes as map due to JSON encoding
		wokwiCfg, ok := resp.BackendConfig.(map[string]interface{})
		if !ok {
			t.Fatalf("BackendConfig is not map, got %T: %+v", resp.BackendConfig, resp.BackendConfig)
		}

		if chipType, ok := wokwiCfg["chip_type"].(string); !ok || chipType != "ESP32-S3" {
			t.Errorf("ChipType = %v, want 'ESP32-S3'", wokwiCfg["chip_type"])
		}
	})

	t.Run("device not found", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/devices/nonexistent/backend", nil)
		w := httptest.NewRecorder()

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/devices/{id}/backend", handler.handleGetBackendConfig).Methods("GET")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}

func TestHandleSetBackendConfig(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create test device without backend config
	testDevice := &persistence.DeviceRecord{
		DeviceID:    "test-device",
		ChipType:    "ESP32-S3",
		Description: "Test device",
		Backend:     "physical",
		LastPath:    "/dev/ttyUSB0",
	}
	if err := store.SaveDevice(testDevice); err != nil {
		t.Fatalf("Failed to save test device: %v", err)
	}

	node := cluster.NewLeaderNode("test", &cluster.LeaderConfig{}, store)
	handler := NewAPIHandler(node, store)

	t.Run("set wokwi backend config", func(t *testing.T) {
		req := BackendConfigRequest{
			Backend: "wokwi",
			BackendConfig: map[string]interface{}{
				"chip_type":    "ESP32-S3",
				"diagram_json": `{"version":1,"parts":[{"type":"esp32-s3-devkitc-1"}]}`,
			},
		}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("PUT", "/api/v1/devices/test-device/backend", bytes.NewReader(body))
		w := httptest.NewRecorder()

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/devices/{id}/backend", handler.handleSetBackendConfig).Methods("PUT", "PATCH")
		router.ServeHTTP(w, httpReq)

		if w.Code != http.StatusOK {
			t.Fatalf("Status = %d, body: %s", w.Code, w.Body.String())
		}

		// Verify config was saved
		dev, err := store.GetDevice("test-device")
		if err != nil {
			t.Fatalf("Failed to get device: %v", err)
		}

		if dev.Backend != "wokwi" {
			t.Errorf("Backend = %q, want 'wokwi'", dev.Backend)
		}

		if dev.BackendConfig == nil || dev.BackendConfig.Wokwi == nil {
			t.Fatal("Wokwi config not saved")
		}

		if dev.BackendConfig.Wokwi.ChipType != "ESP32-S3" {
			t.Errorf("ChipType = %q, want 'ESP32-S3'", dev.BackendConfig.Wokwi.ChipType)
		}
	})

	t.Run("invalid backend type", func(t *testing.T) {
		// Create fresh device for this test
		freshDevice := &persistence.DeviceRecord{
			DeviceID: "test-device-invalid",
			ChipType: "ESP32",
			Backend:  "physical",
			LastPath: "/dev/ttyUSB1",
		}
		store.SaveDevice(freshDevice)

		req := BackendConfigRequest{
			Backend: "invalid",
		}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("PUT", "/api/v1/devices/test-device-invalid/backend", bytes.NewReader(body))
		w := httptest.NewRecorder()

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/devices/{id}/backend", handler.handleSetBackendConfig).Methods("PUT", "PATCH")
		router.ServeHTTP(w, httpReq)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}
