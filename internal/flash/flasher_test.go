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

func TestEraseRequest_FullErase(t *testing.T) {
	req := &EraseRequest{
		Port:     "/dev/ttyUSB0",
		EraseAll: true,
		Progress: nil,
	}
	assert.True(t, req.EraseAll, "EraseAll should be true for full erase")
	assert.Equal(t, uint32(0), req.Address, "Address should be 0 for full erase")
	assert.Equal(t, uint32(0), req.Size, "Size should be 0 for full erase")
}

func TestEraseRequest_RegionErase(t *testing.T) {
	req := &EraseRequest{
		Port:     "/dev/ttyUSB0",
		Address:  0x10000,
		Size:     0x1000,
		Progress: nil,
	}
	assert.False(t, req.EraseAll, "EraseAll should be false for region erase")
	assert.Equal(t, uint32(0x10000), req.Address, "Address should be specified")
	assert.Equal(t, uint32(0x1000), req.Size, "Size should be specified")
}

func TestEraseResult_Success(t *testing.T) {
	result := &EraseResult{
		Success: true,
		Bytes:   4194304,
	}
	assert.True(t, result.Success)
	assert.Equal(t, 4194304, result.Bytes)
	assert.Nil(t, result.Error)
}

func TestEraseResult_Failure(t *testing.T) {
	err := assert.AnError
	result := &EraseResult{
		Success: false,
		Error:   err,
	}
	assert.False(t, result.Success)
	assert.Equal(t, err, result.Error)
}

// TestFlasherEraseDefault tests that erase defaults to false
func TestFlasherEraseDefault(t *testing.T) {
	flasher := NewFlasher(nil)
	assert.NotNil(t, flasher)
	assert.NotNil(t, flasher.opts)
	assert.False(t, flasher.opts.Erase, "Erase should default to false")
}

// TestFlasherSetErase tests the SetErase method
func TestFlasherSetErase(t *testing.T) {
	flasher := NewFlasher(nil)
	assert.NotNil(t, flasher)

	// Test setting to true
	flasher.SetErase(true)
	assert.True(t, flasher.opts.Erase, "Erase should be true after SetErase(true)")

	// Test setting to false
	flasher.SetErase(false)
	assert.False(t, flasher.opts.Erase, "Erase should be false after SetErase(false)")
}

// TestFlasherOptionsWithErase tests creating Flasher with erase enabled
func TestFlasherOptionsWithErase(t *testing.T) {
	opts := &FlasherOptions{
		BaudRate:      115200,
		FlashBaudRate: 460800,
		Compress:      true,
		Erase:         true,
	}

	flasher := NewFlasher(opts)
	assert.NotNil(t, flasher)
	assert.True(t, flasher.opts.Erase, "Erase should be true when set in options")
}

// TestFlasherOptionsAllDefaults tests that FlasherOptions has all correct defaults
func TestFlasherOptionsAllDefaults(t *testing.T) {
	flasher := NewFlasher(nil)

	assert.Equal(t, 115200, flasher.opts.BaudRate, "BaudRate should default to 115200")
	assert.Equal(t, 460800, flasher.opts.FlashBaudRate, "FlashBaudRate should default to 460800")
	assert.True(t, flasher.opts.Compress, "Compress should default to true")
	assert.False(t, flasher.opts.Erase, "Erase should default to false")
}

// TestIsUSBPort tests USB CDC port detection
func TestIsUSBPort(t *testing.T) {
	tests := []struct {
		name     string
		port     string
		expected bool
	}{
		{"Linux USB CDC ACM0", "/dev/ttyACM0", true},
		{"Linux USB CDC ACM1", "/dev/ttyACM1", true},
		{"Linux USB CDC ACM with number", "/dev/ttyACM42", true},
		{"macOS USB CDC", "/dev/cu.usbmodem1234", true},
		{"macOS USB serial", "/dev/cu.usbserial", true},
		{"Linux USB serial", "/dev/ttyUSB0", false},
		{"Linux standard serial", "/dev/ttyS0", false},
		{"Windows COM port", "COM1", false},
		{"Empty string", "", false},
		{"Short string", "/dev/tty", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isUSBPort(tt.port)
			assert.Equal(t, tt.expected, result, "isUSBPort(%q) should return %v", tt.port, tt.expected)
		})
	}
}
