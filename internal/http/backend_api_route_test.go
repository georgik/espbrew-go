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

// TestBackendConfigWithColonInDeviceID tests that backend config works correctly
// when device ID contains a colon (e.g., wokwi:esp32-s3).
// This is a regression test for diagram JSON not being saved for Wokwi devices.
func TestBackendConfigWithColonInDeviceID(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Create test device with URI-style ID containing colon
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

	// Test PUT to /devices/{id}/backend with diagram JSON update
	t.Run("update_diagram_json", func(t *testing.T) {
		reqBody := BackendConfigRequest{
			Backend: "wokwi",
			BackendConfig: map[string]interface{}{
				"chip_type":    "ESP32-S3",
				"diagram_json": `{"version":2,"parts":[{"type":"esp32-s3-devkitc-1","id":"chip"}]}`,
			},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PUT", "/api/v1/devices/wokwi:esp32-s3/backend", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/devices/{id}/backend", handler.handleSetBackendConfig).Methods("PUT")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Verify backend config was saved
		updated, err := store.GetDevice("wokwi:esp32-s3")
		if err != nil {
			t.Fatalf("Failed to get device: %v", err)
		}

		if updated.BackendConfig == nil {
			t.Fatal("Backend config not saved")
		}

		if updated.BackendConfig.Wokwi == nil {
			t.Fatal("Wokwi config not saved")
		}

		// Verify diagram JSON was saved correctly
		var diagram map[string]interface{}
		if err := json.Unmarshal([]byte(updated.BackendConfig.Wokwi.DiagramJSON), &diagram); err != nil {
			t.Fatalf("Failed to parse diagram JSON: %v", err)
		}

		if diagram["version"] != float64(2) {
			t.Errorf("Expected diagram version 2, got %v", diagram["version"])
		}

		// Verify chip type
		if updated.BackendConfig.Wokwi.ChipType != "ESP32-S3" {
			t.Errorf("Expected chip_type ESP32-S3, got %s", updated.BackendConfig.Wokwi.ChipType)
		}
	})

	// Test GET /devices/{id}/backend returns config
	t.Run("get_backend_config", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/devices/wokwi:esp32-s3/backend", nil)
		w := httptest.NewRecorder()

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/devices/{id}/backend", handler.handleGetBackendConfig).Methods("GET")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var resp BackendConfigResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.DeviceID != "wokwi:esp32-s3" {
			t.Errorf("Expected device_id wokwi:esp32-s3, got %s", resp.DeviceID)
		}

		if resp.Backend != "wokwi" {
			t.Errorf("Expected backend wokwi, got %s", resp.Backend)
		}

		// Verify diagram JSON in response
		wokwiCfg, ok := resp.BackendConfig.(map[string]interface{})
		if !ok {
			t.Fatal("BackendConfig is not map")
		}

		diagramJSON, ok := wokwiCfg["diagram_json"].(string)
		if !ok {
			t.Fatal("diagram_json not in response")
		}

		// Should contain version 2 from previous update
		if !bytes.Contains([]byte(diagramJSON), []byte("version")) {
			t.Error("diagram_json should contain version field")
		}
	})
}
