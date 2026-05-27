package inventory

import (
	"fmt"
	"time"
)

// DeviceIdentity represents auto-detected device properties from eFuse
type DeviceIdentity struct {
	MAC       string // Factory MAC address (eFuse)
	Chip      string // Detected chip type (e.g., "ESP32-S3")
	ChipMajor uint8  // Chip major revision
	ChipMinor uint8  // Chip minor revision
	FlashSize uint32 // Detected flash size in bytes (0 = unknown)
	PSRAMSize uint32 // Detected PSRAM size in bytes (0 = none)
	PSRAMType string // PSRAM type ("AP_3v3", "AP_1v8", or "")
}

// DeviceInventory represents a stored device record
type DeviceInventory struct {
	// Primary identifier
	DeviceID string `json:"device_id"` // MAC-based, e.g., "esp-84:f7:03:12:34:56"

	// Auto-detected hardware properties (read-only)
	MACAddress string `json:"mac_address"` // Factory MAC from eFuse
	ChipType   string `json:"chip_type"`   // ESP32, ESP32-S3, etc.
	ChipRev    string `json:"chip_rev"`    // "1.0", "0.1", etc.
	FlashSize  uint32 `json:"flash_size"`  // In bytes
	PSRAMSize  uint32 `json:"psram_size"`  // In bytes (0 if none)
	PSRAMType  string `json:"psram_type"`  // "AP_3v3", "AP_1v8", etc.

	// User-editable properties
	Aliases     []string `json:"aliases"`     // Custom names for CI jobs
	Tags        []string `json:"tags"`        // User-defined classification
	BoardModel  string   `json:"board_model"` // e.g., "ESP32-S3-BOX-3"
	Description string   `json:"description"` // Free-form notes

	// Metadata
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	LastPath  string    `json:"last_path"` // Last known USB path
	NodeID    string    `json:"node_id"`   // Last connected cluster node
}

// FlashRequirement defines requirements for selecting a device
type FlashRequirement struct {
	ChipType   string   `json:"chip_type"`   // Required chip
	MinPSRAM   uint32   `json:"min_psram"`   // Minimum PSRAM in bytes
	MinFlash   uint32   `json:"min_flash"`   // Minimum flash size
	Tags       []string `json:"tags"`        // Required tags (all must match)
	BoardModel string   `json:"board_model"` // Specific board model
}

// Matches returns true if the device satisfies all requirements
func (d *DeviceInventory) Matches(req *FlashRequirement) bool {
	if req.ChipType != "" && d.ChipType != req.ChipType {
		return false
	}
	if req.MinPSRAM > 0 && d.PSRAMSize < req.MinPSRAM {
		return false
	}
	if req.MinFlash > 0 && d.FlashSize < req.MinFlash {
		return false
	}
	if req.BoardModel != "" && d.BoardModel != req.BoardModel {
		return false
	}
	if len(req.Tags) > 0 {
		tagMap := make(map[string]bool)
		for _, t := range d.Tags {
			tagMap[t] = true
		}
		for _, reqTag := range req.Tags {
			if !tagMap[reqTag] {
				return false
			}
		}
	}
	return true
}

// formatRevision formats major and minor revision as "X.Y"
func formatRevision(major, minor uint8) string {
	return fmt.Sprintf("%d.%d", major, minor)
}
