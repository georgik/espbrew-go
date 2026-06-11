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

func TestHandleSetBackendConfig_URIStyleDeviceID(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Create test device with URI-style ID
	device := &persistence.DeviceRecord{
		DeviceID: "wokwi:esp32-s3",
		ChipType: "ESP32-S3",
		Backend:  "wokwi",
		LastPath: "wokwi:esp32-s3",
	}

	if err := store.SaveDevice(device); err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	node := cluster.NewLeaderNode("test", &cluster.LeaderConfig{}, store)
	handler := NewAPIHandler(node, store)

	reqBody := BackendConfigRequest{
		Backend: "wokwi",
		BackendConfig: map[string]interface{}{
			"chip_type": "ESP32-S3",
			"diagram_json": `{
				"version": 1,
				"author": "Uri Shaked",
				"editor": "wokwi",
				"parts": [
					{
						"type": "board-esp32-s3-box-3",
						"id": "esp",
						"attrs": {}
					}
				]
			}`,
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/v1/devices/wokwi:esp32-s3/backend", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/devices/{id}/backend", handler.handleSetBackendConfig).Methods("PUT", "PATCH")
	router.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify device was updated
	updated, err := store.GetDevice("wokwi:esp32-s3")
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}

	if updated.Backend != "wokwi" {
		t.Errorf("Expected backend wokwi, got %s", updated.Backend)
	}

	if updated.BackendConfig == nil {
		t.Fatal("Expected backend config, got nil")
	}

	if updated.BackendConfig.Wokwi == nil {
		t.Fatal("Expected Wokwi config, got nil")
	}

	if updated.BackendConfig.Wokwi.ChipType != "ESP32-S3" {
		t.Errorf("Expected chip_type ESP32-S3, got %s", updated.BackendConfig.Wokwi.ChipType)
	}

	// Verify diagram JSON was saved
	if updated.BackendConfig.Wokwi.DiagramJSON == "" {
		t.Error("Expected diagram_json to be saved, got empty string")
	}
}

func TestHandleGetBackendConfig_URIStyleDeviceID(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Create test device with URI-style ID
	device := &persistence.DeviceRecord{
		DeviceID: "wokwi:esp32-c3",
		ChipType: "ESP32-C3",
		Backend:  "wokwi",
		LastPath: "wokwi:esp32-c3",
		BackendConfig: &persistence.BackendConfigData{
			Wokwi: &persistence.WokwiConfigData{
				ChipType:    "ESP32-C3",
				DiagramJSON: `{"version":1}`,
			},
		},
	}

	if err := store.SaveDevice(device); err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	node := cluster.NewLeaderNode("test", &cluster.LeaderConfig{}, store)
	handler := NewAPIHandler(node, store)

	req := httptest.NewRequest("GET", "/api/v1/devices/wokwi:esp32-c3/backend", nil)
	w := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/devices/{id}/backend", handler.handleGetBackendConfig).Methods("GET")
	router.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var response BackendConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.DeviceID != "wokwi:esp32-c3" {
		t.Errorf("Expected device_id wokwi:esp32-c3, got %s", response.DeviceID)
	}

	if response.Backend != "wokwi" {
		t.Errorf("Expected backend wokwi, got %s", response.Backend)
	}
}
