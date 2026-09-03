//go:build !linux
// +build !linux

package powercontrol

func init() {
	// Provide stub implementations for non-Linux platforms
	listHubs = func() ([]Hub, error) {
		return nil, ErrNotSupported
	}
	setPortPower = func(*Hub, int, bool) error {
		return ErrNotSupported
	}
	getPortStatus = func(*Hub, int) (*PortStatus, error) {
		return nil, ErrNotSupported
	}
}

func init() {
	// Provide stub implementations for non-Linux platforms
	listHubs = func() ([]Hub, error) {
		return nil, ErrNotSupported
	}
	setPortPower = func(*Hub, int, bool) error {
		return ErrNotSupported
	}
	getPortStatus = func(*Hub, int) (*PortStatus, error) {
		return nil, ErrNotSupported
	}
}
