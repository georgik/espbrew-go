//go:build !linux
// +build !linux

package powercontrol

func init() {
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

// linkDualHubs links dual-interface hub counterparts.
// No USB hub enumeration is available on non-Linux platforms, so this is a no-op.
func linkDualHubs(hub *Hub) {}
