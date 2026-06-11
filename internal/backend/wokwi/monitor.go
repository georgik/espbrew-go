package wokwi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

const (
	// DefaultWokwiTimeout is the default timeout for Wokwi simulation
	DefaultWokwiTimeout = 30 * time.Second
	// DefaultSuccessPattern is the default pattern to detect successful execution
	DefaultSuccessPattern = "Returned from app_main()"
	// WokwiCliPath is the path to wokwi-cli executable
	WokwiCliPath = "wokwi-cli"
)

// Monitor implements protocol.Monitor for Wokwi simulator
type Monitor struct {
	config  *protocol.WokwiConfig
	elfPath string
	logCh   chan protocol.LogEntry
	ctx     context.Context
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	mu      sync.Mutex
	timeout time.Duration
	exitOn  string
	running bool
}

// NewMonitor creates a new Wokwi monitor from device info
// Uses API mode if APIToken is configured, otherwise falls back to CLI mode
func NewMonitor(device *protocol.DeviceInfo) (protocol.Monitor, error) {
	if device.Backend != protocol.BackendWokwi {
		return nil, fmt.Errorf("device backend is not wokwi: %s", device.Backend)
	}

	cfg, ok := device.BackendConfig.(*protocol.WokwiConfig)
	if !ok {
		return nil, fmt.Errorf("invalid backend config type for wokwi device")
	}

	// Use API mode if token is configured
	if cfg.APIToken != "" {
		return NewAPIMonitor(device, cfg.APIToken)
	}

	// Fall back to CLI mode
	return &Monitor{
		config:  cfg,
		logCh:   make(chan protocol.LogEntry, 100),
		timeout: DefaultWokwiTimeout,
		exitOn:  DefaultSuccessPattern,
	}, nil
}

// SetELFPath sets the path to the ELF file for simulation
func (m *Monitor) SetELFPath(path string) {
	m.elfPath = path
}

// SetTimeout sets the simulation timeout
func (m *Monitor) SetTimeout(timeout time.Duration) {
	m.timeout = timeout
}

// SetExitPattern sets the pattern to detect successful execution
func (m *Monitor) SetExitPattern(pattern string) {
	m.exitOn = pattern
}

// Start begins the Wokwi simulation
func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("monitor already running")
	}

	if m.elfPath == "" {
		return fmt.Errorf("ELF path not set")
	}

	// Create diagram file
	diagramPath, err := m.createDiagramFile()
	if err != nil {
		return fmt.Errorf("failed to create diagram file: %w", err)
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// Build wokwi-cli command
	args := []string{
		"--elf", m.elfPath,
		"--diagram-file", diagramPath,
		"--timeout", fmt.Sprintf("%d", m.timeout.Milliseconds()),
	}

	if m.exitOn != "" {
		args = append(args, "--expect-text", m.exitOn)
	}

	log.Debug().
		Str("elf", m.elfPath).
		Str("diagram", diagramPath).
		Str("timeout", m.timeout.String()).
		Str("expect_text", m.exitOn).
		Msg("Starting Wokwi simulation")

	m.cmd = exec.CommandContext(m.ctx, WokwiCliPath, args...)

	// Set up pipes
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := m.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start wokwi-cli: %w", err)
	}

	m.running = true

	// Start goroutines to read output
	go m.readOutput(stdout, false)
	go m.readOutput(stderr, true)

	// Wait for command to complete
	go m.waitForCompletion()

	return nil
}

func (m *Monitor) createDiagramFile() (string, error) {
	// Create temporary file for diagram
	tmpDir := os.TempDir()
	diagramPath := filepath.Join(tmpDir, fmt.Sprintf("wokwi-diagram-%d.json", time.Now().UnixNano()))

	if err := os.WriteFile(diagramPath, []byte(m.config.DiagramJSON), 0644); err != nil {
		return "", fmt.Errorf("failed to write diagram file: %w", err)
	}

	return diagramPath, nil
}

func (m *Monitor) readOutput(reader io.Reader, isError bool) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		select {
		case m.logCh <- protocol.LogEntry{
			Timestamp: time.Now().Unix(),
			Data:      line + "\n",
			IsError:   isError,
		}:
		case <-m.ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil {
		log.Debug().Err(err).Msg("Error reading wokwi-cli output")
	}
}

func (m *Monitor) waitForCompletion() {
	m.mu.Lock()
	err := m.cmd.Wait()
	m.running = false
	m.mu.Unlock()

	log.Debug().Err(err).Msg("Wokwi simulation completed")

	// Close log channel when done
	close(m.logCh)
}

// Stop stops the Wokwi simulation
func (m *Monitor) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	if m.cmd != nil && m.cmd.Process != nil {
		if err := m.cmd.Process.Kill(); err != nil {
			log.Warn().Err(err).Msg("Failed to kill wokwi-cli process")
		}
	}

	m.running = false
	return nil
}

// Output returns the log entry channel
func (m *Monitor) Output() <-chan protocol.LogEntry {
	return m.logCh
}

// Send sends data to the simulator (not supported for Wokwi in initial version)
func (m *Monitor) Send(data []byte) error {
	return fmt.Errorf("sending data to Wokwi simulator not yet supported")
}

// Reset resets the simulator (restarts simulation)
func (m *Monitor) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("monitor not running")
	}

	// Stop and restart
	if err := m.Stop(); err != nil {
		return err
	}

	return m.Start(m.ctx)
}

// IsRunning returns whether the monitor is currently running
func (m *Monitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// GetConfig returns the Wokwi configuration
func (m *Monitor) GetConfig() *protocol.WokwiConfig {
	return m.config
}

// Validate checks if the monitor configuration is valid
func (m *Monitor) Validate() error {
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

	return nil
}

// NewMonitorFromConfig creates a Wokwi monitor with explicit config
func NewMonitorFromConfig(cfg *protocol.WokwiConfig, elfPath string) (*Monitor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Monitor{
		config:  cfg,
		elfPath: elfPath,
		logCh:   make(chan protocol.LogEntry, 100),
		timeout: DefaultWokwiTimeout,
		exitOn:  DefaultSuccessPattern,
	}, nil
}

// CheckWokwiCliAvailable checks if wokwi-cli is available in PATH
func CheckWokwiCliAvailable() bool {
	_, err := exec.LookPath(WokwiCliPath)
	return err == nil
}

// GetWokwiCliVersion returns the version of wokwi-cli
func GetWokwiCliVersion() (string, error) {
	cmd := exec.Command(WokwiCliPath, "--short-version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get wokwi-cli version: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
