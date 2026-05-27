package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/device"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

type APIHandler struct {
	node   cluster.Node
	leader *cluster.LeaderNode
	peer   *cluster.PeerNode
}

func NewAPIHandler(node cluster.Node) *APIHandler {
	h := &APIHandler{node: node}

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

	// Collect device paths and sort them
	paths := make([]string, 0, len(state.Devices))
	for path := range state.Devices {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	devices := make([]map[string]interface{}, 0, len(state.Devices))
	for _, path := range paths {
		d := state.Devices[path]

		// Filter to only show ESP devices or virtual devices
		isVirtual := len(path) > 6 && path[:6] == "wokwi-"
		if !isVirtual && !device.IsESPDevice(d.VID, d.PID) {
			continue
		}

		dev := map[string]interface{}{
			"path":    d.Path,
			"vid":     fmt.Sprintf("0x%04x", d.VID),
			"pid":     fmt.Sprintf("0x%04x", d.PID),
			"state":   d.Status,
			"node_id": d.NodeID,
		}
		if isVirtual {
			dev["virtual"] = true
			dev["label"] = "Wokwi " + path[6:]
		}
		if d.SerialNumber != "" {
			dev["serial"] = d.SerialNumber
		}
		devices = append(devices, dev)
	}
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
