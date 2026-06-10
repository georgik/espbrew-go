package http

import (
	"encoding/json"
	"net/http"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// getCameraPathByID looks up a camera's device path by its ID from registry
// Returns the path (e.g. /dev/video0) or empty string if not found
func getCameraPathByID(cameraID string) string {
	registry := camera.GetRegistry()
	if cam, ok := registry.GetByID(cameraID); ok && cam.Path != "" {
		log.Debug().Str("camera_id", cameraID).Str("path", cam.Path).Msg("Found camera path in registry")
		return cam.Path
	}
	log.Warn().Str("camera_id", cameraID).Msg("Camera not found in registry")
	return ""
}

// CameraSettingsHandler handles camera settings API endpoints
type CameraSettingsHandler struct {
	store *persistence.Store
}

// NewCameraSettingsHandler creates a new camera settings handler
func NewCameraSettingsHandler(store *persistence.Store) *CameraSettingsHandler {
	return &CameraSettingsHandler{
		store: store,
	}
}

// RegisterRoutes registers camera settings routes
func (h *CameraSettingsHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	// Camera settings CRUD
	api.HandleFunc("/camera/settings", h.handleListCameraSettings).Methods("GET")
	api.HandleFunc("/camera/settings", h.handleCreateCameraSettings).Methods("POST")
	api.HandleFunc("/camera/settings/{cameraId}", h.handleGetCameraSettings).Methods("GET")
	api.HandleFunc("/camera/settings/{cameraId}", h.handleUpdateCameraSettings).Methods("PUT", "PATCH")
	api.HandleFunc("/camera/settings/{cameraId}", h.handleDeleteCameraSettings).Methods("DELETE")

	// Apply settings to camera
	api.HandleFunc("/camera/settings/{cameraId}/apply", h.handleApplyCameraSettings).Methods("POST")

	// Camera discovery
	api.HandleFunc("/camera/discover", h.handleDiscoverCameras).Methods("GET")
	api.HandleFunc("/camera/{cameraId}/controls", h.handleGetCameraControls).Methods("GET")
}

// handleListCameraSettings lists all camera settings
func (h *CameraSettingsHandler) handleListCameraSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.ListCameraSettings(nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list camera settings")
		respondError(w, http.StatusInternalServerError, "Failed to list camera settings")
		return
	}

	respondJSON(w, map[string]interface{}{
		"settings": settings,
		"count":    len(settings),
	})
}

// handleCreateCameraSettings creates new camera settings
func (h *CameraSettingsHandler) handleCreateCameraSettings(w http.ResponseWriter, r *http.Request) {
	var req persistence.CameraSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.CameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id is required")
		return
	}

	// Validate settings ranges
	if !h.validateSettings(&req) {
		respondError(w, http.StatusBadRequest, "Invalid settings values")
		return
	}

	if err := h.store.StoreCameraSettings(&req); err != nil {
		log.Error().Err(err).Msg("Failed to store camera settings")
		respondError(w, http.StatusInternalServerError, "Failed to store camera settings")
		return
	}

	respondJSON(w, map[string]interface{}{
		"status":    "created",
		"camera_id": req.CameraID,
		"settings":  req,
	})
}

// handleGetCameraSettings retrieves camera settings by camera ID
func (h *CameraSettingsHandler) handleGetCameraSettings(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cameraID := vars["cameraId"]

	if cameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id is required")
		return
	}

	settings, err := h.store.GetCameraSettings(cameraID)
	if err != nil {
		log.Error().Err(err).Str("camera_id", cameraID).Msg("Failed to get camera settings")
		respondError(w, http.StatusNotFound, "Camera settings not found")
		return
	}

	// Check if camera controls are available on this platform
	available := camera.ControllerAvailable()
	platform := camera.Platform()

	respondJSON(w, map[string]interface{}{
		"settings":           settings,
		"controls_available": available,
		"platform":           platform,
	})
}

// handleUpdateCameraSettings updates existing camera settings
func (h *CameraSettingsHandler) handleUpdateCameraSettings(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cameraID := vars["cameraId"]

	if cameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id is required")
		return
	}

	// Get existing settings
	existing, err := h.store.GetCameraSettings(cameraID)
	if err != nil {
		log.Error().Err(err).Str("camera_id", cameraID).Msg("Failed to get existing camera settings")
		respondError(w, http.StatusNotFound, "Camera settings not found")
		return
	}

	// Decode update request
	var updates persistence.CameraSettings
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Update fields
	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Brightness != 0 {
		existing.Brightness = updates.Brightness
	}
	if updates.Contrast != 0 {
		existing.Contrast = updates.Contrast
	}
	if updates.Saturation != 0 {
		existing.Saturation = updates.Saturation
	}
	if updates.Sharpness != 0 {
		existing.Sharpness = updates.Sharpness
	}
	if updates.Gain != 0 {
		existing.Gain = updates.Gain
	}
	if updates.Focus != 0 {
		existing.Focus = updates.Focus
	}
	if updates.Exposure != 0 {
		existing.Exposure = updates.Exposure
	}
	if updates.WhiteBalance != 0 {
		existing.WhiteBalance = updates.WhiteBalance
	}
	// Boolean fields need explicit checks
	isPut := (r.Method == "PUT")
	if isPut || r.FormValue("auto_exposure") != "" {
		existing.AutoExposure = updates.AutoExposure
	}
	if isPut || r.FormValue("auto_focus") != "" {
		existing.AutoFocus = updates.AutoFocus
	}
	if isPut || r.FormValue("auto_white_balance") != "" {
		existing.AutoWhiteBalance = updates.AutoWhiteBalance
	}

	// Validate settings
	if !h.validateSettings(existing) {
		respondError(w, http.StatusBadRequest, "Invalid settings values")
		return
	}

	if err := h.store.StoreCameraSettings(existing); err != nil {
		log.Error().Err(err).Msg("Failed to update camera settings")
		respondError(w, http.StatusInternalServerError, "Failed to update camera settings")
		return
	}

	respondJSON(w, map[string]interface{}{
		"status":    "updated",
		"camera_id": cameraID,
		"settings":  existing,
	})
}

// handleDeleteCameraSettings deletes camera settings
func (h *CameraSettingsHandler) handleDeleteCameraSettings(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cameraID := vars["cameraId"]

	if cameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id is required")
		return
	}

	if err := h.store.DeleteCameraSettings(cameraID); err != nil {
		log.Error().Err(err).Str("camera_id", cameraID).Msg("Failed to delete camera settings")
		respondError(w, http.StatusInternalServerError, "Failed to delete camera settings")
		return
	}

	respondJSON(w, map[string]interface{}{
		"status":    "deleted",
		"camera_id": cameraID,
	})
}

// handleApplyCameraSettings applies settings to the physical camera
// If request body contains settings, applies those. Otherwise uses stored settings.
func (h *CameraSettingsHandler) handleApplyCameraSettings(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cameraID := vars["cameraId"]

	if cameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id is required")
		return
	}

	// Check if camera controls are available
	if !camera.ControllerAvailable() {
		respondJSON(w, map[string]interface{}{
			"status":   "skipped",
			"message":  "Camera controls not available on this platform",
			"platform": camera.Platform(),
		})
		return
	}

	// Try to read settings from request body (optional)
	var requestSettings *persistence.CameraSettings
	bodyErr := json.NewDecoder(r.Body).Decode(&requestSettings)
	if bodyErr == nil && requestSettings != nil {
		// Validate the provided settings
		if !h.validateSettings(requestSettings) {
			respondError(w, http.StatusBadRequest, "Invalid settings values")
			return
		}
		requestSettings.CameraID = cameraID // Ensure camera ID matches
	}

	// Get stored settings
	storedSettings, err := h.store.GetCameraSettings(cameraID)

	// Determine which settings to apply
	var settingsToApply *persistence.CameraSettings
	if requestSettings != nil {
		// Use settings from request body (don't save, just apply)
		settingsToApply = requestSettings
		log.Debug().Str("camera_id", cameraID).Msg("Applying settings from request body")
	} else if storedSettings != nil {
		// Use stored settings
		settingsToApply = storedSettings
		log.Debug().Str("camera_id", cameraID).Msg("Applying stored settings")
	} else {
		// No settings available - return error instead of creating defaults
		log.Warn().Str("camera_id", cameraID).Msg("No settings to apply")
		respondError(w, http.StatusBadRequest, "No settings available. Please save settings first or provide settings in request.")
		return
	}

	// Create camera controller using device path
	cameraPath := getCameraPathByID(cameraID)
	if cameraPath == "" {
		log.Error().Str("camera_id", cameraID).Msg("Camera not found in discovery")
		respondError(w, http.StatusNotFound, "Camera not found or not available")
		return
	}

	ctrl, err := camera.NewController(cameraPath)
	if err != nil {
		log.Error().Err(err).Str("camera_id", cameraID).Str("path", cameraPath).Msg("Failed to create camera controller")
		respondError(w, http.StatusInternalServerError, "Failed to access camera")
		return
	}
	defer ctrl.Close()

	// Apply settings
	if err := h.applySettingsToCamera(ctrl, settingsToApply); err != nil {
		log.Error().Err(err).Str("camera_id", cameraID).Msg("Failed to apply camera settings")
		respondError(w, http.StatusInternalServerError, "Failed to apply camera settings: "+err.Error())
		return
	}

	// Get current settings to verify
	current, _ := ctrl.GetSettings()

	respondJSON(w, map[string]interface{}{
		"status":    "applied",
		"camera_id": cameraID,
		"settings":  settingsToApply,
		"current":   current,
		"platform":  camera.Platform(),
	})
}

// handleDiscoverCameras lists available cameras on the system
func (h *CameraSettingsHandler) handleDiscoverCameras(w http.ResponseWriter, r *http.Request) {
	registry := camera.GetRegistry()
	cameras := registry.List()

	// Check which cameras have stored settings
	for i := range cameras {
		settings, _ := h.store.GetCameraSettings(cameras[i].ID)
		if settings != nil && settings.Name != "" {
			cameras[i].Name = settings.Name
		}
	}

	respondJSON(w, map[string]interface{}{
		"cameras":            cameras,
		"count":              len(cameras),
		"controls_available": camera.ControllerAvailable(),
		"platform":           camera.Platform(),
	})
}

// handleGetCameraControls queries available controls for a camera
func (h *CameraSettingsHandler) handleGetCameraControls(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cameraID := vars["cameraId"]

	if cameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id is required")
		return
	}

	if !camera.ControllerAvailable() {
		respondJSON(w, map[string]interface{}{
			"controls":  []interface{}{},
			"available": false,
			"platform":  camera.Platform(),
			"message":   "Camera controls not available on this platform",
		})
		return
	}

	cameraPath := getCameraPathByID(cameraID)
	if cameraPath == "" {
		log.Error().Str("camera_id", cameraID).Msg("Camera not found in discovery")
		respondError(w, http.StatusNotFound, "Camera not found or not available")
		return
	}

	ctrl, err := camera.NewController(cameraPath)
	if err != nil {
		log.Error().Err(err).Str("camera_id", cameraID).Str("path", cameraPath).Msg("Failed to create camera controller")
		respondError(w, http.StatusInternalServerError, "Failed to access camera")
		return
	}
	defer ctrl.Close()

	// Get current settings
	current, err := ctrl.GetSettings()
	if err != nil {
		log.Error().Err(err).Str("camera_id", cameraID).Msg("Failed to get current settings")
	}

	// Get control ranges for UI sliders
	controlRanges := make(map[string]map[string]int32)
	controlsToQuery := []string{"brightness", "contrast", "saturation", "sharpness", "gain", "focus_absolute", "exposure_absolute"}
	for _, controlName := range controlsToQuery {
		min, max, currentVal, err := ctrl.GetControlInfo(controlName)
		if err != nil {
			// Use default ranges if query fails
			log.Debug().Str("control", controlName).Err(err).Msg("Failed to get control range, using defaults")
			continue
		}
		controlRanges[controlName] = map[string]int32{
			"min":     min,
			"max":     max,
			"current": currentVal,
		}
	}

	respondJSON(w, map[string]interface{}{
		"current":        current,
		"available":      true,
		"platform":       camera.Platform(),
		"display_preset": camera.DisplayPresetSettings,
		"focus_presets":  camera.FocusPresets,
		"ranges":         controlRanges,
	})
}

// validateSettings checks if settings values are within valid ranges
func (h *CameraSettingsHandler) validateSettings(settings *persistence.CameraSettings) bool {
	// Validate range 0-255 for most controls
	controls := []struct {
		value int32
		name  string
	}{
		{settings.Brightness, "brightness"},
		{settings.Contrast, "contrast"},
		{settings.Saturation, "saturation"},
		{settings.Sharpness, "sharpness"},
		{settings.Gain, "gain"},
		{settings.Focus, "focus"},
	}

	for _, ctrl := range controls {
		if ctrl.value < 0 || ctrl.value > 255 {
			log.Warn().
				Int32("value", ctrl.value).
				Str("control", ctrl.name).
				Msg("Invalid control value")
			return false
		}
	}

	return true
}

// applySettingsToCamera applies settings to a physical camera
func (h *CameraSettingsHandler) applySettingsToCamera(ctrl camera.Controller, settings *persistence.CameraSettings) error {
	// Apply each setting
	if err := ctrl.SetBrightness(settings.Brightness); err != nil {
		log.Warn().Err(err).Msg("Failed to set brightness")
	}
	if err := ctrl.SetContrast(settings.Contrast); err != nil {
		log.Warn().Err(err).Msg("Failed to set contrast")
	}
	if err := ctrl.SetSaturation(settings.Saturation); err != nil {
		log.Warn().Err(err).Msg("Failed to set saturation")
	}
	if err := ctrl.SetSharpness(settings.Sharpness); err != nil {
		log.Warn().Err(err).Msg("Failed to set sharpness")
	}

	// Focus if not auto
	if !settings.AutoFocus && settings.Focus > 0 {
		if err := ctrl.SetFocus(settings.Focus); err != nil {
			log.Warn().Err(err).Msg("Failed to set focus")
		}
	}

	return nil
}
