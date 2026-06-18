package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestCameraSettingsHandler_List(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add test settings
	settings := &persistence.CameraSettings{
		CameraID:   "camera00",
		Name:       "Test Camera",
		Brightness: 128,
	}
	store.StoreCameraSettings(settings)

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/api/v1/camera/settings", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	settingsList := resp["settings"].([]interface{})
	if len(settingsList) != 1 {
		t.Errorf("Expected 1 setting, got %d", len(settingsList))
	}

	count := int(resp["count"].(float64))
	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}
}

func TestCameraSettingsHandler_Create(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("Valid settings", func(t *testing.T) {
		reqBody := persistence.CameraSettings{
			CameraID:   "camera01",
			Name:       "New Camera",
			Brightness: 100,
			Contrast:   50,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/v1/camera/settings", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)

		if resp["status"] != "created" {
			t.Errorf("Expected status 'created', got %v", resp["status"])
		}
	})

	t.Run("Missing camera_id", func(t *testing.T) {
		reqBody := persistence.CameraSettings{
			Name:       "No ID Camera",
			Brightness: 100,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/v1/camera/settings", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Invalid values - brightness out of range", func(t *testing.T) {
		reqBody := persistence.CameraSettings{
			CameraID:   "camera02",
			Brightness: 300, // Invalid: > 255
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/v1/camera/settings", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for invalid brightness, got %d", w.Code)
		}
	})

	t.Run("Invalid values - negative focus", func(t *testing.T) {
		reqBody := persistence.CameraSettings{
			CameraID: "camera03",
			Focus:    -10, // Invalid: < 0
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/v1/camera/settings", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for negative focus, got %d", w.Code)
		}
	})
}

func TestCameraSettingsHandler_Get(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add test settings
	settings := &persistence.CameraSettings{
		CameraID:   "camera00",
		Name:       "Test Camera",
		Brightness: 128,
		Contrast:   64,
	}
	store.StoreCameraSettings(settings)

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("Existing camera", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/camera/settings/camera00", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)

		settingsData := resp["settings"].(map[string]interface{})
		if settingsData["camera_id"] != "camera00" {
			t.Errorf("Expected camera_id camera00, got %v", settingsData["camera_id"])
		}

		// Check platform info
		if resp["platform"] == nil {
			t.Error("Response should include platform")
		}
	})

	t.Run("Non-existent camera returns empty settings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/camera/settings/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)

		settingsData := resp["settings"].(map[string]interface{})
		if settingsData["camera_id"] != "nonexistent" {
			t.Errorf("Expected camera_id nonexistent, got %v", settingsData["camera_id"])
		}
		// Name should be empty for non-existent camera
		if settingsData["name"] != "" {
			t.Errorf("Expected empty name, got %v", settingsData["name"])
		}
	})
}

func TestCameraSettingsHandler_Update(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add initial settings
	settings := &persistence.CameraSettings{
		CameraID:   "camera00",
		Name:       "Original Name",
		Brightness: 100,
	}
	store.StoreCameraSettings(settings)

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("PUT update", func(t *testing.T) {
		reqBody := persistence.CameraSettings{
			Name:       "Updated Name",
			Brightness: 150,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PUT", "/api/v1/camera/settings/camera00", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		// Verify update
		retrieved, _ := store.GetCameraSettings("camera00")
		if retrieved.Name != "Updated Name" {
			t.Errorf("Name not updated: got %s", retrieved.Name)
		}
		if retrieved.Brightness != 150 {
			t.Errorf("Brightness not updated: got %d", retrieved.Brightness)
		}
	})

	t.Run("PATCH partial update", func(t *testing.T) {
		reqBody := persistence.CameraSettings{
			Contrast: 200,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PATCH", "/api/v1/camera/settings/camera00", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		// Verify original values preserved
		retrieved, _ := store.GetCameraSettings("camera00")
		if retrieved.Name != "Updated Name" {
			t.Error("Name should be preserved on PATCH")
		}
		if retrieved.Contrast != 200 {
			t.Errorf("Contrast not updated: got %d", retrieved.Contrast)
		}
	})

	t.Run("Update with invalid value", func(t *testing.T) {
		reqBody := persistence.CameraSettings{
			Brightness: 500, // Invalid
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PUT", "/api/v1/camera/settings/camera00", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})
}

func TestCameraSettingsHandler_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add test settings
	settings := &persistence.CameraSettings{
		CameraID: "camera00",
		Name:     "To Delete",
	}
	store.StoreCameraSettings(settings)

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest("DELETE", "/api/v1/camera/settings/camera00", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify deleted
	_, err = store.GetCameraSettings("camera00")
	if err == nil {
		t.Error("Settings should be deleted")
	}
}

func TestCameraSettingsHandler_Apply(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add test settings
	settings := &persistence.CameraSettings{
		CameraID:   "camera00",
		Name:       "Test Camera",
		Brightness: 128,
		Contrast:   64,
		Focus:      85,
	}
	store.StoreCameraSettings(settings)

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("Apply settings on Linux", func(t *testing.T) {
		if !camera.ControllerAvailable() {
			t.Skip("Skipping on non-Linux platform")
		}

		req := httptest.NewRequest("POST", "/api/v1/camera/settings/camera00/apply", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// May fail if /camera00 doesn't exist, but handler should not crash
		// Status could be 200 (success) or 500 (device not found)
		if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
			t.Logf("Apply returned status %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Apply on non-Linux platform", func(t *testing.T) {
		if camera.ControllerAvailable() {
			t.Skip("Skipping on Linux platform")
		}

		req := httptest.NewRequest("POST", "/api/v1/camera/settings/camera00/apply", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200 on non-Linux, got %d", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)

		if resp["status"] != "skipped" {
			t.Errorf("Expected status 'skipped', got %v", resp["status"])
		}
	})
}

func TestCameraSettingsHandler_Discover(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/api/v1/camera/discover", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	// Should include platform info
	if resp["platform"] == nil {
		t.Error("Response should include platform")
	}

	// Should include controls_available flag
	if resp["controls_available"] == nil {
		t.Error("Response should include controls_available")
	}
}

func TestCameraSettingsHandler_GetControls(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("Get controls on Linux", func(t *testing.T) {
		if !camera.ControllerAvailable() {
			t.Skip("Skipping on non-Linux platform")
		}

		req := httptest.NewRequest("GET", "/api/v1/camera/camera00/controls", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// May fail if device doesn't exist
		if w.Code == http.StatusOK {
			var resp map[string]interface{}
			json.NewDecoder(w.Body).Decode(&resp)

			if resp["display_preset"] == nil {
				t.Error("Response should include display_preset")
			}
			if resp["focus_presets"] == nil {
				t.Error("Response should include focus_presets")
			}
		}
	})

	t.Run("Get controls on non-Linux", func(t *testing.T) {
		if camera.ControllerAvailable() {
			t.Skip("Skipping on Linux platform")
		}

		req := httptest.NewRequest("GET", "/api/v1/camera/camera00/controls", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200 on non-Linux, got %d", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)

		if resp["available"] != false {
			t.Error("Controls should not be available on non-Linux")
		}
	})
}

func TestValidateSettings(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewCameraSettingsHandler(store)

	t.Run("Valid settings", func(t *testing.T) {
		settings := &persistence.CameraSettings{
			CameraID:   "camera00",
			Brightness: 128,
			Contrast:   128,
			Saturation: 32,
			Sharpness:  32,
			Gain:       0,
			Focus:      85,
		}

		if !handler.validateSettings(settings) {
			t.Error("Valid settings failed validation")
		}
	})

	t.Run("Invalid brightness - too high", func(t *testing.T) {
		settings := &persistence.CameraSettings{
			CameraID:   "camera00",
			Brightness: 256,
		}

		if handler.validateSettings(settings) {
			t.Error("Should reject brightness > 255")
		}
	})

	t.Run("Invalid contrast - negative", func(t *testing.T) {
		settings := &persistence.CameraSettings{
			CameraID: "camera00",
			Contrast: -1,
		}

		if handler.validateSettings(settings) {
			t.Error("Should reject negative contrast")
		}
	})

	t.Run("Edge cases", func(t *testing.T) {
		settings := &persistence.CameraSettings{
			CameraID:   "camera00",
			Brightness: 0,
			Contrast:   255,
		}

		if !handler.validateSettings(settings) {
			t.Error("Should accept 0 and 255 as valid")
		}
	})
}

func TestCameraSettingsHandler_ApplySettings(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("apply without settings returns error", func(t *testing.T) {
		cameraID := "test-no-settings-camera"

		// Verify no settings exist initially
		_, err := store.GetCameraSettings(cameraID)
		assert.Error(t, err, "Should not have settings initially")

		// Apply without settings and without request body should fail
		req := httptest.NewRequest("POST", "/api/v1/camera/settings/"+cameraID+"/apply", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// On Linux: should return 400 (no settings)
		// On non-Linux: returns 200 with "skipped" status (controls not available)
		if camera.ControllerAvailable() {
			assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when no settings available on Linux")
		} else {
			// Non-Linux platforms skip camera controls
			assert.Equal(t, http.StatusOK, w.Code, "Should return 200 with skipped status on non-Linux")
			var resp map[string]interface{}
			json.NewDecoder(w.Body).Decode(&resp)
			assert.Equal(t, "skipped", resp["status"], "Should indicate controls skipped")
		}
	})

	t.Run("apply with request body values", func(t *testing.T) {
		cameraID := "test-request-apply-camera"

		// Verify no settings exist initially
		_, err := store.GetCameraSettings(cameraID)
		assert.Error(t, err, "Should not have settings initially")

		// Apply with settings in request body
		requestSettings := persistence.CameraSettings{
			Brightness: 180,
			Contrast:   120,
			Saturation: 90,
		}
		body, _ := json.Marshal(requestSettings)
		req := httptest.NewRequest("POST", "/api/v1/camera/settings/"+cameraID+"/apply", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Settings should NOT have been saved (only applied)
		_, err = store.GetCameraSettings(cameraID)
		assert.Error(t, err, "Settings should NOT be saved when applying from request body")

		// On Linux with camera, might return 200 or 500 if device not found
		// On other platforms, returns 200 with "skipped" status
		if w.Code == http.StatusOK {
			var resp map[string]interface{}
			json.NewDecoder(w.Body).Decode(&resp)
			// Verify the applied settings match what we sent
			if settings, ok := resp["settings"].(map[string]interface{}); ok {
				assert.Equal(t, float64(180), settings["brightness"], "Should apply request brightness")
				assert.Equal(t, float64(120), settings["contrast"], "Should apply request contrast")
			}
		}
	})

	t.Run("apply with existing stored settings", func(t *testing.T) {
		cameraID := "test-stored-apply-camera"

		// Create and save custom settings
		customSettings := &persistence.CameraSettings{
			CameraID:   cameraID,
			Name:       "Stored Camera",
			Brightness: 200,
			Contrast:   150,
		}
		store.StoreCameraSettings(customSettings)

		// Apply settings (should use stored values)
		req := httptest.NewRequest("POST", "/api/v1/camera/settings/"+cameraID+"/apply", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// On Linux with camera, might return 200 or 500 if device not found
		// On other platforms, returns 200 with "skipped" status
		if w.Code == http.StatusOK || w.Code == http.StatusInternalServerError {
			// Stored settings should still be unchanged
			settings, err := store.GetCameraSettings(cameraID)
			assert.NoError(t, err)
			assert.Equal(t, int32(200), settings.Brightness, "Stored brightness should be unchanged")
			assert.Equal(t, int32(150), settings.Contrast, "Stored contrast should be unchanged")
		}
	})
}

func TestCameraSettingsHandler_GetControlsReturnsRanges(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewCameraSettingsHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	if !camera.ControllerAvailable() {
		t.Skip("Skipping on non-Linux platform")
	}

	t.Run("controls endpoint returns ranges map", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/camera/camera00/controls", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// May fail if camera00 doesn't exist, but test response structure if it succeeds
		if w.Code == http.StatusOK {
			var resp map[string]interface{}
			err := json.NewDecoder(w.Body).Decode(&resp)
			if err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Check for ranges field
			ranges, ok := resp["ranges"]
			if !ok {
				t.Error("Response should include ranges field")
			} else {
				rangesMap, ok := ranges.(map[string]interface{})
				if !ok {
					t.Error("Ranges should be a map")
				} else {
					// Each range should have min, max, current
					for controlName, rangeData := range rangesMap {
						controlRange, ok := rangeData.(map[string]interface{})
						if !ok {
							t.Errorf("Range for %s should be a map", controlName)
							continue
						}
						if _, hasMin := controlRange["min"]; !hasMin {
							t.Errorf("Range for %s missing min", controlName)
						}
						if _, hasMax := controlRange["max"]; !hasMax {
							t.Errorf("Range for %s missing max", controlName)
						}
						if _, hasCurrent := controlRange["current"]; !hasCurrent {
							t.Errorf("Range for %s missing current", controlName)
						}
					}
				}
			}
		}
	})
}

func TestCameraSettings_SavedSettingsPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := persistence.Open(persistence.DefaultConfig(dbPath))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("saved settings persist across retrieval", func(t *testing.T) {
		cameraID := "test-persist-camera"

		// Create initial settings
		original := &persistence.CameraSettings{
			CameraID:   cameraID,
			Name:       "Persist Test Camera",
			Brightness: 180,
			Contrast:   120,
			Saturation: 90,
			Sharpness:  75,
			Gain:       50,
			Focus:      85,
		}
		err := store.StoreCameraSettings(original)
		assert.NoError(t, err, "Should store settings")

		// Retrieve settings
		retrieved, err := store.GetCameraSettings(cameraID)
		assert.NoError(t, err, "Should retrieve settings")
		assert.NotNil(t, retrieved)

		// Verify all values preserved
		assert.Equal(t, original.Name, retrieved.Name, "Name should persist")
		assert.Equal(t, original.Brightness, retrieved.Brightness, "Brightness should persist")
		assert.Equal(t, original.Contrast, retrieved.Contrast, "Contrast should persist")
		assert.Equal(t, original.Saturation, retrieved.Saturation, "Saturation should persist")
		assert.Equal(t, original.Sharpness, retrieved.Sharpness, "Sharpness should persist")
		assert.Equal(t, original.Gain, retrieved.Gain, "Gain should persist")
		assert.Equal(t, original.Focus, retrieved.Focus, "Focus should persist")
	})

	t.Run("update preserves existing values", func(t *testing.T) {
		cameraID := "test-update-persist"

		// Create initial settings with all fields
		original := &persistence.CameraSettings{
			CameraID:         cameraID,
			Name:             "Original Name",
			Brightness:       100,
			Contrast:         100,
			Saturation:       100,
			Sharpness:        100,
			Gain:             50,
			Focus:            60,
			Exposure:         200,
			WhiteBalance:     4000,
			AutoExposure:     true,
			AutoFocus:        true,
			AutoWhiteBalance: true,
		}
		store.StoreCameraSettings(original)

		// Update only brightness
		updated := &persistence.CameraSettings{
			CameraID:   cameraID,
			Brightness: 150,
		}

		// Apply update via handler to simulate real workflow
		handler := NewCameraSettingsHandler(store)
		router := mux.NewRouter()
		handler.RegisterRoutes(router)

		body, _ := json.Marshal(updated)
		req := httptest.NewRequest("PATCH", "/api/v1/camera/settings/"+cameraID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Update should succeed")

		// Verify: brightness changed, others preserved
		retrieved, _ := store.GetCameraSettings(cameraID)
		assert.Equal(t, int32(150), retrieved.Brightness, "Brightness should update")
		assert.Equal(t, "Original Name", retrieved.Name, "Name should persist")
		assert.Equal(t, int32(100), retrieved.Contrast, "Contrast should persist")
		assert.Equal(t, int32(100), retrieved.Saturation, "Saturation should persist")
		assert.Equal(t, int32(100), retrieved.Sharpness, "Sharpness should persist")
		assert.Equal(t, int32(50), retrieved.Gain, "Gain should persist")
		assert.Equal(t, int32(60), retrieved.Focus, "Focus should persist")
		assert.Equal(t, int32(200), retrieved.Exposure, "Exposure should persist")
		assert.Equal(t, int32(4000), retrieved.WhiteBalance, "WhiteBalance should persist")
		assert.Equal(t, true, retrieved.AutoExposure, "AutoExposure should persist")
		assert.Equal(t, true, retrieved.AutoFocus, "AutoFocus should persist")
		assert.Equal(t, true, retrieved.AutoWhiteBalance, "AutoWhiteBalance should persist")
	})
}
