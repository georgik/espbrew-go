package protocol

import (
	"context"
	"errors"
	"strings"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/flashhash"
)

// BackendType defines the type of backend used for device operations
type BackendType string

const (
	BackendPhysical BackendType = "physical" // Real hardware via serial port
	BackendWokwi    BackendType = "wokwi"    // Wokwi simulator
	BackendQEMU     BackendType = "qemu"     // QEMU emulator (future)
)

// OperationMode defines the operational state of a cluster node
type OperationMode string

const (
	ModeDiscovery   OperationMode = "discovery"   // Discovery enabled, flashing blocked
	ModeOperational OperationMode = "operational" // Discovery disabled, flashing enabled
)

// BackendConfig defines the interface for simulator-specific configuration
type BackendConfig interface {
	GetType() BackendType
	Validate() error
}

// WokwiConfig contains Wokwi simulator configuration
type WokwiConfig struct {
	ChipType    string `json:"chip_type"`           // ESP32, ESP32-S3, ESP32-C3, etc.
	DiagramJSON string `json:"diagram_json"`        // diagram.json content
	APIToken    string `json:"api_token,omitempty"` // Wokwi API token (uses API if set, otherwise CLI)
}

// GetType returns the backend type
func (w *WokwiConfig) GetType() BackendType { return BackendWokwi }

// Validate validates the Wokwi configuration
func (w *WokwiConfig) Validate() error {
	if w.ChipType == "" {
		return errors.New("chip_type is required")
	}
	if w.DiagramJSON == "" {
		return errors.New("diagram_json is required")
	}
	return nil
}

// QEMUConfig contains QEMU emulator configuration (future)
type QEMUConfig struct {
	MachineType string `json:"machine_type"` // esp32, esp32s3, etc.
	MemorySize  int    `json:"memory_size"`  // MB
}

// GetType returns the backend type
func (q *QEMUConfig) GetType() BackendType { return BackendQEMU }

// Validate validates the QEMU configuration
func (q *QEMUConfig) Validate() error {
	if q.MachineType == "" {
		return errors.New("machine_type is required")
	}
	if q.MemorySize <= 0 {
		return errors.New("memory_size must be positive")
	}
	return nil
}

// BackendTypeFromPath determines the backend type from a device path.
// Physical devices use paths like "/dev/ttyUSB0" or "COM5"
// Virtual devices use URI-style paths like "wokwi:esp32s3" or "qemu:esp32"
func BackendTypeFromPath(path string) BackendType {
	if strings.HasPrefix(path, "wokwi:") {
		return BackendWokwi
	}
	if strings.HasPrefix(path, "qemu:") {
		return BackendQEMU
	}
	// Default to physical for /dev/*, COM*, or other paths
	return BackendPhysical
}

// Monitor defines the interface for monitoring device output
type Monitor interface {
	// Start begins monitoring device output
	Start(ctx context.Context) error

	// Stop stops monitoring
	Stop() error

	// Output returns a channel for reading log entries
	Output() <-chan LogEntry

	// Send sends data to the device (for simulators that support input)
	Send(data []byte) error

	// Reset resets the device (if supported)
	Reset() error
}

// Flasher defines the interface for flashing firmware to devices
type Flasher interface {
	// Flash writes firmware to the device
	Flash(ctx context.Context, firmwarePath string, progress chan<- int) error

	// ReadFlash reads flash memory from the device
	ReadFlash(ctx context.Context, address, size uint32) ([]byte, error)
}

// LogEntry represents a single log entry from device output
type LogEntry struct {
	Timestamp int64  `json:"timestamp"`
	Data      string `json:"data"`
	IsError   bool   `json:"is_error,omitempty"`
}

type MessageType string

const (
	NodeJoin             MessageType = "NodeJoin"
	NodeLeave            MessageType = "NodeLeave"
	Heartbeat            MessageType = "Heartbeat"
	DeviceAnnounce       MessageType = "DeviceAnnounce"
	DeviceUpdate         MessageType = "DeviceUpdate"
	CameraAnnounce       MessageType = "CameraAnnounce"
	CameraCapture        MessageType = "CameraCapture"
	JobAssign            MessageType = "JobAssign"
	JobProgress          MessageType = "JobProgress"
	JobComplete          MessageType = "JobComplete"
	JobFailed            MessageType = "JobFailed"
	StateSync            MessageType = "StateSync"
	MsgFlashHashQuery    MessageType = "FlashHashQuery"
	MsgFlashHashResponse MessageType = "FlashHashResponse"
)

type Message struct {
	Type    MessageType `json:"msg_type"`
	Payload interface{} `json:"payload"`
}

type NodeInfo struct {
	ID       string        `json:"id"`
	Address  string        `json:"address"`
	Port     int           `json:"port,omitempty"` // HTTP port for API access
	Role     string        `json:"role"`
	Mode     OperationMode `json:"mode"` // Current operational mode
	LastSeen time.Time     `json:"last_seen"`
}

type DeviceInfo struct {
	Path             string             `json:"path"`
	VID              uint16             `json:"vid"`
	PID              uint16             `json:"pid"`
	SerialNumber     string             `json:"serial"`
	DeviceID         string             `json:"device_id,omitempty"` // Device ID from MAC (esp-xx:xx:xx:xx:xx:xx)
	ChipType         string             `json:"chip_type,omitempty"` // ESP32, ESP32-S3, ESP32-C3, etc.
	NodeID           string             `json:"node_id"`
	Status           string             `json:"status"` // available, busy, offline
	Disabled         bool               `json:"disabled"`
	DisabledReason   string             `json:"disabled_reason,omitempty"`
	DisabledBy       string             `json:"disabled_by,omitempty"`
	DisabledAt       time.Time          `json:"disabled_at,omitempty"`
	Protected        bool               `json:"protected"` // Flash-protected mode - can monitor but not flash
	ProtectedReason  string             `json:"protected_reason,omitempty"`
	ProtectedBy      string             `json:"protected_by,omitempty"`
	ProtectedAt      time.Time          `json:"protected_at,omitempty"`
	AccessError      string             `json:"access_error,omitempty"`   // Error when device cannot be accessed (e.g., permission denied)
	FlashHashes      *DeviceFlashHashes `json:"flash_hashes,omitempty"`   // Latest flash hash data for this device
	Backend          BackendType        `json:"backend"`                  // Backend type: physical, wokwi, qemu
	BackendConfig    BackendConfig      `json:"backend_config,omitempty"` // Backend-specific configuration
	FirstSeen        time.Time          `json:"first_seen"`               // When device was first detected
	LastProbeAttempt time.Time          `json:"last_probe_attempt"`       // When we last tried to probe this device
}

// CameraInfo represents a camera device attached to a node
type CameraInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`    // Platform-specific device path (e.g. /dev/video0)
	Backend string `json:"backend"` // v4l2, avfoundation, directshow
	NodeID  string `json:"node_id"`
	Status  string `json:"status"` // available, busy, offline
}

// DeviceFlashHashes represents flash hash data for a device
type DeviceFlashHashes struct {
	DeviceID  string                      `json:"device_id"`
	Regions   []flashhash.FlashRegionInfo `json:"regions"`
	UpdatedAt string                      `json:"updated_at"` // ISO 8601
}

type JobInfo struct {
	ID         string            `json:"id"`
	Firmware   string            `json:"firmware"`
	DevicePath string            `json:"device_path"`
	Status     string            `json:"status"`
	Progress   int               `json:"progress"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type HeartbeatPayload struct {
	NodeID      string        `json:"node_id"`
	HTTPPort    int           `json:"http_port,omitempty"` // HTTP port for API access
	DeviceCount int           `json:"device_count"`
	CameraCount int           `json:"camera_count"`
	ActiveJobs  int           `json:"active_jobs"`
	Timestamp   int64         `json:"timestamp"`
	Devices     []*DeviceInfo `json:"devices,omitempty"`
	Cameras     []*CameraInfo `json:"cameras,omitempty"`
}

// CameraCaptureRequest is a request to capture an image from a camera
type CameraCaptureRequest struct {
	CameraID string `json:"camera_id"`
	NodeID   string `json:"node_id,omitempty"`
	Width    uint32 `json:"width"`
	Height   uint32 `json:"height"`
	Format   string `json:"format"`  // jpg, png
	Quality  int    `json:"quality"` // 1-100
}

// CameraCaptureResult is the result of a camera capture operation
type CameraCaptureResult struct {
	JobID    string `json:"job_id"`
	Filename string `json:"filename"`
	Format   string `json:"format"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Size     int64  `json:"size"`
	URL      string `json:"url,omitempty"` // Download URL
}

// EraseJobInfo represents a flash erase job
type EraseJobInfo struct {
	ID         string `json:"id"`
	DevicePath string `json:"device_path"`
	Address    uint32 `json:"address"`
	Size       uint32 `json:"size"`
	EraseAll   bool   `json:"erase_all"`
	Status     string `json:"status"`
	Progress   int    `json:"progress"`
}

// FlashHashQuery is sent by client to query flash status
type FlashHashQuery struct {
	DeviceID   string                      `json:"device_id"`
	JobID      string                      `json:"job_id,omitempty"`
	Regions    []flashhash.FlashRegionInfo `json:"regions"`
	ClientMode string                      `json:"client_mode"`
}

// FlashHashResponse is sent by server with optimization recommendations
type FlashHashResponse struct {
	Status            string                       `json:"status"`
	RegionsNeeded     []flashhash.FlashRegionInfo  `json:"regions_needed,omitempty"`
	RegionsCached     []flashhash.CachedRegionInfo `json:"regions_cached,omitempty"`
	JobID             string                       `json:"job_id,omitempty"`
	Message           string                       `json:"message,omitempty"`
	RecommendedAction string                       `json:"recommended_action"`
}
