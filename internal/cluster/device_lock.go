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

// Info reports who currently holds the reservation and how long it has been
// held. It returns ok=false when the device is available (or unknown), so the
// caller can report "blocked by <owner> for <duration>" to a client whose
// reservation could not be granted.
func (d *DeviceLock) Info() (owner string, held time.Duration, locked bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.state == DeviceAvailable {
		return "", 0, false
	}
	return d.owner, time.Since(d.reservedAt), true
}

type DeviceRegistry struct {
	devices map[string]*DeviceLock
	mu      sync.RWMutex
	// reserveHook and releaseHook, when set, fire after a successful
	// Reserve/Release so the caller (leader) can keep an external device
	// status in sync with the reservation for every consumer (flash job,
	// monitor session, read-flash). Without the hook a reserved device would
	// still report "available" and a concurrent operation could race the open.
	reserveHook func(path, owner string)
	releaseHook func(path, owner string)
}

func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{
		devices: make(map[string]*DeviceLock),
	}
}

// SetStatusHooks wires callbacks invoked after a successful Reserve (reserveHook)
// and Release (releaseHook). They let the leader derive the API-facing device
// status from the reservation so flash, monitor and read-flash all report a
// consistent state. Passing nil leaves the hook unset.
func (r *DeviceRegistry) SetStatusHooks(reserve, release func(path, owner string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reserveHook = reserve
	r.releaseHook = release
}

func (r *DeviceRegistry) callReserveHook(path, owner string) {
	r.mu.RLock()
	h := r.reserveHook
	r.mu.RUnlock()
	if h != nil {
		h(path, owner)
	}
}

func (r *DeviceRegistry) callReleaseHook(path, owner string) {
	r.mu.RLock()
	h := r.releaseHook
	r.mu.RUnlock()
	if h != nil {
		h(path, owner)
	}
}

// fireReleaseHook invokes the release hook without touching r.mu. Callers that
// already hold r.mu (e.g. CleanupStaleReservations) must use this instead of
// callReleaseHook, which re-acquires r.mu.RLock() and would deadlock on the
// write lock held by the caller.
func (r *DeviceRegistry) fireReleaseHook(path, owner string) {
	if h := r.releaseHook; h != nil {
		h(path, owner)
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

	if !dev.Reserve(owner) {
		return false
	}

	// Keep any external device status in sync the instant the reservation
	// succeeds, so a concurrent flash/monitor/read-flash sees "busy" rather
	// than racing an open on a port that is already held.
	r.callReserveHook(path, owner)
	return true
}

func (r *DeviceRegistry) Release(path, owner string) bool {
	r.mu.RLock()
	dev, exists := r.devices[path]
	r.mu.RUnlock()

	if !exists {
		return false
	}

	if !dev.Release(owner) {
		return false
	}

	r.callReleaseHook(path, owner)
	return true
}

// ForceRelease clears any reservation on the device regardless of the current
// owner. Unlike Release (which requires the caller to match the owner), it is
// used to recover devices left stuck by a client that failed to release (e.g.
// a monitor whose release was shadowed by a route bug). It returns the owner
// that was holding the reservation before the release.
func (r *DeviceRegistry) ForceRelease(path string) string {
	r.mu.RLock()
	dev, exists := r.devices[path]
	r.mu.RUnlock()

	if !exists {
		return ""
	}

	prevOwner := dev.ForceRelease()
	// ForceRelease is only ever called from paths that already hold l.mu and
	// stamp the API-facing status directly (handleDeviceEvent,
	// cleanupOrphanedDevices), so firing the release hook here would re-enter
	// that lock and deadlock. The external status is left to the caller.
	return prevOwner
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

// Info reports the current reservation owner and how long the device has been
// held. ok is false when the device is available or unknown. Callers use this
// to report "blocked by <owner> for <duration>" to a client whose reservation
// could not be granted, instead of a bare "device not available".
func (r *DeviceRegistry) Info(path string) (owner string, held time.Duration, locked bool) {
	r.mu.RLock()
	dev, exists := r.devices[path]
	r.mu.RUnlock()

	if !exists {
		return "", 0, false
	}

	return dev.Info()
}

func (r *DeviceRegistry) CleanupStaleReservations(maxAge time.Duration) int {
	r.mu.Lock()

	cutoff := time.Now().Add(-maxAge)
	cleaned := 0

	// freed accumulates the (path, owner) pairs released so the status hook
	// can fire AFTER r.mu is released. Firing while holding r.mu would
	// deadlock: the hook stamps the API-facing status via the leader's
	// device lock, while a concurrent device event holds that lock and takes
	// r.mu (handleDeviceEvent/cleanupOrphanedDevices). A lock-ordering
	// inversion between the two would stall both.
	var freed [][2]string
	for path, dev := range r.devices {
		dev.mu.Lock()
		if dev.state == DeviceReserved || dev.state == DeviceBusy {
			if dev.reservedAt.Before(cutoff) {
				log.Info().Str("path", path).Str("prev_owner", dev.owner).
					Msg("Cleaned up stale device reservation")
				freed = append(freed, [2]string{path, dev.owner})
				dev.state = DeviceAvailable
				dev.owner = ""
				cleaned++
			}
		}
		dev.mu.Unlock()
	}

	// Release r.mu before touching the leader's device-status lock so the two
	// locks are never held at the same time.
	r.mu.Unlock()

	for _, f := range freed {
		r.fireReleaseHook(f[0], f[1])
	}

	return cleaned
}
