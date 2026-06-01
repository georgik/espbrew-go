package http

import (
	"encoding/json"
	"net/http"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// ProtectDeviceRequest represents a request to protect a device
type ProtectDeviceRequest struct {
	Reason   string `json:"reason,omitempty"`
	ClientID string `json:"client_id,omitempty"`
}

// UnprotectDeviceRequest represents a request to unprotect a device
type UnprotectDeviceRequest struct {
	ClientID string `json:"client_id,omitempty"`
}

// DeviceProtectHandler handles device protect/unprotect operations
type DeviceProtectHandler struct {
	node  cluster.Node
	store *persistence.Store
}

// NewDeviceProtectHandler creates a new device protect handler
func NewDeviceProtectHandler(node cluster.Node, store *persistence.Store) *DeviceProtectHandler {
	return &DeviceProtectHandler{
		node:  node,
		store: store,
	}
}

// RegisterRoutes registers device protect routes
func (h *DeviceProtectHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/devices/{id}/protect", h.handleProtect).Methods("PUT", "POST")
	api.HandleFunc("/devices/{id}/unprotect", h.handleUnprotect).Methods("PUT", "POST")
}

// handleProtect protects a device from flashing (but allows monitoring)
func (h *DeviceProtectHandler) handleProtect(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	var req ProtectDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = ProtectDeviceRequest{}
	}

	// Get current device state
	state := h.node.State()
	var deviceInfo *protocol.DeviceInfo
	for _, dev := range state.Devices {
		if dev.DeviceID == deviceID || dev.Path == deviceID || dev.SerialNumber == deviceID {
			deviceInfo = dev
			break
		}
	}

	if deviceInfo == nil {
		respondError(w, http.StatusNotFound, "device not found")
		return
	}

	// Find device record in persistence
	var deviceRecord *persistence.DeviceRecord
	var err error

	if deviceInfo.DeviceID != "" {
		deviceRecord, err = h.store.GetDevice(deviceInfo.DeviceID)
	}

	if deviceRecord == nil || err != nil {
		if deviceInfo.Path != "" {
			deviceRecord, err = h.store.GetDeviceByPath(deviceInfo.Path)
		}
	}

	if deviceRecord == nil || err != nil {
		if deviceInfo.DeviceID != "" {
			deviceRecord = &persistence.DeviceRecord{
				DeviceID: deviceInfo.DeviceID,
				LastPath: deviceInfo.Path,
				ChipType: deviceInfo.ChipType,
			}
		} else {
			respondError(w, http.StatusNotFound, "device record not found")
			return
		}
	}

	// Set protected state
	if err := h.store.SetDeviceProtected(deviceRecord.DeviceID, true, req.Reason, req.ClientID); err != nil {
		log.Error().Err(err).Str("device_id", deviceRecord.DeviceID).Msg("Failed to protect device")
		respondError(w, http.StatusInternalServerError, "failed to protect device")
		return
	}

	log.Info().Str("device_id", deviceRecord.DeviceID).Str("path", deviceInfo.Path).
		Str("reason", req.Reason).Str("client_id", req.ClientID).Msg("Device protected")

	// Update in-memory state
	if leader, ok := h.node.(*cluster.LeaderNode); ok {
		leader.UpdateDeviceProtected(deviceRecord.DeviceID, true, req.Reason)
	}

	respondJSON(w, map[string]interface{}{
		"status":    "protected",
		"device_id": deviceRecord.DeviceID,
		"path":      deviceInfo.Path,
	})
}

// handleUnprotect unprotects a device (allows flashing again)
func (h *DeviceProtectHandler) handleUnprotect(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	var req UnprotectDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = UnprotectDeviceRequest{}
	}

	// Find device record
	var deviceRecord *persistence.DeviceRecord
	var err error

	deviceRecord, err = h.store.GetDevice(deviceID)

	if deviceRecord == nil || err != nil {
		deviceRecord, err = h.store.GetDeviceByPath(deviceID)
	}

	if deviceRecord == nil || err != nil {
		respondError(w, http.StatusNotFound, "device not found")
		return
	}

	// Check if device is already unprotected
	if !deviceRecord.Protected {
		respondJSON(w, map[string]interface{}{
			"status":    "unprotected",
			"device_id": deviceRecord.DeviceID,
		})
		return
	}

	// Unprotect the device
	if err := h.store.SetDeviceProtected(deviceRecord.DeviceID, false, "", ""); err != nil {
		log.Error().Err(err).Str("device_id", deviceRecord.DeviceID).Msg("Failed to unprotect device")
		respondError(w, http.StatusInternalServerError, "failed to unprotect device")
		return
	}

	log.Info().Str("device_id", deviceRecord.DeviceID).Str("client_id", req.ClientID).Msg("Device unprotected")

	// Update in-memory state
	if leader, ok := h.node.(*cluster.LeaderNode); ok {
		leader.UpdateDeviceProtected(deviceRecord.DeviceID, false, "")
	}

	respondJSON(w, map[string]interface{}{
		"status":    "unprotected",
		"device_id": deviceRecord.DeviceID,
	})
}

// CheckDeviceProtected checks if a device is protected
func CheckDeviceProtected(state *cluster.ClusterState, devicePath string) (bool, *protocol.DeviceInfo) {
	for _, dev := range state.Devices {
		if dev.Path == devicePath {
			return dev.Protected, dev
		}
	}
	return false, nil
}

// IsDeviceProtected checks if a device is protected by device ID, path, or serial
func IsDeviceProtected(state *cluster.ClusterState, deviceIdentifier string) bool {
	for _, dev := range state.Devices {
		if dev.DeviceID == deviceIdentifier || dev.Path == deviceIdentifier || dev.SerialNumber == deviceIdentifier {
			return dev.Protected
		}
	}
	return false
}
