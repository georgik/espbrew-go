package http

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"codeberg.org/georgik/espbrew-go/internal/cluster"
	flashlib "codeberg.org/georgik/espbrew-go/internal/flash"
	"codeberg.org/georgik/espbrew-go/internal/flash/virtual"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"codeberg.org/georgik/espbrew-go/internal/snap"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// SnapAPI handles device snapshot operations across the cluster.
type SnapAPI struct {
	store  *persistence.Store
	leader *cluster.LeaderNode
	node   cluster.Node
}

// NewSnapAPI creates a new snap API handler.
func NewSnapAPI(store *persistence.Store, node cluster.Node) *SnapAPI {
	h := &SnapAPI{
		store: store,
		node:  node,
	}

	// Type assertion for leader-specific operations
	if l, ok := node.(*cluster.LeaderNode); ok {
		h.leader = l
	}

	return h
}

// RegisterRoutes registers snap-related routes.
func (h *SnapAPI) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	// Device snap operations (use query param to avoid URL encoding issues with device paths)
	api.HandleFunc("/devices/snap", h.handleSnap).Methods("POST")
	api.HandleFunc("/devices/{deviceId}/flash-hash", h.handleHashCheck).Methods("POST")
	api.HandleFunc("/devices/{deviceId}/snaps", h.handleListSnaps).Methods("GET")
	api.HandleFunc("/snaps/{snapId}", h.handleGetSnap).Methods("GET")
}

// SnapRequest represents a request to create a device snapshot.
type SnapRequest struct {
	Duration    int    `json:"duration"`     // Monitor duration in seconds (default: 10)
	CameraID    string `json:"camera_id"`    // Camera device identifier (optional)
	Firmware    string `json:"firmware"`     // Firmware path to flash (required if not skip_flash)
	ForceFlash  bool   `json:"force_flash"`  // Force flash even if hash matches
	SkipFlash   bool   `json:"skip_flash"`   // Skip flashing entirely
	SkipCapture bool   `json:"skip_capture"` // Skip camera capture
	SkipMonitor bool   `json:"skip_monitor"` // Skip serial monitoring
}

// SnapResponse represents the response from a snap operation.
type SnapResponse struct {
	SnapID string      `json:"snap_id"`
	Status string      `json:"status"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// HashCheckRequest represents a request to check flash hash.
type HashCheckRequest struct {
	Firmware string `json:"firmware"` // Firmware path to check (required)
	Chip     string `json:"chip"`     // Chip type (optional, default: esp32s3)
}

// HashCheckResponse represents the response from a hash check operation.
type HashCheckResponse struct {
	DeviceID      string `json:"device_id"`
	Match         bool   `json:"match"`
	DeviceHash    string `json:"device_hash,omitempty"`
	FirmwareHash  string `json:"firmware_hash,omitempty"`
	FlashRequired bool   `json:"flash_required"`
	Status        string `json:"status"` // "checked", "error"
	Error         string `json:"error,omitempty"`
}

// handleSnap processes a device snapshot request.
func (h *SnapAPI) handleSnap(w http.ResponseWriter, r *http.Request) {
	// Get device_id from query parameter (avoids URL encoding issues with device paths)
	deviceID := r.URL.Query().Get("device_id")

	if deviceID == "" {
		respondError(w, http.StatusBadRequest, "device_id query parameter required")
		return
	}

	var req SnapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Set defaults
	if req.Duration == 0 {
		req.Duration = 10 // Default 10 seconds
	}

	log.Info().
		Str("device_id", deviceID).
		Int("duration", req.Duration).
		Str("camera_id", req.CameraID).
		Str("firmware", req.Firmware).
		Bool("force_flash", req.ForceFlash).
		Bool("skip_flash", req.SkipFlash).
		Msg("Snap request")

	// Find device in cluster
	devicePath, deviceNode, err := h.findDeviceInCluster(deviceID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Device not found: "+err.Error())
		return
	}

	// If device is on a remote peer, forward the request
	localNodeID := h.node.ID()
	if deviceNode != "" && deviceNode != localNodeID {
		err := h.forwardSnapRequest(w, r, deviceID, deviceNode, &req)
		if err != nil {
			respondError(w, http.StatusBadGateway, "Failed to forward request: "+err.Error())
		}
		return
	}

	// Device is on local node, execute snap directly
	result, err := h.executeSnap(r.Context(), devicePath, &req)
	if err != nil {
		log.Error().Err(err).Str("device_id", deviceID).Msg("Snap failed")
		respondJSON(w, SnapResponse{
			SnapID: "",
			Status: "failed",
			Error:  err.Error(),
		})
		return
	}

	respondJSON(w, SnapResponse{
		SnapID: result.Metadata.SnapID,
		Status: string(result.Metadata.Status),
		Result: result.ToMap(true), // Include logs for client-side saving
	})
}

// handleHashCheck processes a flash hash check request.
func (h *SnapAPI) handleHashCheck(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["deviceId"]

	if deviceID == "" {
		respondError(w, http.StatusBadRequest, "device_id required")
		return
	}

	var req HashCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.Firmware == "" {
		respondError(w, http.StatusBadRequest, "firmware path required")
		return
	}

	// Set default chip type
	chip := req.Chip
	if chip == "" {
		chip = "esp32s3"
	}

	log.Info().
		Str("device_id", deviceID).
		Str("firmware", req.Firmware).
		Str("chip", chip).
		Msg("Hash check request")

	// Find device in cluster
	devicePath, deviceNode, err := h.findDeviceInCluster(deviceID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Device not found: "+err.Error())
		return
	}

	// If device is on a remote peer, forward the request
	localNodeID := h.node.ID()
	if deviceNode != "" && deviceNode != localNodeID {
		err := h.forwardHashCheckRequest(w, r, deviceID, deviceNode, &req)
		if err != nil {
			respondError(w, http.StatusBadGateway, "Failed to forward request: "+err.Error())
		}
		return
	}

	// Device is on local node, execute hash check directly
	result, err := h.executeHashCheck(r.Context(), devicePath, &req, chip)
	if err != nil {
		log.Error().Err(err).Str("device_id", deviceID).Msg("Hash check failed")
		respondJSON(w, HashCheckResponse{
			DeviceID: deviceID,
			Status:   "error",
			Error:    err.Error(),
		})
		return
	}

	respondJSON(w, result)
}

// handleListSnaps lists available snapshots for a device.
func (h *SnapAPI) handleListSnaps(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["deviceId"]

	if deviceID == "" {
		respondError(w, http.StatusBadRequest, "device_id required")
		return
	}

	// TODO: Implement snap storage and retrieval
	// For now, return empty list
	respondJSON(w, map[string]interface{}{
		"device_id": deviceID,
		"snaps":     []interface{}{},
		"count":     0,
	})
}

// handleGetSnap retrieves a specific snapshot by ID.
func (h *SnapAPI) handleGetSnap(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	snapID := vars["snapId"]

	if snapID == "" {
		respondError(w, http.StatusBadRequest, "snap_id required")
		return
	}

	// TODO: Implement snap retrieval from storage
	respondError(w, http.StatusNotFound, "Snapshot not found")
}

// findDeviceInCluster locates a device in the cluster and returns its path and node ID.
func (h *SnapAPI) findDeviceInCluster(deviceID string) (devicePath string, nodeID string, err error) {
	if h.leader == nil {
		// Not a leader, can't search cluster
		return "", "", fmt.Errorf("cluster search not available on this node")
	}

	state := h.leader.State()

	// Try to find device by device_id
	for path, dev := range state.Devices {
		if dev.DeviceID == deviceID {
			return path, dev.NodeID, nil
		}
	}

	// Try by serial number (MAC address)
	for path, dev := range state.Devices {
		if dev.SerialNumber == deviceID {
			return path, dev.NodeID, nil
		}
	}

	// Try to find by alias in store
	devRecord, err := h.store.GetDeviceByAlias(deviceID)
	if err == nil && devRecord != nil {
		// Found in store, now find in cluster state
		for path, dev := range state.Devices {
			if dev.DeviceID == devRecord.DeviceID {
				return path, dev.NodeID, nil
			}
		}
	}

	// Try as a direct path
	for path, dev := range state.Devices {
		if path == deviceID || path == "/dev/"+deviceID {
			return path, dev.NodeID, nil
		}
	}

	return "", "", fmt.Errorf("device not found in cluster: %s", deviceID)
}

// forwardSnapRequest forwards a snap request to the peer node that owns the device.
func (h *SnapAPI) forwardSnapRequest(w http.ResponseWriter, r *http.Request, deviceID, nodeID string, req *SnapRequest) error {
	if h.leader == nil {
		return fmt.Errorf("leader not available")
	}

	// Find the peer node
	state := h.leader.State()
	peerNode, exists := state.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("peer node not found: %s", nodeID)
	}

	// Build peer URL
	peerURL := fmt.Sprintf("http://%s:%d", peerNode.Address, peerNode.Port)

	// Create request body
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Forward request to peer (use query param to avoid URL encoding issues)
	url := fmt.Sprintf("%s/api/v1/devices/snap?device_id=%s", peerURL, url.QueryEscape(deviceID))
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second} // Long timeout for snap operations
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("forward request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("read response body: %w", readErr)
	}

	// Write status and copy headers
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	// Write response body
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write response body: %w", err)
	}

	return nil
}

// forwardHashCheckRequest forwards a hash check request to the peer node that owns the device.
func (h *SnapAPI) forwardHashCheckRequest(w http.ResponseWriter, r *http.Request, deviceID, nodeID string, req *HashCheckRequest) error {
	if h.leader == nil {
		return fmt.Errorf("leader not available")
	}

	// Find the peer node
	state := h.leader.State()
	peerNode, exists := state.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("peer node not found: %s", nodeID)
	}

	// Build peer URL
	peerURL := fmt.Sprintf("http://%s:%d", peerNode.Address, peerNode.Port)

	// Create request body
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Forward request to peer
	url := fmt.Sprintf("%s/api/v1/devices/%s/flash-hash", peerURL, url.PathEscape(deviceID))
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("forward request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("read response body: %w", readErr)
	}

	// Write status and copy headers
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	// Write response body
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write response body: %w", err)
	}

	return nil
}

// executeHashCheck performs the actual hash check operation on the local node.
func (h *SnapAPI) executeHashCheck(ctx context.Context, devicePath string, req *HashCheckRequest, chip string) (*HashCheckResponse, error) {
	firmwarePath := req.Firmware

	// If firmware looks like a file_id (UUID format), construct temp path
	if firmwarePath != "" && len(firmwarePath) == 36 {
		// Check if it matches UUID format (8-4-4-4-12)
		if isUUIDFormat(firmwarePath) {
			firmwarePath = filepath.Join(os.TempDir(), firmwarePath+".bin")
		}
	}

	// Read firmware and compute hash
	firmwareData, err := os.ReadFile(firmwarePath)
	if err != nil {
		return nil, fmt.Errorf("read firmware: %w", err)
	}

	firmwareHash := md5.Sum(firmwareData)
	firmwareHashStr := hex.EncodeToString(firmwareHash[:])

	// Compute device hash (first 64KB of application region)
	deviceHashStr, err := h.computeDeviceApplicationHash(ctx, devicePath)
	if err != nil {
		return nil, fmt.Errorf("compute device hash: %w", err)
	}

	// Compare hashes
	match := deviceHashStr == firmwareHashStr

	return &HashCheckResponse{
		DeviceID:      filepath.Base(devicePath),
		Match:         match,
		DeviceHash:    deviceHashStr,
		FirmwareHash:  firmwareHashStr,
		FlashRequired: !match,
		Status:        "checked",
	}, nil
}

// computeDeviceApplicationHash computes the MD5 hash of the device's application region.
func (h *SnapAPI) computeDeviceApplicationHash(ctx context.Context, devicePath string) (string, error) {
	// For virtual devices, read directly
	if strings.HasPrefix(devicePath, ":virtual:") || devicePath == ":virtual:" {
		return h.computeVirtualApplicationHash(ctx, devicePath)
	}

	// For real devices, use flash library
	flasher := flashlib.NewFlasher(&flashlib.FlasherOptions{
		BaudRate:      115200,
		FlashBaudRate: 460800,
		Compress:      true,
	})

	// Read application region (typically at 0x10000)
	appOffset := uint32(0x10000)
	readSize := uint32(64 * 1024) // Read 64KB for comparison

	readResult := flasher.ReadFlash(ctx, &flashlib.ReadFlashRequest{
		Port:    devicePath,
		Address: appOffset,
		Size:    readSize,
		Chip:    chips.ChipESP32S3, // Assume ESP32-S3
	})

	if readResult.Error != nil {
		return "", fmt.Errorf("read flash: %w", readResult.Error)
	}

	// Compute MD5 of the read data
	hash := md5.Sum(readResult.Data)
	hashStr := hex.EncodeToString(hash[:])

	return hashStr, nil
}

// computeVirtualApplicationHash computes the MD5 hash for virtual devices.
func (h *SnapAPI) computeVirtualApplicationHash(ctx context.Context, devicePath string) (string, error) {
	deviceID := devicePath
	if deviceID == ":virtual:" {
		deviceID = "default"
	}

	device, err := virtual.OpenDevice(deviceID)
	if err != nil {
		return "", fmt.Errorf("open virtual device: %w", err)
	}
	defer device.Close()

	// Read application region
	appOffset := uint32(0x10000)
	appSize := uint32(64 * 1024) // Read 64KB for comparison

	data, err := device.Read(appOffset, appSize)
	if err != nil {
		return "", fmt.Errorf("read virtual flash: %w", err)
	}

	// Compute MD5
	hash := md5.Sum(data)
	hashStr := hex.EncodeToString(hash[:])

	return hashStr, nil
}

// executeSnap performs the actual snapshot operation on the local node.
func (h *SnapAPI) executeSnap(ctx context.Context, devicePath string, req *SnapRequest) (*snap.SnapResult, error) {
	firmwarePath := req.Firmware

	// If firmware looks like a file_id (UUID format), construct temp path
	if firmwarePath != "" && len(firmwarePath) == 36 {
		// Check if it matches UUID format (8-4-4-4-12)
		if isUUIDFormat(firmwarePath) {
			firmwarePath = filepath.Join(os.TempDir(), firmwarePath+".bin")
		}
	}

	// Create snap executor
	duration := time.Duration(req.Duration) * time.Second
	executor := snap.NewExecutor(devicePath, duration)

	// Determine camera ID from device-to-camera mapping if not specified
	cameraID := req.CameraID
	if cameraID == "" {
		// Try to get device_id from cluster state
		var deviceID string
		if h.leader != nil {
			state := h.leader.State()
			for path, dev := range state.Devices {
				if path == devicePath {
					deviceID = dev.DeviceID
					break
				}
			}
		}

		// Look up camera mapping for this device
		if deviceID != "" && h.leader != nil {
			mappings, err := h.store.ListBoundingBoxesForDevice(deviceID)
			if err == nil && len(mappings) > 0 {
				// Get the camera UUID from mapping
				cameraUUID := mappings[0].CameraID

				// Resolve camera UUID to camera path from cluster state
				state := h.leader.State()
				for _, cam := range state.Cameras {
					if cam.ID == cameraUUID {
						cameraID = cam.Path // Use actual device path (e.g., /dev/video0)
						log.Info().
							Str("device_id", deviceID).
							Str("camera_uuid", cameraUUID).
							Str("camera_path", cameraID).
							Msg("Using camera from device mapping")
						break
					}
				}
			}
		}
	}

	// Configure executor
	if cameraID != "" {
		executor.SetCameraID(cameraID)
	}
	if req.SkipCapture {
		executor.SetNoCapture(true)
	}
	if req.SkipMonitor {
		executor.SetNoMonitor(true)
	}

	// Execute snap
	result, err := executor.Run(ctx)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// isUUIDFormat checks if a string matches UUID format (8-4-4-4-12).
func isUUIDFormat(s string) bool {
	if len(s) != 36 {
		return false
	}
	// Check for UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}
