package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/flash"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// ReadFlashHandler handles read-flash API endpoints
type ReadFlashHandler struct {
	node   cluster.Node
	leader *cluster.LeaderNode
	jobs   map[string]*ReadJob
	mu     sync.RWMutex
	tmpDir string
}

// ReadJob represents a flash read operation
type ReadJob struct {
	ID         string
	DevicePath string
	Address    uint32
	Size       uint32
	Chip       string
	Status     string
	DataPath   string
	Error      string
	CreatedAt  time.Time
	mu         sync.Mutex
}

// ReadFlashRequest represents a read job submission
type ReadFlashRequest struct {
	DevicePath string `json:"device_path"`
	Address    uint32 `json:"address"`
	Size       uint32 `json:"size"`
	Chip       string `json:"chip,omitempty"`
	ClientID   string `json:"client_id,omitempty"`
}

// ReadFlashResponse represents a read job response
type ReadFlashResponse struct {
	JobID       string `json:"job_id"`
	Status      string `json:"status"`
	DevicePath  string `json:"device_path,omitempty"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"download_url,omitempty"`
	Error       string `json:"error,omitempty"`
}

const (
	maxReadSize    = 16 * 1024 * 1024 // 16MB
	jobCleanupTime = 10 * time.Minute
)

// NewReadFlashHandler creates a new read-flash handler
func NewReadFlashHandler(node cluster.Node, leader *cluster.LeaderNode, tmpDir string) *ReadFlashHandler {
	h := &ReadFlashHandler{
		node:   node,
		leader: leader,
		jobs:   make(map[string]*ReadJob),
		tmpDir: tmpDir,
	}
	go h.cleanupLoop()
	return h
}

// RegisterRoutes registers read-flash routes
func (h *ReadFlashHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/flash/read", h.handleSubmit).Methods("POST")
	api.HandleFunc("/flash/read/{id}", h.handleStatus).Methods("GET")
	api.HandleFunc("/flash/download/{id}", h.handleDownload).Methods("GET")
}

// handleSubmit handles read job submission
func (h *ReadFlashHandler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req ReadFlashRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Validate request
	if req.DevicePath == "" {
		respondError(w, http.StatusBadRequest, "device_path is required")
		return
	}

	if req.Size == 0 {
		respondError(w, http.StatusBadRequest, "size must be greater than 0")
		return
	}

	if req.Size > maxReadSize {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("size exceeds maximum of %d bytes", maxReadSize))
		return
	}

	// Check if device is available
	state := h.node.State()
	deviceAvailable := false
	for _, dev := range state.Devices {
		if dev.Path == req.DevicePath && dev.Status == "available" {
			deviceAvailable = true
			break
		}
	}

	if !deviceAvailable {
		respondError(w, http.StatusNotFound, fmt.Sprintf("device not found or not available: %s", req.DevicePath))
		return
	}

	// Create job
	jobID := "read-" + uuid.New().String()
	job := &ReadJob{
		ID:         jobID,
		DevicePath: req.DevicePath,
		Address:    req.Address,
		Size:       req.Size,
		Chip:       req.Chip,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}

	h.mu.Lock()
	h.jobs[jobID] = job
	h.mu.Unlock()

	log.Info().Str("job_id", jobID).Str("device", req.DevicePath).
		Uint32("address", req.Address).Uint32("size", req.Size).Msg("Read job submitted")

	// Start processing in background
	go h.processJob(job)

	respondJSON(w, ReadFlashResponse{
		JobID:      jobID,
		Status:     "pending",
		DevicePath: req.DevicePath,
		Size:       int64(req.Size),
	})
}

// handleStatus handles job status requests
func (h *ReadFlashHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	h.mu.RLock()
	job, exists := h.jobs[jobID]
	h.mu.RUnlock()

	if !exists {
		respondError(w, http.StatusNotFound, fmt.Sprintf("job not found: %s", jobID))
		return
	}

	job.mu.Lock()
	status := job.Status
	var downloadURL string
	if status == "completed" && job.DataPath != "" {
		downloadURL = fmt.Sprintf("/api/v1/flash/download/%s", jobID)
	}
	job.mu.Unlock()

	resp := ReadFlashResponse{
		JobID:       jobID,
		Status:      status,
		DevicePath:  job.DevicePath,
		DownloadURL: downloadURL,
	}

	if status == "completed" {
		if info, err := os.Stat(job.DataPath); err == nil {
			resp.Size = info.Size()
		}
	}

	if status == "failed" {
		resp.Error = job.Error
	}

	respondJSON(w, resp)
}

// handleDownload handles read data downloads
func (h *ReadFlashHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	h.mu.RLock()
	job, exists := h.jobs[jobID]
	h.mu.RUnlock()

	if !exists {
		respondError(w, http.StatusNotFound, fmt.Sprintf("job not found: %s", jobID))
		return
	}

	job.mu.Lock()
	status := job.Status
	dataPath := job.DataPath
	job.mu.Unlock()

	if status != "completed" {
		respondError(w, http.StatusNotFound, fmt.Sprintf("job not completed: %s (status: %s)", jobID, status))
		return
	}

	if dataPath == "" {
		respondError(w, http.StatusInternalServerError, "no data available for download")
		return
	}

	data, err := os.ReadFile(dataPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("read data: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=flash-read-%s.bin", jobID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	if _, err := w.Write(data); err != nil {
		log.Error().Str("job_id", jobID).Err(err).Msg("Failed to send data")
	}
}

// processJob executes the read operation
func (h *ReadFlashHandler) processJob(job *ReadJob) {
	job.mu.Lock()
	job.Status = "running"
	job.mu.Unlock()

	log.Info().Str("job_id", job.ID).Msg("Processing read job")

	flasher := flash.NewFlasher(nil)

	req := &flash.ReadFlashRequest{
		Port:    job.DevicePath,
		Address: job.Address,
		Size:    job.Size,
	}

	ctx := context.Background()
	result := flasher.ReadFlash(ctx, req)

	job.mu.Lock()
	defer job.mu.Unlock()

	if !result.Success {
		job.Status = "failed"
		job.Error = result.Error.Error()
		log.Error().Str("job_id", job.ID).Err(result.Error).Msg("Read job failed")
		return
	}

	// Save data to temp file
	dataPath := filepath.Join(h.tmpDir, job.ID+".bin")
	if err := os.MkdirAll(h.tmpDir, 0755); err != nil {
		job.Status = "failed"
		job.Error = fmt.Sprintf("create temp dir: %v", err)
		log.Error().Str("job_id", job.ID).Err(err).Msg("Failed to create temp dir")
		return
	}

	if err := os.WriteFile(dataPath, result.Data, 0644); err != nil {
		job.Status = "failed"
		job.Error = fmt.Sprintf("save data: %v", err)
		log.Error().Str("job_id", job.ID).Err(err).Msg("Failed to save data")
		return
	}

	job.Status = "completed"
	job.DataPath = dataPath
	log.Info().Str("job_id", job.ID).Int("bytes", len(result.Data)).Msg("Read job completed")
}

// cleanupLoop periodically removes old jobs
func (h *ReadFlashHandler) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.cleanup()
	}
}

// cleanup removes jobs older than jobCleanupTime
func (h *ReadFlashHandler) cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	cutoff := time.Now().Add(-jobCleanupTime)
	for jobID, job := range h.jobs {
		if job.CreatedAt.Before(cutoff) {
			// Remove temp file
			if job.DataPath != "" {
				os.Remove(job.DataPath)
			}
			delete(h.jobs, jobID)
			log.Debug().Str("job_id", jobID).Msg("Cleaned up old read job")
		}
	}
}
