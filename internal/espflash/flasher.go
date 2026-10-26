package espflash

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/georgik/espbrew-go/internal/serialheal"
	"github.com/rs/zerolog/log"
	"go.bug.st/serial"
)

// maxFlashBlockAttempts is the number of resend attempts for a single flash
// block when the chip does not ACK (timeout or a ROM error such as 0xC0).
// esptool uses 3; we use 5 because a UART-bridge board (CH340/CP210x) with a
// small RX FIFO can lose a block to overflow, and a few extra attempts let the
// FIFO drain and the resend succeed without failing the whole download.
const maxFlashBlockAttempts = 5

// flashBlockRetryDelay is the pause between resend attempts. It gives the UART
// bridge's RX FIFO time to drain after an overflow before we resend the block.
const flashBlockRetryDelay = 100 * time.Millisecond

// maxFlashPartAttempts is the number of times to re-flash a whole part (e.g. the
// app) if it fails. Each attempt re-issues FLASH_DEFL_BEGIN and starts the
// download fresh, which recovers from the cascading RX-FIFO overflow that can
// strand a single block on a UART-bridge board at high flash bauds.
const maxFlashPartAttempts = 3

// maxReconnectAttempts is the number of reset+reconnect cycles reconnect()
// runs before giving up. It is intentionally 1: a UART-bridge board that has
// cascaded into a stuck-SPI state cannot be recovered by a DTR/RTS reset
// alone (only a fresh client-side connect clears it), so trying more cycles
// only burns seconds before the flash fails and the client retries the whole
// command. See reconnect().
const maxReconnectAttempts = 1

// reconnectSettleDelay gives the chip time out of its overflow lockup (and lets
// the USB/UART link settle) before we retry the reset+sync+attach.
const reconnectSettleDelay = 200 * time.Millisecond

// FlasherOptions configures the Flasher behavior.
type FlasherOptions struct {
	// BaudRate is the initial baud rate for serial communication.
	// Default: 115200.
	BaudRate int

	// FlashBaudRate is the baud rate used during flash data transfer.
	// If set and higher than BaudRate, the flasher will switch to this rate
	// after connecting. Set to 0 to keep the initial baud rate.
	// Default: 460800.
	FlashBaudRate int

	// ChipType forces a specific chip type instead of auto-detection.
	// Default: ChipAuto (auto-detect).
	ChipType ChipType

	// ResetMode controls how the chip is reset to enter bootloader.
	// Default: ResetDefault.
	ResetMode ResetMode

	// ResetBridge forces the DTR/RTS bootloader reset used for USB-UART-bridge
	// boards (CH340, CH9102, CP210x, FTDI). Such boards have NO native
	// USB-JTAG peripheral — the USB endpoint is a UART bridge — so the
	// USB-JTAG reset path is dead weight and its reopenPort() is disruptive on
	// a device that never disconnects. When true, connect() uses only the
	// atomic DTR/RTS reset (matching esptool.py's bridge reset strategy) and
	// verifies the chip entered download mode via its boot log. The flash
	// layer sets this automatically once it detects a bridge board by its USB
	// vendor id (see internal/flash). Default: false.
	ResetBridge bool

	// ConnectAttempts is the number of connection attempts before failing.
	// Default: 7, use 2 for fast mode.
	ConnectAttempts int

	// FastMode enables optimized connection for rapid development iteration.
	// Reduces connection attempts and delays for sub-second sync.
	// Default: true.
	FastMode bool

	// Compress enables zlib compression for flash data transfer.
	// Significantly faster for large images. Requires stub loader or
	// ESP32+ ROM bootloader.
	// Default: true.
	Compress bool

	// SkipStub disables loading the RAM flash stub after connecting. Without
	// the stub the ROM bootloader handles the flash directly using the
	// uncompressed FlashBegin/FlashData/FlashEnd (0x02/0x03/0x04) sequence.
	// This mirrors espflash's --no-stub mode, which is the path that works on
	// the ESP32-P4 (the compressed stub path stalls at FLASH_DEFL_END).
	// Default: false (load the stub).
	SkipStub bool

	// FlashMode sets the SPI flash access mode in the image header.
	// Valid values: "qio", "qout", "dio", "dout".
	// Empty string or "keep" preserves the value from the binary.
	// Most ESP32 boards work with "dio"; some need "dout".
	FlashMode string

	// FlashFreq sets the SPI flash clock frequency in the image header.
	// Valid values are chip-specific, e.g. "80m", "40m", "26m", "20m".
	// Empty string or "keep" preserves the value from the binary.
	FlashFreq string

	// FlashSize sets the flash chip size in the image header.
	// Valid values: "1MB", "2MB", "4MB", "8MB", "16MB", etc.
	// Empty string or "keep" preserves the value from the binary.
	FlashSize string

	// Erase enables automatic erase before flashing.
	// When false, skips erase in flashBegin for faster development iteration.
	// Default: false (erase disabled).
	Erase bool

	// SkipUnchanged skips flashing segments that haven't changed.
	// Requires stub loader (for FlashMd5 command).
	// Provides significant speedup for iterative development.
	// Default: false.
	SkipUnchanged bool

	// Logger receives informational messages during flashing.
	// If nil, messages are discarded silently.
	Logger Logger
}

// Logger is the interface for receiving progress and status messages.
type Logger interface {
	// Logf logs a formatted informational message.
	Logf(format string, args ...interface{})
}

// ProgressFunc is called with progress updates during flashing.
// current is the bytes transferred so far, total is the total bytes.
type ProgressFunc func(current, total int)

// DefaultOptions returns FlasherOptions with sensible defaults.
// LastConnectAttempts records, in call order, the 0-based connect() attempt
// index on which each successful bootloader sync finally succeeded. It is a
// measurement hook for the reset-reliability sweep and is NOT part of the
// public flashing API. It is written synchronously from connect() on the
// goroutine that performs the connect, so it must only be read from a
// single-threaded (non-parallel) test.
var LastConnectAttempts []int

func DefaultOptions() *FlasherOptions {
	return &FlasherOptions{
		BaudRate:        115200,
		FlashBaudRate:   460800,
		ChipType:        ChipAuto,
		ResetMode:       ResetAuto, // Try USB-JTAG, classic, and no-reset strategies
		ConnectAttempts: 9,         // More attempts for USB-JTAG reset (every 3rd attempt)
		FastMode:        true,
		Compress:        true,
		Erase:           false,
		SkipUnchanged:   false,
	}
}

// connection defines the low-level protocol operations for communicating
// with an ESP bootloader over a serial connection.
type connection interface {
	sync() (uint32, error)
	readReg(addr uint32) (uint32, error)
	writeReg(addr, value, mask, delayUS uint32) error
	securityInfo() ([]byte, error)
	flashBegin(size, offset uint32, encrypted bool, erase bool) error
	flashData(block []byte, seq uint32) error
	flashEnd(reboot bool) error
	flashDeflBegin(uncompSize, compSize, offset uint32, encrypted bool, erase bool) error
	flashDeflData(block []byte, seq uint32) error
	flashDeflEnd(reboot bool) error
	flashMD5(addr, size uint32) ([]byte, error)
	flashWriteSize() uint32
	spiAttach(value uint32) error
	spiSetParams(totalSize, blockSize, sectorSize, pageSize uint32) error
	changeBaud(newBaud, oldBaud uint32) error
	eraseFlash() error
	eraseRegion(offset, size uint32) error
	readFlash(offset, size uint32) ([]byte, error)
	flushInput()
	isStub() bool
	setUSB(v bool)
	setSupportsEncryptedFlash(v bool)
	loadStub(s *stub) error
}

// Flasher manages the connection to an ESP device and provides
// high-level flash operations.
//
// Context lifetime: The flasher creates a child context from the provided
// parent context to manage cancellation during flash operations. This child
// context must live for the entire flasher lifetime, not just during New().
// Previously, a defer cancel() in New() caused the context to be cancelled
// immediately after connection, leading to "context canceled" errors in
// subsequent operations like FlashImages and second attachFlash calls.
type Flasher struct {
	port    serial.Port
	conn    connection
	chip    *chipDef
	opts    *FlasherOptions
	portStr string
	usesUSB bool
	// revision is the detected chip revision (e.g. 301 for P4 rev 3.1). It is
	// filled in by the chip's PostConnect hook and used to pick revision-specific
	// behaviour such as the correct stub flasher. Zero means "unknown/not set".
	revision int
	// currentBaud is the UART baud rate the chip is ACTUALLY running at. It
	// starts at the connect baud and is updated on every successful changeBaud.
	// changeBaud passes this as the old-baud argument to the protocol so the
	// stub loader can compute the new UART divisor correctly. It must reflect
	// the live baud, not just the initial connect baud: after the first
	// changeBaud(921600) the chip is at 921600, and passing a stale 115200 as
	// the old baud makes the stub reject the command, which then makes the
	// next FLASH_DEFL_BEGIN time out.
	currentBaud int
	secInfo     []byte // cached security info from ROM (GET_SECURITY_INFO opcode 0x14)
	ctx         context.Context
	cancel      context.CancelFunc
	closed      bool
	// LastConnectAttempt records the 0-based connect() attempt index on which
	// the bootloader sync finally succeeded. It is a measurement hook: 0 means
	// the FIRST reset strategy caught the download window, >0 means a retry was
	// needed. Used by the reset-reliability sweep to tell a working first-attempt
	// reset from one that only works because a later attempt falls through.
	LastConnectAttempt int
}

// openPortWithRetries opens a serial port, retrying while the port reports a
// transient "busy" condition (EBUSY). That happens when another caller is still
// holding the port (e.g. the boot-log probe) or when a USB CDC device
// momentarily detaches after a reset and re-enumerates. Permission, no-such
// device and similar errors must fail immediately, so only busy conditions are
// retried. The context bounds the retry so a cancelled/expired job stops.
func openPortWithRetries(ctx context.Context, portName string, mode *serial.Mode) (serial.Port, error) {
	const (
		maxRetries = 8
		retryDelay = 500 * time.Millisecond
	)
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		port, err := serialheal.Open(portName, mode)
		if err == nil {
			return port, nil
		}
		lastErr = err
		if !isBusyError(err) {
			return nil, err
		}
		log.Debug().Str("path", portName).Err(err).Msg("Port busy, retrying open")
		time.Sleep(retryDelay)
	}
	return nil, fmt.Errorf("port still busy after %d retries: %w", maxRetries, lastErr)
}

// isBusyError reports whether err is a transient "resource busy" / device
// momentarily-unavailable condition that a retry should wait out.
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	// go.bug.st/serial surfaces a busy port as a *serial.PortError with
	// code==PortBusy (its message is "Serial port busy", not the raw errno).
	var portErr *serial.PortError
	if errors.As(err, &portErr) {
		return portErr.Code() == serial.PortBusy
	}
	if errors.Is(err, syscall.EBUSY) {
		return true
	}
	return strings.Contains(err.Error(), "Serial port busy")
}

// New creates a new Flasher connected to the given serial port.
//
// It opens the serial port, enters the bootloader, syncs with the device,
// and detects the chip type. On success, the Flasher is ready for flash
// operations.
//
// If opts is nil, DefaultOptions() is used.
//
// The context can be used to cancel the connection attempt during
// bootloader sync and chip detection.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//	f, err := espflasher.New(ctx, "/dev/ttyUSB0", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer f.Close()
func New(ctx context.Context, portName string, opts *FlasherOptions) (*Flasher, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	if ctx == nil {
		ctx = context.Background()
	}
	// Create child context for this flasher instance
	// Store cancel for cleanup when flasher is closed
	childCtx, cancel := context.WithCancel(ctx)

	mode := &serial.Mode{
		BaudRate: opts.BaudRate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	port, err := openPortWithRetries(ctx, portName, mode)
	if err != nil {
		cancel() // Clean up child context
		return nil, fmt.Errorf("open serial port %s: %w", portName, err)
	}

	f := &Flasher{
		port:        port,
		conn:        newConn(port, childCtx),
		opts:        opts,
		portStr:     portName,
		currentBaud: opts.BaudRate,
		ctx:         childCtx,
		cancel:      cancel,
		closed:      false,
	}

	// Connect to the bootloader
	if err := f.connect(); err != nil {
		f.port.Close() //nolint:errcheck
		return nil, err
	}

	return f, nil
}

// Close releases the serial port and associated resources.
func (f *Flasher) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	if f.cancel != nil {
		f.cancel()
	}
	return f.port.Close()
}

// FlushInput discards any unread data from the serial port and SLIP reader.
// Useful when reusing a flasher connection across multiple operations.
func (f *Flasher) FlushInput() {
	f.conn.flushInput()
}

// reopenPort closes and reopens the serial port after a USB device
// re-enumeration. TinyUSB CDC devices may briefly disappear during reset.
func (f *Flasher) reopenPort() error {
	f.port.Close() //nolint:errcheck

	var lastErr error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		// Check for context cancellation
		select {
		case <-f.ctx.Done():
			return fmt.Errorf("port reopen cancelled: %w", f.ctx.Err())
		default:
		}

		time.Sleep(500 * time.Millisecond)
		port, err := serialheal.Open(f.portStr, &serial.Mode{
			BaudRate: f.opts.BaudRate,
			Parity:   serial.NoParity,
			DataBits: 8,
			StopBits: serial.OneStopBit,
		})
		if err == nil {
			f.port = port
			f.conn = newConn(port, f.ctx)
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("reopen port %s: %w", f.portStr, lastErr)
}

// connectViaUSBJTAG performs a USB-JTAG/Serial reset and attempts to
// reconnect. The reset causes the USB device to disconnect and
// re-enumerate, so the old port is closed and a new one is opened.
//
// The reset sequence is run in a goroutine because ioctls on a
// disconnected cdc_acm fd can block for several seconds on Linux.
// After a short timeout we close the old port (which unblocks the
// goroutine) and wait for the device to re-enumerate.
func (f *Flasher) connectViaUSBJTAG() bool {
	done := make(chan struct{})
	go func() {
		usbJTAGSerialReset(f.port)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		// Reset likely triggered but post-disconnect ioctls are blocking.
		// reopenPort will close the fd, unblocking the goroutine.
	}
	if err := f.reopenPort(); err != nil {
		return false
	}
	f.conn.flushInput()
	for range 5 {
		_, err := f.conn.sync()
		if err == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// ChipType returns the detected chip type.
func (f *Flasher) ChipType() ChipType {
	if f.chip != nil {
		return f.chip.ChipType
	}
	return ChipAuto
}

// ChipName returns the detected chip name (e.g. "ESP32-S3").
func (f *Flasher) ChipName() string {
	if f.chip != nil {
		return f.chip.Name
	}
	return "Unknown"
}

// BootloaderFlashOffset returns the flash offset where the bootloader image
// lives for the connected chip. Returns (0, false) if the chip has not been
// detected yet (e.g. connect() has not completed).
func (f *Flasher) BootloaderFlashOffset() (uint32, bool) {
	if f.chip == nil {
		return 0, false
	}
	return f.chip.BootloaderFlashOffset, true
}

// Revision returns the detected chip revision (e.g. 301 for ESP32-P4 rev 3.1).
// It is filled in by the chip's PostConnect hook and used to select
// revision-specific artifacts such as the P4 bootloader (v0 vs v3).
// Returns 0 if the chip has not been detected yet (e.g. connect() has not
// completed).
func (f *Flasher) Revision() int {
	return f.revision
}

// connect performs the bootloader connection sequence:
// reset → sync → detect chip.
// enterBootloaderReset drives the chip into the serial bootloader using the
// reset strategy appropriate for the requested chip. For the ESP32-P4 the
// classic DTR/RTS sequences latch the wrong IO0 polarity on the native
// USB-Serial-JTAG peripheral, so the chip boots the application instead of
// entering download mode. forceP4Download holds IO0 (DTR) LOW across the EN
// reset pulse, which reliably latches the P4 into the serial bootloader. See
// wiki/esp32-p4-flash-solution.md.
func (f *Flasher) enterBootloaderReset(port serial.Port, delay time.Duration) {
	if f.opts.ChipType == ChipESP32P4 {
		forceP4Download(port)
		return
	}
	unixTightReset(port, delay)
}

func (f *Flasher) connect() error {
	f.logf("Connecting to %s...", f.portStr)

	attempts := f.opts.ConnectAttempts
	if attempts <= 0 {
		if f.opts.FastMode {
			attempts = 2 // Fast mode: minimal attempts
		} else {
			attempts = 7 // Normal mode: comprehensive attempts
		}
	}

	for attempt := 0; attempt < attempts; attempt++ {
		// Check for context cancellation
		select {
		case <-f.ctx.Done():
			return fmt.Errorf("connection cancelled: %w", f.ctx.Err())
		default:
		}

		if attempt > 0 {
			f.logf("Connection attempt %d/%d", attempt+1, attempts)
		}

		// Reset the chip into bootloader mode
		switch f.opts.ResetMode {
		case ResetDefault:
			// On Unix systems (darwin, linux), use the esptool sequence:
			// unixTightReset with 50ms, then 550ms, then fallback to classic resets.
			// For the ESP32-P4 the helper routes to forceP4Download, which latches
			// the native USB-Serial-JTAG peripheral into download mode.
			if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
				switch attempt % 4 {
				case 0:
					f.enterBootloaderReset(f.port, defaultResetDelay)
				case 1:
					f.enterBootloaderReset(f.port, extraResetDelay)
				case 2:
					f.enterBootloaderReset(f.port, defaultResetDelay)
				case 3:
					f.enterBootloaderReset(f.port, extraResetDelay)
				}
			} else {
				// On other systems, use classic and tight reset alternation
				if attempt%2 == 0 {
					f.enterBootloaderReset(f.port, defaultResetDelay)
				} else {
					f.enterBootloaderReset(f.port, defaultResetDelay)
				}
			}
		case ResetUSBJTAG:
			// A native USB-JTAG/Serial board enters bootloader mode via the
			// USB-JTAG/Serial peripheral. When that path succeeds we're done
			// connecting; otherwise fall through to the standard reset + sync
			// sequence on the next attempt.
			if f.connectViaUSBJTAG() {
				f.LastConnectAttempt = attempt
				LastConnectAttempts = append(LastConnectAttempts, attempt)
				return f.finishConnect()
			}
			continue
		case ResetAuto:
			// A USB-UART-bridge board (CH340, CH9102, CP210x, FTDI) enumerates
			// as /dev/ttyACM* too, but it has NO native USB-JTAG peripheral: the
			// USB endpoint is a UART bridge, so the reset is driven purely with
			// DTR/RTS. The USB-JTAG path below is therefore dead weight for
			// such a board, and its reopenPort() is disruptive on a device that
			// never disconnects. When the flash layer has flagged the board as a
			// bridge (FlasherOptions.ResetBridge), use only the atomic DTR/RTS
			// reset — matching esptool.py's bridge reset strategy — and confirm
			// the chip actually entered download mode via its boot log.
			if f.opts.ResetBridge {
				f.enterBootloaderReset(f.port, defaultResetDelay)
				f.probeBootLog()
				break
			}

			// Uniform reset cycle for every port. A UART-bridge board
			// (CH340, CH9102, CP210x) also enumerates as /dev/ttyACM* but has
			// no USB-JTAG peripheral, so the first attempt must be the DTR/RTS
			// reset. Native-USB boards (ESP32-S3/C3/C5/C6/H2 USB-JTAG) fall
			// through to attempt 1, which tries the USB-JTAG reset.
			//
			// The first attempt uses the atomic `unixTightReset` (DTR + RTS
			// toggled in a single ioctl) rather than the sequential
			// `classicReset`. This matches espflash's `UnixTightReset`-first
			// strategy for USB-to-Serial bridges: the simultaneous transition
			// is what reliably enters download mode, and a marginal reset
			// leaves the chip in a state that drops out mid-flash (see
			// docs/flashing-implementation.md, "Intermittent Flash Failure").
			switch attempt % 3 {
			case 0:
				f.enterBootloaderReset(f.port, defaultResetDelay)
			case 1:
				// Second attempt: try the USB-JTAG/Serial reset path. When it
				// succeeds we're done connecting; otherwise fall through to the
				// standard sync on the next attempt.
				if f.connectViaUSBJTAG() {
					return f.finishConnect()
				}
				continue
			case 2:
				// No reset — device may already be in bootloader
			}
		}
		// ResetNoReset: skip reset entirely

		// Try to sync with the bootloader. syncToBootloader runs the
		// sync loop and, on failure, reopens the port before returning
		// so the caller can try another reset attempt.
		connected, err := f.syncToBootloader()
		if err != nil {
			return err
		}
		if connected {
			// Measurement hook: remember which attempt won, so the
			// reset-reliability sweep can distinguish a working first-attempt
			// reset from one that only succeeds because a later attempt
			// falls through.
			f.LastConnectAttempt = attempt
			LastConnectAttempts = append(LastConnectAttempts, attempt)
			return f.finishConnect()
		}
	}

	return &SyncError{Attempts: attempts}
}

func (f *Flasher) finishConnect() error {
	f.logf("Connected.")

	// Detect chip type
	f.logf("Detecting chip...")
	if f.opts.ChipType == ChipAuto {
		chip, err := f.detectChip()
		if err != nil {
			return err
		}
		f.chip = chip
	} else {
		def, ok := chipDefs[f.opts.ChipType]
		if !ok {
			return fmt.Errorf("unsupported chip type: %s", f.opts.ChipType)
		}
		f.chip = def
	}

	f.logf("Detected chip: %s", f.chip.Name)

	// Run chip-specific post-connect initialization.
	if f.chip.PostConnect != nil {
		if err := f.chip.PostConnect(f); err != nil {
			f.logf("Warning: post-connect: %v", err)
		}
	}

	// Propagate chip capabilities to the connection layer.
	f.conn.setSupportsEncryptedFlash(f.chip.SupportsEncryptedFlash)

	// Propagate USB flag to connection layer for block size optimization.
	if f.usesUSB {
		f.conn.setUSB(true)
	}

	// Upload the stub loader to enable advanced features (erase, compression, etc.).
	// The revision is honoured here: some chips (notably ESP32-P4) ship
	// revision-specific stub flashers whose OCRAM load address differs, so the
	// stub selected without the detected revision would load at the wrong
	// address and fail to start (no OHAI greeting).
	//
	// On the ESP32-P4 the compressed stub path stalls at FLASH_DEFL_END (command
	// 0x12): the chip ACKs the begin/data blocks but never answers the flash
	// end, so the finish times out. espflash's working path for the P4 is ROM
	// mode (--no-stub), where the ROM bootloader handles the flash with the
	// uncompressed FlashBegin/FlashData/FlashEnd sequence. When SkipStub is set
	// we skip the stub loader and let the ROM bootloader flash directly.
	if !f.opts.SkipStub {
		if s, ok := stubForRevision(f.chip.ChipType, f.revision); ok {
			f.logf("Loading stub loader...")
			if err := f.conn.loadStub(s); err != nil {
				f.logf("Warning: could not load stub: %v", err)
			} else {
				f.logf("Stub running.")
			}
		}
	} else {
		f.logf("Stub loading skipped (ROM mode).")
	}

	return nil
}

// syncToBootloader attempts to establish a SLIP connection with the
// chip's ROM bootloader after the reset sequence in connect(). It
// returns true when the bootloader responds to the sync command. On
// failure it reopens the serial port (the device may have
// re-enumerated after the reset) and returns false so the caller can
// try another reset attempt. A non-nil error is returned only when
// the caller's context is cancelled.
func (f *Flasher) syncToBootloader() (bool, error) {
	// Give the bootloader time to start after the reset.
	syncDelay := 100 * time.Millisecond
	if f.opts.FastMode {
		syncDelay = 50 * time.Millisecond // Faster: bootloader ready sooner
	}
	if f.ctx != nil {
		select {
		case <-f.ctx.Done():
			return false, fmt.Errorf("connection cancelled: %w", f.ctx.Err())
		default:
		}
	}
	time.Sleep(syncDelay) // Give bootloader time to start
	f.conn.flushInput()

	f.logf("Syncing with bootloader...")
	syncAttempts := 5
	if f.opts.FastMode {
		syncAttempts = 3 // Fast mode: fewer sync attempts
	}

	for range syncAttempts {
		// Check for context cancellation
		select {
		case <-f.ctx.Done():
			return false, fmt.Errorf("connection cancelled: %w", f.ctx.Err())
		default:
		}

		_, syncErr := f.conn.sync()
		if syncErr == nil {
			return true, nil
		}
		f.logf("Sync attempt failed: %v", syncErr)
		// Check context before sleep between sync attempts
		if f.ctx != nil {
			select {
			case <-f.ctx.Done():
				return false, fmt.Errorf("connection cancelled: %w", f.ctx.Err())
			default:
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Sync failed — try reopening port (USB CDC may have re-enumerated
	// after reset, causing the old port handle to be stale).
	// In fast mode, skip reopen to save time - just try next attempt
	if f.opts.FastMode {
		f.logf("Fast mode: skipping port reopen, trying next reset attempt")
		return false, nil
	}

	if err := f.reopenPort(); err != nil {
		f.logf("Port reopen failed: %v", err)
		return false, nil // port reopen failed, try next attempt
	}

	f.logf("Port reopened, trying next reset attempt")

	// Port reopened successfully — the device likely just finished
	// USB re-enumeration after the reset above. Return false so the
	// caller loops back and issues another reset attempt.
	return false, nil
}

// probeBootLog captures the bytes the chip prints on the serial line right
// after the reset and logs whether they confirm the chip entered download
// mode. The ESP32 bootloader announces download mode with a
// "boot: ... waiting for download" line; seeing it means the DTR/RTS reset
// worked and the chip is ready to take a flash. Not seeing it is not fatal —
// the sync loop is the real gate — but is logged so a marginal reset surfaces
// as a clear early signal instead of a late, confusing SLIP timeout.
//
// It runs only on the first reset attempt so a multi-attempt connect pays the
// capture cost once; the sync loop re-checks on every attempt.
func (f *Flasher) probeBootLog() {
	log := readBootLog(f.port, 500*time.Millisecond)
	lower := strings.ToLower(log)
	switch {
	case strings.Contains(lower, "waiting for download"):
		f.logf("boot log confirms download mode")
	case strings.TrimSpace(log) != "":
		f.logf("boot log seen but no 'waiting for download' (chip may still be running the app; a power-cycle-coordinated reset may be needed)")
	default:
		f.logf("boot log empty after reset (no serial output captured)")
	}
}

// readBootLog captures whatever the chip prints on the serial line during the
// `timeout` window and returns it as a string. Used to confirm, right after a
// reset, that the chip entered bootloader download mode. An empty result means
// the chip printed nothing in the window (or the read never produced data).
func readBootLog(port serial.Port, timeout time.Duration) string {
	if port == nil {
		return ""
	}
	if err := port.SetReadTimeout(timeout); err != nil {
		return ""
	}
	var out bytes.Buffer
	buf := make([]byte, 512)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		n, err := port.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if err != nil {
			// Read timeout or EOF ends the capture window.
			break
		}
	}
	return out.String()
}

// detectChip identifies the connected ESP chip.
func (f *Flasher) detectChip() (*chipDef, error) {
	si, err := f.readSecurityInfo()
	if err != nil {
		f.logf("unable to read security info: %s", err)
	}

	for _, def := range chipDefs {
		if def.UsesMagicValue {
			continue
		}

		if si != nil && si.ChipID != nil && *si.ChipID == uint32(def.ImageChipID) {
			def.SecureDownloadMode = si.ParsedFlags.SecureDownloadEnable
			return def, nil
		}
	}

	// Otherwise, try to read the chip magic value to verify the chip type (ESP8266, ESP32, ESP32-S2)
	magic, err := f.conn.readReg(chipDetectMagicRegAddr)
	if err != nil {
		// Only ESP32-S2 does not support chip id detection
		// and supports secure download mode
		f.logf("unable to read chip magic value. Defaulting to ESP32-S2: %s", err)

		chipDefs[ChipESP32S2].SecureDownloadMode = si.ParsedFlags.SecureDownloadEnable
		return chipDefs[ChipESP32S2], nil
	}

	// Check magic value for older chips (ESP8266, ESP32, ESP32-S2)
	if def := detectChipByMagic(magic); def != nil {
		return def, nil
	}

	return nil, &ChipDetectError{MagicValue: magic}
}

// FlashImage writes a firmware image to flash at the given offset.
//
// The data should be a raw .bin file (not ELF). The offset is typically
// 0x0 for a merged/combined binary, or a specific address like 0x10000
// for the application partition.
//
// If progress is non-nil, it will be called periodically with the number
// of bytes transferred so far.
//
// Example:
//
//	data, _ := os.ReadFile("firmware.bin")
//	err := f.FlashImage(data, 0x0, func(cur, total int) {
//	    fmt.Printf("\r%d/%d bytes", cur, total)
//	})
func (f *Flasher) FlashImage(data []byte, offset uint32, progress ProgressFunc) error {
	if len(data) == 0 {
		return fmt.Errorf("empty image data")
	}

	// Auto-detect flash size when not explicitly set.
	// This matches esptool.py's default --flash_size=detect behavior.
	// On ESP8266, the ROM bootloader uses the flash size from the image
	// header to configure SPI flash memory mapping, so it must be correct
	// for firmware to execute properly.
	if (f.opts.FlashSize == "" || f.opts.FlashSize == "keep") &&
		len(data) >= 4 && data[0] == espImageMagic {
		if detected := f.detectFlashSize(); detected != "" {
			f.logf("Configuring flash size...")
			f.logf("Auto-detected flash size: %s", detected)
			f.opts.FlashSize = detected
		}
	}

	// ESP8266 only supports DOUT (and DIO on some boards). Force DOUT
	// when no explicit flash mode was requested to ensure the image boots.
	if f.chip != nil && f.chip.ChipType == ChipESP8266 &&
		(f.opts.FlashMode == "" || f.opts.FlashMode == "keep") {
		f.opts.FlashMode = "dout"
	}

	// Patch flash parameters in image header (mode, frequency, size)
	var err error
	data, err = f.patchImageHeader(data)
	if err != nil {
		return fmt.Errorf("patch image header: %w", err)
	}

	// Pad data to 4-byte alignment
	if pad := len(data) % 4; pad != 0 {
		data = append(data, make([]byte, 4-pad)...)
	}

	// Attach SPI flash if not already done
	if err := f.attachFlash(); err != nil {
		return fmt.Errorf("attach flash: %w", err)
	}

	// Optionally switch to higher baud rate (not supported by ESP8266 ROM).
	// Cap the flash baud for UART-bridge boards (CH340/CH9102/CP210x), whose
	// small RX FIFO overflows at high bauds and makes the ROM stop answering
	// SLIP during flash page writes. Native-USB boards have flow control and
	// are unaffected (usesUSB == true).
	flashBaud := f.opts.FlashBaudRate
	if f.chip != nil && f.chip.MaxUARTFlashBaud > 0 && !f.usesUSB && flashBaud > f.chip.MaxUARTFlashBaud {
		flashBaud = f.chip.MaxUARTFlashBaud
		f.logf("Capping flash baud to %d for UART-bridge board", flashBaud)
	}
	canChangeBaud := f.chip == nil || f.chip.ROMHasChangeBaud || f.conn.isStub()
	if canChangeBaud && flashBaud > 0 && flashBaud != f.opts.BaudRate {
		if err := f.changeBaud(flashBaud); err != nil {
			f.logf("Warning: could not change baud rate to %d: %v", flashBaud, err)
			// Continue at original baud rate
		}
	}

	// Use compressed flash only with the stub loader. While most ESP32+ ROM
	// bootloaders support compressed flash commands, we must skip flashDeflEnd
	// for ROM (it exits the bootloader), which can leave data unflushed in
	// the ROM's decompressor buffer. esptool also defaults to uncompressed
	// writes for ROM (compress = IS_STUB).
	if f.opts.Compress && f.conn.isStub() {
		return f.flashCompressed(data, offset, progress)
	}
	return f.flashUncompressed(data, offset, progress)
}

// FlashImages writes multiple firmware images to flash at their respective offsets.
// This is useful for flashing bootloader + partition table + application in one go.
//
// Each entry is a (data, offset) pair.
//
// Example:
//
//	images := []espflasher.ImagePart{
//	    {Data: bootloader, Offset: 0x1000},
//	    {Data: partTable, Offset: 0x8000},
//	    {Data: app, Offset: 0x10000},
//	}
//	err := f.FlashImages(images, progress)
func (f *Flasher) FlashImages(images []ImagePart, progress ProgressFunc) error {
	// Attach SPI flash first
	if err := f.attachFlash(); err != nil {
		return fmt.Errorf("attach flash: %w", err)
	}

	// Auto-detect flash size when not explicitly set.
	if f.opts.FlashSize == "" || f.opts.FlashSize == "keep" {
		if detected := f.detectFlashSize(); detected != "" {
			f.logf("Configuring flash size...")
			f.logf("Auto-detected flash size: %s", detected)
			f.opts.FlashSize = detected
		}
	}

	// ESP8266 only supports DOUT (and DIO on some boards). Force DOUT
	// when no explicit flash mode was requested to ensure the image boots.
	if f.chip != nil && f.chip.ChipType == ChipESP8266 &&
		(f.opts.FlashMode == "" || f.opts.FlashMode == "keep") {
		f.opts.FlashMode = "dout"
	}

	// Optionally switch to higher baud rate (not supported by ESP8266 ROM).
	// Cap the flash baud for UART-bridge boards (CH340/CH9102/CP210x), whose
	// small RX FIFO overflows at high bauds and makes the ROM stop answering
	// SLIP during flash page writes. Native-USB boards have flow control and
	// are unaffected (usesUSB == true).
	flashBaud := f.opts.FlashBaudRate
	if f.chip != nil && f.chip.MaxUARTFlashBaud > 0 && !f.usesUSB && flashBaud > f.chip.MaxUARTFlashBaud {
		flashBaud = f.chip.MaxUARTFlashBaud
		f.logf("Capping flash baud to %d for UART-bridge board", flashBaud)
	}
	canChangeBaud := f.chip == nil || f.chip.ROMHasChangeBaud || f.conn.isStub()
	if canChangeBaud && flashBaud > 0 && flashBaud != f.opts.BaudRate {
		if err := f.changeBaud(flashBaud); err != nil {
			f.logf("Warning: could not change baud rate to %d: %v", flashBaud, err)
		}
	}

	totalSize := 0
	for _, img := range images {
		totalSize += len(img.Data)
	}

	written := 0
	for _, img := range images {
		data := img.Data

		// Patch flash parameters in image header (mode, frequency, size)
		var err error
		data, err = f.patchImageHeader(data)
		if err != nil {
			return fmt.Errorf("patch image header at 0x%08X: %w", img.Offset, err)
		}

		// Pad to 4-byte alignment
		if pad := len(data) % 4; pad != 0 {
			data = append(data, make([]byte, 4-pad)...)
		}

		f.logf("Writing %d bytes at 0x%08X...", len(data), img.Offset)

		// Flash the part, retrying the whole part if it fails. A single block
		// is already retried inside flashCompressed/flashUncompressed, but a
		// UART-bridge board (CH340/CP210x) can lose a block to RX-FIFO overflow
		// during the sustained app transfer, after which the chip stops
		// responding for the rest of that attempt window. Re-flashing the part
		// re-issues FLASH_DEFL_BEGIN and starts the download fresh, which is
		// what recovers from that cascade. This mirrors esptool's habit of
		// retrying a failed write rather than aborting on the first error.
		//
		// If the overflow has stranded the bootloader (even FLASH_DEFL_BEGIN
		// times out), a plain re-send cannot recover, so we reset and reconnect
		// before the next attempt.
		var partErr error
		for attempt := 0; attempt < maxFlashPartAttempts; attempt++ {
			var partProgress ProgressFunc
			if progress != nil {
				base := written
				partProgress = func(cur, total int) {
					progress(base+cur, totalSize)
				}
			}

			if f.opts.Compress && f.conn.isStub() {
				partErr = f.flashCompressed(data, img.Offset, partProgress)
			} else {
				partErr = f.flashUncompressed(data, img.Offset, partProgress)
			}
			if partErr == nil {
				break
			}
			if attempt < maxFlashPartAttempts-1 {
				f.logf("Warning: flash at 0x%08X failed: %v, retrying (%d/%d)", img.Offset, partErr, attempt+1, maxFlashPartAttempts)
				if rerr := f.reconnect(); rerr != nil {
					f.logf("Warning: reconnect failed: %v", rerr)
				}
			}
		}
		if partErr != nil {
			return fmt.Errorf("flash at 0x%08X: %w", img.Offset, partErr)
		}

		written += len(img.Data)
	}

	return nil
}

// ImagePart represents a firmware image segment with its flash offset.
type ImagePart struct {
	// Data is the raw binary data to flash.
	Data []byte

	// Offset is the flash address to write to (e.g. 0x0, 0x1000, 0x10000).
	Offset uint32
}

// reconnect recovers from a bootloader lockup. When a UART-bridge board
// (CH340/CP210x) loses a flash block to RX-FIFO overflow at high flash bauds,
// the overflow can cascade: the chip stops answering not just the next block
// but also FLASH_DEFL_BEGIN, and its SPI flash controller can stay stuck so
// even SPI_FLASH_ATTACH times out. A plain re-send can never recover from this.
// This resets the board, re-syncs with the ROM bootloader, re-uploads the stub,
// and restores the SPI attach + flash baud, giving the failed part a clean
// session to retry in. It is only invoked as a last-resort recovery inside
// FlashImages, never during the normal single-session flow.
func (f *Flasher) reconnect() error {
	for attempt := 0; attempt < maxReconnectAttempts; attempt++ {
		f.logf("Reconnecting to recover from flash failure...")

		// A UART-bridge board (CH340/CH9102/CP210x) that cascades into a
		// stuck-SPI state leaves stale bytes in the RX FIFO and the SLIP
		// reader mid-frame. Before re-syncing we must drain both, otherwise
		// the next SLIP frame is misframed and the sync/stub/attach all time
		// out again. flushInput resets the reader and clears the buffer.
		f.conn.flushInput()
		time.Sleep(reconnectSettleDelay)

		// Reconnect must fail fast: a single fast sync+attach is enough to
		// decide whether the chip recovered. A deep cascade will not come back
		// on a quick sync, so keep the attempt count low and let the client
		// retry clear it.
		prevAttempts := f.opts.ConnectAttempts
		if prevAttempts > 2 {
			f.opts.ConnectAttempts = 2
		}
		if err := f.connect(); err != nil {
			f.opts.ConnectAttempts = prevAttempts
			f.logf("Warning: reconnect sync failed: %v", err)
			time.Sleep(reconnectSettleDelay)
			continue
		}
		f.opts.ConnectAttempts = prevAttempts
		// Re-attach the SPI flash and restore the high-speed flash baud so the
		// retried part runs in the same session configuration as the first
		// attempt. After a cascade the SPI controller can be transiently
		// stuck, so retry the attach with a settle delay.
		attachErr := f.attachFlash()
		for r := 0; attachErr != nil && r < maxReconnectAttempts; r++ {
			f.logf("Warning: re-attach flash failed: %v, retrying (%d/%d)", attachErr, r+1, maxReconnectAttempts)
			time.Sleep(reconnectSettleDelay)
			attachErr = f.attachFlash()
		}
		if attachErr != nil {
			f.logf("Warning: re-attach flash still failed: %v", attachErr)
			continue
		}
		flashBaud := f.opts.FlashBaudRate
		if f.chip != nil && f.chip.MaxUARTFlashBaud > 0 && !f.usesUSB && flashBaud > f.chip.MaxUARTFlashBaud {
			flashBaud = f.chip.MaxUARTFlashBaud
		}
		canChangeBaud := f.chip == nil || f.chip.ROMHasChangeBaud || f.conn.isStub()
		if canChangeBaud && flashBaud > 0 && flashBaud != f.opts.BaudRate {
			if err := f.changeBaud(flashBaud); err != nil {
				f.logf("Warning: could not change baud rate to %d after reconnect: %v", flashBaud, err)
			}
		}
		f.logf("Reconnected.")
		return nil
	}
	return fmt.Errorf("could not reconnect after flash failure")
}

// flashCompressed writes data to flash using zlib compression.
func (f *Flasher) flashCompressed(data []byte, offset uint32, progress ProgressFunc) error {
	compressed, err := compressData(data)
	if err != nil {
		return fmt.Errorf("compress: %w", err)
	}

	uncompSize := uint32(len(data))
	compSize := uint32(len(compressed))
	f.logf("Compressed %d bytes to %d (%.0f%%)", uncompSize, compSize,
		float64(compSize)/float64(uncompSize)*100)

	// Only use compression if it actually helps
	if compSize >= uncompSize {
		f.logf("Compression not beneficial, using uncompressed write")
		return f.flashUncompressed(data, offset, progress)
	}

	// Skip unchanged check (requires stub loader for FlashMd5 command)
	if f.opts.SkipUnchanged && f.conn.isStub() {
		expected := md5.Sum(data)
		actual, err := f.conn.flashMD5(offset, uncompSize)
		if err == nil {
			actualMD5 := [16]byte{}
			copy(actualMD5[:], actual)
			if expected == actualMD5 {
				f.logf("Skipping unchanged data at 0x%08X (%d bytes)", offset, uncompSize)
				if progress != nil {
					progress(int(uncompSize), int(uncompSize))
				}
				return nil // Skip the write
			}
			f.logf("Data changed at 0x%08X, flashing", offset)
		} else {
			f.logf("Skip-unchanged check failed: %v, continuing with flash", err)
		}
	}

	writeSize := f.conn.flashWriteSize()
	numBlocks := (compSize + writeSize - 1) / writeSize

	f.logf("Flash begin: %d bytes at 0x%08X (%d compressed blocks)", compSize, offset, numBlocks)

	if err := f.conn.flashDeflBegin(uncompSize, compSize, offset, false, f.opts.Erase); err != nil {
		return err
	}

	// Send compressed data blocks
	sent := uint32(0)
	seq := uint32(0)

	for sent < compSize {
		blockLen := min(compSize-sent, writeSize)

		block := compressed[sent : sent+blockLen]
		// Resend the block if the chip does not ACK, mirroring esptool's
		// WRITE_BLOCK_ATTEMPTS retry. A lost ACK (e.g. from a transient
		// CH340 RX-FIFO overflow during sustained app flash) can only be
		// recovered by resending the block, not by merely re-reading.
		//
		// On failure we also flush the input buffer before retrying: a timeout
		// or a ROM 0xC0 error means the chip's response (or the SLIP frame we
		// sent) was lost/garbled in the UART bridge's RX FIFO, leaving the SLIP
		// reader mid-frame. Resending without clearing that stale state just
		// corrupts the next attempt too, so we reset the input buffer + SLIP
		// reader first.
		var blockErr error
		for attempt := 0; attempt < maxFlashBlockAttempts; attempt++ {
			blockErr = f.conn.flashDeflData(block, seq)
			if blockErr == nil {
				break
			}
			f.logf("Warning: compressed flash block %d failed: %v, retrying (%d/%d)", seq, blockErr, attempt+1, maxFlashBlockAttempts)
			f.conn.flushInput()
			time.Sleep(flashBlockRetryDelay)
		}
		if blockErr != nil {
			return fmt.Errorf("flash block %d of %d: %w", seq, numBlocks, blockErr)
		}

		sent += blockLen
		seq++

		if progress != nil {
			// Map compressed bytes sent to approximate uncompressed progress
			approxUncomp := min(int(float64(sent)/float64(compSize)*float64(uncompSize)), int(uncompSize))
			progress(approxUncomp, int(uncompSize))
		}
	}

	// End the compressed flash session.
	// For ROM bootloaders, skip sending FLASH_DEFL_END — the ROM exits the
	// bootloader upon receiving it, which can interfere with flash operations.
	// esptool also skips this for ROM: "skip sending flash_finish to ROM loader,
	// as it causes the loader to exit and run user code."
	// For the stub, the end command acts as a write barrier: the stub ACKs each
	// block on receive but writes to flash asynchronously, so the end command
	// ensures the last block is actually written before we proceed.
	if f.conn.isStub() {
		if err := f.conn.flashDeflEnd(false); err != nil {
			return err
		}
	}

	f.logf("Flash complete. Verifying...")

	// Verify via MD5 if stub is running
	if f.conn.isStub() {
		if err := f.verifyMD5(data, offset); err != nil {
			return err
		}
	}

	return nil
}

// flashUncompressed writes data to flash without compression.
func (f *Flasher) flashUncompressed(data []byte, offset uint32, progress ProgressFunc) error {
	dataSize := uint32(len(data))

	// Skip unchanged check (requires stub loader for FlashMd5 command)
	if f.opts.SkipUnchanged && f.conn.isStub() {
		expected := md5.Sum(data)
		actual, err := f.conn.flashMD5(offset, dataSize)
		if err == nil {
			actualMD5 := [16]byte{}
			copy(actualMD5[:], actual)
			if expected == actualMD5 {
				f.logf("Skipping unchanged data at 0x%08X (%d bytes)", offset, dataSize)
				if progress != nil {
					progress(int(dataSize), int(dataSize))
				}
				return nil // Skip the write
			}
			f.logf("Data changed at 0x%08X, flashing", offset)
		} else {
			f.logf("Skip-unchanged check failed: %v, continuing with flash", err)
		}
	}

	writeSize := f.conn.flashWriteSize()
	numBlocks := (dataSize + writeSize - 1) / writeSize

	f.logf("Flash begin: %d bytes at 0x%08X (%d blocks)", dataSize, offset, numBlocks)

	if err := f.conn.flashBegin(dataSize, offset, false, f.opts.Erase); err != nil {
		return err
	}

	sent := uint32(0)
	seq := uint32(0)

	for sent < dataSize {
		blockLen := dataSize - sent
		if blockLen > writeSize {
			blockLen = writeSize
		}

		block := data[sent : sent+blockLen]

		// Pad last block to writeSize
		if uint32(len(block)) < writeSize {
			padded := make([]byte, writeSize)
			copy(padded, block)
			for i := len(block); i < len(padded); i++ {
				padded[i] = 0xFF
			}
			block = padded
		}

		// Resend the block if the chip does not ACK, mirroring esptool's
		// WRITE_BLOCK_ATTEMPTS retry. A lost ACK (e.g. from a transient
		// CH340 RX-FIFO overflow during sustained app flash) can only be
		// recovered by resending the block, not by merely re-reading.
		//
		// On failure we also flush the input buffer before retrying: a timeout
		// or a ROM error means the chip's response (or the SLIP frame we sent)
		// was lost/garbled in the UART bridge's RX FIFO, leaving the SLIP
		// reader mid-frame. Resending without clearing that stale state just
		// corrupts the next attempt too, so we reset the input buffer + SLIP
		// reader first.
		var blockErr error
		for attempt := 0; attempt < maxFlashBlockAttempts; attempt++ {
			blockErr = f.conn.flashData(block, seq)
			if blockErr == nil {
				break
			}
			f.logf("Warning: flash block %d failed: %v, retrying (%d/%d)", seq, blockErr, attempt+1, maxFlashBlockAttempts)
			f.conn.flushInput()
			time.Sleep(flashBlockRetryDelay)
		}
		if blockErr != nil {
			return fmt.Errorf("flash block %d of %d: %w", seq, numBlocks, blockErr)
		}

		sent += blockLen
		seq++

		if progress != nil {
			progress(int(sent), int(dataSize))
		}
	}

	// End the flash session.
	// For ROM bootloaders, skip sending FLASH_END — the ROM exits the
	// bootloader upon receiving it. esptool also skips this for ROM.
	// For the stub, the end command acts as a write barrier.
	if f.conn.isStub() {
		if err := f.conn.flashEnd(false); err != nil {
			return err
		}
	}

	f.logf("Flash complete. Verifying...")

	// Verify via MD5 if stub is running
	if f.conn.isStub() {
		if err := f.verifyMD5(data, offset); err != nil {
			return err
		}
	}

	return nil
}

// verifyMD5 checks the MD5 of the flashed data against the expected value.
func (f *Flasher) verifyMD5(data []byte, offset uint32) error {
	expected := md5.Sum(data)
	expectedHex := hex.EncodeToString(expected[:])

	actual, err := f.conn.flashMD5(offset, uint32(len(data)))
	if err != nil {
		f.logf("Warning: MD5 verification failed: %v", err)
		return nil // Don't fail on MD5 check failure
	}

	actualHex := hex.EncodeToString(actual)
	if actualHex != expectedHex {
		return fmt.Errorf("MD5 mismatch: expected %s, got %s", expectedHex, actualHex)
	}

	f.logf("MD5 verified: %s", actualHex)
	return nil
}

// EraseFlash erases the entire flash memory.
// This operation can take a significant amount of time (30-120 seconds).
// Requires the stub loader to be running.
func (f *Flasher) EraseFlash() error {
	if !f.conn.isStub() {
		return &UnsupportedCommandError{Command: "erase flash (requires stub)"}
	}

	f.logf("Erasing entire flash...")
	if err := f.conn.eraseFlash(); err != nil {
		return err
	}
	f.logf("Flash erased.")
	return nil
}

// EraseRegion erases a region of flash memory.
// Requires the stub loader to be running.
// Both offset and size must be aligned to the flash sector size (4096 bytes).
func (f *Flasher) EraseRegion(offset, size uint32) error {
	if !f.conn.isStub() {
		return &UnsupportedCommandError{Command: "erase region (requires stub)"}
	}

	if offset%flashSectorSize != 0 {
		return fmt.Errorf("offset 0x%X is not aligned to sector size 0x%X", offset, flashSectorSize)
	}
	if size%flashSectorSize != 0 {
		return fmt.Errorf("size 0x%X is not aligned to sector size 0x%X", size, flashSectorSize)
	}

	f.logf("Erasing %d bytes at 0x%08X...", size, offset)
	return f.conn.eraseRegion(offset, size)
}

// ReadRegister reads a 32-bit register from the device.
func (f *Flasher) ReadRegister(addr uint32) (uint32, error) {
	return f.conn.readReg(addr)
}

// WriteRegister writes a 32-bit value to a register on the device.
func (f *Flasher) WriteRegister(addr, value uint32) error {
	return f.conn.writeReg(addr, value, 0xFFFFFFFF, 0)
}

// GetSecurityInfo returns security-related information from the device.
func (f *Flasher) GetSecurityInfo() (*SecurityInfo, error) {
	return f.readSecurityInfo()
}

// GetFlashMD5 returns the MD5 hash of a flash region.
// Requires the stub loader to be running.
func (f *Flasher) GetFlashMD5(offset, size uint32) (string, error) {
	if !f.conn.isStub() {
		return "", &UnsupportedCommandError{Command: "flash MD5 (requires stub)"}
	}

	if err := f.attachFlash(); err != nil {
		return "", err
	}

	result, err := f.conn.flashMD5(offset, size)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(result), nil
}

// ReadFlash reads data from flash memory.
// Requires the stub loader to be running.
func (f *Flasher) ReadFlash(offset, size uint32) ([]byte, error) {
	if !f.conn.isStub() {
		return nil, &UnsupportedCommandError{Command: "read flash (requires stub)"}
	}

	if err := f.attachFlash(); err != nil {
		return nil, err
	}

	data, err := f.conn.readFlash(offset, size)
	// Clear any stale data left in the serial buffer and SLIP reader
	// after the raw block-read protocol. Without this, leftover bytes
	// can corrupt subsequent command responses.
	f.conn.flushInput()
	return data, err
}

// Reset performs a hard reset of the device, causing it to run user code.
func (f *Flasher) Reset() {
	if f.conn.isStub() {
		// Tell the stub to cleanly exit flash mode and reboot.
		f.conn.flashBegin(0, 0, false, false) //nolint:errcheck
		f.conn.flashEnd(true)                 //nolint:errcheck
	} else {
		// ROM bootloader: send FLASH_END with reboot=true to finalize the
		// write and exit download mode. This mirrors espflash's
		// finish() (flash_target/esp32.rs), where
		// flash_end_reboot = secure_download_mode || reboot; for a P4 in
		// normal (non-SDM) mode that is reboot=true.
		//
		// Without this the ROM never receives the end-of-download command in
		// ROM mode, so the last flash page is left uncommitted (the app at
		// 0x10000 boots into an Illegal-instruction panic) and the chip
		// stays latched in download mode after the flash. Sending it is safe
		// for other chips too: their ROM also accepts FLASH_END and simply
		// reboots.
		f.conn.flashEnd(true) //nolint:errcheck
	}

	// For ROM bootloaders, skip flash_begin/flash_end — sending
	// CMD_FLASH_BEGIN after a compressed download may interfere with
	// the flash controller state at offset 0.
	if f.usesUSB {
		hardResetUSB(f.port)
	} else {
		hardReset(f.port, false)
	}
	f.logf("Device reset.")
}

// attachFlash attaches the SPI flash and configures parameters.
// ESP8266 does not need (or support) SPI attach; its ROM handles flash directly.
func (f *Flasher) attachFlash() error {
	if f.chip != nil && f.chip.ChipType == ChipESP8266 {
		return nil // ESP8266 ROM handles flash internally
	}

	f.logf("Attaching SPI flash...")

	if err := f.conn.spiAttach(0); err != nil {
		return err
	}

	// Configure flash parameters for common 4MB flash
	// These defaults work for most development boards
	err := f.conn.spiSetParams(
		4*1024*1024, // 4MB total
		64*1024,     // 64KB block
		4*1024,      // 4KB sector
		256,         // 256B page
	)
	if err != nil {
		// Don't fail - some ROM versions don't support this
		f.logf("Warning: SPI params config failed (may be OK): %v", err)
	}

	return nil
}

// changeBaud switches to a higher baud rate for faster data transfer.
func (f *Flasher) changeBaud(newBaud int) error {
	f.logf("Switching to %d baud...", newBaud)

	// oldBaud must reflect the baud the chip is ACTUALLY running at, not just
	// the initial connect baud. The stub uses it to compute the new UART
	// divisor; a stale value (e.g. the initial 115200 when the chip is already
	// at 921600) makes the stub reject the command, which then makes the next
	// FLASH_DEFL_BEGIN time out. Track the live baud so repeated changeBaud
	// calls (e.g. when flashing the app after other parts) stay correct.
	oldBaud := uint32(f.currentBaud)
	if err := f.conn.changeBaud(uint32(newBaud), oldBaud); err != nil {
		return err
	}

	// Change the local port baud rate
	time.Sleep(50 * time.Millisecond)
	if err := f.port.SetMode(&serial.Mode{
		BaudRate: newBaud,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}); err != nil {
		return fmt.Errorf("set local baud rate: %w", err)
	}

	// The port is now at newBaud; record it so subsequent changeBaud calls
	// pass the correct old-baud argument.
	f.currentBaud = newBaud

	time.Sleep(50 * time.Millisecond)
	f.conn.flushInput()

	f.logf("Running at %d baud.", newBaud)
	return nil
}

// compressData compresses data using zlib (best compression).
func compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		w.Close() //nolint:errcheck
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// logf logs a message if a logger is configured.
func (f *Flasher) logf(format string, args ...interface{}) {
	if f.opts.Logger != nil {
		f.opts.Logger.Logf(format, args...)
	}
}

// isUSBPort detects USB CDC ports that re-enumerate after reset.
// These include /dev/ttyACM* (Linux USB CDC) and /dev/cu.usb* (macOS).
func isUSBPort(port string) bool {
	return len(port) > 11 && port[:11] == "/dev/ttyACM" || len(port) > 11 && port[:11] == "/dev/cu.usb"
}

// FlashID reads the SPI flash chip manufacturer and device ID.
// Returns (manufacturer_id, device_id, error).
func (f *Flasher) FlashID() (uint8, uint16, error) {
	if err := f.attachFlash(); err != nil {
		return 0, 0, err
	}

	// SPIFLASH_RDID command (0x9F) via run_spiflash_command
	// For simplicity, we read it via the SPI registers
	flashID, err := f.runSPIFlashCommand(0x9F, nil, 24)
	if err != nil {
		return 0, 0, err
	}

	// RDID response in W0: byte 0 (LSB) = manufacturer, byte 1 = memory type,
	// byte 2 = capacity. The ESP SPI peripheral stores the first received
	// byte in the lowest bits. This matches esptool.py:
	//   manufacturer = flash_id & 0xFF
	//   capacity     = (flash_id >> 16) & 0xFF
	mfgID := uint8(flashID & 0xFF)
	devID := uint16(((flashID >> 16) & 0xFF) | ((flashID >> 8) & 0xFF << 8))

	return mfgID, devID, nil
}

// runSPIFlashCommand executes a SPI flash command at the register level.
// It configures the SPI peripheral to send 'cmd' as an 8-bit command,
// optionally write 'data' bytes, and read back 'readBits' bits of response.
func (f *Flasher) runSPIFlashCommand(cmd byte, data []byte, readBits int) (uint32, error) {
	if f.chip == nil {
		return 0, fmt.Errorf("chip not detected")
	}

	base := f.chip.SPIRegBase
	spiUSRReg := base + f.chip.SPIUSROffs
	spiUSR1Reg := base + f.chip.SPIUSR1Offs
	spiUSR2Reg := base + f.chip.SPIUSR2Offs
	spiW0Reg := base + f.chip.SPIW0Offs
	spiCMDReg := base // CMD reg is at base

	const (
		spiUSRCommand = 1 << 31
		spiUSRMISO    = 1 << 28
		spiUSRMOSI    = 1 << 27
		spiCMDUSR     = 1 << 18
	)

	// Save old SPI register values
	oldSPIUSR, _ := f.conn.readReg(spiUSRReg)
	oldSPIUSR1, _ := f.conn.readReg(spiUSR1Reg)
	oldSPIUSR2, _ := f.conn.readReg(spiUSR2Reg)

	flags := uint32(spiUSRCommand)
	if readBits > 0 {
		flags |= spiUSRMISO
	}
	if len(data) > 0 {
		flags |= spiUSRMOSI
	}

	f.conn.writeReg(spiUSRReg, flags, 0xFFFFFFFF, 0)                //nolint:errcheck
	f.conn.writeReg(spiUSR2Reg, (7<<28)|uint32(cmd), 0xFFFFFFFF, 0) //nolint:errcheck

	// Configure MISO/MOSI data bit lengths.
	// ESP32-S2+ use dedicated MISO_DLEN / MOSI_DLEN registers.
	// ESP8266 and ESP32 encode lengths in the SPI_USR1 register:
	//   MISO length: bits [7:0] (ESP32) or [4:0] (ESP8266) — value = bitlen-1
	//   MOSI length: bits [25:17] — value = bitlen-1
	if f.chip.SPIMISODLenOffs != 0 {
		// Newer chips: dedicated DLEN registers
		if readBits > 0 {
			f.conn.writeReg(base+f.chip.SPIMISODLenOffs, uint32(readBits-1), 0xFFFFFFFF, 0) //nolint:errcheck
		}
		if len(data) > 0 {
			f.conn.writeReg(base+f.chip.SPIMOSIDLenOffs, uint32(len(data)*8-1), 0xFFFFFFFF, 0) //nolint:errcheck
		}
	} else {
		// ESP8266 / ESP32: set lengths via SPI_USR1 fields
		usr1 := oldSPIUSR1
		if readBits > 0 {
			usr1 = (usr1 &^ 0xFF) | uint32(readBits-1) // bits [7:0] = MISO bitlen-1
		}
		if len(data) > 0 {
			usr1 = (usr1 &^ (0x1FF << 17)) | (uint32(len(data)*8-1) << 17) // bits [25:17] = MOSI bitlen-1
		}
		f.conn.writeReg(spiUSR1Reg, usr1, 0xFFFFFFFF, 0) //nolint:errcheck
	}

	// Write MOSI data to W0 register if present
	if len(data) > 0 {
		var mosiWord uint32
		for i, b := range data {
			mosiWord |= uint32(b) << (8 * uint(i))
		}
		f.conn.writeReg(spiW0Reg, mosiWord, 0xFFFFFFFF, 0) //nolint:errcheck
	}

	// Trigger the SPI user command
	f.conn.writeReg(spiCMDReg, spiCMDUSR, 0xFFFFFFFF, 0) //nolint:errcheck

	// Wait for SPI command to complete
	for i := 0; i < 10; i++ {
		val, _ := f.conn.readReg(spiCMDReg)
		if val&spiCMDUSR == 0 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	result, _ := f.conn.readReg(spiW0Reg)

	// Restore SPI registers
	f.conn.writeReg(spiUSRReg, oldSPIUSR, 0xFFFFFFFF, 0)   //nolint:errcheck
	f.conn.writeReg(spiUSR1Reg, oldSPIUSR1, 0xFFFFFFFF, 0) //nolint:errcheck
	f.conn.writeReg(spiUSR2Reg, oldSPIUSR2, 0xFFFFFFFF, 0) //nolint:errcheck

	return result, nil
}

// flashSizeFromJEDEC converts a JEDEC flash capacity byte to a standard
// size string (e.g. "1MB", "4MB"). The JEDEC capacity byte encodes size
// as 2^N bytes (e.g. 0x14 = 2^20 = 1MB).
// Returns empty string if the capacity byte is out of range or unrecognized.
func flashSizeFromJEDEC(capByte uint8) string {
	if capByte < 18 || capByte > 27 {
		return "" // outside 256KB..128MB range
	}

	sizeBytes := uint64(1) << capByte

	switch {
	case sizeBytes == 256*1024:
		return "256KB"
	case sizeBytes == 512*1024:
		return "512KB"
	case sizeBytes >= 1024*1024:
		return fmt.Sprintf("%dMB", sizeBytes/(1024*1024))
	default:
		return ""
	}
}

// detectFlashSize reads the SPI flash JEDEC ID and determines the flash
// chip capacity. Returns a size string (e.g. "1MB", "4MB") that matches
// the chip's FlashSizes map, or empty string if detection fails.
func (f *Flasher) detectFlashSize() string {
	if f.chip == nil || f.conn == nil {
		return ""
	}

	_, devID, err := f.FlashID()
	if err != nil {
		return ""
	}

	// The lower byte of devID is the JEDEC capacity byte.
	capByte := uint8(devID & 0xFF)
	sizeName := flashSizeFromJEDEC(capByte)
	if sizeName == "" {
		return ""
	}

	// Verify this size is supported by the current chip.
	if _, ok := f.chip.FlashSizes[sizeName]; ok {
		return sizeName
	}

	return ""
}

// StdoutLogger is a simple Logger implementation that writes to an io.Writer.
type StdoutLogger struct {
	W io.Writer
}

// Logf implements the Logger interface.
func (l *StdoutLogger) Logf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(l.W, format+"\n", args...)
}
