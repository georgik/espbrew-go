package monitor

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"go.bug.st/serial"
)

type StreamConfig struct {
	Port     string
	BaudRate int
	ExitOn   string
	Timeout  time.Duration
}

type StreamSession struct {
	id      string
	config  StreamConfig
	port    serial.Port
	dataCh  chan []byte
	errorCh chan error
	control chan *ControlMessage
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

type ControlMessage struct {
	Type string // "reset", "close"
	Data string
}

func NewStreamSession(id string, cfg StreamConfig) *StreamSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &StreamSession{
		id:      id,
		config:  cfg,
		dataCh:  make(chan []byte, 256),
		errorCh: make(chan error, 1),
		control: make(chan *ControlMessage, 10),
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (s *StreamSession) Start() error {
	mode := &serial.Mode{
		BaudRate: s.config.BaudRate,
	}

	port, err := serial.Open(s.config.Port, mode)
	if err != nil {
		return fmt.Errorf("open serial: %w", err)
	}
	s.port = port

	port.SetReadTimeout(50 * time.Millisecond)

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
						s.port.Close()
					}

					// Try to reopen
					mode := &serial.Mode{BaudRate: s.config.BaudRate}
					port, openErr := serial.Open(s.config.Port, mode)
					if openErr == nil {
						s.port = port
						s.port.SetReadTimeout(50 * time.Millisecond)
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
				s.port.SetDTR(false)
				s.port.SetRTS(true)
				time.Sleep(100 * time.Millisecond)
				s.port.SetRTS(false)
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
		s.Close()
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
