package snap

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"go.bug.st/serial"
)

// Monitor handles serial port monitoring for snapshot operations.
// It captures serial output, parses log levels, and detects boot completion.
type Monitor struct {
	// Configuration
	port     string        // Serial port path (e.g., "/dev/ttyUSB0")
	baudRate int           // Baud rate for serial communication
	timeout  time.Duration // Read timeout for serial operations

	// Internal state
	portImpl   serial.Port        // Opened serial port
	mu         sync.Mutex         // Protects portImpl and internal state
	cancelFunc context.CancelFunc // For stopping goroutines
	buffer     []byte             // Read buffer
	scanner    *bufio.Scanner     // Line scanner
}

// NewMonitor creates a new Monitor with the specified configuration.
//
// Parameters:
//   - port: Serial port path (e.g., "/dev/ttyUSB0")
//   - baudRate: Baud rate (common values: 115200, 460800, 921600)
//   - timeout: Read timeout (recommend 50-100ms for responsive monitoring)
//
// Returns a configured Monitor ready to open the serial port.
func NewMonitor(port string, baudRate int, timeout time.Duration) *Monitor {
	return &Monitor{
		port:     port,
		baudRate: baudRate,
		timeout:  timeout,
		buffer:   make([]byte, 4096),
	}
}

// Open opens the serial port with the configured settings.
// The port is opened in exclusive mode and configured with the specified baud rate.
//
// Returns an error if the port cannot be opened or configured.
func (m *Monitor) Open() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.portImpl != nil {
		return fmt.Errorf("port already open")
	}

	mode := &serial.Mode{
		BaudRate: m.baudRate,
	}

	port, err := serial.Open(m.port, mode)
	if err != nil {
		return fmt.Errorf("open serial port %s: %w", m.port, err)
	}

	// Set read timeout for non-blocking reads
	if err := port.SetReadTimeout(m.timeout); err != nil {
		port.Close()
		return fmt.Errorf("set read timeout: %w", err)
	}

	m.portImpl = port
	log.Debug().
		Str("port", m.port).
		Int("baud", m.baudRate).
		Dur("timeout", m.timeout).
		Msg("Serial monitor opened")

	return nil
}

// Close closes the serial port and releases resources.
func (m *Monitor) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.portImpl == nil {
		return nil
	}

	if err := m.portImpl.Close(); err != nil {
		return fmt.Errorf("close serial port: %w", err)
	}

	m.portImpl = nil
	return nil
}

// Reset performs a hardware reset on the device using DTR/RTS lines.
// This is useful for capturing boot logs from the start of the boot sequence.
//
// The reset sequence:
// 1. DTR=low, RTS=high (enter reset mode)
// 2. Wait 100ms
// 3. RTS=low (release reset)
// 4. Wait 50ms
// 5. DTR=low (normal operation)
func (m *Monitor) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.portImpl == nil {
		return fmt.Errorf("port not open")
	}

	log.Debug().Str("port", m.port).Msg("Resetting device")

	// Enter reset mode
	if err := m.portImpl.SetDTR(false); err != nil {
		log.Warn().Err(err).Msg("SetDTR failed during reset")
	}
	if err := m.portImpl.SetRTS(true); err != nil {
		log.Warn().Err(err).Msg("SetRTS failed during reset")
	}

	time.Sleep(100 * time.Millisecond)

	// Release reset
	if err := m.portImpl.SetRTS(false); err != nil {
		log.Warn().Err(err).Msg("SetRTS release failed")
	}

	time.Sleep(50 * time.Millisecond)

	// Normal operation
	if err := m.portImpl.SetDTR(false); err != nil {
		log.Warn().Err(err).Msg("SetDTR final failed")
	}

	return nil
}

// Capture captures serial output for the specified duration.
// It reads lines from the serial port, parses log levels, and returns
// all captured log entries.
//
// Parameters:
//   - ctx: Context for cancellation (respects deadline and cancellation)
//   - duration: How long to capture (use 0 for indefinite until ctx cancel)
//
// Returns a slice of SerialLogEntry containing all captured log lines.
// The context error is returned if the operation is cancelled.
func (m *Monitor) Capture(ctx context.Context, duration time.Duration) ([]SerialLogEntry, error) {
	m.mu.Lock()
	if m.portImpl == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("port not open")
	}
	m.mu.Unlock()

	// Create context with deadline if duration specified
	captureCtx := ctx
	if duration > 0 {
		var cancel context.CancelFunc
		captureCtx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}

	log.Debug().
		Str("port", m.port).
		Dur("duration", duration).
		Msg("Starting serial capture")

	// Channel for log entries
	entryCh := make(chan SerialLogEntry, 100)
	errorCh := make(chan error, 1)

	// Start reader goroutine
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer close(entryCh)

		m.readLoop(captureCtx, entryCh, errorCh)
	}()

	// Collect entries
	var entries []SerialLogEntry
	doneCh := make(chan struct{})
	go func() {
		for entry := range entryCh {
			entries = append(entries, entry)
		}
		close(doneCh)
	}()

	// Wait for completion or error
	select {
	case <-doneCh:
		wg.Wait()
		log.Debug().Int("entries", len(entries)).Msg("Capture completed")
		return entries, nil

	case err := <-errorCh:
		wg.Wait()
		return entries, fmt.Errorf("capture error: %w", err)

	case <-captureCtx.Done():
		// Allow goroutine to finish
		wg.Wait()
		// Drain any remaining entries
		<-doneCh
		log.Debug().Int("entries", len(entries)).Msg("Capture cancelled")
		return entries, ctx.Err()
	}
}

// readLoop continuously reads from the serial port and sends log entries.
// This runs in a separate goroutine and handles the raw reading and line parsing.
func (m *Monitor) readLoop(ctx context.Context, entryCh chan<- SerialLogEntry, errorCh chan<- error) {
	lineBuf := make([]byte, 0, 256)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-tick.C:
			m.mu.Lock()
			if m.portImpl == nil {
				m.mu.Unlock()
				errorCh <- fmt.Errorf("port closed")
				return
			}

			// Read available data
			n, err := m.portImpl.Read(m.buffer)
			m.mu.Unlock()

			if err != nil {
				// Timeout is expected, continue
				if strings.Contains(err.Error(), "timeout") {
					continue
				}
				errorCh <- fmt.Errorf("read error: %w", err)
				return
			}

			if n == 0 {
				continue
			}

			// Process buffer line by line
			data := m.buffer[:n]
			for _, b := range data {
				if b == '\n' {
					// Complete line
					if len(lineBuf) > 0 {
						entry := m.parseLine(string(lineBuf))
						entryCh <- entry
						lineBuf = lineBuf[:0] // Reset buffer
					}
				} else if b != '\r' {
					// Skip carriage return
					lineBuf = append(lineBuf, b)
				}
			}
		}
	}
}

// parseLine converts a raw line into a SerialLogEntry with parsed log level.
func (m *Monitor) parseLine(line string) SerialLogEntry {
	// Trim whitespace
	line = strings.TrimSpace(line)
	if line == "" {
		return SerialLogEntry{
			Timestamp: time.Now(),
			Message:   line,
		}
	}

	return SerialLogEntry{
		Timestamp: time.Now(),
		Message:   line,
		Level:     m.parseLogLevel(line),
		Source:    "serial",
	}
}

// parseLogLevel detects the log level from a log line.
// It looks for common patterns like "E:", "ERROR", "WARN", etc.
//
// Returns the detected log level or empty string if not detected.
func (m *Monitor) parseLogLevel(line string) string {
	lower := strings.ToLower(line)

	// Error patterns
	if strings.HasPrefix(lower, "e:") ||
		strings.HasPrefix(lower, "err:") ||
		strings.HasPrefix(lower, "error:") ||
		strings.Contains(lower, "[error]") ||
		strings.Contains(lower, "(err)") {
		return "error"
	}

	// Warning patterns
	if strings.HasPrefix(lower, "w:") ||
		strings.HasPrefix(lower, "warn:") ||
		strings.HasPrefix(lower, "warning:") ||
		strings.Contains(lower, "[warn]") ||
		strings.Contains(lower, "(wrn)") {
		return "warn"
	}

	// Info patterns
	if strings.HasPrefix(lower, "i:") ||
		strings.HasPrefix(lower, "info:") ||
		strings.Contains(lower, "[info]") ||
		strings.Contains(lower, "(inf)") {
		return "info"
	}

	// Debug patterns
	if strings.HasPrefix(lower, "d:") ||
		strings.HasPrefix(lower, "debug:") ||
		strings.HasPrefix(lower, "dbg:") ||
		strings.Contains(lower, "[debug]") ||
		strings.Contains(lower, "(dbg)") {
		return "debug"
	}

	// Verbose patterns
	if strings.HasPrefix(lower, "v:") ||
		strings.HasPrefix(lower, "verbose:") ||
		strings.Contains(lower, "[verbose]") ||
		strings.Contains(lower, "(vb)") {
		return "verbose"
	}

	return ""
}

// detectBootComplete checks if a line indicates successful boot completion.
// This is useful for determining when a device has fully started.
//
// Boot detection patterns:
//   - "rst:" - ESP32 reset/boot message
//   - "Boot complete" - Explicit boot complete message
//   - "entry" - Application entry point
//   - "main" - Main function execution
//
// Returns true if the line indicates boot completion.
func (m *Monitor) detectBootComplete(line string) bool {
	lower := strings.ToLower(line)

	bootPatterns := []string{
		"boot complete",
		"boot succeeded",
		"boot finished",
		"entry 0x",
		"entry function",
		"app started",
		"application started",
		"application running",
		"main task",
		"running main",
		"cpu started",
		"cpu up",
		"heap",
		"ready",
		"pro cpu",
	}

	// Check for explicit patterns
	for _, pattern := range bootPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	// ESP32 reset/boot patterns
	if strings.HasPrefix(lower, "rst:") ||
		strings.Contains(lower, "boot mode") ||
		strings.Contains(lower, "pro cpu up") ||
		strings.Contains(lower, "app cpu up") {
		return true
	}

	// ESP-IDF patterns
	if strings.Contains(lower, "i (") &&
		(strings.Contains(lower, "cpu start") ||
			strings.Contains(lower, "heap cap") ||
			strings.Contains(lower, "wifi init")) {
		return true
	}

	return false
}

// CaptureUntilBoot captures serial output until boot is detected or timeout.
// This is a convenience method for common use cases where you want to monitor
// until the device has fully booted.
//
// Parameters:
//   - ctx: Context for cancellation
//   - timeout: Maximum time to wait for boot completion
//
// Returns captured entries and true if boot was detected.
func (m *Monitor) CaptureUntilBoot(ctx context.Context, timeout time.Duration) ([]SerialLogEntry, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	entryCh := make(chan SerialLogEntry, 100)
	errorCh := make(chan error, 1)
	bootCh := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer close(entryCh)

		m.readLoopUntilBoot(ctx, entryCh, errorCh, bootCh)
	}()

	var entries []SerialLogEntry
	doneCh := make(chan struct{})
	go func() {
		for entry := range entryCh {
			entries = append(entries, entry)
		}
		close(doneCh)
	}()

	select {
	case <-bootCh:
		wg.Wait()
		<-doneCh
		return entries, true, nil

	case <-doneCh:
		wg.Wait()
		return entries, false, nil

	case err := <-errorCh:
		wg.Wait()
		return entries, false, err

	case <-ctx.Done():
		wg.Wait()
		<-doneCh
		return entries, false, ctx.Err()
	}
}

// readLoopUntilBoot reads serial output until boot is detected.
// Similar to readLoop but checks each line for boot completion.
func (m *Monitor) readLoopUntilBoot(ctx context.Context, entryCh chan<- SerialLogEntry, errorCh chan<- error, bootCh chan<- struct{}) {
	lineBuf := make([]byte, 0, 256)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-tick.C:
			m.mu.Lock()
			if m.portImpl == nil {
				m.mu.Unlock()
				errorCh <- fmt.Errorf("port closed")
				return
			}

			n, err := m.portImpl.Read(m.buffer)
			m.mu.Unlock()

			if err != nil {
				if strings.Contains(err.Error(), "timeout") {
					continue
				}
				errorCh <- fmt.Errorf("read error: %w", err)
				return
			}

			if n == 0 {
				continue
			}

			data := m.buffer[:n]
			for _, b := range data {
				if b == '\n' {
					if len(lineBuf) > 0 {
						line := string(lineBuf)
						entry := m.parseLine(line)
						entryCh <- entry

						if m.detectBootComplete(line) {
							close(bootCh)
							return
						}

						lineBuf = lineBuf[:0]
					}
				} else if b != '\r' {
					lineBuf = append(lineBuf, b)
				}
			}
		}
	}
}

// IsBootComplete checks if any of the captured entries indicates boot completion.
// This is useful for post-capture analysis.
func (m *Monitor) IsBootComplete(entries []SerialLogEntry) bool {
	for _, entry := range entries {
		if m.detectBootComplete(entry.Message) {
			return true
		}
	}
	return false
}

// GetEntriesByLevel filters log entries by log level.
// Returns entries matching the specified level (case-insensitive).
func (m *Monitor) GetEntriesByLevel(entries []SerialLogEntry, level string) []SerialLogEntry {
	var result []SerialLogEntry
	levelLower := strings.ToLower(level)

	for _, entry := range entries {
		if strings.ToLower(entry.Level) == levelLower {
			result = append(result, entry)
		}
	}

	return result
}

// GetPort returns the configured serial port path.
func (m *Monitor) GetPort() string {
	return m.port
}

// GetBaudRate returns the configured baud rate.
func (m *Monitor) GetBaudRate() int {
	return m.baudRate
}

// IsOpen returns true if the serial port is currently open.
func (m *Monitor) IsOpen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.portImpl != nil
}
