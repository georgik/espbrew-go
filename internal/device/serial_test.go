package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanner_Scan(t *testing.T) {
	scanner := NewScanner()
	ports, err := scanner.Scan()

	// Should not error
	assert.NoError(t, err)
	// Returns a list (may be empty if no devices)
	assert.NotNil(t, ports)

	if len(ports) > 0 {
		for _, p := range ports {
			assert.NotEmpty(t, p.Path, "Port path should not be empty")
		}
	}
}

func TestScanner_ScanESP(t *testing.T) {
	scanner := NewScanner()
	devices, err := scanner.ScanESP()

	assert.NoError(t, err)
	assert.NotNil(t, devices)

	for _, dev := range devices {
		assert.NotEmpty(t, dev.Path)
		assert.NotEqual(t, uint16(0), dev.VID)
	}
}

func TestIsLikelyESP(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/dev/ttyUSB0", true},
		{"/dev/ttyACM0", true},
		{"/dev/cu.usbserial", true},
		{"/dev/cu.usbmodem", true},
		{"/dev/tty.wchusbserial", true},
		{"/dev/ttyS0", false},
		{"/dev/pts/0", false},
		{"/dev/null", false},
	}

	watcher := &Watcher{}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := watcher.isLikelyESP(tt.path)
			assert.Equal(t, tt.want, got, "isLikelyESP(%q)", tt.path)
		})
	}
}

// MockScanner for testing
type MockScanner struct {
	ports []Port
	err   error
}

func (m *MockScanner) Scan() ([]Port, error) {
	return m.ports, m.err
}
