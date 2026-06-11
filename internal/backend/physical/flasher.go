package physical

import (
	"context"
	"fmt"

	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

// Flasher wraps the existing flash operations for physical devices
type Flasher struct {
	device *protocol.DeviceInfo
}

// NewPhysicalFlasher creates a new physical device flasher
func NewPhysicalFlasher(device *protocol.DeviceInfo) (protocol.Flasher, error) {
	if device.Backend != protocol.BackendPhysical && device.Backend != "" {
		return nil, fmt.Errorf("device backend is not physical: %s", device.Backend)
	}

	return &Flasher{
		device: device,
	}, nil
}

// Flash writes firmware to the physical device
func (f *Flasher) Flash(ctx context.Context, firmwarePath string, progress chan<- int) error {
	// TODO: Implement using existing flash infrastructure
	// This will integrate with internal/flash operations
	return fmt.Errorf("physical flash not yet implemented in backend package")
}

// ReadFlash reads flash memory from the physical device
func (f *Flasher) ReadFlash(ctx context.Context, address, size uint32) ([]byte, error) {
	// TODO: Implement using existing read flash infrastructure
	return nil, fmt.Errorf("physical read flash not yet implemented in backend package")
}
