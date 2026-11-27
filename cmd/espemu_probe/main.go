// Command espemu_probe drives an emulated ESP32-C3 (via esp-emu's USB-Serial/JTAG
// socket) using the REAL espflash-go ROM-bootloader protocol, so we can observe
// whether the emulated C3 ROM hangs on the first FLASH_DATA exactly like real HW.
//
// Usage: espemu_probe <socket-host:port> <image-file> [read-bytes]
//
// It connects, flashes the image at offset 0, reads it back, and compares.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/georgik/espbrew-go/internal/espflash"
	"go.bug.st/serial"
)

// tcpPort implements serial.Port over a raw TCP connection. It is only used to
// reach an emulated target (esp-emu) whose download console is bridged to TCP;
// it carries no real serial semantics, which is fine because the target is
// already in download mode (ResetNoReset) and has no modem-control lines.
type tcpPort struct {
	conn    net.Conn
	timeout time.Duration
	logf    func(string, ...interface{})
}

func (p *tcpPort) SetMode(mode *serial.Mode) error { return nil }
func (p *tcpPort) Write(b []byte) (int, error) {
	if p.logf != nil {
		p.logf("tx %d B", len(b))
	}
	return p.conn.Write(b)
}
func (p *tcpPort) Read(b []byte) (int, error) {
	if p.timeout > 0 {
		_ = p.conn.SetReadDeadline(time.Now().Add(p.timeout))
	} else {
		_ = p.conn.SetReadDeadline(time.Time{})
	}
	n, err := p.conn.Read(b)
	if err != nil && p.logf != nil {
		p.logf("rx err: %v", err)
	}
	return n, err
}
func (p *tcpPort) Drain() error             { return nil }
func (p *tcpPort) ResetInputBuffer() error  { return nil }
func (p *tcpPort) ResetOutputBuffer() error { return nil }
func (p *tcpPort) SetDTR(bool) error        { return nil }
func (p *tcpPort) SetRTS(bool) error        { return nil }
func (p *tcpPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}
func (p *tcpPort) SetReadTimeout(t time.Duration) error { p.timeout = t; return nil }
func (p *tcpPort) Close() error                         { return p.conn.Close() }
func (p *tcpPort) Break(time.Duration) error            { return nil }

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: espemu_probe <host:port> <image-file> [read-bytes]")
		os.Exit(2)
	}
	addr := os.Args[1]
	imagePath := os.Args[2]
	readBytes := 128 * 1024
	if len(os.Args) >= 4 {
		fmt.Sscanf(os.Args[3], "%d", &readBytes)
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read image: %v\n", err)
		os.Exit(1)
	}

	raw, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer raw.Close()
	p := &tcpPort{conn: raw, logf: func(f string, a ...interface{}) {
		fmt.Printf("[c3] "+f+"\n", a...)
	}}

	opts := espflash.DefaultOptions()
	opts.ChipType = espflash.ChipESP32C3
	opts.ResetMode = espflash.ResetNoReset // target is already in download mode
	opts.FlashSize = "2MB"                 // avoid header-based auto-detect
	opts.FastMode = false
	opts.ConnectAttempts = 3
	// Uncompressed ROM path (FlashBegin/FlashData/FlashEnd 0x02/0x03/0x04) —
	// mirrors our flasher.rs, which sends raw 512B blocks with no stub.
	if os.Getenv("ESPEMU_NOSTUB") != "" {
		opts.SkipStub = true
		opts.Compress = false
		fmt.Println("[c3] mode: UNCOMPRESSED ROM path (SkipStub) — mirrors flasher.rs")
	} else {
		fmt.Println("[c3] mode: stub + zlib compression (default espflash-go)")
	}
	opts.Logger = logAdapter{out: os.Stdout, prefix: "[flash]"}

	f, err := espflash.NewWithPort(context.Background(), p, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	fmt.Printf("[c3] connected chip=%s\n", f.ChipName())

	// Flash the image at offset 0. If the emulated ROM hangs on the first
	// FLASH_DATA here, it reproduces the real-HW WRITE_TIMEOUT.
	fmt.Printf("[c3] flashing %d bytes at 0x0\n", len(data))
	if err := f.FlashImage(data, 0, func(cur, tot int) {
		fmt.Printf("[c3]  progress %d/%d\n", cur, tot)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "FLASH_IMAGE FAILED (hang/timeout on emulated C3 ROM):\n  %v\n", err)
		os.Exit(3)
	}
	fmt.Println("[c3] flash write+verify OK")

	// Read back and compare.
	back, err := f.ReadFlash(0, uint32(readBytes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read-flash: %v\n", err)
		os.Exit(1)
	}
	if len(back) != len(data) || !equalBytes(back, data) {
		fmt.Fprintf(os.Stderr, "MISMATCH: read back %d bytes vs %d written\n", len(back), len(data))
		os.Exit(4)
	}
	fmt.Printf("[c3] read-back BYTE-IDENTICAL (%d bytes)\n", len(back))
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// logAdapter routes espflash-go's Logger to stdout.
type logAdapter struct {
	out    io.Writer
	prefix string
}

func (l logAdapter) Logf(format string, a ...interface{}) {
	fmt.Fprintf(l.out, "%s "+format+"\n", append([]interface{}{l.prefix}, a...)...)
}
