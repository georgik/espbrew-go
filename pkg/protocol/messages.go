package protocol

import "time"

type MessageType string

const (
	NodeJoin       MessageType = "NodeJoin"
	NodeLeave      MessageType = "NodeLeave"
	Heartbeat      MessageType = "Heartbeat"
	DeviceAnnounce MessageType = "DeviceAnnounce"
	DeviceUpdate   MessageType = "DeviceUpdate"
	CameraAnnounce MessageType = "CameraAnnounce"
	CameraCapture  MessageType = "CameraCapture"
	JobAssign      MessageType = "JobAssign"
	JobProgress    MessageType = "JobProgress"
	JobComplete    MessageType = "JobComplete"
	JobFailed      MessageType = "JobFailed"
	StateSync      MessageType = "StateSync"
)

type Message struct {
	Type    MessageType `json:"msg_type"`
	Payload interface{} `json:"payload"`
}

type NodeInfo struct {
	ID       string    `json:"id"`
	Address  string    `json:"address"`
	Role     string    `json:"role"`
	LastSeen time.Time `json:"last_seen"`
}

type DeviceInfo struct {
	Path         string `json:"path"`
	VID          uint16 `json:"vid"`
	PID          uint16 `json:"pid"`
	SerialNumber string `json:"serial"`
	NodeID       string `json:"node_id"`
	Status       string `json:"status"` // available, busy, offline
}

// CameraInfo represents a camera device attached to a node
type CameraInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Backend string `json:"backend"` // v4l2, avfoundation, directshow
	NodeID  string `json:"node_id"`
	Status  string `json:"status"` // available, busy, offline
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
