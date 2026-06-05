package http

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

type CameraHandler struct {
	capturesDir string
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
	r.HandleFunc("/api/v1/devices/{deviceId}/captures", h.handleDeviceCaptures).Methods("GET")
	r.HandleFunc("/captures/{path:.*}", h.handleServeCapture).Methods("GET")
	r.HandleFunc("/captures/{path:.*}", h.handleDeleteCapture).Methods("DELETE")
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

	respondJSON(w, map[string]interface{}{
		"captures": captures,
		"count":    len(captures),
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
		respondError(w, http.StatusBadRequest, "Path required")
		return
	}

	fullPath, ok := h.sanitizePath(path)
	if !ok {
		log.Warn().Str("path", path).Msg("Rejecting unsafe delete request")
		respondError(w, http.StatusBadRequest, "Invalid path")
		return
	}

	// Verify file exists and is within captures dir
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "File not found")
		return
	}

	// Delete the file
	if err := os.Remove(fullPath); err != nil {
		log.Error().Err(err).Str("path", fullPath).Msg("Failed to delete capture")
		respondError(w, http.StatusInternalServerError, "Failed to delete file")
		return
	}

	log.Info().Str("path", fullPath).Msg("Capture deleted")

	respondJSON(w, map[string]interface{}{
		"status": "deleted",
		"path":   path,
	})
}

// handleListDeviceCaptures lists device-specific captures for a given full capture
func (h *CameraHandler) handleListDeviceCaptures(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	captureId := vars["captureId"]
	if captureId == "" {
		respondError(w, http.StatusBadRequest, "capture ID required")
		return
	}

	// Build full path to capture
	fullPath := filepath.Join(h.capturesDir, captureId)

	// Verify file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "Capture not found")
		return
	}

	// Check if it's an image file
	ext := strings.ToLower(filepath.Ext(fullPath))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		respondError(w, http.StatusBadRequest, "Not an image file")
		return
	}

	// Load device captures metadata
	store, err := camera.NewStore(h.capturesDir)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create store")
		respondError(w, http.StatusInternalServerError, "Failed to access storage")
		return
	}

	deviceCaptures, err := store.LoadDeviceCaptures(fullPath)
	if err != nil {
		log.Error().Err(err).Str("capture", captureId).Msg("Failed to load device captures")
		respondError(w, http.StatusInternalServerError, "Failed to load device captures")
		return
	}

	respondJSON(w, map[string]interface{}{
		"capture_id":      captureId,
		"device_captures": deviceCaptures,
		"count":           len(deviceCaptures),
	})
}

// handleDeviceCaptures lists all captures for a specific device across all cameras
func (h *CameraHandler) handleDeviceCaptures(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceId := vars["deviceId"]
	if deviceId == "" {
		respondError(w, http.StatusBadRequest, "device ID required")
		return
	}

	// Scan all captures to find device-specific ones
	var deviceCaptures []map[string]interface{}

	err := filepath.Walk(h.capturesDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Check for device capture metadata files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" {
			return nil
		}

		// Check if this is a device capture metadata file (ends with .json but not metadata.json)
		if filepath.Base(path) == "metadata.json" {
			return nil
		}

		// Read device capture metadata
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var captures []camera.DeviceCaptureInfo
		if err := json.Unmarshal(data, &captures); err != nil {
			return nil
		}

		// Find captures for the requested device
		for _, cap := range captures {
			if cap.DeviceID == deviceId {
				relPath, _ := filepath.Rel(h.capturesDir, path)
				deviceCaptures = append(deviceCaptures, map[string]interface{}{
					"device_id":     cap.DeviceID,
					"bounds":        cap.Bounds,
					"subimage":      "/captures/" + cap.Subimage,
					"adjustment":    cap.Adjustment,
					"generated_at":  cap.GeneratedAt.Format(time.RFC3339),
					"metadata_path": "/captures/" + relPath,
				})
			}
		}

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		log.Error().Err(err).Msg("Failed to scan captures")
		respondError(w, http.StatusInternalServerError, "Failed to scan captures")
		return
	}

	respondJSON(w, map[string]interface{}{
		"device_id":       deviceId,
		"device_captures": deviceCaptures,
		"count":           len(deviceCaptures),
	})
}
