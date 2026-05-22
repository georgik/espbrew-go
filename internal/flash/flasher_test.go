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
