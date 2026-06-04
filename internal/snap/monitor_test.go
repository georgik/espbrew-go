package snap

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"go.bug.st/serial"
)

// ============================================================================
// Mock Serial Port Implementation
// ============================================================================

// mockPort is a mock implementation of serial.Port for testing.
type mockPort struct {
	mu          sync.Mutex
	readData    [][]byte      // Data chunks to return from Read()
	readIndex   int           // Current position in readData
	closed      bool          // Track if port is closed
	readTimeout time.Duration // Configured read timeout
	dtr         bool          // DTR state
	rts         bool          // RTS state
	writeData   [][]byte      // Data written to the port
	delay       time.Duration // Artificial delay for reads
}

func newMockPort(data ...string) *mockPort {
	mp := &mockPort{
		readData: make([][]byte, 0),
	}
	for _, d := range data {
		mp.readData = append(mp.readData, []byte(d))
	}
	return mp
}

func (m *mockPort) Read(p []byte) (n int, err error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, nil
	}

	if m.readIndex >= len(m.readData) {
		// Simulate timeout when no more data
		if m.readTimeout > 0 {
			return 0, &mockTimeoutError{}
		}
		return 0, nil
	}

	data := m.readData[m.readIndex]
	m.readIndex++

	if len(data) > len(p) {
		copy(p, data[:len(p)])
		return len(p), nil
	}

	copy(p, data)
	return len(data), nil
}

func (m *mockPort) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data := make([]byte, len(p))
	copy(data, p)
	m.writeData = append(m.writeData, data)
	return len(p), nil
}

func (m *mockPort) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockPort) SetMode(mode *serial.Mode) error {
	return nil
}

func (m *mockPort) Drain() error {
	return nil
}

func (m *mockPort) ResetInputBuffer() error {
	return nil
}

func (m *mockPort) ResetOutputBuffer() error {
	return nil
}

func (m *mockPort) SetDTR(dtr bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dtr = dtr
	return nil
}

func (m *mockPort) SetRTS(rts bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rts = rts
	return nil
}

func (m *mockPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}

func (m *mockPort) SetReadTimeout(t time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readTimeout = t
	return nil
}

func (m *mockPort) Break(d time.Duration) error {
	return nil
}

// mockTimeoutError simulates a serial timeout error.
type mockTimeoutError struct{}

func (e *mockTimeoutError) Error() string {
	return "timeout"
}

func (e *mockTimeoutError) Timeout() bool {
	return true
}

// Helper to inject mock port into Monitor
func (m *Monitor) setMockPort(port *mockPort) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.portImpl = port
}

// ============================================================================
// TestMonitor_CaptureWithContext
// ============================================================================

func TestMonitor_CaptureWithContext(t *testing.T) {
	tests := []struct {
		name          string
		data          []string
		cancelAfter   time.Duration
		expectedCount int
		expectError   bool
	}{
		{
			name: "capture before cancel",
			data: []string{
				"I: System starting\n",
				"I: Boot mode: 1\n",
				"I: CPU up\n",
			},
			cancelAfter:   50 * time.Millisecond,
			expectedCount: 3,
			expectError:   true, // context cancelled
		},
		{
			name: "empty capture",
			data: []string{
				"",
			},
			cancelAfter:   50 * time.Millisecond,
			expectedCount: 0,
			expectError:   true, // context cancelled
		},
		{
			name: "capture with mixed content",
			data: []string{
				"E: Error occurred\n",
				"W: Warning message\n",
				"I: Info message\n",
				"D: Debug message\n",
			},
			cancelAfter:   50 * time.Millisecond,
			expectedCount: 4,
			expectError:   true, // context cancelled
		},
		{
			name: "context cancelled immediately",
			data: []string{
				"I: System starting\n",
			},
			cancelAfter:   1 * time.Millisecond,
			expectedCount: 0,
			expectError:   true, // context.Canceled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockPort(tt.data...)
			mock.delay = 10 * time.Millisecond

			monitor := NewMonitor("/dev/ttyUSB0", 115200, 100*time.Millisecond)
			monitor.setMockPort(mock)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Cancel after specified duration
			go func() {
				time.Sleep(tt.cancelAfter)
				cancel()
			}()

			entries, err := monitor.Capture(ctx, 0)

			if tt.expectError {
				assert.Error(t, err)
			}
			// Verify entries were captured even if context was cancelled
			assert.Len(t, entries, tt.expectedCount)
		})
	}
}

// ============================================================================
// TestMonitor_CaptureWithTimeout
// ============================================================================

func TestMonitor_CaptureWithTimeout(t *testing.T) {
	tests := []struct {
		name          string
		data          []string
		duration      time.Duration
		expectedCount int
		expectError   bool
	}{
		{
			name: "capture within timeout",
			data: []string{
				"I: Line 1\n",
				"I: Line 2\n",
				"I: Line 3\n",
			},
			duration:      100 * time.Millisecond,
			expectedCount: 3,
			expectError:   false,
		},
		{
			name:     "timeout occurs",
			data:     []string{},
			duration: 50 * time.Millisecond,
			// No data within timeout
			expectedCount: 0,
			expectError:   false,
		},
		{
			name: "short timeout with data",
			data: []string{
				"I: Quick message\n",
			},
			duration:      25 * time.Millisecond,
			expectedCount: 1,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockPort(tt.data...)
			mock.delay = 5 * time.Millisecond

			monitor := NewMonitor("/dev/ttyUSB0", 115200, 100*time.Millisecond)
			monitor.setMockPort(mock)

			ctx := context.Background()
			entries, err := monitor.Capture(ctx, tt.duration)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Len(t, entries, tt.expectedCount)
		})
	}
}

// ============================================================================
// TestMonitor_BootPatternDetection
// ============================================================================

func TestMonitor_BootPatternDetection(t *testing.T) {
	tests := []struct {
		name         string
		lines        []string
		shouldDetect bool
	}{
		{
			name: "ESP32 boot message",
			lines: []string{
				"rst:0x1 (POWERON_RESET),boot:0x13 (SPI_FAST_FLASH_BOOT)",
			},
			shouldDetect: true,
		},
		{
			name: "Boot complete message",
			lines: []string{
				"I: Boot complete, starting application",
			},
			shouldDetect: true,
		},
		{
			name: "App started message",
			lines: []string{
				"I: Application started",
			},
			shouldDetect: true,
		},
		{
			name: "Ready message",
			lines: []string{
				"I: System ready",
			},
			shouldDetect: true,
		},
		{
			name: "Heap message (ESP-IDF)",
			lines: []string{
				"I (123) main: Heap: 12345 bytes",
			},
			shouldDetect: true,
		},
		{
			name: "Pro CPU message",
			lines: []string{
				"Pro CPU up",
			},
			shouldDetect: true,
		},
		{
			name: "Non-boot message",
			lines: []string{
				"I: Some random log",
			},
			shouldDetect: false,
		},
		{
			name: "Empty line",
			lines: []string{
				"",
			},
			shouldDetect: false,
		},
		{
			name: "Multiple boot patterns",
			lines: []string{
				"rst:0x1 (POWERON_RESET)",
				"Boot mode: 1",
				"I: System ready",
			},
			shouldDetect: true, // All lines should be detected as boot
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := NewMonitor("/dev/ttyUSB0", 115200, 100*time.Millisecond)

			for _, line := range tt.lines {
				detected := monitor.detectBootComplete(line)
				if tt.shouldDetect {
					assert.True(t, detected, "Line should be detected as boot complete: %s", line)
				} else {
					assert.False(t, detected, "Line should NOT be detected as boot complete: %s", line)
				}
			}
		})
	}
}

// ============================================================================
// TestMonitor_LogLevelParsing
// ============================================================================

func TestMonitor_LogLevelParsing(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		// Error patterns
		{
			name:     "error colon prefix",
			line:     "E: Failed to initialize",
			expected: "error",
		},
		{
			name:     "err colon prefix",
			line:     "err: Something went wrong",
			expected: "error",
		},
		{
			name:     "error word prefix",
			line:     "error: Critical failure",
			expected: "error",
		},
		{
			name:     "error bracket",
			line:     "[ERROR] System halted",
			expected: "error",
		},
		{
			name:     "error parens",
			line:     "(ERR) Invalid parameter",
			expected: "error",
		},
		// Warning patterns
		{
			name:     "warn colon prefix",
			line:     "W: Deprecated feature",
			expected: "warn",
		},
		{
			name:     "warning word prefix",
			line:     "warning: Low memory",
			expected: "warn",
		},
		{
			name:     "warn bracket",
			line:     "[WARN] Retrying operation",
			expected: "warn",
		},
		{
			name:     "wrn parens",
			line:     "(WRN) Connection unstable",
			expected: "warn",
		},
		// Info patterns
		{
			name:     "info colon prefix",
			line:     "I: System started",
			expected: "info",
		},
		{
			name:     "info word prefix",
			line:     "info: Processing data",
			expected: "info",
		},
		{
			name:     "info bracket",
			line:     "[INFO] User logged in",
			expected: "info",
		},
		{
			name:     "inf parens",
			line:     "(INF) Task completed",
			expected: "info",
		},
		// Debug patterns
		{
			name:     "debug colon prefix",
			line:     "D: Variable value",
			expected: "debug",
		},
		{
			name:     "debug word prefix",
			line:     "debug: Entering loop",
			expected: "debug",
		},
		{
			name:     "dbg prefix",
			line:     "dbg: Memory address",
			expected: "debug",
		},
		{
			name:     "debug bracket",
			line:     "[DEBUG] Function call",
			expected: "debug",
		},
		{
			name:     "dbg parens",
			line:     "(DBG) Stack trace",
			expected: "debug",
		},
		// Verbose patterns
		{
			name:     "verbose colon prefix",
			line:     "V: Detailed trace",
			expected: "verbose",
		},
		{
			name:     "verbose word prefix",
			line:     "verbose: Step 1",
			expected: "verbose",
		},
		{
			name:     "verbose bracket",
			line:     "[VERBOSE] Processing item 1",
			expected: "verbose",
		},
		{
			name:     "vb parens",
			line:     "(VB) Iteration 10",
			expected: "verbose",
		},
		// No log level
		{
			name:     "plain message",
			line:     "Some random text without level",
			expected: "",
		},
		{
			name:     "empty line",
			line:     "",
			expected: "",
		},
		{
			name:     "whitespace only",
			line:     "   ",
			expected: "",
		},
		// Case insensitivity
		{
			name:     "uppercase ERROR",
			line:     "ERROR: Critical failure",
			expected: "error",
		},
		{
			name:     "mixed case Error",
			line:     "Error: Something happened",
			expected: "error",
		},
		{
			name:     "uppercase WARNING",
			line:     "WARNING: Check configuration",
			expected: "warn",
		},
		// Log level in middle
		{
			name:     "error in brackets middle",
			line:     "Task failed: [ERROR] timeout",
			expected: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := NewMonitor("/dev/ttyUSB0", 115200, 100*time.Millisecond)
			result := monitor.parseLogLevel(tt.line)

			assert.Equal(t, tt.expected, result, "Log level mismatch for line: %s", tt.line)
		})
	}
}
