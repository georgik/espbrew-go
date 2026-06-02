package flashhash

// Region names for ESP32 flash layout
const (
	RegionBootloader     = "bootloader"
	RegionPartitionTable = "partition-table"
	RegionOTASelect      = "ota-select"
	RegionApplication    = "application"
	RegionNVS            = "nvs"
	RegionPHYInit        = "phy-init"
	RegionCustom         = "custom"
)

// FlashRegionInfo describes a single flash region with its hash
type FlashRegionInfo struct {
	Name   string `json:"name"`   // Region identifier (e.g., "bootloader", "application")
	Offset uint32 `json:"offset"` // Flash offset in bytes
	Size   uint32 `json:"size"`   // Region size in bytes
	MD5    string `json:"md5"`    // MD5 hash of region content (hex encoded)
}

// FlashStatusRequest is sent by client to query flash status
type FlashStatusRequest struct {
	DeviceID string            `json:"device_id"`
	Regions  []FlashRegionInfo `json:"regions"`
}

// CachedRegionInfo describes why a region was skipped
type CachedRegionInfo struct {
	Name   string `json:"name"`
	Reason string `json:"reason"` // "hash_match", "not_in_job", "ignored"
}

// FlashStatusResponse is returned by server with optimization recommendations
type FlashStatusResponse struct {
	Status        string             `json:"status"` // "full_flash", "partial_update", "skip_all"
	RegionsNeeded []FlashRegionInfo  `json:"regions_needed,omitempty"`
	RegionsCached []CachedRegionInfo `json:"regions_cached,omitempty"`
	JobID         string             `json:"job_id,omitempty"`
	Message       string             `json:"message,omitempty"`
}

// JobFlashHashes stores expected hashes for a job
type JobFlashHashes struct {
	JobID     string            `json:"job_id"`
	DeviceID  string            `json:"device_id"` // Optional: for device-specific jobs
	Regions   []FlashRegionInfo `json:"regions"`
	CreatedAt string            `json:"created_at"` // ISO 8601
}

// StandardESP32S3Layout4MB returns the standard flash layout for ESP32-S3 with 4MB flash
func StandardESP32S3Layout4MB() []FlashRegionInfo {
	return []FlashRegionInfo{
		{Name: RegionBootloader, Offset: 0x1000, Size: 0x7000},
		{Name: RegionPartitionTable, Offset: 0x8000, Size: 0x1000},
		{Name: RegionOTASelect, Offset: 0x9000, Size: 0x2000},
		{Name: RegionApplication, Offset: 0x10000, Size: 0x400000},
	}
}

// StandardESP32Layout4MB returns the standard flash layout for ESP32 with 4MB flash
func StandardESP32Layout4MB() []FlashRegionInfo {
	return []FlashRegionInfo{
		{Name: RegionBootloader, Offset: 0x1000, Size: 0x7000},
		{Name: RegionPartitionTable, Offset: 0x8000, Size: 0x1000},
		{Name: RegionOTASelect, Offset: 0x9000, Size: 0x2000},
		{Name: RegionApplication, Offset: 0x10000, Size: 0x400000},
	}
}

// StandardESP32C3Layout4MB returns the standard flash layout for ESP32-C3 with 4MB flash
func StandardESP32C3Layout4MB() []FlashRegionInfo {
	return []FlashRegionInfo{
		{Name: RegionBootloader, Offset: 0x1000, Size: 0x7000},
		{Name: RegionPartitionTable, Offset: 0x8000, Size: 0x1000},
		{Name: RegionOTASelect, Offset: 0x9000, Size: 0x2000},
		{Name: RegionApplication, Offset: 0x10000, Size: 0x400000},
	}
}

// IsStandardRegion checks if a region name is a standard ESP32 region
func IsStandardRegion(name string) bool {
	switch name {
	case RegionBootloader, RegionPartitionTable, RegionOTASelect,
		RegionApplication, RegionNVS, RegionPHYInit:
		return true
	default:
		return false
	}
}

// Validate checks if the region info is valid
func (r *FlashRegionInfo) Validate() error {
	if r.Name == "" {
		return &RegionError{Field: "name", Reason: "name cannot be empty"}
	}
	if r.Size == 0 {
		return &RegionError{Field: "size", Reason: "size must be greater than 0"}
	}
	if r.MD5 == "" {
		return &RegionError{Field: "md5", Reason: "MD5 hash cannot be empty"}
	}
	if len(r.MD5) != 32 {
		return &RegionError{Field: "md5", Reason: "MD5 hash must be 32 characters (hex encoded)"}
	}
	return nil
}

// RegionError represents a validation error for a region
type RegionError struct {
	Field  string
	Reason string
}

func (e *RegionError) Error() string {
	return e.Field + ": " + e.Reason
}
