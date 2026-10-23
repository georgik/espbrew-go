package espflash

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"go.bug.st/serial"
)

// captureLogger records Logf output so tests can assert on probeBootLog's
// verdict without a real logger attached.
type captureLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (c *captureLogger) Logf(format string, args ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, fmt.Sprintf(format, args...))
}

func (c *captureLogger) has(sub string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.msgs {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}

// bootLogPort is a serial.Port whose Read returns a fixed boot-log string once,
// then io.EOF so readBootLog stops. It lets probeBootLog be tested without a
// real chip.
type bootLogPort struct {
	log   string
	mu    sync.Mutex
	reads int
}

func (b *bootLogPort) SetDTR(bool) error                   { return nil }
func (b *bootLogPort) SetRTS(bool) error                   { return nil }
func (b *bootLogPort) Write(p []byte) (int, error)         { return len(p), nil }
func (b *bootLogPort) SetMode(mode *serial.Mode) error     { return nil }
func (b *bootLogPort) SetReadTimeout(time.Duration) error  { return nil }
func (b *bootLogPort) SetWriteTimeout(time.Duration) error { return nil }
func (b *bootLogPort) Close() error                        { return nil }
func (b *bootLogPort) ResetInputBuffer() error             { return nil }
func (b *bootLogPort) ResetOutputBuffer() error            { return nil }
func (b *bootLogPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return nil, nil
}
func (b *bootLogPort) Break(time.Duration) error { return nil }
func (b *bootLogPort) Drain() error              { return nil }

func (b *bootLogPort) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reads++
	if b.reads == 1 && b.log != "" {
		return copy(p, []byte(b.log)), nil
	}
	return 0, io.EOF
}

func newProbeFlasher(log string) (*Flasher, *captureLogger) {
	p := &bootLogPort{log: log}
	cl := &captureLogger{}
	return &Flasher{port: p, opts: &FlasherOptions{Logger: cl}}, cl
}

// TestProbeBootLogConfirmsDownload checks that the ROM's "waiting for download"
// banner (capitalized, as the real bootloader prints it) is recognised.
func TestProbeBootLogConfirmsDownload(t *testing.T) {
	f, cl := newProbeFlasher("boot: Waiting for download mode (0x00000000)\n")
	f.probeBootLog()
	if !cl.has("download mode") {
		t.Fatalf("expected download-mode confirmation, got: %v", cl.msgs)
	}
}

// TestProbeBootLogAppRunning checks that a normal app boot log (no download
// banner) is reported as "still running the app" so a marginal reset is visible.
func TestProbeBootLogAppRunning(t *testing.T) {
	f, cl := newProbeFlasher("boot: Chip: ESP32-S3\nRestart reason...\nRunning app\n")
	f.probeBootLog()
	if cl.has("download mode") {
		t.Fatal("app boot log must not confirm download mode")
	}
	if !cl.has("still be running the app") {
		t.Fatalf("expected the app-running warning, got: %v", cl.msgs)
	}
}

// TestProbeBootLogEmpty checks the no-output case is reported plainly.
func TestProbeBootLogEmpty(t *testing.T) {
	f, cl := newProbeFlasher("")
	f.probeBootLog()
	if !cl.has("boot log empty") {
		t.Fatalf("expected empty-boot-log report, got: %v", cl.msgs)
	}
}
