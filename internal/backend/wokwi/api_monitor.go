package wokwi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"codeberg.org/georgik/espbrew-go/internal/backend/wokwi/api"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

// MonitorMode determines whether to use CLI or API
type MonitorMode int

const (
	// MonitorModeCLI uses wokwi-cli subprocess
	MonitorModeCLI MonitorMode = iota
	// MonitorModeAPI uses Wokwi WebSocket API
	MonitorModeAPI
)

// APIMonitor implements protocol.Monitor using Wokwi API
type APIMonitor struct {
	config   *protocol.WokwiConfig
	elfPath  string
	firmware string
	logCh    chan protocol.LogEntry
	ctx      context.Context
	cancel   context.CancelFunc
	client   *api.Client
	mu       sync.Mutex
	timeout  time.Duration
	exitOn   string
	running  bool
	apiToken string
}

// NewAPIMonitor creates a new Wokwi API monitor
func NewAPIMonitor(device *protocol.DeviceInfo, apiToken string) (protocol.Monitor, error) {
	if device.Backend != protocol.BackendWokwi {
		return nil, fmt.Errorf("device backend is not wokwi: %s", device.Backend)
	}

	cfg, ok := device.BackendConfig.(*protocol.WokwiConfig)
	if !ok {
		return nil, fmt.Errorf("invalid backend config type for wokwi device")
	}

	return &APIMonitor{
		config:   cfg,
		logCh:    make(chan protocol.LogEntry, 100),
		timeout:  DefaultWokwiTimeout,
		exitOn:   DefaultSuccessPattern,
		apiToken: apiToken,
	}, nil
}

// SetELFPath sets the path to the ELF file for simulation
func (m *APIMonitor) SetELFPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.elfPath = path
	// For Wokwi API, firmware and ELF are the same (we use the ELF path)
	m.firmware = path
}

// SetTimeout sets the simulation timeout
func (m *APIMonitor) SetTimeout(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timeout = timeout
}

// SetExitPattern sets the pattern to detect successful execution
func (m *APIMonitor) SetExitPattern(pattern string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exitOn = pattern
}

// Start begins the Wokwi simulation using the API
func (m *APIMonitor) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("monitor already running")
	}

	if m.elfPath == "" {
		return fmt.Errorf("ELF path not set")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	log.Debug().
		Str("elf", m.elfPath).
		Str("timeout", m.timeout.String()).
		Str("expect_text", m.exitOn).
		Msg("Starting Wokwi simulation via API")

	// Create API client
	m.client = api.NewClient(m.apiToken)

	// Connect to Wokwi API
	if _, err := m.client.Connect(m.ctx); err != nil {
		return fmt.Errorf("failed to connect to Wokwi API: %w", err)
	}

	// Set diagram
	m.client.SetDiagram(m.config.DiagramJSON)

	// Upload diagram
	if err := m.client.UploadDiagram(m.ctx); err != nil {
		_ = m.client.Close()
		return fmt.Errorf("failed to upload diagram: %w", err)
	}

	// Set and upload firmware
	m.client.SetFirmware(m.elfPath)
	if err := m.client.UploadFirmware(m.ctx); err != nil {
		_ = m.client.Close()
		return fmt.Errorf("failed to upload firmware: %w", err)
	}

	// Upload ELF (optional, for better debugging)
	m.client.SetELF(m.elfPath)
	if err := m.client.UploadELF(m.ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to upload ELF (non-critical)")
	}

	// Start simulation
	if err := m.client.StartSimulation(m.ctx); err != nil {
		_ = m.client.Close()
		return fmt.Errorf("failed to start simulation: %w", err)
	}

	m.running = true

	// Start serial monitor in background
	go m.serialMonitor()

	// Start timeout watcher
	go m.timeoutWatcher()

	return nil
}

func (m *APIMonitor) serialMonitor() {
	serialCh := m.client.SubscribeSerial()
	defer m.client.UnsubscribeSerial(serialCh)

	for {
		select {
		case <-m.ctx.Done():
			return
		case event, ok := <-serialCh:
			if !ok {
				return
			}
			if data, ok := event.Result["data"].(string); ok {
				// Split data into lines for log entries
				scanner := bufio.NewScanner(strings.NewReader(data))
				for scanner.Scan() {
					line := scanner.Text()
					select {
					case m.logCh <- protocol.LogEntry{
						Timestamp: time.Now().Unix(),
						Data:      line + "\n",
						IsError:   false,
					}:
					case <-m.ctx.Done():
						return
					}
				}
			}
		}
	}
}

func (m *APIMonitor) timeoutWatcher() {
	timer := time.NewTimer(m.timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		log.Info().Msg("Wokwi simulation timeout reached")
		_ = m.Stop()
	case <-m.ctx.Done():
		return
	}
}

// Stop stops the Wokwi simulation
func (m *APIMonitor) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	if m.client != nil {
		m.client.Close()
	}

	m.running = false
	close(m.logCh)
	return nil
}

// Output returns the log entry channel
func (m *APIMonitor) Output() <-chan protocol.LogEntry {
	return m.logCh
}

// Send sends data to the simulator serial port
func (m *APIMonitor) Send(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.client == nil {
		return fmt.Errorf("monitor not running")
	}

	return m.client.WriteSerial(m.ctx, data)
}

// Reset resets the simulator (restarts simulation)
func (m *APIMonitor) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("monitor not running")
	}

	if m.client == nil {
		return fmt.Errorf("client not initialized")
	}

	return m.client.RestartSimulation(m.ctx)
}

// IsRunning returns whether the monitor is currently running
func (m *APIMonitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// GetConfig returns the Wokwi configuration
func (m *APIMonitor) GetConfig() *protocol.WokwiConfig {
	return m.config
}

// Validate checks if the monitor configuration is valid
func (m *APIMonitor) Validate() error {
	if m.config == nil {
		return fmt.Errorf("wokwi config is required")
	}

	if err := m.config.Validate(); err != nil {
		return fmt.Errorf("invalid wokwi config: %w", err)
	}

	if m.elfPath == "" {
		return fmt.Errorf("ELF path is required")
	}

	if _, err := os.Stat(m.elfPath); os.IsNotExist(err) {
		return fmt.Errorf("ELF file does not exist: %s", m.elfPath)
	}

	if m.apiToken == "" {
		return fmt.Errorf("API token is required for Wokwi API mode")
	}

	return nil
}

// NewAPIMonitorFromConfig creates a Wokwi API monitor with explicit config
func NewAPIMonitorFromConfig(cfg *protocol.WokwiConfig, elfPath, apiToken string) (*APIMonitor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &APIMonitor{
		config:   cfg,
		elfPath:  elfPath,
		firmware: elfPath,
		logCh:    make(chan protocol.LogEntry, 100),
		timeout:  DefaultWokwiTimeout,
		exitOn:   DefaultSuccessPattern,
		apiToken: apiToken,
	}, nil
}

// StreamOutput streams serial output to a writer
func (m *APIMonitor) StreamOutput(ctx context.Context, out io.Writer) error {
	return m.client.StreamOutput(ctx, out)
}

// WaitForOutput waits for a specific pattern in serial output
func (m *APIMonitor) WaitForOutput(pattern string, timeout time.Duration) error {
	return m.client.WaitForOutput(m.ctx, pattern, timeout)
}
