//go:build js
// +build js

package api

// Camera represents a camera device
type Camera struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Status  string `json:"status"`
	Backend string `json:"backend,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
}

// Device represents a device
type Device struct {
	DeviceID      string                 `json:"device_id"`
	Path          string                 `json:"path"`
	ChipType      string                 `json:"chip_type"`
	ChipRev       string                 `json:"chip_rev,omitempty"`
	FlashSize     uint32                 `json:"flash_size,omitempty"`
	PSRAMSize     uint32                 `json:"psram_size,omitempty"`
	PSRAMType     string                 `json:"psram_type,omitempty"`
	BoardModel    string                 `json:"board_model,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Status        string                 `json:"status"`
	Aliases       []string               `json:"aliases,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	MACAddress    string                 `json:"mac_address,omitempty"`
	NodeID        string                 `json:"node_id,omitempty"`
	Protected     bool                   `json:"protected,omitempty"`
	Disabled      bool                   `json:"disabled,omitempty"`
	AccessError   string                 `json:"access_error,omitempty"`
	Backend       string                 `json:"backend,omitempty"`
	BackendConfig map[string]interface{} `json:"backend_config,omitempty"`
}

// Capture represents a saved capture
type Capture struct {
	Path       string `json:"path"`
	Filename   string `json:"filename"`
	CameraID   string `json:"camera_id"`
	CameraName string `json:"camera_name"`
	Timestamp  int64  `json:"timestamp"`
	Size       int64  `json:"size"`
}

// CaptureRequest is a request to capture an image
type CaptureRequest struct {
	CameraID string `json:"camera_id"`
	Width    uint32 `json:"width,omitempty"`
	Height   uint32 `json:"height,omitempty"`
	Quality  int    `json:"quality,omitempty"`
	Format   string `json:"format,omitempty"`
	Preview  bool   `json:"preview,omitempty"`
}

// CaptureResponse is the response from a capture request
type CaptureResponse struct {
	Status    string `json:"status"`
	Path      string `json:"path"`
	CameraID  string `json:"camera_id"`
	Timestamp int64  `json:"timestamp"`
}

// CameraSettings represents camera settings
type CameraSettings struct {
	CameraID         string `json:"camera_id"`
	Name             string `json:"name"`
	Brightness       int32  `json:"brightness,omitempty"`
	Contrast         int32  `json:"contrast,omitempty"`
	Saturation       int32  `json:"saturation,omitempty"`
	Sharpness        int32  `json:"sharpness,omitempty"`
	Gain             int32  `json:"gain,omitempty"`
	Focus            int32  `json:"focus,omitempty"`
	Exposure         int32  `json:"exposure,omitempty"`
	WhiteBalance     int32  `json:"white_balance,omitempty"`
	AutoExposure     bool   `json:"auto_exposure,omitempty"`
	AutoFocus        bool   `json:"auto_focus,omitempty"`
	AutoWhiteBalance bool   `json:"auto_white_balance,omitempty"`
}

// BoundingBox represents a normalized bounding box
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ImageAdjustment represents image adjustments for a region
type ImageAdjustment struct {
	Brightness int `json:"brightness,omitempty"`
	Contrast   int `json:"contrast,omitempty"`
	Saturation int `json:"saturation,omitempty"`
}

// IsZero returns true if all adjustments are zero
func (a *ImageAdjustment) IsZero() bool {
	return a.Brightness == 0 && a.Contrast == 0 && a.Saturation == 0
}

// DeviceBoundingBoxMapping represents a device-to-camera mapping
type DeviceBoundingBoxMapping struct {
	ID                 string          `json:"id"`
	DeviceID           string          `json:"device_id"`
	CameraID           string          `json:"camera_id"`
	CameraName         string          `json:"camera_name,omitempty"`
	Bounds             BoundingBox     `json:"bounds"`
	CalibrationVersion int             `json:"calibration_version"`
	Adjustment         ImageAdjustment `json:"adjustment,omitempty"`
}

// DeviceMappingWithDevice extends mapping with device details
type DeviceMappingWithDevice struct {
	ID                 string          `json:"id"`
	DeviceID           string          `json:"device_id"`
	CameraID           string          `json:"camera_id"`
	CameraName         string          `json:"camera_name,omitempty"`
	Bounds             BoundingBox     `json:"bounds"`
	CalibrationVersion int             `json:"calibration_version"`
	Adjustment         ImageAdjustment `json:"adjustment,omitempty"`
	CreatedAt          string          `json:"created_at,omitempty"`
	UpdatedAt          string          `json:"updated_at,omitempty"`
	Device             *DeviceInfo     `json:"device,omitempty"`
}

// DeviceInfo represents basic device information
type DeviceInfo struct {
	DeviceID   string   `json:"device_id"`
	ChipType   string   `json:"chip_type"`
	Aliases    []string `json:"aliases,omitempty"`
	MACAddress string   `json:"mac_address,omitempty"`
}

// CameraMappingsResponse is the API response for camera mappings
type CameraMappingsResponse struct {
	CameraID    string                    `json:"camera_id"`
	Calibration *CalibrationInfo          `json:"calibration,omitempty"`
	Mappings    []DeviceMappingWithDevice `json:"mappings"`
}

// CalibrationInfo represents calibration summary info
type CalibrationInfo struct {
	Version     int    `json:"version"`
	Description string `json:"description"`
}

// FlashRequest represents a flash operation request
type FlashRequest struct {
	DeviceID string `json:"device_id"`
	BaudRate int    `json:"baud_rate,omitempty"`
	FileName string `json:"file_name,omitempty"`
}

// FlashStatus represents the status of a flash operation
type FlashStatus struct {
	DeviceID string `json:"device_id"`
	Status   string `json:"status"`
	Progress int    `json:"progress,omitempty"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

// CreateMappingRequest is a request to create a device mapping
type CreateMappingRequest struct {
	DeviceID   string                 `json:"device_id"`
	CameraID   string                 `json:"camera_id"`
	CameraName string                 `json:"camera_name,omitempty"` // Stable camera identifier
	Bounds     BoundingBox            `json:"bounds"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

// CreateMappingResponse is the response from creating a mapping
type CreateMappingResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// DeviceCaptureInfo represents a device-specific subimage capture
type DeviceCaptureInfo struct {
	DeviceID    string          `json:"device_id"`
	Bounds      BoundingBox     `json:"bounds"`
	Subimage    string          `json:"subimage_path"`
	Adjustment  ImageAdjustment `json:"adjustment"`
	GeneratedAt string          `json:"generated_at"`
}

// CameraControlsResponse represents camera control information
type CameraControlsResponse struct {
	Current       map[string]int32        `json:"current"`
	Available     bool                    `json:"available"`
	Platform      string                  `json:"platform"`
	DisplayPreset map[string]int32        `json:"display_preset"`
	FocusPresets  map[string]int32        `json:"focus_presets"`
	Ranges        map[string]ControlRange `json:"ranges"`
}

// ControlRange represents a control's valid range
type ControlRange struct {
	Min     int32 `json:"min"`
	Max     int32 `json:"max"`
	Current int32 `json:"current"`
}

// CameraSettingsRequest represents settings to apply to a camera
type CameraSettingsRequest struct {
	CameraID         string `json:"camera_id"`
	Name             string `json:"name,omitempty"`
	Brightness       int32  `json:"brightness,omitempty"`
	Contrast         int32  `json:"contrast,omitempty"`
	Saturation       int32  `json:"saturation,omitempty"`
	Sharpness        int32  `json:"sharpness,omitempty"`
	Gain             int32  `json:"gain,omitempty"`
	Focus            int32  `json:"focus,omitempty"`
	Exposure         int32  `json:"exposure,omitempty"`
	WhiteBalance     int32  `json:"white_balance,omitempty"`
	AutoExposure     bool   `json:"auto_exposure,omitempty"`
	AutoFocus        bool   `json:"auto_focus,omitempty"`
	AutoWhiteBalance bool   `json:"auto_white_balance,omitempty"`
}

// OperationMode represents the operational state of the cluster
type OperationMode string

const (
	ModeDiscovery   OperationMode = "discovery"
	ModeOperational OperationMode = "operational"
)

// ModeResponse represents the current operational mode
type ModeResponse struct {
	Mode OperationMode `json:"mode"`
}

// ModeRequest represents a request to set the operational mode
type ModeRequest struct {
	Mode OperationMode `json:"mode"`
}

// StatusResponse represents cluster status
type StatusResponse struct {
	Nodes       []NodeStatus  `json:"nodes"`
	Mode        OperationMode `json:"mode,omitempty"`
	DeviceCount int           `json:"device_count,omitempty"`
	CameraCount int           `json:"camera_count,omitempty"`
}

// NodeStatus represents a node in the status response
type NodeStatus struct {
	ID      string        `json:"id"`
	Address string        `json:"address"`
	Role    string        `json:"role"`
	Mode    OperationMode `json:"mode,omitempty"`
}
