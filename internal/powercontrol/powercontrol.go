// Package powercontrol provides USB hub power control functionality.
// It supports Linux via sysfs interface (kernel >= 6.0).
package powercontrol

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrHubNotFound      = errors.New("hub not found")
	ErrPortNotFound     = errors.New("port not found")
	ErrNotSupported     = errors.New("not supported on this platform")
	ErrPermissionDenied = errors.New("permission denied")
	ErrKernelTooOld     = errors.New("kernel version too old (requires >= 6.0)")
)

// Hub represents a USB hub with power control capabilities.
type Hub struct {
	Location     string // USB location, e.g., "1-2"
	Vendor       string // Vendor ID, e.g., "0bda"
	Product      string // Product ID, e.g., "0411"
	NumPorts     int    // Number of ports
	interfaceDir string // Interface directory, e.g., "1-2:1.0"
	superSpeed   bool   // true if USB 3.0 hub
}

// PortStatus represents the current status of a hub port.
type PortStatus struct {
	Port      int    // Port number (1-based)
	Power     bool   // true if power is on
	Connected bool   // true if device is connected
	Enabled   bool   // true if port is enabled
	Speed     string // Connection speed ("low", "high", "super", "5gbps", etc.)
}

// PowerController is the interface for USB hub power control.
type PowerController interface {
	// ListHubs returns all available USB hubs with per-port power switching.
	ListHubs() ([]Hub, error)

	// FindHubByLocation finds a hub by its USB location string.
	FindHubByLocation(loc string) (*Hub, error)

	// FindHubByVendorProduct finds a hub by vendor and product IDs.
	FindHubByVendorProduct(vendor, product string) (*Hub, error)

	// SetPortPower turns power on or off for a specific port.
	SetPortPower(hub *Hub, port int, on bool) error

	// GetPortStatus returns the current status of a port.
	GetPortStatus(hub *Hub, port int) (*PortStatus, error)

	// PowerCycle powers off a port, waits, then powers on.
	PowerCycle(hub *Hub, port int, delay time.Duration) error
}

// NewController returns the platform-specific power controller.
func NewController() PowerController {
	return &controller{}
}

// controller implements PowerController.
type controller struct{}

// ListHubs returns all available USB hubs with per-port power switching.
func (c *controller) ListHubs() ([]Hub, error) {
	return listHubs()
}

// FindHubByLocation finds a hub by its USB location string.
func (c *controller) FindHubByLocation(loc string) (*Hub, error) {
	hubs, err := listHubs()
	if err != nil {
		return nil, err
	}
	for _, h := range hubs {
		if h.Location == loc {
			return &h, nil
		}
	}
	return nil, ErrHubNotFound
}

// FindHubByVendorProduct finds a hub by vendor and product IDs.
func (c *controller) FindHubByVendorProduct(vendor, product string) (*Hub, error) {
	hubs, err := listHubs()
	if err != nil {
		return nil, err
	}
	for _, h := range hubs {
		if h.Vendor == vendor && h.Product == product {
			return &h, nil
		}
	}
	return nil, ErrHubNotFound
}

// SetPortPower turns power on or off for a specific port.
func (c *controller) SetPortPower(hub *Hub, port int, on bool) error {
	if port < 1 || port > hub.NumPorts {
		return ErrPortNotFound
	}
	return setPortPower(hub, port, on)
}

// GetPortStatus returns the current status of a port.
func (c *controller) GetPortStatus(hub *Hub, port int) (*PortStatus, error) {
	if port < 1 || port > hub.NumPorts {
		return nil, ErrPortNotFound
	}
	return getPortStatus(hub, port)
}

// PowerCycle powers off a port, waits, then powers on.
func (c *controller) PowerCycle(hub *Hub, port int, delay time.Duration) error {
	if port < 1 || port > hub.NumPorts {
		return ErrPortNotFound
	}

	// Power off
	if err := setPortPower(hub, port, false); err != nil {
		return fmt.Errorf("power off: %w", err)
	}

	// Calculate wait time (USB3 hubs need extra delay)
	waitTime := delay
	if hub.superSpeed {
		waitTime += 150 * time.Millisecond
	}
	time.Sleep(waitTime)

	// Power on
	if err := setPortPower(hub, port, true); err != nil {
		return fmt.Errorf("power on: %w", err)
	}

	return nil
}

// Platform-specific functions implemented in sysfs_linux.go or stub_other.go
var (
	listHubs      func() ([]Hub, error)
	setPortPower  func(*Hub, int, bool) error
	getPortStatus func(*Hub, int) (*PortStatus, error)
)

func init() {
	// Initialize platform-specific implementations
	// These are set in sysfs_linux.go or stub_other.go
}
