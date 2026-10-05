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

// hubParseFallback is a no-op on non-Linux platforms: sysfs is Linux-only, so
// the FindHubByLocation fallback cannot resolve a hub here.
func hubParseFallback(loc string) (Hub, error) {
	return Hub{}, ErrNotSupported
}

// newPreferredController has no uhubctl opt-in off Linux, so it always returns
// the default controller (a no-op where power-controllable hubs are absent).
func newPreferredController() SleepController {
	return NewController()
}
