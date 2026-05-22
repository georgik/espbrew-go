package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/georgik/esp-ci-cluster/internal/cluster"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

type FlashHandler struct {
	master    *cluster.MasterNode
	uploadDir string
}

func NewFlashHandler(master *cluster.MasterNode, uploadDir string) *FlashHandler {
	return &FlashHandler{
		master:    master,
		uploadDir: uploadDir,
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
}

type FlashSubmitResponse struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	DevicePath string `json:"device_path"`
}

func (h *FlashHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if h.master == nil {
		respondError(w, http.StatusNotImplemented, "Upload only available on master")
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
	if h.master == nil {
		respondError(w, http.StatusNotImplemented, "Remote flash only available on master")
		return
	}

	var req FlashSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
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

	// Enqueue job
	job, err := h.master.EnqueueJob(firmwarePath, req.DevicePath)
	if err != nil {
		respondError(w, http.StatusConflict, err.Error())
		return
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
