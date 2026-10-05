package wokwi

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/georgik/espbrew-go/internal/backend/wokwi/api"
	"github.com/georgik/espbrew-go/pkg/protocol"
)

// MonitorMode determines whether to use CLI or API
type MonitorMode int

const (
	// MonitorModeCLI uses wokwi-cli subprocess
	MonitorModeCLI MonitorMode = iota
	// MonitorModeAPI uses Wokwi WebSocket API
	MonitorModeAPI
)

// Environment variables that let the leader point its Wokwi API client at a
// self-hosted wokwi-ci-server instead of the public wss://wokwi.com endpoint.
//   - WOKWI_CLI_TOKEN : API token (falls back to WokwiConfig.APIToken).
//   - WOKWI_CLI_SERVER: WS server URL, e.g. ws://wokwi-ci-server:3000/api/ws/beta.
//     Empty means the public wokwi.com endpoint (default).
const (
	envWokwiToken  = "WOKWI_CLI_TOKEN"
	envWokwiServer = "WOKWI_CLI_SERVER"
)

// APIMonitor implements protocol.Monitor using Wokwi API
type APIMonitor struct {
	config      *protocol.WokwiConfig
	elfPath     string // Firmware source path (ELF or assembled image) used for simulation
	logCh       chan protocol.LogEntry
	logChClosed bool // true once Stop has closed logCh; Start recreates it
	ctx         context.Context
	cancel      context.CancelFunc
	client      *api.Client
	mu          sync.Mutex
	timeout     time.Duration
	exitOn      string
	running     bool
	apiToken    string
	serverURL   string
}

// NewAPIMonitor creates a new Wokwi API monitor.
// serverURL points at the Wokwi WebSocket server; empty means the public
// wss://wokwi.com endpoint.
func NewAPIMonitor(device *protocol.DeviceInfo, apiToken, serverURL string) (protocol.Monitor, error) {
	if device.Backend != protocol.BackendWokwi {
		return nil, fmt.Errorf("device backend is not wokwi: %s", device.Backend)
	}

	cfg, ok := device.BackendConfig.(*protocol.WokwiConfig)
	if !ok {
		return nil, fmt.Errorf("invalid backend config type for wokwi device")
	}

	return &APIMonitor{
		config:    cfg,
		logCh:     make(chan protocol.LogEntry, 100),
		timeout:   DefaultWokwiTimeout,
		exitOn:    DefaultSuccessPattern,
		apiToken:  apiToken,
		serverURL: serverURL,
	}, nil
}

// SetELFPath sets the path to the firmware source (ELF or assembled image) used
// for simulation. The file is assembled into flash sections and uploaded to the
// Wokwi server when Start is called.
func (m *APIMonitor) SetELFPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.elfPath = path
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

	// Recreate the output channel if a previous run closed it (e.g. after a
	// Reset), so streaming can resume on the fresh channel.
	if m.logChClosed {
		m.logCh = make(chan protocol.LogEntry, 100)
		m.logChClosed = false
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	log.Debug().
		Str("elf", m.elfPath).
		Str("timeout", m.timeout.String()).
		Str("expect_text", m.exitOn).
		Msg("Starting Wokwi simulation via API")

	// Create API client
	// Point at the configured Wokwi server. Empty serverURL falls back to the
	// public wss://wokwi.com endpoint inside NewClientWithServer.
	m.client = api.NewClientWithServer(m.apiToken, m.serverURL)

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

	// Read the received firmware and assemble it into flash sections. An ELF is
	// turned into a bootable image (bootloader + partition table + app) using
	// the same mechanism physical/RUST flashing uses (see assembleSections and
	// flash.ConvertELFToESPImage); an already-assembled image is split directly.
	// A lone app image cannot boot on its own, so it is placed at the app offset.
	firmwareData, err := os.ReadFile(m.elfPath)
	if err != nil {
		_ = m.client.Close()
		return fmt.Errorf("read firmware: %w", err)
	}

	sections, err := assembleSections(m.chipForImage(), firmwareData)
	if err != nil {
		_ = m.client.Close()
		return fmt.Errorf("assemble firmware: %w", err)
	}
	log.Debug().Int("sections", len(sections)).Msg("Assembled firmware into flash sections")

	// Upload each section as flash-<offset>.bin so the simulator boots the full
	// image (bootloader + partition + app), not just a lone app blob.
	uploaded, err := m.client.UploadFirmwareSections(m.ctx, sections)
	if err != nil {
		_ = m.client.Close()
		return fmt.Errorf("failed to upload firmware sections: %w", err)
	}

	// Upload ELF (optional, for better debugging)
	elfName, err := m.client.UploadELF(m.ctx, m.elfPath)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to upload ELF (non-critical)")
	} else if elfName != "" {
		log.Debug().Str("elf", elfName).Msg("Uploaded ELF")
	}

	// Start simulation with the uploaded sections.
	if err := m.client.StartSimulation(m.ctx, api.SimStartParams{
		Firmware:  uploaded,
		Elf:       elfName,
		FlashSize: defaultFlashSizeBytes(),
	}); err != nil {
		_ = m.client.Close()
		return fmt.Errorf("failed to start simulation: %w", err)
	}

	// Begin listening for serial output before the sim runs.
	if err := m.client.ListenSerial(m.ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to listen on serial monitor (non-critical)")
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
			// The Wokwi protocol delivers serial bytes as a numeric array
			// under payload.bytes (see wiki/wokwi-sim-api-analysis.md §2).
			data := m.client.ReadSerialBytes(event)
			if len(data) == 0 {
				continue
			}
			// Split data into lines for log entries
			scanner := bufio.NewScanner(bytes.NewReader(data))
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
	m.logChClosed = true
	return nil
}

// Output returns the log entry channel
func (m *APIMonitor) Output() <-chan protocol.LogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
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

	return m.client.RestartSimulation(m.ctx, false)
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

// NewAPIMonitorFromConfig creates a Wokwi API monitor with explicit config.
// serverURL points at the Wokwi WebSocket server; empty means the public
// wss://wokwi.com endpoint.
func NewAPIMonitorFromConfig(cfg *protocol.WokwiConfig, elfPath, apiToken, serverURL string) (*APIMonitor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &APIMonitor{
		config:    cfg,
		elfPath:   elfPath,
		logCh:     make(chan protocol.LogEntry, 100),
		timeout:   DefaultWokwiTimeout,
		exitOn:    DefaultSuccessPattern,
		apiToken:  apiToken,
		serverURL: serverURL,
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
