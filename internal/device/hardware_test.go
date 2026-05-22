//go:build hardware
// +build hardware

package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRealDeviceDetection(t *testing.T) {
	scanner := NewScanner()
	devices, err := scanner.ScanESP()

	assert.NoError(t, err)

	if len(devices) == 0 {
		t.Skip("No ESP devices connected. Connect an ESP32 to test hardware detection.")
	}

	t.Logf("Found %d ESP device(s)", len(devices))
	for _, dev := range devices {
		t.Logf("  - %s: VID=%04x PID=%04x", dev.Path, dev.VID, dev.PID)
	}
}
