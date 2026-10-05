// Package devicesleep manages auto sleep/wake of ESP boards that are attached
// to a power-controllable USB hub.
//
// When a board is registered with a USB hub location, espbrew can switch its
// power. The board is powered off (put to "sleep") whenever it is not in use,
// and powered on (woken) right before an operation starts. This gives two
// benefits:
//
//   - Cold-start testing: a board that is only ever rebooted keeps peripheral
//     state hidden. Powering it fully off and on makes those bugs visible.
//   - Power saving: idle boards draw nothing while they sleep.
//
// The package only talks to the outside world through the Controller interface,
// so the whole behaviour can be exercised in tests with an in-memory mock and
// no real USB hub attached.
package devicesleep

import (
	"sync"
	"time"

	"github.com/georgik/espbrew-go/internal/powercontrol"
	"github.com/rs/zerolog/log"
)

// Controller is the minimal slice of powercontrol.PowerController that the
// manager needs. The real powercontrol controller satisfies it, and tests
// provide a lightweight mock instead of a physical hub.
type Controller interface {
	// FindHubByLocation returns the hub at the given USB location (e.g. "1-2").
	FindHubByLocation(loc string) (*powercontrol.Hub, error)
	// SetPortPowerDual switches power for a port on a dual-interface hub.
	SetPortPowerDual(hub *powercontrol.Hub, port int, on bool) error
}

// Config holds the tunables that decide when a board sleeps and how long we
// wait after powering it on before it is ready to use. Every field has a
// sensible default (see DefaultConfig) but is overridable so tests can run
// without real wall-clock delays.
type Config struct {
	// WakeDelay is the pause after powering a hub port on, before an operation
	// is allowed to start. It gives the board time to re-enumerate. Default 1s.
	WakeDelay time.Duration
	// IdleTimeout is how long a board may sit unused before it is powered off.
	// Default 2m.
	IdleTimeout time.Duration
	// SweepPeriod is how often the background sweeper checks for idle boards.
	// Default 5s.
	SweepPeriod time.Duration
}

// DefaultConfig returns the production defaults.
func DefaultConfig() Config {
	return Config{
		WakeDelay:   1 * time.Second,
		IdleTimeout: 2 * time.Minute,
		SweepPeriod: 5 * time.Second,
	}
}

// device is the manager's private view of a power-managed board.
type device struct {
	path     string
	location string
	port     int
	sleeping bool
	busy     bool
	lastUsed time.Time
}

// Manager tracks the power state of every registered board and decides when to
// power a port on or off. All methods are safe for concurrent use.
type Manager struct {
	ctrl Controller
	cfg  Config

	mu   sync.Mutex
	devs map[string]*device
	stop chan struct{}
	done chan struct{}
}

// New returns a Manager that drives the given power controller. The manager is
// not running until Start is called.
func New(ctrl Controller, cfg Config) *Manager {
	if cfg == (Config{}) {
		cfg = DefaultConfig()
	}
	return &Manager{
		ctrl: ctrl,
		cfg:  cfg,
		devs: make(map[string]*device),
	}
}

// Start launches the background sweeper that powers down idle boards. It stops
// when Stop is called (or the manager's internal context is cancelled).
func (m *Manager) Start() {
	m.mu.Lock()
	if m.stop != nil {
		m.mu.Unlock()
		return // already running
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	cfg := m.cfg
	m.stop = stop
	m.done = done
	m.mu.Unlock()

	go func() {
		defer close(done)
		t := time.NewTicker(cfg.SweepPeriod)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if paths := m.Sweep(time.Now()); len(paths) > 0 {
					log.Info().Int("count", len(paths)).Msg("Powered down idle devices")
				}
			}
		}
	}()
}

// Stop halts the background sweeper and waits for it to exit.
func (m *Manager) Stop() {
	m.mu.Lock()
	stop := m.stop
	done := m.done
	m.stop = nil
	m.done = nil
	m.mu.Unlock()

	if stop != nil {
		close(stop)
		<-done
	}
}

// Config returns a copy of the manager's configuration (used by callers that
// need the wake delay, e.g. the leader before dispatching a job).
func (m *Manager) Config() Config { return m.cfg }

// WakeDelay returns how long to wait after powering a board on before using it.
func (m *Manager) WakeDelay() time.Duration { return m.cfg.WakeDelay }

// Register makes the manager track a power-managed board. location is the hub
// USB location and port is the 1-based port number on that hub. Calling Register
// again for the same path updates the location/port and preserves prior state.
func (m *Manager) Register(path, location string, port int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.devs[path]
	if !ok {
		d = &device{path: path}
		m.devs[path] = d
	}
	d.location = location
	d.port = port
}

// Unregister stops the manager tracking a board.
func (m *Manager) Unregister(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.devs, path)
}

// IsSleeping reports whether the board is currently powered off.
func (m *Manager) IsSleeping(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devs[path]
	if !ok {
		return false
	}
	return d.sleeping
}

// IsBusy reports whether an operation is currently using the board.
func (m *Manager) IsBusy(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devs[path]
	if !ok {
		return false
	}
	return d.busy
}

// SetBusy marks whether an operation is running against the board. A busy board
// is never powered down by the sweeper.
func (m *Manager) SetBusy(path string, busy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.devs[path]; ok {
		d.busy = busy
	}
}

// Touch records that the board was just used. It clears the sleeping state and
// resets the idle timer, so a board that is actively in use will not be
// powered down.
func (m *Manager) Touch(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.devs[path]; ok {
		d.lastUsed = time.Now()
		d.sleeping = false
	}
}

// LastUsed returns the last time the board was touched.
func (m *Manager) LastUsed(path string) (time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devs[path]
	if !ok {
		return time.Time{}, false
	}
	return d.lastUsed, true
}

// SetSleeping records the power state of a board without switching power. It is
// used at startup to mirror the live USB tree: a board that is not present is
// marked sleeping, one that is present is marked awake.
func (m *Manager) SetSleeping(path string, sleeping bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.devs[path]; ok {
		d.sleeping = sleeping
	}
}

// Wake powers the board's hub port on. It is safe to call on an already-awake
// board: the hub port power command is idempotent, so Wake always guarantees
// the board has power even if the internal sleeping flag is stale.
func (m *Manager) Wake(path string) error {
	m.mu.Lock()
	d, ok := m.devs[path]
	m.mu.Unlock()
	if !ok {
		return ErrUnknownDevice
	}
	hub, err := m.ctrl.FindHubByLocation(d.location)
	if err != nil {
		return err
	}
	if err := m.ctrl.SetPortPowerDual(hub, d.port, true); err != nil {
		return err
	}
	m.mu.Lock()
	d.sleeping = false
	m.mu.Unlock()
	log.Info().Str("path", path).Str("location", d.location).
		Int("port", d.port).Msg("Device woken (hub port powered on)")
	return nil
}

// Sleep powers the board's hub port off and marks it sleeping. It is
// idempotent: sleeping an already-sleeping board is a no-op.
func (m *Manager) Sleep(path string) error {
	m.mu.Lock()
	d, ok := m.devs[path]
	m.mu.Unlock()
	if !ok {
		return ErrUnknownDevice
	}
	if d.sleeping {
		return nil
	}
	hub, err := m.ctrl.FindHubByLocation(d.location)
	if err != nil {
		return err
	}
	if err := m.ctrl.SetPortPowerDual(hub, d.port, false); err != nil {
		return err
	}
	m.mu.Lock()
	d.sleeping = true
	m.mu.Unlock()
	log.Info().Str("path", path).Str("location", d.location).
		Int("port", d.port).Msg("Device put to sleep (hub port powered off)")
	return nil
}

// Sweep powers down every registered board that has been idle longer than
// IdleTimeout and is neither sleeping nor busy. It returns the list of board
// paths that were powered down so the caller can reconcile its own device
// records. It never blocks on the (slow) power switch.
func (m *Manager) Sweep(now time.Time) []string {
	m.mu.Lock()
	var idle []*device
	for _, d := range m.devs {
		if d.sleeping || d.busy {
			continue
		}
		if d.lastUsed.IsZero() {
			continue
		}
		if now.Sub(d.lastUsed) >= m.cfg.IdleTimeout {
			idle = append(idle, d)
		}
	}
	m.mu.Unlock()

	var down []string
	for _, d := range idle {
		if err := m.Sleep(d.path); err != nil {
			log.Warn().Str("path", d.path).Err(err).Msg("Failed to sleep idle device")
			continue
		}
		down = append(down, d.path)
	}
	return down
}

// Snapshot returns a read-only copy of every tracked board, for inspection and
// tests.
func (m *Manager) Snapshot() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.devs))
	for _, d := range m.devs {
		out = append(out, Info{
			Path:     d.path,
			Location: d.location,
			Port:     d.port,
			Sleeping: d.sleeping,
			Busy:     d.busy,
			LastUsed: d.lastUsed,
		})
	}
	return out
}

// Info is an immutable snapshot of a tracked board.
type Info struct {
	Path     string
	Location string
	Port     int
	Sleeping bool
	Busy     bool
	LastUsed time.Time
}
