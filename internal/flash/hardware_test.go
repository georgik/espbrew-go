//go:build hardware
// +build hardware

package flash

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRealFlash(t *testing.T) {
	// This test requires actual ESP32 hardware
	// Run with: go test -tags hardware ./internal/flash
	t.Skip("Hardware test - requires ESP32 device")

	ctx := context.Background()
	f := NewFlasher(nil)

	req := &FlashRequest{
		Port:     "/dev/ttyUSB0", // Change to actual port
		Firmware: make([]byte, 100),
		Progress: make(chan int, 10),
	}

	result := f.Flash(ctx, req)
	assert.NotNil(t, result)
}
