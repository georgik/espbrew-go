package http

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

type CameraHandler struct {
	capturesDir string
	cameras     []*camera.CameraInfo
	camerasMu   sync.RWMutex
}

type CaptureInfo struct {
	Path       string `json:"path"`
	Filename   string `json:"filename"`
	CameraID   string `json:"camera_id"`
	CameraName string `json:"camera_name"`
	Timestamp  int64  `json:"timestamp"`
	Size       int64  `json:"size"`
}

type CaptureMetadata struct {
	CameraID   string `json:"camera_id"`
	CameraName string `json:"camera_name"`
	Timestamp  int64  `json:"timestamp"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Quality    int    `json:"quality"`
}

func NewCameraHandler() *CameraHandler {
	homeDir, _ := os.UserHomeDir()
	capturesDir := filepath.Join(homeDir, ".espbrew", "captures")
	return &CameraHandler{
		capturesDir: capturesDir,
	}
}

func (h *CameraHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/api/v1/captures", h.handleListCaptures).Methods("GET")
	r.HandleFunc("/api/v1/captures/{captureId:.*}/devices", h.handleListDeviceCaptures).Methods("GET")
	r.HandleFunc("/api/v1/captures/{path:.*}", h.handleDeleteCapture).Methods("DELETE")
	r.HandleFunc("/captures/{path:.*}", h.handleServeCapture).Methods("GET")
}

func (h *CameraHandler) handleListCaptures(w http.ResponseWriter, r *http.Request) {
	captures, err := h.scanCaptures()
	if err != nil {
		log.Error().Err(err).Msg("Failed to scan captures")
		respondJSON(w, map[string]interface{}{
			"captures": []interface{}{},
		})
		return
	}

	// Parse pagination parameters
	query := r.URL.Query()
	page := 1
	limit := 40

	if pageStr := query.Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	total := len(captures)
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	// Validate page number
	if page > totalPages {
		page = totalPages
	}

	// Slice captures for current page
	start := (page - 1) * limit
	end := start + limit
	if end > total {
		end = total
	}

	var pageCaptures []*CaptureInfo
	if start < total {
		pageCaptures = captures[start:end]
	}

	respondJSON(w, map[string]interface{}{
		"captures":    pageCaptures,
		"count":       len(pageCaptures),
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

func (h *CameraHandler) handleServeCapture(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := vars["path"]
	if path == "" {
		respondError(w, http.StatusNotFound, "Not found")
		return
	}

	fullPath := filepath.Join(h.capturesDir, path)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "File not found")
		return
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	http.ServeFile(w, r, fullPath)
}

// getCameraNameByID looks up camera name by ID from registry
func getCameraNameByID(cameraID string) string {
	if cameraID == "" || cameraID == "unknown" {
		return "Unknown Camera"
	}
	// Use global camera registry
	registry := camera.GetRegistry()
	return registry.GetNameByID(cameraID)
}

func (h *CameraHandler) scanCaptures() ([]*CaptureInfo, error) {
	var captures []*CaptureInfo

	err := filepath.Walk(h.capturesDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil
		}

		relPath, _ := filepath.Rel(h.capturesDir, path)
		urlPath := "/captures/" + relPath

		stat, _ := os.Stat(path)
		size := stat.Size()
		modTime := stat.ModTime().Unix()

		cameraID := "unknown"
		cameraName := "Unknown Camera"

		metadataPath := filepath.Join(filepath.Dir(path), "metadata.json")
		if metadataData, err := os.ReadFile(metadataPath); err == nil {
			// Metadata structure: { "camera-id": { "camera_id": "...", "captures": [...] } }
			type StorageCaptureMetadata struct {
				Filename  string    `json:"filename"`
				Timestamp time.Time `json:"timestamp"`
				CameraID  string    `json:"camera_id"`
				Format    string    `json:"format"`
				SizeBytes int64     `json:"size_bytes"`
			}
			var metadata map[string]struct {
				CameraID  string                   `json:"camera_id"`
				CreatedAt string                   `json:"created_at"`
				Captures  []StorageCaptureMetadata `json:"captures"`
			}
			if json.Unmarshal(metadataData, &metadata) == nil {
				filename := filepath.Base(path)
				// Search through all cameras' captures to find this file
				for _, camData := range metadata {
					for _, capture := range camData.Captures {
						if capture.Filename == filename {
							cameraID = capture.CameraID
							modTime = capture.Timestamp.Unix()
							break
						}
					}
					if cameraID != "unknown" {
						break
					}
				}
			}
		}

		// Look up camera name from camera ID if found
		if cameraID != "unknown" {
			cameraName = getCameraNameByID(cameraID)
		}

		captures = append(captures, &CaptureInfo{
			Path:       urlPath,
			Filename:   filepath.Base(path),
			CameraID:   cameraID,
			CameraName: cameraName,
			Timestamp:  modTime,
			Size:       size,
		})

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	sort.Slice(captures, func(i, j int) bool {
		return captures[i].Timestamp > captures[j].Timestamp
	})

	return captures, nil
}

// sanitizePath validates the path is within captures directory and safe
func (h *CameraHandler) sanitizePath(path string) (string, bool) {
	// Remove any leading slashes
	path = strings.TrimPrefix(path, "/")
	// Clean the path to resolve any .. components
	path = filepath.Clean(path)

	// Ensure path doesn't escape captures directory
	fullPath := filepath.Join(h.capturesDir, path)
	relPath, err := filepath.Rel(h.capturesDir, fullPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", false
	}

	// Only allow image files
	ext := strings.ToLower(filepath.Ext(fullPath))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return "", false
	}

	return fullPath, true
}

func (h *CameraHandler) handleDeleteCapture(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := vars["path"]
	if path == "" {
		respondError(w, http.StatusBadRequest, "Path is required")
		return
	}

	// Validate path
	fullPath, valid := h.sanitizePath(path)
	if !valid {
		respondError(w, http.StatusBadRequest, "Invalid path")
		return
	}

	// Delete file
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "File not found")
		} else {
			respondError(w, http.StatusInternalServerError, "Failed to delete file")
		}
		return
	}

	respondJSON(w, map[string]interface{}{
		"status": "deleted",
		"path":   path,
	})
}

func (h *CameraHandler) handleListDeviceCaptures(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	captureID := vars["captureId"]
	if captureID == "" {
		respondError(w, http.StatusBadRequest, "Capture ID is required")
		return
	}

	// Get capture path
	captures, err := h.scanCaptures()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to scan captures")
		return
	}

	var capture *CaptureInfo
	for _, c := range captures {
		if strings.HasPrefix(c.Path, "/captures/"+captureID) {
			capture = c
			break
		}
	}

	if capture == nil {
		respondError(w, http.StatusNotFound, "Capture not found")
		return
	}

	// Load metadata for device captures
	metadataPath := filepath.Join(h.capturesDir, captureID, "metadata.json")
	var deviceCaptures []map[string]interface{}

	if metadataData, err := os.ReadFile(metadataPath); err == nil {
		var metadata map[string]interface{}
		if json.Unmarshal(metadataData, &metadata) == nil {
			// Extract device capture info
			for camID, camData := range metadata {
				if camMap, ok := camData.(map[string]interface{}); ok {
					if captures, ok := camMap["captures"].([]interface{}); ok {
						for _, cap := range captures {
							if capMap, ok := cap.(map[string]interface{}); ok {
								deviceCaptures = append(deviceCaptures, map[string]interface{}{
									"camera_id": camID,
									"filename":  capMap["filename"],
									"timestamp": capMap["timestamp"],
								})
							}
						}
					}
				}
			}
		}
	}

	respondJSON(w, map[string]interface{}{
		"capture":         capture,
		"device_captures": deviceCaptures,
	})
}
