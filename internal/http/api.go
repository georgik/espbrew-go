package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

type APIHandler struct {
	node   cluster.Node
	leader *cluster.LeaderNode
	peer   *cluster.PeerNode
	store  *persistence.Store
}

func NewAPIHandler(node cluster.Node, store *persistence.Store) *APIHandler {
	h := &APIHandler{node: node, store: store}

	// Type assertions for leader/peer specific APIs
	if l, ok := node.(*cluster.LeaderNode); ok {
		h.leader = l
	}
	if p, ok := node.(*cluster.PeerNode); ok {
		h.peer = p
	}

	return h
}

func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	// Cluster status
	api.HandleFunc("/status", h.handleStatus).Methods("GET")
	api.HandleFunc("/nodes", h.handleNodes).Methods("GET")
	api.HandleFunc("/devices", h.handleDevices).Methods("GET")
	api.HandleFunc("/devices", h.handleAddDevice).Methods("POST")
	api.HandleFunc("/devices/probe", h.handleProbeDevice).Methods("POST")
	api.HandleFunc("/devices/{id}", h.handleDeviceDetail).Methods("GET")
	api.HandleFunc("/devices/{id}", h.handleUpdateDevice).Methods("PUT", "PATCH")
	api.HandleFunc("/devices/{id}", h.handleDeleteDevice).Methods("DELETE")
	api.HandleFunc("/cameras", h.handleCameras).Methods("GET")

	// Device reservation (use device name without /dev/ prefix)
	api.HandleFunc("/devices/{name}/reserve", h.handleReserveDevice).Methods("POST", "DELETE")

	// Jobs
	api.HandleFunc("/jobs", h.handleListJobs).Methods("GET")
	api.HandleFunc("/jobs", h.handleCreateJob).Methods("POST")
	api.HandleFunc("/jobs/{id}", h.handleGetJob).Methods("GET")
	api.HandleFunc("/jobs/{id}", h.handleCancelJob).Methods("DELETE")

	// Node registration (for peer nodes)
	api.HandleFunc("/nodes/register", h.handleRegisterNode).Methods("POST")
	api.HandleFunc("/nodes/{id}/heartbeat", h.handleNodeHeartbeat).Methods("POST")

	// Leader-specific
	if h.leader != nil {
		api.HandleFunc("/queue", h.handleQueue).Methods("GET")
		api.HandleFunc("/cameras/capture", h.handleCameraCapture).Methods("POST")
	}
}

func (h *APIHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	state := h.node.State()

	response := map[string]interface{}{
		"nodes_count":   len(state.Nodes),
		"devices_count": len(state.Devices),
		"cameras_count": len(state.Cameras),
		"jobs_count":    len(state.Jobs),
	}

	if h.leader != nil {
		response["role"] = "leader"
		response["queue_size"] = h.leader.GetJobQueue().PendingCount()
	} else if h.peer != nil {
		response["role"] = "peer"
	}

	respondJSON(w, response)
}

func (h *APIHandler) handleNodes(w http.ResponseWriter, r *http.Request) {
	state := h.node.State()
	nodes := make([]interface{}, 0, len(state.Nodes))
	for _, n := range state.Nodes {
		nodes = append(nodes, n)
	}

	// Add mDNS peers for leader
	if h.leader != nil {
		peers := h.leader.GetPeers()
		for _, p := range peers {
			nodes = append(nodes, map[string]interface{}{
				"id":         p.NodeID,
				"role":       p.Role,
				"address":    p.Address,
				"port":       p.Port,
				"discovered": true,
				"last_seen":  p.LastSeen,
			})
		}
	}

	respondJSON(w, nodes)
}

func (h *APIHandler) handleDevices(w http.ResponseWriter, r *http.Request) {
	state := h.node.State()

	// Get all devices from store (authoritative source for known devices)
	storeDevices, err := h.store.ListDevices()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list devices")
		return
	}

	// Build map of store devices by device_id and MAC for quick lookup
	knownByDeviceID := make(map[string]*persistence.DeviceRecord)
	knownByMAC := make(map[string]*persistence.DeviceRecord)
	for _, dev := range storeDevices {
		if dev.DeviceID != "" {
			knownByDeviceID[dev.DeviceID] = dev
		}
		if dev.MACAddress != "" {
			knownByMAC[dev.MACAddress] = dev
		}
	}

	// Build response: combine store devices with in-memory connection status
	devices := make([]map[string]interface{}, 0)

	// First, add all known devices from store with their current connection status
	for _, dev := range storeDevices {
		status := "offline"
		currentPath := dev.LastPath
		nodeID := dev.NodeID
		vid := ""
		pid := ""
		serial := dev.MACAddress

		// Check if currently connected
		if dev.MACAddress != "" {
			if conn, ok := state.Devices[dev.LastPath]; ok && (conn.DeviceID == dev.DeviceID || conn.SerialNumber == dev.MACAddress) {
				status = conn.Status
				vid = fmt.Sprintf("0x%04x", conn.VID)
				pid = fmt.Sprintf("0x%04x", conn.PID)
				nodeID = conn.NodeID
				if conn.SerialNumber != "" {
					serial = conn.SerialNumber
				}
			}
		}
		if dev.DeviceID != "" {
			for _, conn := range state.Devices {
				if conn.DeviceID == dev.DeviceID {
					status = conn.Status
					currentPath = conn.Path
					vid = fmt.Sprintf("0x%04x", conn.VID)
					pid = fmt.Sprintf("0x%04x", conn.PID)
					nodeID = conn.NodeID
					if conn.SerialNumber != "" {
						serial = conn.SerialNumber
					}
					break
				}
			}
		}

		// Skip garbage records (offline with no chip_type)
		if status == "offline" && dev.ChipType == "" {
			continue
		}

		devMap := map[string]interface{}{
			"path":      currentPath,
			"device_id": dev.DeviceID,
			"chip_type": dev.ChipType,
			"status":    status,
			"connected": status != "offline",
		}
		if vid != "" {
			devMap["vid"] = vid
		}
		if pid != "" {
			devMap["pid"] = pid
		}
		if nodeID != "" {
			devMap["node_id"] = nodeID
		}
		if serial != "" {
			devMap["serial"] = serial
		}
		if len(dev.Aliases) > 0 {
			devMap["aliases"] = dev.Aliases
		}
		if len(dev.Tags) > 0 {
			devMap["tags"] = dev.Tags
		}
		if dev.Disabled {
			devMap["disabled"] = true
			devMap["disabled_reason"] = dev.DisabledReason
			devMap["disabled_by"] = dev.DisabledBy
			if !dev.DisabledAt.IsZero() {
				devMap["disabled_at"] = dev.DisabledAt.Format(time.RFC3339)
			}
		}

		devices = append(devices, devMap)
	}

	// Add in-memory devices that aren't in store (virtual devices, newly connected)
	for _, conn := range state.Devices {
		alreadyListed := false
		if conn.DeviceID != "" {
			if _, ok := knownByDeviceID[conn.DeviceID]; ok {
				alreadyListed = true
			}
		}
		if conn.SerialNumber != "" {
			if _, ok := knownByMAC[conn.SerialNumber]; ok {
				alreadyListed = true
			}
		}

		if !alreadyListed {
			// Virtual device or unprobed device - add to list
			devMap := map[string]interface{}{
				"path":      conn.Path,
				"vid":       fmt.Sprintf("0x%04x", conn.VID),
				"pid":       fmt.Sprintf("0x%04x", conn.PID),
				"status":    conn.Status,
				"connected": true,
			}
			if conn.DeviceID != "" {
				devMap["device_id"] = conn.DeviceID
			}
			if conn.ChipType != "" {
				devMap["chip_type"] = conn.ChipType
			}
			if conn.NodeID != "" {
				devMap["node_id"] = conn.NodeID
			}
			if conn.SerialNumber != "" {
				devMap["serial"] = conn.SerialNumber
			}
			if conn.Disabled {
				devMap["disabled"] = true
				devMap["disabled_reason"] = conn.DisabledReason
				devMap["disabled_by"] = conn.DisabledBy
				if !conn.DisabledAt.IsZero() {
					devMap["disabled_at"] = conn.DisabledAt.Format(time.RFC3339)
				}
			}
			// Check for virtual device
			if len(conn.Path) > 6 && conn.Path[:6] == "wokwi-" {
				devMap["virtual"] = true
				devMap["label"] = "Wokwi " + conn.Path[6:]
			}
			devices = append(devices, devMap)
		}
	}

	// Sort devices by path (use empty string for devices without path)
	sort.Slice(devices, func(i, j int) bool {
		pathI, _ := devices[i]["path"].(string)
		pathJ, _ := devices[j]["path"].(string)
		return pathI < pathJ
	})

	respondJSON(w, devices)
}

func (h *APIHandler) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Job list only available on leader")
		return
	}

	queue := h.leader.GetJobQueue()
	jobs := queue.List()

	result := make([]interface{}, 0, len(jobs))
	for _, j := range jobs {
		result = append(result, j.ToMap())
	}

	respondJSON(w, result)
}

type CreateJobRequest struct {
	Firmware   string `json:"firmware"`
	DevicePath string `json:"device_path"`
}

func (h *APIHandler) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Job creation only available on leader")
		return
	}

	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	job, err := h.leader.EnqueueJob(req.Firmware, req.DevicePath)
	if err != nil {
		respondError(w, http.StatusConflict, err.Error())
		return
	}

	respondJSON(w, job.ToMap())
}

func (h *APIHandler) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Job queries only available on leader")
		return
	}

	vars := mux.Vars(r)
	jobID := vars["id"]

	queue := h.leader.GetJobQueue()
	job := queue.Get(jobID)

	if job == nil {
		respondError(w, http.StatusNotFound, "Job not found")
		return
	}

	respondJSON(w, job.ToMap())
}

func (h *APIHandler) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Job cancellation only available on leader")
		return
	}

	vars := mux.Vars(r)
	jobID := vars["id"]

	if err := h.leader.CancelJob(jobID); err != nil {
		if err.Error() == "job not found: "+jobID || err.Error()[:12] == "job not found" {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondError(w, http.StatusConflict, err.Error())
		return
	}

	respondJSON(w, map[string]interface{}{
		"status": "cancelled",
		"job_id": jobID,
	})
}

func (h *APIHandler) handleQueue(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Queue info only available on leader")
		return
	}

	queue := h.leader.GetJobQueue()

	respondJSON(w, map[string]interface{}{
		"pending": queue.PendingCount(),
		"jobs":    queue.List(),
	})
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

type ReserveDeviceRequest struct {
	ClientID string `json:"client_id"`
	TTL      int    `json:"ttl"`
}

// findDeviceByName looks up a device by its base name (without /dev/ prefix)
func (h *APIHandler) findDeviceByName(deviceName string) (string, *protocol.DeviceInfo, bool) {
	if h.leader == nil {
		return "", nil, false
	}

	state := h.leader.State()
	for path, dev := range state.Devices {
		// Match by base name
		if path == "/dev/"+deviceName || path == deviceName {
			return path, dev, true
		}
	}
	return "", nil, false
}

func (h *APIHandler) handleReserveDevice(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Device reservation only available on leader")
		return
	}

	vars := mux.Vars(r)
	deviceName := vars["name"]

	// Look up device by name
	devicePath, device, exists := h.findDeviceByName(deviceName)
	if !exists {
		respondError(w, http.StatusNotFound, "Device not found")
		return
	}

	// Check if device is disabled
	if device.Disabled {
		respondError(w, http.StatusForbidden, "Device is disabled and cannot be reserved")
		return
	}

	if r.Method == "POST" {
		var req ReserveDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if req.ClientID == "" {
			respondError(w, http.StatusBadRequest, "client_id required")
			return
		}

		// Try to reserve
		if !h.leader.GetDevices().Reserve(devicePath, req.ClientID) {
			owner := h.leader.GetDevices().GetOwner(devicePath)
			respondError(w, http.StatusConflict, fmt.Sprintf("Device already reserved by: %s", owner))
			return
		}

		// Update device state
		device.Status = "busy"
		h.leader.State().Devices[devicePath] = device

		respondJSON(w, map[string]interface{}{
			"status":    "reserved",
			"device":    deviceName,
			"client_id": req.ClientID,
		})
		return
	}

	if r.Method == "DELETE" {
		var req ReserveDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Empty body is OK for release
			req.ClientID = ""
		}

		// Release if we own it or client_id matches
		owner := h.leader.GetDevices().GetOwner(devicePath)
		if owner != "" && req.ClientID != "" && owner != req.ClientID {
			respondError(w, http.StatusForbidden, fmt.Sprintf("Device reserved by: %s", owner))
			return
		}

		h.leader.GetDevices().Release(devicePath, owner)

		// Update device state
		device.Status = "available"
		h.leader.State().Devices[devicePath] = device

		respondJSON(w, map[string]interface{}{
			"status": "released",
			"device": deviceName,
		})
		return
	}

	respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (h *APIHandler) handleCameras(w http.ResponseWriter, r *http.Request) {
	state := h.node.State()

	cameras := make([]interface{}, 0, len(state.Cameras))
	for _, c := range state.Cameras {
		cameras = append(cameras, c)
	}

	respondJSON(w, map[string]interface{}{
		"cameras": cameras,
		"count":   len(cameras),
	})
}

type CameraCaptureRequest struct {
	CameraID string `json:"camera_id"`
	Width    uint32 `json:"width"`
	Height   uint32 `json:"height"`
	Format   string `json:"format"`
	Quality  int    `json:"quality"`
}

func (h *APIHandler) handleCameraCapture(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Camera capture only available on leader")
		return
	}

	var req CameraCaptureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Debug().Err(err).Msg("Failed to decode capture request")
		respondError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Set defaults
	if req.Width == 0 {
		req.Width = 1280
	}
	if req.Height == 0 {
		req.Height = 720
	}
	if req.Format == "" {
		req.Format = "jpg"
	}
	if req.Quality == 0 {
		req.Quality = 85
	}

	log.Info().
		Str("camera_id", req.CameraID).
		Uint32("width", req.Width).
		Uint32("height", req.Height).
		Msg("Camera capture request")

	// Find camera - if no ID specified, use first available
	state := h.leader.State()
	var targetCam *protocol.CameraInfo

	if req.CameraID != "" {
		if cam, exists := state.Cameras[req.CameraID]; exists {
			targetCam = cam
		} else {
			respondError(w, http.StatusNotFound, "Camera not found")
			return
		}
	} else {
		// Use first available camera
		for _, cam := range state.Cameras {
			if cam.Status == "available" {
				targetCam = cam
				break
			}
		}
		if targetCam == nil {
			respondError(w, http.StatusServiceUnavailable, "No cameras available")
			return
		}
	}

	// Perform actual capture
	store, err := camera.DefaultStore()
	if err != nil {
		log.Error().Err(err).Msg("Failed to create store")
		respondError(w, http.StatusInternalServerError, "Failed to initialize storage: "+err.Error())
		return
	}
	capturer := camera.NewCapturer(store)

	captureReq := &camera.CaptureRequest{
		CameraID: targetCam.ID,
		Width:    req.Width,
		Height:   req.Height,
		Format:   "jpg",
		Quality:  req.Quality,
	}

	result, err := capturer.Capture(r.Context(), captureReq)
	if err != nil {
		log.Error().Err(err).Str("camera_id", targetCam.ID).Msg("Capture failed")
		respondError(w, http.StatusInternalServerError, "Capture failed: "+err.Error())
		return
	}

	savedPath, err := store.Save(targetCam.ID, req.Format, result.Data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save capture")
		respondError(w, http.StatusInternalServerError, "Failed to save capture")
		return
	}

	relPath, _ := store.GetRelativePath(savedPath)

	respondJSON(w, map[string]interface{}{
		"status":    "success",
		"camera_id": targetCam.ID,
		"path":      "/captures/" + relPath,
		"timestamp": time.Now().Unix(),
	})
}

func (h *APIHandler) handleRegisterNode(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Node registration only available on leader")
		return
	}

	var payload protocol.HeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Debug().Err(err).Msg("Failed to decode register payload")
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Info().Str("node_id", payload.NodeID).Str("remote_addr", r.RemoteAddr).
		Int("devices", len(payload.Devices)).Msg("Node registration request")

	// Create node info from heartbeat payload
	node := &protocol.NodeInfo{
		ID:       payload.NodeID,
		Address:  r.RemoteAddr,
		Role:     "peer",
		LastSeen: time.Now(),
	}

	h.leader.RegisterNode(node)

	// Process devices from heartbeat
	h.leader.UpdateHeartbeat(payload.NodeID, &payload)

	respondJSON(w, map[string]interface{}{
		"status":  "registered",
		"node_id": payload.NodeID,
	})
}

func (h *APIHandler) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Heartbeat only available on leader")
		return
	}

	vars := mux.Vars(r)
	nodeID := vars["id"]

	var payload protocol.HeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Debug().Err(err).Str("node_id", nodeID).Msg("Failed to decode heartbeat payload")
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Debug().Str("node_id", nodeID).Str("remote_addr", r.RemoteAddr).
		Int("devices", len(payload.Devices)).Msg("Heartbeat received")

	// Update heartbeat and devices
	h.leader.UpdateHeartbeat(nodeID, &payload)

	respondJSON(w, map[string]interface{}{
		"status": "ok",
	})
}

// handleDeviceDetail returns detailed device information from persistence
func (h *APIHandler) handleDeviceDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	// Try to find by device ID
	dev, err := h.store.GetDevice(deviceID)
	if err != nil {
		// Try by alias
		dev, err = h.store.GetDeviceByAlias(deviceID)
		if err != nil {
			// Try by MAC (with esp- prefix)
			if len(deviceID) == 17 && deviceID[2] == ':' {
				dev, err = h.store.GetDeviceByMAC(deviceID)
				if err == nil {
					// Found by MAC, but need full device record
					dev, err = h.store.GetDevice(dev.DeviceID)
				}
			}
			if err != nil {
				respondError(w, http.StatusNotFound, "Device not found in inventory")
				return
			}
		}
	}

	// Build response with full device details
	response := map[string]interface{}{
		"device_id":   dev.DeviceID,
		"mac_address": dev.MACAddress,
		"chip_type":   dev.ChipType,
		"chip_rev":    dev.ChipRev,
		"flash_size":  dev.FlashSize,
		"psram_size":  dev.PSRAMSize,
		"psram_type":  dev.PSRAMType,
		"board_model": dev.BoardModel,
		"description": dev.Description,
		"first_seen":  dev.FirstSeen.Format(time.RFC3339),
		"last_seen":   dev.LastSeen.Format(time.RFC3339),
		"last_path":   dev.LastPath,
		"node_id":     dev.NodeID,
		"aliases":     dev.Aliases,
		"tags":        dev.Tags,
	}

	// Add disabled fields if disabled
	if dev.Disabled {
		response["disabled"] = true
		response["disabled_reason"] = dev.DisabledReason
		response["disabled_by"] = dev.DisabledBy
		if !dev.DisabledAt.IsZero() {
			response["disabled_at"] = dev.DisabledAt.Format(time.RFC3339)
		}
	}

	// Check if device is currently connected
	state := h.node.State()
	connectedPath := ""
	for path, d := range state.Devices {
		if d.DeviceID == dev.DeviceID || d.SerialNumber == dev.MACAddress {
			connectedPath = path
			response["connected"] = true
			response["current_path"] = path
			response["status"] = d.Status
			break
		}
	}
	if connectedPath == "" {
		response["connected"] = false
		response["status"] = "offline"
	}

	respondJSON(w, response)
}

// handleUpdateDevice updates device tags and aliases
type UpdateDeviceRequest struct {
	MACAddress  string   `json:"mac_address"`
	ChipType    string   `json:"chip_type"`
	ChipRev     string   `json:"chip_rev"`
	FlashSize   uint32   `json:"flash_size"`
	PSRAMSize   uint32   `json:"psram_size"`
	PSRAMType   string   `json:"psram_type"`
	BoardModel  string   `json:"board_model"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
	Tags        []string `json:"tags"`
}

func (h *APIHandler) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	// Find device (reuse logic from handleDeviceDetail)
	dev, err := h.store.GetDevice(deviceID)
	if err != nil {
		dev, err = h.store.GetDeviceByAlias(deviceID)
		if err != nil {
			if len(deviceID) == 17 && deviceID[2] == ':' {
				macDev, err := h.store.GetDeviceByMAC(deviceID)
				if err == nil {
					dev = macDev
				}
			}
			if err != nil {
				respondError(w, http.StatusNotFound, "Device not found in inventory")
				return
			}
		}
	}

	var req UpdateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Update device fields (only update non-empty values)
	updated := &persistence.DeviceRecord{
		DeviceID:    dev.DeviceID,
		MACAddress:  dev.MACAddress,
		ChipType:    dev.ChipType,
		ChipRev:     dev.ChipRev,
		FlashSize:   dev.FlashSize,
		PSRAMSize:   dev.PSRAMSize,
		PSRAMType:   dev.PSRAMType,
		BoardModel:  dev.BoardModel,
		Description: dev.Description,
		Aliases:     dev.Aliases,
		Tags:        dev.Tags,
		FirstSeen:   dev.FirstSeen,
		LastSeen:    dev.LastSeen,
		LastPath:    dev.LastPath,
		NodeID:      dev.NodeID,
	}

	// Update fields from request if provided
	if req.MACAddress != "" {
		updated.MACAddress = req.MACAddress
	}
	if req.ChipType != "" {
		updated.ChipType = req.ChipType
	}
	if req.ChipRev != "" {
		updated.ChipRev = req.ChipRev
	}
	if req.FlashSize > 0 {
		updated.FlashSize = req.FlashSize
	}
	if req.PSRAMSize > 0 {
		updated.PSRAMSize = req.PSRAMSize
	}
	if req.PSRAMType != "" {
		updated.PSRAMType = req.PSRAMType
	}
	if req.BoardModel != "" {
		updated.BoardModel = req.BoardModel
	}
	if req.Description != "" {
		updated.Description = req.Description
	}
	if req.Aliases != nil {
		updated.Aliases = req.Aliases
	}
	if req.Tags != nil {
		updated.Tags = req.Tags
	}

	if err := h.store.SaveDevice(updated); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update device")
		return
	}

	// Update in-memory state if device is currently connected
	// Use the device's LastPath from store to find the correct in-memory entry
	if h.leader != nil && updated.LastPath != "" {
		state := h.leader.State()
		if dev, exists := state.Devices[updated.LastPath]; exists {
			// Verify this is the correct device before updating
			if dev.DeviceID == updated.DeviceID || dev.SerialNumber == updated.MACAddress {
				h.leader.UpdateDeviceInfo(updated.LastPath, updated.DeviceID, updated.ChipType, updated.MACAddress)
				log.Debug().Str("path", updated.LastPath).Str("device_id", updated.DeviceID).
					Msg("In-memory state updated via API")
			}
		}
	}

	respondJSON(w, map[string]interface{}{
		"status":    "updated",
		"device_id": dev.DeviceID,
	})
}

// handleDeleteDevice removes a device from inventory
func (h *APIHandler) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	// Find device
	dev, err := h.store.GetDevice(deviceID)
	if err != nil {
		dev, err = h.store.GetDeviceByAlias(deviceID)
		if err != nil {
			respondError(w, http.StatusNotFound, "Device not found")
			return
		}
	}

	// Delete from persistence
	if err := h.store.DeleteDevice(dev.DeviceID); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete device")
		return
	}

	// Remove from in-memory state if present
	if h.leader != nil {
		state := h.leader.State()
		for path, stateDev := range state.Devices {
			if stateDev.DeviceID == dev.DeviceID || stateDev.SerialNumber == dev.MACAddress {
				// Remove from state (need to access internal state through leader)
				// This is a bit of a hack - we're directly manipulating the map
				// In production, add a proper method to LeaderNode
				delete(state.Devices, path)
				log.Info().Str("path", path).Str("device_id", dev.DeviceID).
					Msg("Device removed from cluster state via API")
				break
			}
		}
	}

	respondJSON(w, map[string]interface{}{
		"status":    "deleted",
		"device_id": dev.DeviceID,
	})
}

// AddDeviceRequest is the request body for manually adding a device
type AddDeviceRequest struct {
	Path        string   `json:"path"` // Device path (e.g., /dev/cu.usbserial-xxx)
	MACAddress  string   `json:"mac_address"`
	ChipType    string   `json:"chip_type"`
	ChipRev     string   `json:"chip_rev"`
	FlashSize   uint32   `json:"flash_size"`
	PSRAMSize   uint32   `json:"psram_size"`
	PSRAMType   string   `json:"psram_type"`
	BoardModel  string   `json:"board_model"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
	Tags        []string `json:"tags"`
}

// handleAddDevice manually adds a device to inventory (for devices that can't be probed)
func (h *APIHandler) handleAddDevice(w http.ResponseWriter, r *http.Request) {
	var req AddDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields (only chip_type is required, MAC is optional)
	if req.ChipType == "" {
		respondError(w, http.StatusBadRequest, "chip_type is required")
		return
	}

	// Generate device ID from MAC if provided, otherwise generate one
	var deviceID string
	if req.MACAddress != "" {
		deviceID = "esp-" + req.MACAddress
		// Check if device already exists
		if _, err := h.store.GetDevice(deviceID); err == nil {
			respondError(w, http.StatusConflict, "Device with this MAC already exists")
			return
		}
	} else {
		// Generate ID from store
		var err error
		deviceID, err = h.store.GenerateManualID(req.ChipType)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to generate device ID")
			return
		}
	}

	// Create new device entry
	now := time.Now()
	dev := &persistence.DeviceRecord{
		DeviceID:    deviceID,
		MACAddress:  req.MACAddress,
		ChipType:    req.ChipType,
		ChipRev:     req.ChipRev,
		FlashSize:   req.FlashSize,
		PSRAMSize:   req.PSRAMSize,
		PSRAMType:   req.PSRAMType,
		BoardModel:  req.BoardModel,
		Description: req.Description,
		Aliases:     req.Aliases,
		Tags:        req.Tags,
		FirstSeen:   now,
		LastSeen:    now,
		LastPath:    req.Path,
	}

	if req.Aliases == nil {
		dev.Aliases = []string{}
	}
	if req.Tags == nil {
		dev.Tags = []string{}
	}

	if err := h.store.SaveDevice(dev); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save device")
		return
	}

	// If path provided and we're leader, link to cluster state
	if req.Path != "" && h.leader != nil {
		h.leader.UpdateDeviceInfo(req.Path, deviceID, req.ChipType, req.MACAddress)
	}

	respondJSON(w, map[string]interface{}{
		"status":    "created",
		"device_id": deviceID,
	})
}

// handleProbeDevice manually probes a device (user must put device in bootloader first)
type ProbeDeviceRequest struct {
	Path string `json:"path"`
}

func (h *APIHandler) handleProbeDevice(w http.ResponseWriter, r *http.Request) {
	if h.leader == nil {
		respondError(w, http.StatusNotImplemented, "Device probe only available on leader")
		return
	}

	var req ProbeDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Path == "" {
		respondError(w, http.StatusBadRequest, "path is required")
		return
	}

	devInfo, err := h.leader.ProbeDevice(req.Path)
	if err != nil {
		respondError(w, http.StatusServiceUnavailable, "Probe failed: "+err.Error())
		return
	}

	respondJSON(w, map[string]interface{}{
		"status":    "probed",
		"device_id": devInfo.DeviceID,
		"chip_type": devInfo.ChipType,
		"path":      devInfo.Path,
	})
}
