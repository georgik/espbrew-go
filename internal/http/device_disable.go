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

// DisableDeviceRequest represents a request to disable a device
type DisableDeviceRequest struct {
	Reason    string `json:"reason,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
}

// EnableDeviceRequest represents a request to enable a device
type EnableDeviceRequest struct {
	ClientID string `json:"client_id,omitempty"`
}

// DeviceDisableHandler handles device enable/disable operations
type DeviceDisableHandler struct {
	node  cluster.Node
	store *persistence.Store
}

// NewDeviceDisableHandler creates a new device disable handler
func NewDeviceDisableHandler(node cluster.Node, store *persistence.Store) *DeviceDisableHandler {
	return &DeviceDisableHandler{
		node:  node,
		store: store,
	}
}

// RegisterRoutes registers device disable routes
func (h *DeviceDisableHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/devices/{id}/disable", h.handleDisable).Methods("PUT", "POST")
	api.HandleFunc("/devices/{id}/enable", h.handleEnable).Methods("PUT", "POST")
}

// handleDisable disables a device
func (h *DeviceDisableHandler) handleDisable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	var req DisableDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body
		req = DisableDeviceRequest{}
	}

	// Get current device state to check if it's in use
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

	// Check if device is currently in use
	if deviceInfo.Status == "busy" || deviceInfo.Status == "reserved" {
		respondError(w, http.StatusConflict, "cannot disable device that is currently in use")
		return
	}

	// Find device record in persistence to get proper deviceID if using path
	var deviceRecord *persistence.DeviceRecord
	var err error

	// Try by device_id first
	if deviceInfo.DeviceID != "" {
		deviceRecord, err = h.store.GetDevice(deviceInfo.DeviceID)
	}

	// If not found, try by path
	if deviceRecord == nil || err != nil {
		if deviceInfo.Path != "" {
			deviceRecord, err = h.store.GetDeviceByPath(deviceInfo.Path)
		}
	}

	// If still not found, create a record based on current info
	if deviceRecord == nil || err != nil {
		if deviceInfo.DeviceID != "" {
			deviceRecord = &persistence.DeviceRecord{
				DeviceID: deviceInfo.DeviceID,
				LastPath:  deviceInfo.Path,
				ChipType:  deviceInfo.ChipType,
			}
		} else {
			respondError(w, http.StatusNotFound, "device record not found and no device_id available")
			return
		}
	}

	// Set disabled state
	if err := h.store.SetDeviceDisabled(deviceRecord.DeviceID, true, req.Reason, req.ClientID); err != nil {
		log.Error().Err(err).Str("device_id", deviceRecord.DeviceID).Msg("Failed to disable device")
		respondError(w, http.StatusInternalServerError, "failed to disable device")
		return
	}

	log.Info().Str("device_id", deviceRecord.DeviceID).Str("path", deviceInfo.Path).
		Str("reason", req.Reason).Str("client_id", req.ClientID).Msg("Device disabled")

	// Update in-memory state
	if leader, ok := h.node.(*cluster.LeaderNode); ok {
		leader.UpdateDeviceDisabled(deviceRecord.DeviceID, true, req.Reason)
	}

	respondJSON(w, map[string]interface{}{
		"status":    "disabled",
		"device_id": deviceRecord.DeviceID,
		"path":      deviceInfo.Path,
	})
}

// handleEnable enables a disabled device
func (h *DeviceDisableHandler) handleEnable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	var req EnableDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body
		req = EnableDeviceRequest{}
	}

	// Find device record
	var deviceRecord *persistence.DeviceRecord
	var err error

	// Try by device_id
	deviceRecord, err = h.store.GetDevice(deviceID)

	// Try by path if not found
	if deviceRecord == nil || err != nil {
		deviceRecord, err = h.store.GetDeviceByPath(deviceID)
	}

	if deviceRecord == nil || err != nil {
		respondError(w, http.StatusNotFound, "device not found")
		return
	}

	// Check if device is already enabled
	if !deviceRecord.Disabled {
		respondJSON(w, map[string]interface{}{
			"status":    "enabled",
			"device_id": deviceRecord.DeviceID,
		})
		return
	}

	// Enable the device
	if err := h.store.SetDeviceDisabled(deviceRecord.DeviceID, false, "", ""); err != nil {
		log.Error().Err(err).Str("device_id", deviceRecord.DeviceID).Msg("Failed to enable device")
		respondError(w, http.StatusInternalServerError, "failed to enable device")
		return
	}

	log.Info().Str("device_id", deviceRecord.DeviceID).Str("client_id", req.ClientID).Msg("Device enabled")

	// Update in-memory state
	if leader, ok := h.node.(*cluster.LeaderNode); ok {
		status := "available"
		state := h.node.State()
		for _, dev := range state.Devices {
			if dev.DeviceID == deviceRecord.DeviceID || dev.Path == deviceRecord.LastPath {
				status = dev.Status
				if status == "disabled" || status == "" {
					status = "available"
				}
				break
			}
		}
		leader.UpdateDeviceDisabled(deviceRecord.DeviceID, false, "")
	}

	respondJSON(w, map[string]interface{}{
		"status":    "enabled",
		"device_id": deviceRecord.DeviceID,
	})
}

// CheckDeviceDisabled checks if a device is disabled
func CheckDeviceDisabled(state *cluster.ClusterState, devicePath string) (bool, *protocol.DeviceInfo) {
	for _, dev := range state.Devices {
		if dev.Path == devicePath {
			return dev.Disabled, dev
		}
	}
	return false, nil
}

// IsDeviceDisabled checks if a device is disabled by device ID, path, or serial
func IsDeviceDisabled(state *cluster.ClusterState, deviceIdentifier string) bool {
	for _, dev := range state.Devices {
		if dev.DeviceID == deviceIdentifier || dev.Path == deviceIdentifier || dev.SerialNumber == deviceIdentifier {
			return dev.Disabled
		}
	}
	return false
}
