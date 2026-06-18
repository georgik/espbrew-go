// Package snap provides functionality for creating device snapshots.
// A snapshot captures the complete state of a device including:
// - Serial monitor logs
// - Camera capture (if available)
// - Flash state information
// - Device metadata
//
// The package supports both manual and automated snapshot creation,
// with configurable timeouts and partial capture support.
package snap

import "time"

// SnapStatus represents the completion status of a snapshot operation.
type SnapStatus string

const (
	// SnapStatusSuccess indicates all requested operations completed successfully
	SnapStatusSuccess SnapStatus = "success"
	// SnapStatusPartial indicates some operations completed but others failed
	SnapStatusPartial SnapStatus = "partial"
	// SnapStatusFailed indicates the snapshot operation failed completely
	SnapStatusFailed SnapStatus = "failed"
)

// SnapMetadata contains descriptive information about a snapshot.
type SnapMetadata struct {
	SnapID    string    `json:"snap_id"`     // Unique identifier for this snapshot
	Timestamp time.Time `json:"timestamp"`   // When the snapshot was created
	Duration  int64     `json:"duration_ms"` // How long the snapshot took to complete (milliseconds)

	// Device Information
	DevicePath string `json:"device_path,omitempty"` // Serial port path
	DeviceNode string `json:"device_node,omitempty"` // Cluster node that owns the device
	ChipID     string `json:"chip_id,omitempty"`     // Detected chip identifier
	ChipName   string `json:"chip_name,omitempty"`   // Human-readable chip name

	// Flash Information
	FlashEnabled    bool   `json:"flash_enabled"`               // Whether flashing was performed
	FlashFirmware   string `json:"firmware,omitempty"`          // Firmware path that was flashed
	FlashOffset     int    `json:"flash_offset"`                // Flash offset used
	FlashSkipped    bool   `json:"flash_skipped"`               // True if flash was skipped (force_skip)
	FlashHashBefore string `json:"flash_hash_before,omitempty"` // MD5 hash before flash
	FlashHashAfter  string `json:"flash_hash_after,omitempty"`  // MD5 hash after flash

	// Monitor Information
	MonitorEnabled  bool          `json:"monitor_enabled"`  // Whether serial monitoring was enabled
	MonitorDuration time.Duration `json:"monitor_duration"` // How long serial was monitored
	MonitorBaudRate int           `json:"monitor_baud"`     // Baud rate used for monitoring
	LogEntryCount   int           `json:"log_entry_count"`  // Number of log lines captured

	// Capture Information
	CaptureEnabled bool   `json:"capture_enabled"`        // Whether camera capture was enabled
	CameraID       string `json:"camera_id,omitempty"`    // Camera device identifier
	ImageFormat    string `json:"image_format,omitempty"` // Format of captured image (e.g., "png", "jpeg")
	ImageSize      int    `json:"image_size"`             // Size of captured image in bytes

	// Status Information
	Status SnapStatus `json:"status"`          // Overall snapshot status
	Error  string     `json:"error,omitempty"` // Error message if failed
}

// SerialLogEntry represents a single line from the serial monitor.
type SerialLogEntry struct {
	Timestamp time.Time `json:"timestamp"`        // When the log entry was received
	Message   string    `json:"message"`          // The log message content
	Level     string    `json:"level,omitempty"`  // Log level (info, warn, error, debug)
	Source    string    `json:"source,omitempty"` // Source of the log entry
}

// SnapResult contains the complete snapshot data.
type SnapResult struct {
	Metadata    SnapMetadata     `json:"metadata"`               // Snapshot metadata
	Logs        []SerialLogEntry `json:"logs"`                   // Captured serial log entries
	ImageData   []byte           `json:"-"`                      // Raw image data (not JSON serialized)
	ImageBase64 string           `json:"image_base64,omitempty"` // Base64-encoded image data
}

// ToMap converts SnapResult to a map for JSON serialization.
// For cluster operations, set includeLogs=false to reduce response size.
func (r *SnapResult) ToMap(includeLogs bool) map[string]interface{} {
	m := map[string]interface{}{
		"metadata": r.Metadata,
	}

	if includeLogs {
		m["logs"] = r.Logs
	}

	if r.ImageBase64 != "" {
		m["image_base64"] = r.ImageBase64
	}

	return m
}
