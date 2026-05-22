package flash

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFlasher(t *testing.T) {
	f := NewFlasher(nil)
	assert.NotNil(t, f)
	assert.NotNil(t, f.opts)
	assert.Equal(t, 115200, f.opts.BaudRate)
}

func TestNewFlasherWithOptions(t *testing.T) {
	opts := &FlasherOptions{
		BaudRate:      921600,
		FlashBaudRate: 921600,
		Compress:      false,
	}
	f := NewFlasher(opts)
	assert.Equal(t, 921600, f.opts.BaudRate)
	assert.Equal(t, 921600, f.opts.FlashBaudRate)
	assert.False(t, f.opts.Compress)
}

func TestFlashResult_Success(t *testing.T) {
	result := &FlashResult{
		Success: true,
		Bytes:   1024,
	}
	assert.True(t, result.Success)
	assert.Equal(t, 1024, result.Bytes)
	assert.Nil(t, result.Error)
}

func TestFlashResult_Failure(t *testing.T) {
	err := assert.AnError
	result := &FlashResult{
		Success: false,
		Error:   err,
	}
	assert.False(t, result.Success)
	assert.Equal(t, err, result.Error)
}

func TestFlashRequest_DefaultOffset(t *testing.T) {
	req := &FlashRequest{
		Port:     "/dev/ttyUSB0",
		Firmware: []byte{0x01, 0x02, 0x03},
		Progress: nil,
	}
	assert.Equal(t, 0, req.Offset, "Default offset should be 0")
}

func TestFlashRequest_CustomOffset(t *testing.T) {
	req := &FlashRequest{
		Port:     "/dev/ttyUSB0",
		Firmware: []byte{0x01, 0x02, 0x03},
		Offset:   0x10000,
		Progress: nil,
	}
	assert.Equal(t, 0x10000, req.Offset, "Custom offset should be preserved")
}

func TestFlashRequest_ApplicationPartitionOffset(t *testing.T) {
	req := &FlashRequest{
		Port:     "/dev/ttyUSB0",
		Firmware: []byte{0x01, 0x02, 0x03},
		Offset:   0x10000,
		Progress: nil,
	}
	assert.Equal(t, 65536, req.Offset, "Application partition offset should be 0x10000 (65536)")
}
