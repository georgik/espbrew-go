package http

import (
	"encoding/json"
	"net/http"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// PeerAPIHandler handles peer-specific API endpoints for job assignment.
type PeerAPIHandler struct {
	peer *cluster.PeerNode
}

// NewPeerAPIHandler creates a new peer API handler.
func NewPeerAPIHandler(peer *cluster.PeerNode) *PeerAPIHandler {
	return &PeerAPIHandler{peer: peer}
}

// JobAssignRequest is the payload for job assignment from leader.
type JobAssignRequest struct {
	JobID        string `json:"job_id"`
	JobType      string `json:"job_type"` // "flash" or "erase"
	DevicePath   string `json:"device_path"`
	Firmware     string `json:"firmware"`      // Firmware path (for flash jobs)
	Offset       int    `json:"offset"`        // Flash offset (for flash jobs)
	Erase        bool   `json:"erase"`         // Enable erase before flash
	EraseAll     bool   `json:"erase_all"`     // For erase jobs
	EraseAddress uint32 `json:"erase_address"` // For erase jobs
	EraseSize    uint32 `json:"erase_size"`    // For erase jobs
}

// JobAssignResponse is the response to a job assignment.
type JobAssignResponse struct {
	Status  string `json:"status"` // "accepted", "rejected"
	Message string `json:"message,omitempty"`
	JobID   string `json:"job_id"`
}

// handlePeerJobAssign handles POST /api/v1/jobs/assign
func (h *PeerAPIHandler) handlePeerJobAssign(w http.ResponseWriter, r *http.Request) {
	var req JobAssignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	log.Info().
		Str("job_id", req.JobID).
		Str("device", req.DevicePath).
		Str("job_type", req.JobType).
		Msg("Job assignment received from leader")

	// Validate device ownership
	state := h.peer.State()
	dev, exists := state.Devices[req.DevicePath]
	if !exists {
		log.Warn().Str("device", req.DevicePath).Msg("Device not found on peer")
		h.writeAssignResponse(w, "rejected", "device not found", req.JobID, http.StatusNotFound)
		return
	}

	if dev.NodeID != h.peer.ID() {
		log.Warn().
			Str("device", req.DevicePath).
			Str("device_node", dev.NodeID).
			Str("peer_id", h.peer.ID()).
			Msg("Device does not belong to this peer")
		h.writeAssignResponse(w, "rejected", "device ownership mismatch", req.JobID, http.StatusForbidden)
		return
	}

	if dev.Status != "available" {
		log.Warn().
			Str("device", req.DevicePath).
			Str("status", dev.Status).
			Msg("Device not available")
		h.writeAssignResponse(w, "rejected", "device not available", req.JobID, http.StatusServiceUnavailable)
		return
	}

	// Create job object
	jobType := cluster.JobType(req.JobType)
	job := &cluster.Job{
		ID:           req.JobID,
		Type:         jobType,
		DevicePath:   req.DevicePath,
		Firmware:     req.Firmware,
		Offset:       req.Offset,
		Erase:        req.Erase,
		EraseAll:     req.EraseAll,
		EraseAddress: req.EraseAddress,
		EraseSize:    req.EraseSize,
		Status:       cluster.JobPending,
		CreatedAt:    time.Now(),
	}

	// Assign job to peer
	ctx := r.Context()
	if err := h.peer.AssignJob(ctx, job); err != nil {
		log.Error().Err(err).Str("job_id", req.JobID).Msg("Failed to assign job")
		h.writeAssignResponse(w, "rejected", err.Error(), req.JobID, http.StatusInternalServerError)
		return
	}

	h.writeAssignResponse(w, "accepted", "job assigned", req.JobID, http.StatusAccepted)
	log.Info().Str("job_id", req.JobID).Msg("Job accepted")
}

// handlePeerJobCancel handles DELETE /api/v1/jobs/{id}/cancel
func (h *PeerAPIHandler) handlePeerJobCancel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]
	if jobID == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}

	log.Info().Str("job_id", jobID).Msg("Job cancellation received from leader")

	if err := h.peer.CancelJob(jobID); err != nil {
		log.Error().Err(err).Str("job_id", jobID).Msg("Failed to cancel job")
		if err == cluster.ErrJobNotFound {
			http.Error(w, "job not found", http.StatusNotFound)
		} else {
			http.Error(w, "cancel failed: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "cancelled",
		"job_id": jobID,
	})
}

// handlePeerHealth handles GET /health
func (h *PeerAPIHandler) handlePeerHealth(w http.ResponseWriter, r *http.Request) {
	state := h.peer.State()
	health := map[string]interface{}{
		"status":  "healthy",
		"node_id": h.peer.ID(),
		"mode":    h.peer.GetMode(),
		"devices": len(state.Devices),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (h *PeerAPIHandler) writeAssignResponse(w http.ResponseWriter, status, message, jobID string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(JobAssignResponse{
		Status:  status,
		Message: message,
		JobID:   jobID,
	})
}
