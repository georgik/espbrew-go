package wokwi

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/georgik/espbrew-go/internal/backend/wokwi/api"
	"github.com/georgik/espbrew-go/pkg/protocol"
	"github.com/rs/zerolog/log"
)

// SessionManager runs and reuses APIMonitor instances for Wokwi devices on the
// leader. Both the flash and monitor handlers go through it so that `flash`
// starts the simulation and `monitor` attaches to the running simulation,
// sharing a single Wokwi session instead of starting two.
//
// The session is keyed by device path. The first caller that supplies an
// elfPath starts a new simulation; later callers (e.g. `monitor` after a
// `flash`) reuse the running monitor without needing the firmware path again.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]*wokwiSession
}

// wokwiSession holds the running monitor for a device plus the firmware path
// used to start it.
type wokwiSession struct {
	monitor *APIMonitor
	elfPath string
}

// NewSessionManager creates an empty session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{sessions: make(map[string]*wokwiSession)}
}

// NewAPIMonitorForDevice builds a Wokwi API monitor for a device, resolving the
// API token (device config, then WOKWI_CLI_TOKEN) and the server URL
// (WOKWI_CLI_SERVER, empty means the public wss://wokwi.com endpoint). It is the
// leader-facing constructor: it always returns an *APIMonitor so flashing and
// monitoring go through the pure-Go Wokwi API client rather than the CLI.
func NewAPIMonitorForDevice(dev *protocol.DeviceInfo) (*APIMonitor, error) {
	if dev.Backend != protocol.BackendWokwi {
		return nil, fmt.Errorf("device backend is not wokwi: %s", dev.Backend)
	}

	cfg, ok := dev.BackendConfig.(*protocol.WokwiConfig)
	if !ok {
		return nil, fmt.Errorf("invalid backend config type for wokwi device")
	}

	token := cfg.APIToken
	if token == "" {
		token = os.Getenv(envWokwiToken)
	}
	if token == "" {
		return nil, fmt.Errorf("Wokwi API token not configured (set WokwiConfig.APIToken or the WOKWI_CLI_TOKEN env var)")
	}

	server := os.Getenv(envWokwiServer)

	mon, err := NewAPIMonitor(dev, token, server)
	if err != nil {
		return nil, err
	}
	apimon, ok := mon.(*APIMonitor)
	if !ok {
		return nil, fmt.Errorf("unexpected wokwi monitor type")
	}
	return apimon, nil
}

// Get returns the running monitor for dev, starting one if necessary.
//
// An empty elfPath is only accepted when a session already exists (the monitor
// attaching to a simulation a prior flash started); a fresh session always
// requires the firmware path so the image can be assembled and uploaded.
func (m *SessionManager) Get(dev *protocol.DeviceInfo, elfPath string) (protocol.Monitor, error) {
	m.mu.Lock()
	if st, ok := m.sessions[dev.Path]; ok {
		if st.monitor.IsRunning() {
			m.mu.Unlock()
			log.Debug().Str("device", dev.Path).Msg("Reusing existing Wokwi session")
			return st.monitor, nil
		}
		// A stale session (the monitor stopped, e.g. on timeout) must be torn
		// down before a fresh one is started.
		_ = st.monitor.Stop()
		delete(m.sessions, dev.Path)
	}
	m.mu.Unlock()

	if elfPath == "" {
		return nil, fmt.Errorf("no firmware flashed for device %s yet (run flash first)", dev.Path)
	}

	monitor, err := NewAPIMonitorForDevice(dev)
	if err != nil {
		return nil, err
	}
	monitor.SetELFPath(elfPath)

	log.Info().
		Str("device", dev.Path).
		Str("elf", elfPath).
		Msg("Starting Wokwi simulation session")

	// Start blocks until the image is assembled, uploaded, and sim:start has
	// been sent, so by the time this returns the simulation is running.
	if err := monitor.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("start wokwi session: %w", err)
	}

	st := &wokwiSession{monitor: monitor, elfPath: elfPath}
	m.mu.Lock()
	m.sessions[dev.Path] = st
	m.mu.Unlock()
	return monitor, nil
}

// Stop tears down the session for devPath, if any.
func (m *SessionManager) Stop(devPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.sessions[devPath]; ok {
		_ = st.monitor.Stop()
		delete(m.sessions, devPath)
		log.Debug().Str("device", devPath).Msg("Stopped Wokwi session")
	}
}

// Running reports whether an active simulation exists for devPath.
func (m *SessionManager) Running(devPath string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.sessions[devPath]
	return ok && st.monitor.IsRunning()
}

// ensure the api package stays referenced even if the constructor is refactored;
// it provides the protocol-correct client the sessions rely on.
var _ = api.NewClient
