package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

type FlashHandler struct {
	leader    *cluster.LeaderNode
	uploadDir string
	progress  *ProgressHandler
}

func NewFlashHandler(leader *cluster.LeaderNode, uploadDir string, progress *ProgressHandler) *FlashHandler {
	return &FlashHandler{
		leader:    leader,
		uploadDir: uploadDir,
		progress:  progress,
	}
}

func (h *FlashHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/flash/upload", h.handleUpload).Methods("POST")
	api.HandleFunc("/flash", h.handleFlashSubmit).Methods("POST")
}

type FlashUploadResponse struct {
	FileID string `json:"file_id"`
	Size   int64  `json:"size"`
}

type FlashSubmitRequest struct {
	DevicePath  string                 `json:"device_path"`
	FileID      string                 `json:"file_id"`
	FirmwareURL string                 `json:"firmware_url,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
	ClientID    string                 `json:"client_id,omitempty"`
	Offset      int                    `json:"offset,omitempty"`
}

type FlashSubmitResponse struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	DevicePath string `json:"device_path"`
}

func (h *FlashHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Upload only available on leader")
		return
	}

	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("parse form: %v", err))
		return
	}

	file, _, err := r.FormFile("firmware")
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("get file: %v", err))
		return
	}
	defer file.Close()

	fileID := uuid.New().String()
	filePath := filepath.Join(h.uploadDir, fileID+".bin")

	out, err := os.Create(filePath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("create file: %v", err))
		return
	}
	defer out.Close()

	size, err := io.Copy(out, file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("save file: %v", err))
		return
	}

	log.Info().Str("file_id", fileID).Int64("size", size).Msg("Firmware uploaded")

	respondJSON(w, FlashUploadResponse{
		FileID: fileID,
		Size:   size,
	})
}

func (h *FlashHandler) handleFlashSubmit(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Remote flash only available on leader")
		return
	}

	var req FlashSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
		return
	}

	// Check if device is disabled
	if IsDeviceDisabled(h.leader.State(), req.DevicePath) {
		respondError(w, http.StatusForbidden, "device is disabled and cannot be flashed")
		return
	}

	// Determine firmware path
	var firmwarePath string
	if req.FileID != "" {
		firmwarePath = filepath.Join(h.uploadDir, req.FileID+".bin")
		if _, err := os.Stat(firmwarePath); os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "uploaded file not found")
			return
		}
	} else if req.FirmwareURL != "" {
		// TODO: Implement URL fetch
		respondError(w, http.StatusNotImplemented, "URL fetch not yet implemented")
		return
	} else {
		respondError(w, http.StatusBadRequest, "either file_id or firmware_url required")
		return
	}

	// Enqueue job with offset
	job, err := h.leader.EnqueueJobWithOffset(firmwarePath, req.DevicePath, req.Offset)
	if err != nil {
		respondError(w, http.StatusConflict, err.Error())
		return
	}

	// Start progress streaming in background
	if h.progress != nil {
		go h.progress.StartProgressStreamer(job.ID)
	}

	log.Info().
		Str("job_id", job.ID).
		Str("device", req.DevicePath).
		Str("client_id", req.ClientID).
		Msg("Remote flash job created")

	respondJSON(w, FlashSubmitResponse{
		JobID:      job.ID,
		Status:     string(job.Status),
		DevicePath: req.DevicePath,
	})
}

// PeerFlashHandler handles flash operations on peer nodes
type PeerFlashHandler struct {
	peer      *cluster.PeerNode
	uploadDir string
}

func NewPeerFlashHandler(peer *cluster.PeerNode) *PeerFlashHandler {
	return &PeerFlashHandler{
		peer:      peer,
		uploadDir: os.TempDir(),
	}
}

func (h *PeerFlashHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/flash/upload", h.handleUpload).Methods("POST")
	api.HandleFunc("/flash", h.handleFlashSubmit).Methods("POST")
}

func (h *PeerFlashHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("parse form: %v", err))
		return
	}

	file, _, err := r.FormFile("firmware")
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("get file: %v", err))
		return
	}
	defer file.Close()

	fileID := uuid.New().String()
	filePath := filepath.Join(h.uploadDir, fileID+".bin")

	out, err := os.Create(filePath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("create file: %v", err))
		return
	}
	defer out.Close()

	size, err := io.Copy(out, file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("save file: %v", err))
		return
	}

	log.Info().Str("file_id", fileID).Int64("size", size).Msg("Firmware uploaded on peer")

	respondJSON(w, FlashUploadResponse{
		FileID: fileID,
		Size:   size,
	})
}

func (h *PeerFlashHandler) handleFlashSubmit(w http.ResponseWriter, r *http.Request) {
	var req FlashSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
		return
	}

	// Check if device exists locally
	state := h.peer.State()
	if _, exists := state.Devices[req.DevicePath]; !exists {
		respondError(w, http.StatusNotFound, fmt.Sprintf("device not found: %s", req.DevicePath))
		return
	}

	// Check if device is disabled
	if IsDeviceDisabled(state, req.DevicePath) {
		respondError(w, http.StatusForbidden, "device is disabled and cannot be flashed")
		return
	}

	// Determine firmware path
	var firmwarePath string
	if req.FileID != "" {
		firmwarePath = filepath.Join(h.uploadDir, req.FileID+".bin")
		if _, err := os.Stat(firmwarePath); os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "uploaded file not found")
			return
		}
	} else {
		respondError(w, http.StatusBadRequest, "file_id required")
		return
	}

	// Create job and execute directly
	job := &cluster.Job{
		ID:         uuid.New().String(),
		Firmware:   firmwarePath,
		DevicePath: req.DevicePath,
		Status:     cluster.JobPending,
		CreatedAt:  time.Now(),
	}

	// Execute flash synchronously
	ctx := context.Background()
	if err := h.peer.ExecuteJob(ctx, job); err != nil {
		log.Error().Err(err).Str("job_id", job.ID).Msg("Flash failed on peer")
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Info().
		Str("job_id", job.ID).
		Str("device", req.DevicePath).
		Msg("Flash completed on peer")

	respondJSON(w, FlashSubmitResponse{
		JobID:      job.ID,
		Status:     string(job.Status),
		DevicePath: req.DevicePath,
	})
}
