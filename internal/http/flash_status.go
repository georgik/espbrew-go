package http

import (
	"encoding/json"
	"net/http"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/flashhash"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// FlashStatusHandler handles flash status queries
type FlashStatusHandler struct {
	store *persistence.Store
}

// NewFlashStatusHandler creates a new flash status handler
func NewFlashStatusHandler(store *persistence.Store) *FlashStatusHandler {
	return &FlashStatusHandler{
		store: store,
	}
}

// RegisterRoutes registers flash status routes
func (h *FlashStatusHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	// Flash status query
	api.HandleFunc("/devices/{deviceId}/flash-status", h.handleFlashStatus).Methods("POST")

	// Job hash management
	api.HandleFunc("/jobs/{jobId}/hashes", h.handleGetJobHashes).Methods("GET")
	api.HandleFunc("/jobs/{jobId}/hashes", h.handlePutJobHashes).Methods("PUT")
	api.HandleFunc("/jobs/{jobId}/hashes", h.handleDeleteJobHashes).Methods("DELETE")
}

// handleFlashStatus processes a flash status query
func (h *FlashStatusHandler) handleFlashStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["deviceId"]

	if deviceID == "" {
		respondError(w, http.StatusBadRequest, "device_id required")
		return
	}

	var req flashhash.FlashStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate device ID matches
	if req.DeviceID != deviceID {
		respondError(w, http.StatusBadRequest, "device_id mismatch")
		return
	}

	log.Info().
		Str("device_id", deviceID).
		Int("regions", len(req.Regions)).
		Msg("Flash status query")

	// Validate client regions
	for i, region := range req.Regions {
		if err := region.Validate(); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid region at index "+string(rune(i))+": "+err.Error())
			return
		}
	}

	// Get job hashes for this device (most recent job)
	jobHashes, err := h.getLatestJobHashes(deviceID)
	if err != nil || jobHashes == nil {
		log.Warn().Err(err).Str("device_id", deviceID).Msg("No job hashes found, full flash required")
		respondJSON(w, flashhash.FlashStatusResponse{
			Status:  "full_flash",
			Message: "No previous job hashes found",
		})
		return
	}

	// Compare regions
	needed, cached := flashhash.CompareRegions(req.Regions, jobHashes.Regions)

	// Find regions that are in job but not on client
	missing := flashhash.MergeRegions(req.Regions, jobHashes.Regions)
	needed = append(needed, missing...)

	response := flashhash.FlashStatusResponse{
		Status:        "partial_update",
		RegionsNeeded: needed,
		RegionsCached: make([]flashhash.CachedRegionInfo, 0, len(cached)),
		JobID:         jobHashes.JobID,
	}

	// Build cached regions info
	for _, region := range cached {
		response.RegionsCached = append(response.RegionsCached, flashhash.CachedRegionInfo{
			Name:   region.Name,
			Reason: "hash_match",
		})
	}

	// Determine final status
	if len(needed) == 0 {
		response.Status = "skip_all"
		response.Message = "All regions match hashes"
	} else if len(needed) == len(jobHashes.Regions) {
		response.Status = "full_flash"
		response.Message = "No regions match hashes"
	} else {
		response.Message = "Partial update available"
	}

	log.Info().
		Str("device_id", deviceID).
		Str("status", response.Status).
		Int("needed", len(needed)).
		Int("cached", len(cached)).
		Msg("Flash status response")

	respondJSON(w, response)
}

// handleGetJobHashes returns flash hashes for a specific job
func (h *FlashStatusHandler) handleGetJobHashes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["jobId"]

	if jobID == "" {
		respondError(w, http.StatusBadRequest, "job_id required")
		return
	}

	hashes, err := h.store.GetFlashHashes(jobID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Job hashes not found")
		return
	}

	respondJSON(w, hashes)
}

// handlePutJobHashes updates flash hashes for a specific job
func (h *FlashStatusHandler) handlePutJobHashes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["jobId"]

	if jobID == "" {
		respondError(w, http.StatusBadRequest, "job_id required")
		return
	}

	var hashes flashhash.JobFlashHashes
	if err := json.NewDecoder(r.Body).Decode(&hashes); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Set job ID from URL
	hashes.JobID = jobID

	// Validate regions
	for _, region := range hashes.Regions {
		if err := region.Validate(); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid region: "+err.Error())
			return
		}
	}

	hashes.CreatedAt = time.Now().Format(time.RFC3339)

	if err := h.store.SaveFlashHashes(&hashes); err != nil {
		log.Error().Err(err).Str("job_id", jobID).Msg("Failed to save job hashes")
		respondError(w, http.StatusInternalServerError, "Failed to save job hashes")
		return
	}

	log.Info().
		Str("job_id", jobID).
		Int("regions", len(hashes.Regions)).
		Msg("Job hashes updated")

	respondJSON(w, hashes)
}

// handleDeleteJobHashes removes flash hashes for a job
func (h *FlashStatusHandler) handleDeleteJobHashes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["jobId"]

	if jobID == "" {
		respondError(w, http.StatusBadRequest, "job_id required")
		return
	}

	if err := h.store.DeleteFlashHashes(jobID); err != nil {
		log.Error().Err(err).Str("job_id", jobID).Msg("Failed to delete job hashes")
		respondError(w, http.StatusInternalServerError, "Failed to delete job hashes")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getLatestJobHashes retrieves the most recent job hashes for a device
func (h *FlashStatusHandler) getLatestJobHashes(deviceID string) (*flashhash.JobFlashHashes, error) {
	hashes, err := h.store.ListFlashHashesForDevice(deviceID)
	if err != nil || len(hashes) == 0 {
		return nil, err
	}

	// Return the most recent one (first in list, should be sorted by creation)
	return hashes[0], nil
}
