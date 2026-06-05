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

	t.Run("Non-existent camera", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/camera/settings/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
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

func TestCameraSettingsHandler_AutoCreateOnApply(t *testing.T) {
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

	t.Run("apply without existing settings creates defaults", func(t *testing.T) {
		cameraID := "test-autocreate-camera"

		// Verify no settings exist initially
		_, err := store.GetCameraSettings(cameraID)
		assert.Error(t, err, "Should not have settings initially")

		// Apply settings (should auto-create)
		req := httptest.NewRequest("POST", "/api/v1/camera/settings/"+cameraID+"/apply", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Settings should have been auto-created
		settings, err := store.GetCameraSettings(cameraID)
		assert.NoError(t, err, "Settings should have been auto-created")
		assert.NotNil(t, settings)
		assert.Equal(t, cameraID, settings.CameraID)
		assert.Equal(t, int32(128), settings.Brightness, "Default brightness should be 128")
		assert.Equal(t, int32(128), settings.Contrast, "Default contrast should be 128")
		assert.Equal(t, true, settings.AutoExposure, "Default auto exposure should be true")
	})

	t.Run("apply with existing settings uses them", func(t *testing.T) {
		cameraID := "test-existing-camera"

		// Create custom settings
		customSettings := &persistence.CameraSettings{
			CameraID:   cameraID,
			Name:       "Custom Camera",
			Brightness: 200,
			Contrast:   150,
		}
		store.StoreCameraSettings(customSettings)

		// Apply settings (should use existing, not create defaults)
		req := httptest.NewRequest("POST", "/api/v1/camera/settings/"+cameraID+"/apply", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Verify existing settings were preserved
		settings, err := store.GetCameraSettings(cameraID)
		assert.NoError(t, err)
		assert.Equal(t, int32(200), settings.Brightness, "Should use custom brightness")
		assert.Equal(t, int32(150), settings.Contrast, "Should use custom contrast")
		assert.Equal(t, "Custom Camera", settings.Name, "Should use custom name")
	})
}
