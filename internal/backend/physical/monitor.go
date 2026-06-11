package physical

import (
	"context"
	"fmt"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/monitor"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

// Monitor wraps the existing monitor.StreamSession for physical devices
type Monitor struct {
	session *monitor.StreamSession
	config  monitor.StreamConfig
	logCh   chan protocol.LogEntry
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewPhysicalMonitor creates a new physical device monitor
func NewPhysicalMonitor(device *protocol.DeviceInfo) (protocol.Monitor, error) {
	if device.Backend != protocol.BackendPhysical && device.Backend != "" {
		return nil, fmt.Errorf("device backend is not physical: %s", device.Backend)
	}

	cfg := monitor.StreamConfig{
		Port:     device.Path,
		BaudRate: 115200,
		Timeout:  30 * time.Second,
	}

	return &Monitor{
		config: cfg,
		logCh:  make(chan protocol.LogEntry, 100),
	}, nil
}

// Start begins monitoring the physical device
func (m *Monitor) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	sessionID := fmt.Sprintf("physical-%s", m.config.Port)
	session := monitor.NewStreamSession(sessionID, m.config)

	if err := session.Start(); err != nil {
		return fmt.Errorf("failed to start physical monitor: %w", err)
	}

	m.session = session

	// Convert data channel to log entries
	go m.convertDataToLogs()

	return nil
}

func (m *Monitor) convertDataToLogs() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case data, ok := <-m.session.Data():
			if !ok {
				return
			}
			m.logCh <- protocol.LogEntry{
				Timestamp: time.Now().Unix(),
				Data:      string(data),
			}
		case <-ticker.C:
			// Keep select alive
		}
	}
}

// Stop stops monitoring the physical device
func (m *Monitor) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.session != nil {
		return m.session.Close()
	}
	close(m.logCh)
	return nil
}

// Output returns the log entry channel
func (m *Monitor) Output() <-chan protocol.LogEntry {
	return m.logCh
}

// Send sends data to the physical device
func (m *Monitor) Send(data []byte) error {
	if m.session == nil {
		return fmt.Errorf("session not started")
	}
	return m.session.Write(data)
}

// Reset resets the physical device via DTR/RTS toggle
func (m *Monitor) Reset() error {
	if m.session == nil {
		return fmt.Errorf("session not started")
	}
	m.session.SendControl(&monitor.ControlMessage{Type: "reset"})
	return nil
}

// SetBaudRate sets the baud rate for the physical device
func (m *Monitor) SetBaudRate(baud int) {
	m.config.BaudRate = baud
}

// SetExitPattern sets the pattern to exit monitoring
func (m *Monitor) SetExitPattern(pattern string) {
	m.config.ExitOn = pattern
}
