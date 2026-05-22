package cluster

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type DeviceState string

const (
	DeviceAvailable DeviceState = "available"
	DeviceReserved  DeviceState = "reserved"
	DeviceBusy      DeviceState = "busy"
	DeviceError     DeviceState = "error"
)

type DeviceLock struct {
	state      DeviceState
	owner      string // job ID or client ID
	reservedAt time.Time
	mu         sync.RWMutex
}

func (d *DeviceLock) State() DeviceState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

func (d *DeviceLock) Owner() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.owner
}

func (d *DeviceLock) Reserve(owner string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state != DeviceAvailable {
		log.Debug().Str("owner", d.owner).Str("state", string(d.state)).
			Msg("Device not available for reservation")
		return false
	}

	d.state = DeviceReserved
	d.owner = owner
	d.reservedAt = time.Now()
	log.Debug().Str("owner", owner).Msg("Device reserved")
	return true
}

func (d *DeviceLock) Release(owner string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.owner != owner {
		log.Warn().Str("owner", d.owner).Str("requester", owner).
			Msg("Release denied: not owner")
		return false
	}

	d.state = DeviceAvailable
	d.owner = ""
	log.Debug().Msg("Device released")
	return true
}

func (d *DeviceLock) Acquire(owner string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state == DeviceError {
		return false
	}

	// Owner can reacquire (e.g., monitor after flash)
	if d.state == DeviceBusy && d.owner == owner {
		return true
	}

	if d.state != DeviceReserved || d.owner != owner {
		log.Debug().Str("state", string(d.state)).Str("owner", d.owner).
			Msg("Device not available for acquisition")
		return false
	}

	d.state = DeviceBusy
	log.Debug().Str("owner", owner).Msg("Device acquired")
	return true
}

func (d *DeviceLock) ForceRelease() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	prevOwner := d.owner
	d.state = DeviceAvailable
	d.owner = ""
	log.Warn().Str("prev_owner", prevOwner).Msg("Device force released")
	return prevOwner
}

type DeviceRegistry struct {
	devices map[string]*DeviceLock
	mu      sync.RWMutex
}

func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{
		devices: make(map[string]*DeviceLock),
	}
}

func (r *DeviceRegistry) Register(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.devices[path]; !exists {
		r.devices[path] = &DeviceLock{state: DeviceAvailable}
		log.Debug().Str("path", path).Msg("Device registered")
	}
}

func (r *DeviceRegistry) Unregister(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.devices, path)
	log.Debug().Str("path", path).Msg("Device unregistered")
}

func (r *DeviceRegistry) Reserve(path, owner string) bool {
	r.mu.RLock()
	dev, exists := r.devices[path]
	r.mu.RUnlock()

	if !exists {
		return false
	}

	return dev.Reserve(owner)
}

func (r *DeviceRegistry) Release(path, owner string) bool {
	r.mu.RLock()
	dev, exists := r.devices[path]
	r.mu.RUnlock()

	if !exists {
		return false
	}

	return dev.Release(owner)
}

func (r *DeviceRegistry) GetState(path string) DeviceState {
	r.mu.RLock()
	dev, exists := r.devices[path]
	r.mu.RUnlock()

	if !exists {
		return DeviceError
	}

	return dev.State()
}

func (r *DeviceRegistry) ListDevices() map[string]DeviceState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]DeviceState)
	for path, dev := range r.devices {
		result[path] = dev.State()
	}
	return result
}

func (r *DeviceRegistry) AvailableDevices() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var avail []string
	for path, dev := range r.devices {
		if dev.State() == DeviceAvailable {
			avail = append(avail, path)
		}
	}
	return avail
}

func (r *DeviceRegistry) GetOwner(path string) string {
	r.mu.RLock()
	dev, exists := r.devices[path]
	r.mu.RUnlock()

	if !exists {
		return ""
	}

	return dev.Owner()
}

func (r *DeviceRegistry) CleanupStaleReservations(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	cleaned := 0

	for path, dev := range r.devices {
		dev.mu.Lock()
		if dev.state == DeviceReserved || dev.state == DeviceBusy {
			if dev.reservedAt.Before(cutoff) {
				prevOwner := dev.owner
				dev.state = DeviceAvailable
				dev.owner = ""
				log.Info().Str("path", path).Str("prev_owner", prevOwner).
					Msg("Cleaned up stale device reservation")
				cleaned++
			}
		}
		dev.mu.Unlock()
	}

	return cleaned
}
