package devicesleep

import (
	"fmt"
	"sync"

	"github.com/georgik/espbrew-go/internal/powercontrol"
)

// MockController is an in-memory stand-in for powercontrol.PowerController. It
// simulates a set of power-controllable USB hubs so the sleep/wake behaviour
// can be tested with no physical hub attached (e.g. in CI).
//
// Each registered hub reports the given number of ports. SetPortPowerDual
// records the power state per (hub, port), so tests can assert that a board was
// actually powered on and off. It is safe for concurrent use.
type MockController struct {
	mu sync.Mutex

	// hubs maps a USB location (e.g. "1-2") to its simulated hub.
	hubs map[string]*powercontrol.Hub
	// power maps a location to the set of powered-on ports.
	power map[string]map[int]bool
	// SetPortPowerDual call log, in order.
	PowerCalls []PowerCall
}

// PowerCall records a single power switch performed through the mock.
type PowerCall struct {
	Location string
	Port     int
	On       bool
}

// NewMockController returns an empty mock controller.
func NewMockController() *MockController {
	return &MockController{
		hubs:       make(map[string]*powercontrol.Hub),
		power:      make(map[string]map[int]bool),
		PowerCalls: make([]PowerCall, 0),
	}
}

// AddHub registers a simulated hub with the given location and port count.
func (m *MockController) AddHub(location string, numPorts int) *MockController {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hubs[location] = &powercontrol.Hub{
		Location:   location,
		NumPorts:   numPorts,
		Vendor:     "0bda",
		Product:    "0411",
		SuperSpeed: true,
	}
	return m
}

// FindHubByLocation returns the simulated hub at loc, or powercontrol.ErrHubNotFound.
func (m *MockController) FindHubByLocation(loc string) (*powercontrol.Hub, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.hubs[loc]
	if !ok {
		return nil, powercontrol.ErrHubNotFound
	}
	cp := *h
	return &cp, nil
}
func (m *MockController) SetPortPowerDual(hub *powercontrol.Hub, port int, on bool) error {
	if hub == nil {
		return powercontrol.ErrHubNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	sh, ok := m.hubs[hub.Location]
	if !ok {
		return powercontrol.ErrHubNotFound
	}
	if port < 1 || port > sh.NumPorts {
		return fmt.Errorf("port %d out of range for hub %s (1..%d)", port, hub.Location, sh.NumPorts)
	}
	if m.power[hub.Location] == nil {
		m.power[hub.Location] = make(map[int]bool)
	}
	m.power[hub.Location][port] = on
	m.PowerCalls = append(m.PowerCalls, PowerCall{Location: hub.Location, Port: port, On: on})
	return nil
}

// IsPoweredOn reports whether the given (location, port) is currently powered.
func (m *MockController) IsPoweredOn(location string, port int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.power[location][port]
}

// PowerCallCount returns how many power switches have been performed.
func (m *MockController) PowerCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.PowerCalls)
}
