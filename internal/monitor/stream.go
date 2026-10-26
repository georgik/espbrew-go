package monitor

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/georgik/espbrew-go/internal/serialheal"
	"github.com/rs/zerolog/log"
	"go.bug.st/serial"
)

// DeviceReservation is the subset of the cluster device reservation the stream
// session couples to the OS serial-port lifecycle. It is an interface so the
// monitor package can hold a reservation without importing the cluster
// package (which would create an import cycle). The concrete adapter, wired in
// the http package, delegates to the leader's *cluster.DeviceRegistry.
//
// The contract is simple: Reserve must succeed before the OS port is opened
// and Release must run after it is closed, so the reservation is held for the
// whole session lifetime. This makes the cluster-level lock track the exact
// port the OS mutex protects, instead of leaking until the 30-minute TTL
// cleanup. The server session is the sole holder of the reservation: the
// remote CLI no longer reserves the device over HTTP, so there is a single
// owner and the lock is freed the instant the port closes.
type DeviceReservation interface {
	// Reserve claims the reservation for owner before the port opens. It
	// reports whether the claim succeeded; a false result means another
	// owner already holds it.
	Reserve(path, owner string) bool
	// Release frees the reservation after the port closes.
	Release(path, owner string)
	// Owner reports who currently holds the reservation, for error messages.
	Owner(path string) string
}

type StreamConfig struct {
	Port       string
	BaudRate   int
	ExitOn     string
	Timeout    time.Duration
	TimeoutCtx context.Context // Parent context for timeout
	// Reservation, when set, is acquired before the port opens and released
	// after it closes (see StreamSession.Start / StreamManager.Remove).
	Reservation DeviceReservation
}

type StreamSession struct {
	id        string
	config    StreamConfig
	port      serial.Port
	dataCh    chan []byte
	errorCh   chan error
	control   chan *ControlMessage
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	parentCtx context.Context // Parent context for timeout
	// reservation couples the session to the cluster reservation lock. When
	// set, the reservation is acquired before the port opens and released
	// after it closes, so a concurrent flash is blocked while a monitor holds
	// the port (and freed the moment the monitor detaches).
	reservation DeviceReservation
}

type ControlMessage struct {
	Type string // "reset", "close"
	Data string
}

func NewStreamSession(id string, cfg StreamConfig) *StreamSession {
	ctx, cancel := context.WithCancel(context.Background())
	parentCtx := cfg.TimeoutCtx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	return &StreamSession{
		id:          id,
		config:      cfg,
		reservation: cfg.Reservation,
		dataCh:      make(chan []byte, 256),
		errorCh:     make(chan error, 1),
		control:     make(chan *ControlMessage, 10),
		ctx:         ctx,
		cancel:      cancel,
		parentCtx:   parentCtx,
	}
}

func (s *StreamSession) Start() error {
	mode := &serial.Mode{
		BaudRate: s.config.BaudRate,
	}

	// Reserve the cluster device reservation before touching the OS serial
	// port, so the reservation lock tracks the same port the OS mutex
	// protects. A concurrent flash is blocked while a monitor holds the port,
	// and the reservation is held for the whole session lifetime.
	if s.reservation != nil {
		if !s.reservation.Reserve(s.config.Port, s.id) {
			owner := s.reservation.Owner(s.config.Port)
			return fmt.Errorf("device reserved by: %s", owner)
		}
	}

	port, err := serialheal.Open(s.config.Port, mode)
	if err != nil {
		// A failed open must not leak the reservation until the TTL cleanup.
		if s.reservation != nil {
			s.reservation.Release(s.config.Port, s.id)
		}
		return fmt.Errorf("open serial: %w", err)
	}
	s.port = port

	_ = port.SetReadTimeout(50 * time.Millisecond)

	go s.readLoop()
	go s.controlLoop()

	return nil
}

func (s *StreamSession) readLoop() {
	defer close(s.dataCh)

	buf := make([]byte, 1024)
	exitPattern := s.config.ExitOn

	maxRetries := 10
	retryDelay := 200 * time.Millisecond

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.parentCtx.Done():
			return
		default:
			n, err := s.port.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])

				select {
				case s.dataCh <- data:
				default:
					// Channel full, drop
				}

				if exitPattern != "" && s.containsMatch(data, exitPattern) {
					log.Info().Str("session", s.id).Str("pattern", exitPattern).
						Msg("Exit pattern matched")
					s.cancel()
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					return
				}

				// Timeout errors are expected when no data available - continue
				if err.Error() == "timeout" || err.Error() == "read timeout" {
					continue
				}

				// Other errors (device disconnect) - try to reconnect
				log.Warn().Str("session", s.id).Err(err).Msg("Device read error, attempting reconnect")

				// Try to reconnect with backoff
				for attempt := 1; attempt <= maxRetries; attempt++ {
					select {
					case <-s.ctx.Done():
						return
					case <-time.After(retryDelay * time.Duration(attempt)):
					}

					// Close old port
					if s.port != nil {
						_ = s.port.Close()
					}

					// Try to reopen
					mode := &serial.Mode{BaudRate: s.config.BaudRate}
					port, openErr := serialheal.Open(s.config.Port, mode)
					if openErr == nil {
						s.port = port
						_ = s.port.SetReadTimeout(50 * time.Millisecond)
						log.Info().Str("session", s.id).Int("attempt", attempt).Msg("Reconnected to device")
						break // Success, continue reading
					}

					if attempt == maxRetries {
						log.Error().Str("session", s.id).Err(err).Msg("Failed to reconnect after retries")
						select {
						case s.errorCh <- err:
						default:
						}
						return
					}
				}
			}
		}
	}
}

func (s *StreamSession) controlLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.control:
			switch msg.Type {
			case "reset":
				// Toggle DTR/RTS to reset device
				_ = s.port.SetDTR(false)
				_ = s.port.SetRTS(true)
				time.Sleep(100 * time.Millisecond)
				_ = s.port.SetRTS(false)
				time.Sleep(50 * time.Millisecond)
				log.Debug().Str("session", s.id).Msg("Device reset triggered")
			case "close":
				s.cancel()
				return
			}
		}
	}
}

func (s *StreamSession) Data() <-chan []byte {
	return s.dataCh
}

func (s *StreamSession) Errors() <-chan error {
	return s.errorCh
}

func (s *StreamSession) SendControl(msg *ControlMessage) {
	select {
	case s.control <- msg:
	case <-s.ctx.Done():
	}
}

func (s *StreamSession) Close() error {
	s.cancel()
	if s.port != nil {
		return s.port.Close()
	}
	return nil
}

// Write writes data to the serial port
func (s *StreamSession) Write(data []byte) error {
	if s.port == nil {
		return io.ErrClosedPipe
	}
	_, err := s.port.Write(data)
	return err
}

func (s *StreamSession) containsMatch(data []byte, pattern string) bool {
	dataStr := string(data)
	for i := 0; i <= len(dataStr)-len(pattern); i++ {
		if dataStr[i:i+len(pattern)] == pattern {
			return true
		}
	}
	return false
}

type StreamManager struct {
	sessions map[string]*StreamSession
	mu       sync.RWMutex
}

func NewStreamManager() *StreamManager {
	return &StreamManager{
		sessions: make(map[string]*StreamSession),
	}
}

func (m *StreamManager) Create(id string, cfg StreamConfig) (*StreamSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[id]; exists {
		return nil, fmt.Errorf("session already exists: %s", id)
	}

	session := NewStreamSession(id, cfg)
	if err := session.Start(); err != nil {
		return nil, err
	}

	m.sessions[id] = session
	return session, nil
}

func (m *StreamManager) Get(id string) (*StreamSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *StreamManager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, exists := m.sessions[id]; exists {
		_ = s.Close()
		// Release the reservation now that the OS port is closed, coupling the
		// cluster lock to the serial-port lifecycle so a waiting flash can
		// proceed the instant the monitor detaches.
		if s.reservation != nil {
			s.reservation.Release(s.config.Port, s.id)
		}
		delete(m.sessions, id)
	}
}

func (m *StreamManager) List() map[string]*StreamSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*StreamSession, len(m.sessions))
	for k, v := range m.sessions {
		result[k] = v
	}
	return result
}
