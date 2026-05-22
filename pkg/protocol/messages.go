package protocol

import "time"

type MessageType string

const (
	NodeJoin       MessageType = "NodeJoin"
	NodeLeave      MessageType = "NodeLeave"
	Heartbeat      MessageType = "Heartbeat"
	DeviceAnnounce MessageType = "DeviceAnnounce"
	DeviceUpdate   MessageType = "DeviceUpdate"
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
	ActiveJobs  int           `json:"active_jobs"`
	Timestamp   int64         `json:"timestamp"`
	Devices     []*DeviceInfo `json:"devices,omitempty"`
}
