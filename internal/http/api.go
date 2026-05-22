package http

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/georgik/esp-ci-cluster/internal/cluster"
)

type APIHandler struct {
	node   cluster.Node
	master *cluster.MasterNode
	worker *cluster.WorkerNode
}

func NewAPIHandler(node cluster.Node) *APIHandler {
	h := &APIHandler{node: node}

	// Type assertions for master/worker specific APIs
	if m, ok := node.(*cluster.MasterNode); ok {
		h.master = m
	}
	if w, ok := node.(*cluster.WorkerNode); ok {
		h.worker = w
	}

	return h
}

func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	// Cluster status
	api.HandleFunc("/status", h.handleStatus).Methods("GET")
	api.HandleFunc("/nodes", h.handleNodes).Methods("GET")
	api.HandleFunc("/devices", h.handleDevices).Methods("GET")

	// Jobs
	api.HandleFunc("/jobs", h.handleListJobs).Methods("GET")
	api.HandleFunc("/jobs", h.handleCreateJob).Methods("POST")
	api.HandleFunc("/jobs/{id}", h.handleGetJob).Methods("GET")
	api.HandleFunc("/jobs/{id}", h.handleCancelJob).Methods("DELETE")

	// Master-specific
	if h.master != nil {
		api.HandleFunc("/queue", h.handleQueue).Methods("GET")
	}
}

func (h *APIHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	state := h.node.State()

	response := map[string]interface{}{
		"nodes_count":   len(state.Nodes),
		"devices_count": len(state.Devices),
		"jobs_count":    len(state.Jobs),
	}

	if h.master != nil {
		response["role"] = "master"
		response["queue_size"] = h.master.GetJobQueue().PendingCount()
	} else if h.worker != nil {
		response["role"] = "worker"
	}

	respondJSON(w, response)
}

func (h *APIHandler) handleNodes(w http.ResponseWriter, r *http.Request) {
	state := h.node.State()
	nodes := make([]interface{}, 0, len(state.Nodes))
	for _, n := range state.Nodes {
		nodes = append(nodes, n)
	}

	// Add mDNS peers for master
	if h.master != nil {
		peers := h.master.GetPeers()
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
	devices := make([]interface{}, 0, len(state.Devices))
	for _, d := range state.Devices {
		devices = append(devices, d)
	}
	respondJSON(w, devices)
}

func (h *APIHandler) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if h.master == nil {
		respondError(w, http.StatusNotImplemented, "Job list only available on master")
		return
	}

	queue := h.master.GetJobQueue()
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
	if h.master == nil {
		respondError(w, http.StatusNotImplemented, "Job creation only available on master")
		return
	}

	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	job, err := h.master.EnqueueJob(req.Firmware, req.DevicePath)
	if err != nil {
		respondError(w, http.StatusConflict, err.Error())
		return
	}

	respondJSON(w, job.ToMap())
}

func (h *APIHandler) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if h.master == nil {
		respondError(w, http.StatusNotImplemented, "Job queries only available on master")
		return
	}

	vars := mux.Vars(r)
	jobID := vars["id"]

	queue := h.master.GetJobQueue()
	job := queue.Get(jobID)

	if job == nil {
		respondError(w, http.StatusNotFound, "Job not found")
		return
	}

	respondJSON(w, job.ToMap())
}

func (h *APIHandler) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement job cancellation
	respondError(w, http.StatusNotImplemented, "Job cancellation not yet implemented")
}

func (h *APIHandler) handleQueue(w http.ResponseWriter, r *http.Request) {
	if h.master == nil {
		respondError(w, http.StatusNotImplemented, "Queue info only available on master")
		return
	}

	queue := h.master.GetJobQueue()

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
