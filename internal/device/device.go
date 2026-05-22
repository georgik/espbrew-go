package device

import "github.com/georgik/esp-ci-cluster/pkg/protocol"

type EventType string

const (
	DeviceAdded   EventType = "added"
	DeviceRemoved EventType = "removed"
)

type DeviceEvent struct {
	Type   EventType
	Path   string
	VID    uint16
	PID    uint16
	Serial string
}

type DeviceInfo struct {
	Path         string
	VID          uint16
	PID          uint16
	SerialNumber string
}

const (
	ESP_VID = 0x4348
	ESP_PID_S2 = 0x0027
	ESP_PID_S3 = 0x0028
	ESP_PID_C3 = 0x0029
	ESP_PID_C6 = 0x002a
)

func IsESPDevice(vid, pid uint16) bool {
	if vid != ESP_VID {
		return false
	}
	switch pid {
	case ESP_PID_S2, ESP_PID_S3, ESP_PID_C3, ESP_PID_C6:
		return true
	}
	return true // Allow any ESP device
}

func EventToProtocol(event *DeviceEvent, nodeID string) *protocol.DeviceInfo {
	return &protocol.DeviceInfo{
		Path:         event.Path,
		VID:          event.VID,
		PID:          event.PID,
		SerialNumber: event.Serial,
		NodeID:       nodeID,
		Status:       "available",
	}
}
