package http

import (
	"encoding/json"
	"net/http"

	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// BackendConfigRequest represents a request to set backend configuration
type BackendConfigRequest struct {
	Backend       string                 `json:"backend"`                  // physical, wokwi, qemu
	BackendConfig map[string]interface{} `json:"backend_config,omitempty"` // Backend-specific config
}

// BackendConfigResponse represents backend configuration response
type BackendConfigResponse struct {
	Backend       string      `json:"backend"`
	BackendConfig interface{} `json:"backend_config,omitempty"`
	DeviceID      string      `json:"device_id"`
}

// handleGetBackendConfig retrieves backend configuration for a device
func (h *APIHandler) handleGetBackendConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	dev, err := h.store.GetDevice(deviceID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Device not found")
		return
	}

	response := BackendConfigResponse{
		Backend:  dev.Backend,
		DeviceID: deviceID,
	}

	// Convert backend config to response format
	if dev.BackendConfig != nil {
		switch dev.Backend {
		case "wokwi":
			response.BackendConfig = dev.BackendConfig.Wokwi
		case "qemu":
			response.BackendConfig = dev.BackendConfig.QEMU
		}
	}

	respondJSON(w, response)
}

// handleSetBackendConfig sets backend configuration for a device
func (h *APIHandler) handleSetBackendConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	var req BackendConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate backend type
	if req.Backend == "" {
		req.Backend = "physical"
	}

	switch req.Backend {
	case "physical", "wokwi", "qemu":
	default:
		respondError(w, http.StatusBadRequest, "Invalid backend type. Must be: physical, wokwi, or qemu")
		return
	}

	// Get existing device
	dev, err := h.store.GetDevice(deviceID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Device not found")
		return
	}

	// Build backend config data
	var backendConfig *persistence.BackendConfigData
	if req.BackendConfig != nil {
		backendConfig = &persistence.BackendConfigData{}

		switch req.Backend {
		case "wokwi":
			wokwiCfg := &persistence.WokwiConfigData{}
			if chipType, ok := req.BackendConfig["chip_type"].(string); ok {
				wokwiCfg.ChipType = chipType
			}
			if diagram, ok := req.BackendConfig["diagram_json"].(string); ok {
				wokwiCfg.DiagramJSON = diagram
			}
			if wokwiCfg.ChipType == "" {
				respondError(w, http.StatusBadRequest, "chip_type required for wokwi backend")
				return
			}
			if wokwiCfg.DiagramJSON == "" {
				respondError(w, http.StatusBadRequest, "diagram_json required for wokwi backend")
				return
			}
			backendConfig.Wokwi = wokwiCfg

		case "qemu":
			qemuCfg := &persistence.QEMUConfigData{}
			if machineType, ok := req.BackendConfig["machine_type"].(string); ok {
				qemuCfg.MachineType = machineType
			}
			if memSize, ok := req.BackendConfig["memory_size"].(float64); ok {
				qemuCfg.MemorySize = int(memSize)
			}
			if qemuCfg.MachineType == "" {
				respondError(w, http.StatusBadRequest, "machine_type required for qemu backend")
				return
			}
			if qemuCfg.MemorySize <= 0 {
				respondError(w, http.StatusBadRequest, "memory_size must be positive for qemu backend")
				return
			}
			backendConfig.QEMU = qemuCfg
		}
	}

	// Update device
	dev.Backend = req.Backend
	dev.BackendConfig = backendConfig

	if err := h.store.SaveDevice(dev); err != nil {
		log.Error().Err(err).Str("device_id", deviceID).Msg("Failed to save device backend config")
		respondError(w, http.StatusInternalServerError, "Failed to save device")
		return
	}

	response := BackendConfigResponse{
		Backend:  dev.Backend,
		DeviceID: deviceID,
	}

	if dev.BackendConfig != nil {
		switch dev.Backend {
		case "wokwi":
			response.BackendConfig = dev.BackendConfig.Wokwi
		case "qemu":
			response.BackendConfig = dev.BackendConfig.QEMU
		}
	}

	respondJSON(w, response)
}

// handleCreateVirtualDevice creates a new virtual device (simulator)
func (h *APIHandler) handleCreateVirtualDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID      string                 `json:"device_id"`
		ChipType      string                 `json:"chip_type"`
		Description   string                 `json:"description,omitempty"`
		Backend       string                 `json:"backend"`
		BackendConfig map[string]interface{} `json:"backend_config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate backend type
	if req.Backend != "wokwi" && req.Backend != "qemu" {
		respondError(w, http.StatusBadRequest, "Virtual devices require wokwi or qemu backend")
		return
	}

	if req.DeviceID == "" {
		respondError(w, http.StatusBadRequest, "device_id is required")
		return
	}

	if req.ChipType == "" {
		respondError(w, http.StatusBadRequest, "chip_type is required")
		return
	}

	// Build backend config data
	backendConfig := &persistence.BackendConfigData{}

	switch req.Backend {
	case "wokwi":
		wokwiCfg := &persistence.WokwiConfigData{}
		wokwiCfg.ChipType = req.ChipType
		if diagram, ok := req.BackendConfig["diagram_json"].(string); ok {
			wokwiCfg.DiagramJSON = diagram
		} else {
			// Generate default diagram
			wokwiCfg.DiagramJSON = generateDefaultDiagram(req.ChipType)
		}
		backendConfig.Wokwi = wokwiCfg

	case "qemu":
		qemuCfg := &persistence.QEMUConfigData{}
		qemuCfg.MachineType = req.ChipType
		if memSize, ok := req.BackendConfig["memory_size"].(float64); ok {
			qemuCfg.MemorySize = int(memSize)
		} else {
			qemuCfg.MemorySize = 4 // Default 4MB
		}
		backendConfig.QEMU = qemuCfg
	}

	// Create device record
	dev := &persistence.DeviceRecord{
		DeviceID:      req.DeviceID,
		ChipType:      req.ChipType,
		Description:   req.Description,
		Backend:       req.Backend,
		BackendConfig: backendConfig,
	}

	if err := h.store.SaveDevice(dev); err != nil {
		log.Error().Err(err).Str("device_id", req.DeviceID).Msg("Failed to create virtual device")
		respondError(w, http.StatusInternalServerError, "Failed to create device")
		return
	}

	respondJSON(w, dev)
}

// handleListVirtualDevices lists all virtual devices (simulators)
func (h *APIHandler) handleListVirtualDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.store.ListDevices()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list devices")
		return
	}

	virtualDevices := []persistence.DeviceRecord{}
	for _, dev := range devices {
		if dev.Backend == "wokwi" || dev.Backend == "qemu" {
			virtualDevices = append(virtualDevices, *dev)
		}
	}

	respondJSON(w, virtualDevices)
}

// handleDeleteVirtualDevice deletes a virtual device
func (h *APIHandler) handleDeleteVirtualDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	dev, err := h.store.GetDevice(deviceID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Device not found")
		return
	}

	// Only allow deletion of virtual devices
	if dev.Backend != "wokwi" && dev.Backend != "qemu" {
		respondError(w, http.StatusBadRequest, "Can only delete virtual devices (wokwi, qemu)")
		return
	}

	if err := h.store.DeleteDevice(deviceID); err != nil {
		log.Error().Err(err).Str("device_id", deviceID).Msg("Failed to delete virtual device")
		respondError(w, http.StatusInternalServerError, "Failed to delete device")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// generateDefaultDiagram generates a default diagram.json for a chip type
func generateDefaultDiagram(chipType string) string {
	diagram := map[string]interface{}{
		"version": 1,
		"parts": []map[string]interface{}{
			{
				"type":     boardTypeForChip(chipType),
				"id":       "chip",
				"position": map[string]float64{"x": 0, "y": 0},
			},
		},
	}

	data, _ := json.Marshal(diagram)
	return string(data)
}

// boardTypeForChip maps chip type to Wokwi board type
func boardTypeForChip(chipType string) string {
	switch chipType {
	case "ESP32":
		return "esp32-devkitC"
	case "ESP32-S2":
		return "esp32-s2-kaluga-1"
	case "ESP32-S3":
		return "esp32-s3-devkitc-1"
	case "ESP32-C3":
		return "esp32-c3-devkitm-1"
	case "ESP32-C6":
		return "esp32-c6-devkitc-1"
	default:
		return "esp32-devkitC"
	}
}

// RegisterBackendRoutes registers backend configuration routes
func (h *APIHandler) RegisterBackendRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	// Backend configuration endpoints
	api.HandleFunc("/devices/{id}/backend", h.handleGetBackendConfig).Methods("GET")
	api.HandleFunc("/devices/{id}/backend", h.handleSetBackendConfig).Methods("PUT", "PATCH")

	// Virtual device management
	api.HandleFunc("/devices/virtual", h.handleCreateVirtualDevice).Methods("POST")
	api.HandleFunc("/devices/virtual", h.handleListVirtualDevices).Methods("GET")
	api.HandleFunc("/devices/virtual/{id}", h.handleDeleteVirtualDevice).Methods("DELETE")
}
