package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsESPDevice(t *testing.T) {
	tests := []struct {
		vid  uint16
		pid  uint16
		want bool
	}{
		{0x4348, ESP_PID_S2, true},
		{0x4348, ESP_PID_S3, true},
		{0x4348, ESP_PID_C3, true},
		{0x4348, ESP_PID_C6, true},
		{0x1234, 0x5678, false},
		{0x0000, 0x0000, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := IsESPDevice(tt.vid, tt.pid)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEventToProtocol(t *testing.T) {
	event := &DeviceEvent{
		Type:   DeviceAdded,
		Path:   "/dev/ttyUSB0",
		VID:    ESP_VID,
		PID:    ESP_PID_S3,
		Serial: "abc123",
	}

	proto := EventToProtocol(event, "node-1")

	assert.Equal(t, "/dev/ttyUSB0", proto.Path)
	assert.Equal(t, uint16(ESP_VID), proto.VID)
	assert.Equal(t, uint16(ESP_PID_S3), proto.PID)
	assert.Equal(t, "node-1", proto.NodeID)
	assert.Equal(t, "available", proto.Status)
}
