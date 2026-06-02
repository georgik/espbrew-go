package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"codeberg.org/georgik/espbrew-go/internal/persistence"
)

// MappingHandler handles bounding box mapping API requests
type MappingHandler struct {
	store *persistence.Store
}

// NewMappingHandler creates a new mapping handler
func NewMappingHandler(store *persistence.Store) *MappingHandler {
	return &MappingHandler{
		store: store,
	}
}

// RegisterRoutes registers mapping routes
func (h *MappingHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	// Camera-specific routes
	api.HandleFunc("/cameras/{id}/boxes", h.handleCameraBoxes).Methods("GET")
	api.HandleFunc("/cameras/{id}/calibration", h.handleGetCalibration).Methods("GET")
	api.HandleFunc("/cameras/{id}/calibration", h.handleCreateCalibration).Methods("POST")

	// Bounding box CRUD
	api.HandleFunc("/bounding_boxes", h.handleCreateBox).Methods("POST")
	api.HandleFunc("/bounding_boxes/{id}", h.handleUpdateBox).Methods("PUT")
	api.HandleFunc("/bounding_boxes/{id}", h.handleDeleteBox).Methods("DELETE")
}

// Request/Response structs

// CreateBoundingBoxRequest represents a request to create a bounding box mapping
type CreateBoundingBoxRequest struct {
	DeviceID   string                  `json:"device_id"`
	CameraID   string                  `json:"camera_id"`
	CameraName string                  `json:"camera_name,omitempty"` // Stable identifier
	Bounds     persistence.BoundingBox `json:"bounds"`
}

// UpdateBoundingBoxRequest represents a request to update a bounding box mapping
type UpdateBoundingBoxRequest struct {
	Bounds     *persistence.BoundingBox     `json:"bounds"`
	Adjustment *persistence.ImageAdjustment `json:"adjustment"`
}

// CreateCalibrationRequest represents a request to create a new calibration version
type CreateCalibrationRequest struct {
	Description string `json:"description"`
}

// CameraBoundingBoxesResponse represents the response for camera boxes query
type CameraBoundingBoxesResponse struct {
	CameraID    string                        `json:"camera_id"`
	Calibration *CalibrationInfo              `json:"calibration,omitempty"`
	Mappings    []DeviceBoundingBoxWithDevice `json:"mappings"`
}

// CalibrationInfo represents calibration summary info
type CalibrationInfo struct {
	Version     int    `json:"version"`
	Description string `json:"description"`
}

// DeviceBoundingBoxWithDevice extends mapping with device details
type DeviceBoundingBoxWithDevice struct {
	ID                 string                      `json:"id"`
	DeviceID           string                      `json:"device_id"`
	CameraID           string                      `json:"camera_id"`
	CameraName         string                      `json:"camera_name,omitempty"`
	Bounds             persistence.BoundingBox     `json:"bounds"`
	CalibrationVersion int                         `json:"calibration_version"`
	Adjustment         persistence.ImageAdjustment `json:"adjustment"`
	CreatedAt          string                      `json:"created_at"`
	UpdatedAt          string                      `json:"updated_at"`
	Device             *DeviceInfo                 `json:"device,omitempty"`
}

// DeviceInfo represents basic device information
type DeviceInfo struct {
	DeviceID   string   `json:"device_id"`
	ChipType   string   `json:"chip_type"`
	Aliases    []string `json:"aliases,omitempty"`
	MACAddress string   `json:"mac_address,omitempty"`
}

// Handlers

// handleCameraBoxes returns all bounding boxes for a camera with device details
func (h *MappingHandler) handleCameraBoxes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cameraID := vars["id"]

	log.Info().Str("camera_id", cameraID).Msg("Listing bounding boxes for camera")

	if cameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id required")
		return
	}

	// Get bounding boxes for camera
	mappings, err := h.store.ListBoundingBoxesForCamera(cameraID)
	if err != nil {
		log.Error().Err(err).Str("camera_id", cameraID).Msg("Failed to list bounding boxes")
		respondError(w, http.StatusInternalServerError, "Failed to retrieve bounding boxes")
		return
	}

	log.Info().
		Str("camera_id", cameraID).
		Int("count", len(mappings)).
		Msg("Retrieved bounding boxes")

	// Get current calibration for camera
	calib, err := h.store.GetCalibration(cameraID)
	var calibInfo *CalibrationInfo
	if err == nil && calib != nil {
		calibInfo = &CalibrationInfo{
			Version:     calib.Version,
			Description: calib.Description,
		}
	}

	// Build response with device details
	response := CameraBoundingBoxesResponse{
		CameraID:    cameraID,
		Calibration: calibInfo,
		Mappings:    make([]DeviceBoundingBoxWithDevice, 0, len(mappings)),
	}

	for _, mapping := range mappings {
		withDevice := DeviceBoundingBoxWithDevice{
			ID:                 mapping.ID,
			DeviceID:           mapping.DeviceID,
			CameraID:           mapping.CameraID,
			CameraName:         mapping.CameraName,
			Bounds:             mapping.Bounds,
			CalibrationVersion: mapping.CalibrationVersion,
			Adjustment:         mapping.Adjustment,
			CreatedAt:          mapping.CreatedAt.Format(time.RFC3339),
			UpdatedAt:          mapping.UpdatedAt.Format(time.RFC3339),
		}

		// Try to get device details
		dev, err := h.store.GetDevice(mapping.DeviceID)
		if err == nil && dev != nil {
			withDevice.Device = &DeviceInfo{
				DeviceID:   dev.DeviceID,
				ChipType:   dev.ChipType,
				Aliases:    dev.Aliases,
				MACAddress: dev.MACAddress,
			}
		}

		response.Mappings = append(response.Mappings, withDevice)
	}

	respondJSON(w, response)
}

// handleGetCalibration returns the current calibration for a camera
func (h *MappingHandler) handleGetCalibration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cameraID := vars["id"]

	if cameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id required")
		return
	}

	calib, err := h.store.GetCalibration(cameraID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Calibration not found")
		return
	}

	respondJSON(w, calib)
}

// handleCreateCalibration creates a new calibration version for a camera
func (h *MappingHandler) handleCreateCalibration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cameraID := vars["id"]

	if cameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id required")
		return
	}

	var req CreateCalibrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.store.IncrementCalibrationVersion(cameraID, req.Description); err != nil {
		log.Error().Err(err).Str("camera_id", cameraID).Msg("Failed to create calibration")
		respondError(w, http.StatusInternalServerError, "Failed to create calibration")
		return
	}

	// Fetch the new calibration
	calib, err := h.store.GetCalibration(cameraID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to retrieve new calibration")
		return
	}

	respondJSON(w, calib)
}

// handleCreateBox creates a new bounding box mapping
func (h *MappingHandler) handleCreateBox(w http.ResponseWriter, r *http.Request) {
	var req CreateBoundingBoxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.DeviceID == "" {
		respondError(w, http.StatusBadRequest, "device_id required")
		return
	}
	if req.CameraID == "" {
		respondError(w, http.StatusBadRequest, "camera_id required")
		return
	}

	// Validate bounds
	if err := req.Bounds.Validate(); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid bounds: %s", err.Error()))
		return
	}

	// Check if mapping already exists for this device+camera
	existing, err := h.store.GetBoundingBoxForDeviceAndCamera(req.DeviceID, req.CameraID)
	now := time.Now()

	if err == nil && existing != nil {
		// Update existing mapping
		log.Info().
			Str("mapping_id", existing.ID).
			Str("device_id", req.DeviceID).
			Str("camera_id", req.CameraID).
			Msg("Updating existing bounding box mapping")

		existing.Bounds = req.Bounds
		existing.UpdatedAt = now

		if err := h.store.SaveBoundingBox(existing); err != nil {
			log.Error().Err(err).Str("device_id", req.DeviceID).Str("camera_id", req.CameraID).
				Msg("Failed to update bounding box")
			respondError(w, http.StatusInternalServerError, "Failed to update bounding box")
			return
		}

		respondJSON(w, existing)
		return
	}

	// Get current calibration version
	calibVersion := 0
	calib, err := h.store.GetCalibration(req.CameraID)
	if err == nil && calib != nil {
		calibVersion = calib.Version
	}

	// Create new mapping
	mapping := &persistence.DeviceBoundingBoxMapping{
		ID:                 uuid.New().String(),
		DeviceID:           req.DeviceID,
		CameraID:           req.CameraID,
		CameraName:         req.CameraName, // Store stable identifier
		Bounds:             req.Bounds,
		CalibrationVersion: calibVersion,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	log.Info().
		Str("mapping_id", mapping.ID).
		Str("device_id", req.DeviceID).
		Str("camera_id", req.CameraID).
		Msg("Creating new bounding box mapping")

	if err := h.store.SaveBoundingBox(mapping); err != nil {
		log.Error().Err(err).Str("device_id", req.DeviceID).Str("camera_id", req.CameraID).
			Msg("Failed to save bounding box")
		respondError(w, http.StatusInternalServerError, "Failed to create bounding box")
		return
	}

	log.Info().
		Str("mapping_id", mapping.ID).
		Str("device_id", req.DeviceID).
		Str("camera_id", req.CameraID).
		Msg("Bounding box mapping saved successfully")

	w.WriteHeader(http.StatusCreated)
	respondJSON(w, mapping)
}

// handleUpdateBox updates an existing bounding box mapping
func (h *MappingHandler) handleUpdateBox(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	boxID := vars["id"]

	if boxID == "" {
		respondError(w, http.StatusBadRequest, "bounding box id required")
		return
	}

	// Get existing mapping
	mapping, err := h.store.GetBoundingBox(boxID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Bounding box not found")
		return
	}

	// Parse request
	var req UpdateBoundingBoxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Update bounds if provided
	if req.Bounds != nil {
		if err := req.Bounds.Validate(); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid bounds: %s", err.Error()))
			return
		}
		mapping.Bounds = *req.Bounds
	}

	// Update adjustment if provided
	if req.Adjustment != nil {
		if err := req.Adjustment.Validate(); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid adjustment: %s", err.Error()))
			return
		}
		mapping.Adjustment = *req.Adjustment
	}

	// Save updated mapping
	if err := h.store.SaveBoundingBox(mapping); err != nil {
		log.Error().Err(err).Str("id", boxID).Msg("Failed to update bounding box")
		respondError(w, http.StatusInternalServerError, "Failed to update bounding box")
		return
	}

	respondJSON(w, mapping)
}

// handleDeleteBox deletes a bounding box mapping
func (h *MappingHandler) handleDeleteBox(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	boxID := vars["id"]

	if boxID == "" {
		respondError(w, http.StatusBadRequest, "bounding box id required")
		return
	}

	if err := h.store.DeleteBoundingBox(boxID); err != nil {
		log.Error().Err(err).Str("id", boxID).Msg("Failed to delete bounding box")
		respondError(w, http.StatusNotFound, "Bounding box not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
