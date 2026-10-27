package espflash

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	"go.bug.st/serial"
)

// ROM bootloader command opcodes.
// These are shared across all ESP chip families.
const (
	cmdFlashBegin      byte = 0x02 // Start flash download
	cmdFlashData       byte = 0x03 // Flash data block
	cmdFlashEnd        byte = 0x04 // Finish flash download
	cmdMemBegin        byte = 0x05 // Start RAM download
	cmdMemEnd          byte = 0x06 // Finish RAM download / execute
	cmdMemData         byte = 0x07 // RAM data block
	cmdSync            byte = 0x08 // Sync with bootloader
	cmdWriteReg        byte = 0x09 // Write 32-bit memory-mapped register
	cmdReadReg         byte = 0x0A // Read 32-bit memory-mapped register
	cmdSecurityInfoReg byte = 0x14 // Read security info (chip ID, flash encryption, etc.)
	cmdSPISetParams    byte = 0x0B // Configure SPI flash parameters
	cmdSPIAttach       byte = 0x0D // Attach SPI flash
	cmdChangeBaud      byte = 0x0F // Change UART baud rate
	cmdFlashDeflBeg    byte = 0x10 // Start compressed flash download
	cmdFlashDeflData   byte = 0x11 // Compressed flash data block
	cmdFlashDeflEnd    byte = 0x12 // Finish compressed flash download
	cmdSPIFlashMD5     byte = 0x13 // Calculate MD5 of flash region

	// Stub-only commands (available after stub loader is running)
	cmdEraseFlash  byte = 0xD0 // Erase entire flash
	cmdEraseRegion byte = 0xD1 // Erase flash region
	cmdReadFlash   byte = 0xD2 // Read flash contents
	cmdRunUserCode byte = 0xD3 // Run user code

	// Command response indicators
	respDirectionReq  byte = 0x00 // Request direction
	respDirectionResp byte = 0x01 // Response direction
)

// Protocol constants
const (
	// checksumMagic is the initial value for the XOR checksum.
	checksumMagic uint32 = 0xEF

	// espRAMBlock is the maximum block size for RAM writes.
	espRAMBlock uint32 = 0x1800 // 6KB

	// flashWriteSizeROM is the flash write block size when using ROM loader.
	flashWriteSizeROM uint32 = 0x400 // 1KB

	// flashWriteSizeStub is the flash write block size when using stub loader.
	flashWriteSizeStub uint32 = 0x4000 // 16KB

	// flashSectorSize is the minimum flash erase unit.
	flashSectorSize uint32 = 0x1000 // 4KB

	// readFlashBlockSize is the block size for read flash operations.
	readFlashBlockSize uint32 = 0x1000 // 4KB

	// espImageMagic is the first byte of a valid ESP firmware image.
	espImageMagic byte = 0xE9
)

// Default timeouts
const (
	defaultTimeout      = 3 * time.Second
	syncTimeout         = 100 * time.Millisecond
	chipEraseTimeout    = 120 * time.Second
	md5Timeout          = 30 * time.Second
	eraseWritePerMBRate = 10 * time.Second // per megabyte
)

// p4CommandDeadline is the overall wall-clock budget for the command()
// read loop. The ESP32-P4 ROM defers each reply until the NEXT command
// arrives (one-command pipeline lag), and a priming-sync backlog can
// leave many stale frames to drain first, so we allow up to 8 s before
// giving up on a silent chip. See wiki/esp32-p4-flash-solution.md.
//
// It is a var (not a const) purely so tests can temporarily shorten it
// to keep the "silent chip fails fast" test fast instead of blocking for
// 8 s. Production code always uses the 8 s value.
var p4CommandDeadline = 8 * time.Second

// p4MaxStaleFrames caps how many non-matching frames (poke replies,
// priming-sync leftovers) we tolerate before failing a command.
var p4MaxStaleFrames = 256

// syncPokePayload is the SYNC command frame sent as a follow-up "poke" after
// every other command. The ESP32-P4 ROM defers a command's reply until the
// next command is received, so this poke flushes the queued reply. See
// wiki/esp32-p4-flash-solution.md.
var syncPokePayload = func() []byte {
	p := make([]byte, 36)
	p[0], p[1], p[2], p[3] = 0x07, 0x07, 0x12, 0x20
	for i := 4; i < len(p); i++ {
		p[i] = 0x55
	}
	return p
}()

// commandPost is the per-chip follow-up applied after a command frame is sent
// and before its response is read. Keeping it a small interface means each chip
// family expresses its own wire discipline in isolation: adding a new MCU is a
// new type, not a new branch inside command().
type commandPost interface {
	apply(c *conn) error
}

// plainCommandPost is the follow-up for chips that reply immediately
// (ESP32, ESP32-S2/S3/C3/C5/C6/H2, ESP8266). It does nothing: the reply is
// already in the buffer and is read first by command()'s loop, so no poke and
// no Drain are needed. This is the historical behavior that works for the
// ESP32-S3.
type plainCommandPost struct{}

func (plainCommandPost) apply(*conn) error { return nil }

// commandPostRegistry maps a forced chip type to its command follow-up. Families
// register themselves through registerCommandPost (usually from an init in their
// own file), so adding a new MCU is a new file, not a change to the selection
// logic here. This keeps the per-family discipline isolated and free of
// if/switch spaghetti.
var commandPostRegistry = map[ChipType]commandPost{}

// registerCommandPost records the command follow-up for a chip type. It is
// called once per family (typically from init) so each family lives in its own
// file. See command_p4.go.
func registerCommandPost(chip ChipType, post commandPost) {
	commandPostRegistry[chip] = post
}

// commandPostFor selects the command follow-up for a forced chip type. Chips
// that have not registered a family follow-up (including ChipAuto) fall back to
// the plain no-op, which is the safe default.
func commandPostFor(chip ChipType) commandPost {
	if post, ok := commandPostRegistry[chip]; ok {
		return post
	}
	return plainCommandPost{}
}

// conn wraps the serial port and provides the low-level protocol operations.
type conn struct {
	port    serial.Port
	reader  *slipReader
	stub    bool
	usesUSB bool // set for USB-OTG and USB-JTAG/Serial connections
	// post is the per-chip command follow-up. It defaults to the plain no-op
	// (the pre-P4 behavior that works for S3 and every chip that replies
	// immediately). Production selects the discipline explicitly from the forced
	// opts.ChipType via setCommandPost, in New() and again after reopenPort()
	// rebuilds the conn. See commandPost / plainCommandPost / command_p4.go.
	post                   commandPost
	supportsEncryptedFlash bool
	ctx                    context.Context
}

// isStub returns whether the stub loader is running.
func (c *conn) isStub() bool {
	return c.stub
}

// setUSB sets whether the connection uses USB-OTG or USB-JTAG endpoints.
func (c *conn) setUSB(v bool) {
	c.usesUSB = v
}

// setSupportsEncryptedFlash sets whether the ROM supports encrypted flash commands.
func (c *conn) setSupportsEncryptedFlash(v bool) {
	c.supportsEncryptedFlash = v
}

// newConn creates a new protocol connection over the given serial port. The
// command follow-up defaults to the plain no-op; production overrides it from
// the forced chip type via setCommandPost (see New / reopenPort).
func newConn(port serial.Port, ctx context.Context) *conn {
	return &conn{
		port:   port,
		reader: newSlipReader(port, ctx),
		post:   plainCommandPost{},
		ctx:    ctx,
	}
}

// setCommandPost selects the per-chip command follow-up. The Flasher calls it
// from New() and again from reopenPort() (which rebuilds the conn), so the
// correct discipline is in place during the initial sync that runs before chip
// detection.
func (c *conn) setCommandPost(p commandPost) {
	c.post = p
}

// checksum calculates the ESP bootloader checksum (XOR of all bytes).
func checksum(data []byte) uint32 {
	state := checksumMagic
	for _, b := range data {
		state ^= uint32(b)
	}
	return state
}

// sendCommand sends a command packet to the ESP device.
//
// Packet format: [direction=0x00] [opcode] [data_length:16LE] [checksum:32LE] [data...]
func (c *conn) sendCommand(opcode byte, data []byte, chk uint32) error {
	if os.Getenv("EB_DEBUG") != "" {
		fmt.Printf("[DBG send] op=0x%02X len=%d usesUSB=%v\n", opcode, len(data), c.usesUSB)
	}
	pkt := make([]byte, 8+len(data))
	pkt[0] = respDirectionReq
	pkt[1] = opcode
	binary.LittleEndian.PutUint16(pkt[2:4], uint16(len(data)))
	binary.LittleEndian.PutUint32(pkt[4:8], chk)
	copy(pkt[8:], data)

	frame := slipEncode(pkt)
	// USB CDC endpoints have limited buffer sizes. Writing large SLIP frames
	// in one shot can overflow the endpoint buffer and cause data loss.
	// Chunk writes to 64 bytes (standard USB Full Speed bulk endpoint size).
	// Only use chunking for native USB connections (USB-OTG, USB-JTAG/Serial);
	// UART bridge connections (CH340, CP2102 etc.) handle large writes fine
	// and chunking just adds unnecessary syscall overhead.
	if c.usesUSB {
		const maxChunk = 64
		for off := 0; off < len(frame); off += maxChunk {
			end := off + maxChunk
			if end > len(frame) {
				end = len(frame)
			}
			if _, err := c.port.Write(frame[off:end]); err != nil {
				return err
			}
		}
	} else {
		if _, err := c.port.Write(frame); err != nil {
			return err
		}
	}
	return nil
}

// commandResponse represents a parsed response from the ESP device.
type commandResponse struct {
	// Value is the 32-bit value field from the response header.
	Value uint32
	// Data is the response payload (after the 8-byte header).
	Data []byte
}

// command sends a command and reads the response.
// If waitResponse is false, the command is sent but no response is read.
//
// The ESP32-P4 ROM defers each reply until the NEXT command is received
// (one-command pipeline lag). We therefore send a follow-up SYNC "poke" to
// flush the queued reply, and (in ROM mode) Drain() the write so the command
// is fully transmitted before we read. The read loop is guarded by a
// wall-clock deadline and drains stale frames (poke replies, priming-sync
// leftovers). See wiki/esp32-p4-flash-solution.md.
func (c *conn) command(opcode byte, data []byte, chk uint32, timeout time.Duration, waitResponse bool) (*commandResponse, error) {
	if os.Getenv("EB_DEBUG") != "" {
		fmt.Printf("[DBG command] op=0x%02X waitResponse=%v stub=%v\n", opcode, waitResponse, c.stub)
	}
	if err := c.sendCommand(opcode, data, chk); err != nil {
		return nil, fmt.Errorf("send command 0x%02X: %w", opcode, err)
	}

	if !waitResponse {
		return nil, nil
	}

	// Per-chip follow-up. The ESP32-P4 ROM defers each reply until the next
	// command (one-command pipeline lag), so in ROM mode it needs a SYNC poke to
	// flush the queued reply and a Drain() to fully transmit the command before
	// reading; in stub mode the RAM stub replies immediately, so both are
	// skipped. Every other chip family uses a plain no-op follow-up, so it
	// never pokes or drains — this is what keeps the ESP32-S3 from hanging on
	// tcdrain during the initial sync. An unset post is treated as plain. See
	// wiki/esp32-p4-flash-solution.md.
	if c.post != nil {
		if err := c.post.apply(c); err != nil {
			return nil, err
		}
	}

	// Read loop guarded by a wall-clock deadline. Drain stale frames (poke
	// replies / priming-sync leftovers) until we find the matching opcode or
	// run out of time / tolerance.
	deadline := time.Now().Add(p4CommandDeadline)
	stale := 0
	for time.Now().Before(deadline) {
		// Check for context cancellation between retries
		if c.ctx != nil {
			select {
			case <-c.ctx.Done():
				return nil, fmt.Errorf("command cancelled: %w", c.ctx.Err())
			default:
			}
		}

		// Cap each read by the time left until the deadline so we fail fast
		// instead of looping up to 100 * timeout on a silent chip.
		readTimeout := timeout
		if rem := time.Until(deadline); rem < readTimeout {
			readTimeout = rem
		}
		if readTimeout <= 0 {
			readTimeout = 50 * time.Millisecond
		}

		frame, err := c.reader.ReadFrame(readTimeout)
		if err != nil {
			// Timeout (or any read error): the reply may simply be late, or we
			// are draining a stale frame. Count it and keep going.
			stale++
			if stale > p4MaxStaleFrames {
				break
			}
			continue
		}

		if len(frame) < 8 {
			stale++
			if stale > p4MaxStaleFrames {
				break
			}
			continue
		}

		resp := frame[0]
		opRet := frame[1]
		val := binary.LittleEndian.Uint32(frame[4:8])
		respData := frame[8:]

		if os.Getenv("EB_DEBUG") != "" {
			fmt.Printf("[DBG read] op=0x%02X resp=0x%02X val=0x%08X len=%d (stale=%d)\n", opRet, resp, val, len(respData), stale)
		}

		if resp != respDirectionResp {
			continue
		}

		if opRet != opcode {
			// Non-matching opcode: a stale poke reply or priming-sync
			// leftover. Count it like espflash does and stop if too many
			// accumulate, so a chip that never answers this opcode fails
			// fast instead of looping until the wall-clock deadline.
			stale++
			if stale > p4MaxStaleFrames {
				break
			}
			continue
		}

		return &commandResponse{
			Value: val,
			Data:  respData,
		}, nil
	}

	return nil, fmt.Errorf("no valid response for command 0x%02X after poke", opcode)
}

// checkCommand sends a command and checks the status in the response.
// Returns the response data on success, or an error if the status is non-zero.
// respDataLen specifies how many bytes of actual response data to expect before the status bytes.
func (c *conn) checkCommand(opDesc string, opcode byte, data []byte, chk uint32, timeout time.Duration, respDataLen int) ([]byte, error) {
	resp, err := c.command(opcode, data, chk, timeout, true)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", opDesc, err)
	}

	const statusLen = 2 // status bytes are 2 bytes

	if len(resp.Data) < respDataLen+statusLen {
		// Not enough data; check first 2 bytes as status
		if len(resp.Data) >= statusLen {
			if resp.Data[0] != 0 {
				return nil, &CommandError{
					OpCode:  opcode,
					Status:  resp.Data[0],
					ErrCode: resp.Data[1],
				}
			}
		}
		return nil, fmt.Errorf("%s: response too short (%d bytes)", opDesc, len(resp.Data))
	}

	// Status bytes are after the expected response data
	statusOff := respDataLen
	if resp.Data[statusOff] != 0 {
		return nil, &CommandError{
			OpCode:  opcode,
			Status:  resp.Data[statusOff],
			ErrCode: resp.Data[statusOff+1],
		}
	}

	if respDataLen > 0 {
		return resp.Data[:respDataLen], nil
	}

	// Return the 32-bit value from the header when no response data expected
	result := make([]byte, 4)
	binary.LittleEndian.PutUint32(result, resp.Value)
	return result, nil
}

// sync sends the SYNC command to synchronize with the ESP bootloader.
// Returns the response value to detect if a stub flasher is already running.
func (c *conn) sync() (uint32, error) {
	// Sync payload: 0x07 0x07 0x12 0x20 followed by 32 bytes of 0x55
	payload := make([]byte, 36)
	payload[0] = 0x07
	payload[1] = 0x07
	payload[2] = 0x12
	payload[3] = 0x20
	for i := 4; i < 36; i++ {
		payload[i] = 0x55
	}

	resp, err := c.command(cmdSync, payload, 0, syncTimeout, true)
	if err != nil {
		return 0, err
	}

	// The ROM bootloader sends up to 7 additional sync responses after the
	// first one, plus the reply to the SYNC "poke" that command() sent. Drain
	// them by reading frames directly (don't send new commands). Use a short
	// timeout: the responses should already be in the serial buffer or arrive
	// within a few milliseconds.
	for range 10 {
		if _, err := c.reader.ReadFrame(50 * time.Millisecond); err != nil {
			break // no more responses waiting — stop early
		}
	}

	return resp.Value, nil
}

// readReg reads a 32-bit register at the given address.
func (c *conn) readReg(addr uint32) (uint32, error) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, addr)

	result, err := c.checkCommand("read register", cmdReadReg, data, 0, defaultTimeout, 0)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(result), nil
}

// writeReg writes a 32-bit value to a register with an optional mask and delay.
func (c *conn) writeReg(addr, value, mask, delayUS uint32) error {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], addr)
	binary.LittleEndian.PutUint32(data[4:8], value)
	binary.LittleEndian.PutUint32(data[8:12], mask)
	binary.LittleEndian.PutUint32(data[12:16], delayUS)

	_, err := c.checkCommand("write register", cmdWriteReg, data, 0, defaultTimeout, 0)
	return err
}

// securityInfo reads security-related information from the device.
// Try with 20 bytes first (most chips), fallback to 12 bytes (ESP32-S2).
func (c *conn) securityInfo() ([]byte, error) {
	c.flushInput()
	data := make([]byte, 20)

	result, err := c.checkCommand("get security info", cmdSecurityInfoReg, data, 0, defaultTimeout, 20)
	if err == nil {
		// early return if successful with 20-byte response
		return result, nil
	}

	// fallback to 12-byte response for older ROMs (ESP32-S2)
	data = make([]byte, 12)
	result, err = c.checkCommand("get security info", cmdSecurityInfoReg, data, 0, defaultTimeout, 12)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// memBegin starts a RAM download operation.
func (c *conn) memBegin(size, blocks, blockSize, offset uint32) error {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], size)
	binary.LittleEndian.PutUint32(data[4:8], blocks)
	binary.LittleEndian.PutUint32(data[8:12], blockSize)
	binary.LittleEndian.PutUint32(data[12:16], offset)

	_, err := c.checkCommand("begin RAM download", cmdMemBegin, data, 0, defaultTimeout, 0)
	return err
}

// memData sends a block of data for RAM download.
func (c *conn) memData(block []byte, seq uint32) error {
	data := make([]byte, 16+len(block))
	binary.LittleEndian.PutUint32(data[0:4], uint32(len(block)))
	binary.LittleEndian.PutUint32(data[4:8], seq)
	binary.LittleEndian.PutUint32(data[8:12], 0)
	binary.LittleEndian.PutUint32(data[12:16], 0)
	copy(data[16:], block)

	_, err := c.checkCommand("write RAM block", cmdMemData, data, checksum(block), defaultTimeout, 0)
	return err
}

// memEnd finishes RAM download. If execute is true, starts execution at entrypoint.
func (c *conn) memEnd(execute bool, entrypoint uint32) error {
	data := make([]byte, 8)
	if !execute {
		binary.LittleEndian.PutUint32(data[0:4], 1) // 1 = don't execute
	}
	binary.LittleEndian.PutUint32(data[4:8], entrypoint)

	if execute {
		// When executing, ROM may reset before sending response
		return c.sendCommand(cmdMemEnd, data, 0)
	}

	_, err := c.checkCommand("finish RAM download", cmdMemEnd, data, 0, defaultTimeout, 0)
	return err
}

// flashBegin starts a flash download operation. Optionally performs an erase of the target region.
func (c *conn) flashBegin(size, offset uint32, encrypted bool, erase bool) error {
	writeSize := c.flashWriteSize()
	numBlocks := (size + writeSize - 1) / writeSize

	eraseSize := size
	if erase && !c.stub {
		eraseSize = (numBlocks * writeSize)
	} else if !erase {
		// No erase requested - use minimal size for timeout calculation
		eraseSize = size
	}

	// Use shorter timeout when erase is disabled
	var timeout time.Duration
	if erase {
		timeout = eraseTimeoutForSize(size)
	} else {
		timeout = defaultTimeout
	}

	// ESP32-S2 and newer ROM bootloaders support a 5th parameter (encrypted
	// flag). ESP8266 and original ESP32 ROM only accept 4 parameters (16 bytes).
	// Sending 20 bytes to those older ROMs causes error 0x05 (invalid message).
	paramLen := 16
	if c.supportsEncryptedFlash || c.stub {
		paramLen = 20
	}
	data := make([]byte, paramLen)
	binary.LittleEndian.PutUint32(data[0:4], eraseSize)
	binary.LittleEndian.PutUint32(data[4:8], numBlocks)
	binary.LittleEndian.PutUint32(data[8:12], writeSize)
	binary.LittleEndian.PutUint32(data[12:16], offset)
	if paramLen == 20 && encrypted {
		binary.LittleEndian.PutUint32(data[16:20], 1)
	}

	_, err := c.checkCommand("begin flash download", cmdFlashBegin, data, 0, timeout, 0)
	return err
}

// flashData sends a block of flash data.
func (c *conn) flashData(block []byte, seq uint32) error {
	data := make([]byte, 16+len(block))
	binary.LittleEndian.PutUint32(data[0:4], uint32(len(block)))
	binary.LittleEndian.PutUint32(data[4:8], seq)
	binary.LittleEndian.PutUint32(data[8:12], 0)
	binary.LittleEndian.PutUint32(data[12:16], 0)
	copy(data[16:], block)

	_, err := c.checkCommand("write flash block", cmdFlashData, data, checksum(block), defaultTimeout, 0)
	return err
}

// flashEnd finishes flash download. If reboot is true, the device reboots.
func (c *conn) flashEnd(reboot bool) error {
	data := make([]byte, 4)
	if !reboot {
		binary.LittleEndian.PutUint32(data, 1) // 1 = don't reboot
	}

	_, err := c.checkCommand("finish flash download", cmdFlashEnd, data, 0, defaultTimeout, 0)
	return err
}

// flashDeflBegin starts a compressed flash download.
func (c *conn) flashDeflBegin(uncompSize, compSize, offset uint32, encrypted bool, erase bool) error {
	writeSize := c.flashWriteSize()
	numBlocks := (compSize + writeSize - 1) / writeSize

	var writeArg uint32
	if c.stub {
		writeArg = uncompSize // stub handles erase internally
	} else if erase {
		eraseBlocks := (uncompSize + writeSize - 1) / writeSize
		writeArg = eraseBlocks * writeSize
	} else {
		writeArg = uncompSize // No erase - use actual size
	}

	// Use shorter timeout when erase is disabled
	var timeout time.Duration
	if erase {
		timeout = eraseTimeoutForSize(uncompSize)
	} else {
		timeout = defaultTimeout
	}

	// ESP32-S2 and newer ROM bootloaders support a 5th parameter (encrypted
	// flag). ESP8266 and original ESP32 ROM only accept 4 parameters (16 bytes).
	paramLen := 16
	if c.supportsEncryptedFlash || c.stub {
		paramLen = 20
	}
	data := make([]byte, paramLen)
	binary.LittleEndian.PutUint32(data[0:4], writeArg)
	binary.LittleEndian.PutUint32(data[4:8], numBlocks)
	binary.LittleEndian.PutUint32(data[8:12], writeSize)
	binary.LittleEndian.PutUint32(data[12:16], offset)
	if paramLen == 20 && encrypted {
		binary.LittleEndian.PutUint32(data[16:20], 1)
	}

	_, err := c.checkCommand("begin compressed flash download", cmdFlashDeflBeg, data, 0, timeout, 0)
	return err
}

// flashDeflData sends a block of compressed flash data.
func (c *conn) flashDeflData(block []byte, seq uint32) error {
	data := make([]byte, 16+len(block))
	binary.LittleEndian.PutUint32(data[0:4], uint32(len(block)))
	binary.LittleEndian.PutUint32(data[4:8], seq)
	binary.LittleEndian.PutUint32(data[8:12], 0)
	binary.LittleEndian.PutUint32(data[12:16], 0)
	copy(data[16:], block)

	_, err := c.checkCommand("write compressed flash block", cmdFlashDeflData, data, checksum(block), defaultTimeout, 0)
	return err
}

// flashDeflEnd finishes compressed flash download.
func (c *conn) flashDeflEnd(reboot bool) error {
	data := make([]byte, 4)
	if !reboot {
		binary.LittleEndian.PutUint32(data, 1)
	}

	_, err := c.checkCommand("finish compressed flash download", cmdFlashDeflEnd, data, 0, defaultTimeout, 0)
	return err
}

// spiAttach attaches the SPI flash.
// value=0 for default internal flash.
// The stub accepts 4 bytes; the ROM bootloader accepts 8 (extra 4 for HSPI config).
func (c *conn) spiAttach(value uint32) error {
	var data []byte
	if c.stub {
		data = make([]byte, 4)
	} else {
		data = make([]byte, 8)
	}
	binary.LittleEndian.PutUint32(data[0:4], value)

	_, err := c.checkCommand("attach SPI flash", cmdSPIAttach, data, 0, defaultTimeout, 0)
	return err
}

// spiSetParams configures the SPI flash chip parameters.
func (c *conn) spiSetParams(totalSize, blockSize, sectorSize, pageSize uint32) error {
	data := make([]byte, 24)
	binary.LittleEndian.PutUint32(data[0:4], 0)            // fl_id = auto
	binary.LittleEndian.PutUint32(data[4:8], totalSize)    // total_size
	binary.LittleEndian.PutUint32(data[8:12], blockSize)   // block_size
	binary.LittleEndian.PutUint32(data[12:16], sectorSize) // sector_size
	binary.LittleEndian.PutUint32(data[16:20], pageSize)   // page_size
	binary.LittleEndian.PutUint32(data[20:24], 0xFFFF)     // status_mask

	_, err := c.checkCommand("set SPI flash params", cmdSPISetParams, data, 0, defaultTimeout, 0)
	return err
}

// flashMD5 reads the MD5 hash of a flash region (stub-only).
func (c *conn) flashMD5(addr, size uint32) ([]byte, error) {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], addr)
	binary.LittleEndian.PutUint32(data[4:8], size)
	binary.LittleEndian.PutUint32(data[8:12], 0)
	binary.LittleEndian.PutUint32(data[12:16], 0)

	// MD5 can take a while for large regions
	timeout := md5Timeout
	if size > 1024*1024 {
		timeout = time.Duration(float64(md5Timeout) * float64(size) / float64(1024*1024))
	}

	result, err := c.checkCommand("flash MD5", cmdSPIFlashMD5, data, 0, timeout, 16)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// changeBaud changes the UART baud rate.
func (c *conn) changeBaud(newBaud, oldBaud uint32) error {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], newBaud)
	// ROM bootloader ignores the second parameter; stub uses it to know
	// the current baud rate. Send 0 for ROM to match esptool behavior.
	if c.stub {
		binary.LittleEndian.PutUint32(data[4:8], oldBaud)
	}

	_, err := c.checkCommand("change baud rate", cmdChangeBaud, data, 0, defaultTimeout, 0)
	return err
}

// eraseFlash erases the entire flash chip (stub-only).
func (c *conn) eraseFlash() error {
	_, err := c.checkCommand("erase flash", cmdEraseFlash, nil, 0, chipEraseTimeout, 0)
	return err
}

// eraseRegion erases a region of flash (stub-only).
func (c *conn) eraseRegion(offset, size uint32) error {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], offset)
	binary.LittleEndian.PutUint32(data[4:8], size)

	timeout := eraseTimeoutForSize(size)
	_, err := c.checkCommand("erase region", cmdEraseRegion, data, 0, timeout, 0)
	return err
}

// readFlash reads data from flash memory (stub-only).
func (c *conn) readFlash(offset, size uint32) ([]byte, error) {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], offset)
	binary.LittleEndian.PutUint32(data[4:8], size)
	binary.LittleEndian.PutUint32(data[8:12], readFlashBlockSize)
	binary.LittleEndian.PutUint32(data[12:16], 64) // max_inflight (stub clamps to 1)

	if _, err := c.checkCommand("read flash", cmdReadFlash, data, 0, defaultTimeout, 0); err != nil {
		return nil, err
	}

	blockTimeout := defaultTimeout + time.Duration(readFlashBlockSize/256)*100*time.Millisecond
	numBlocks := (size + readFlashBlockSize - 1) / readFlashBlockSize
	result := make([]byte, 0, size)

	for i := uint32(0); i < numBlocks; i++ {
		// Read SLIP-framed data block
		block, err := c.reader.ReadFrame(blockTimeout)
		if err != nil {
			return nil, fmt.Errorf("read flash block %d/%d: %w", i+1, numBlocks, err)
		}
		result = append(result, block...)

		// Send ACK: cumulative bytes received (SLIP-framed)
		ack := make([]byte, 4)
		binary.LittleEndian.PutUint32(ack, uint32(len(result)))
		ackFrame := slipEncode(ack)
		if _, err := c.port.Write(ackFrame); err != nil {
			return nil, fmt.Errorf("read flash ACK %d/%d: %w", i+1, numBlocks, err)
		}
	}

	// Read final 16-byte MD5 digest (SLIP-framed)
	_, err := c.reader.ReadFrame(defaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("read flash MD5: %w", err)
	}

	if uint32(len(result)) > size {
		result = result[:size]
	}
	return result, nil
}

// flashWriteSize returns the appropriate block size based on loader type.
func (c *conn) flashWriteSize() uint32 {
	if c.stub {
		return flashWriteSizeStub
	}
	return flashWriteSizeROM
}

// flushInput discards any unread data from the serial port and SLIP reader.
func (c *conn) flushInput() {
	c.port.ResetInputBuffer() //nolint:errcheck
	c.reader.reset()
}

// eraseTimeoutForSize calculates an appropriate timeout for erase operations.
func eraseTimeoutForSize(size uint32) time.Duration {
	// Base timeout + per-MB rate
	t := defaultTimeout + time.Duration(float64(eraseWritePerMBRate)*float64(size)/float64(1024*1024))
	if t < 10*time.Second {
		t = 10 * time.Second
	}
	return t
}

// loadStub uploads the stub flasher to RAM and executes it.
// The stub provides extended commands like erase_flash, erase_region, and
// compressed flash writes that are not available in the ROM bootloader.
func (c *conn) loadStub(s *stub) error {
	// Upload the text (code) segment.
	if err := c.uploadToRAM(s.text, s.textStart); err != nil {
		return fmt.Errorf("upload stub text: %w", err)
	}

	// Upload the data segment (if any).
	if len(s.data) > 0 {
		if err := c.uploadToRAM(s.data, s.dataStart); err != nil {
			return fmt.Errorf("upload stub data: %w", err)
		}
	}

	// Execute the stub at its entry point.
	// memEnd sends CMD_MEM_END without waiting for a response since the ROM
	// jumps to the entry point immediately and will not send one.
	if err := c.memEnd(true, s.entry); err != nil {
		return fmt.Errorf("execute stub: %w", err)
	}

	// The stub prints "OHAI" as plain bytes (not SLIP-encoded) when it starts.
	if err := c.waitForOHAI(); err != nil {
		return err
	}

	// Flush any leftover bytes, then reset the SLIP reader so it starts clean.
	c.port.ResetInputBuffer() //nolint:errcheck
	c.reader = newSlipReader(c.port, c.ctx)
	c.stub = true

	// Give the stub a brief moment to enter its command loop after the
	// MemEnd jump and to let the USB-JTAG/serial link settle. The very
	// first command sent immediately after OHAI is otherwise lost on the
	// P4, which previously left the stub appearing silent to the first
	// command. This mirrors espflash's load_stub() (see
	// wiki/esp32-p4-flash-solution.md).
	time.Sleep(250 * time.Millisecond)
	return nil
}

// uploadToRAM writes a binary segment to the device's RAM via mem_begin/mem_data.
func (c *conn) uploadToRAM(data []byte, addr uint32) error {
	// USB CDC endpoints have limited buffer sizes. Use 1KB blocks for
	// USB connections instead of the default 6KB to avoid timeout.
	blockSize := espRAMBlock
	if c.usesUSB {
		blockSize = 0x400 // 1KB
	}

	dataLen := uint32(len(data))
	numBlocks := (dataLen + blockSize - 1) / blockSize

	if err := c.memBegin(dataLen, numBlocks, blockSize, addr); err != nil {
		return err
	}

	seq := uint32(0)
	offset := uint32(0)
	for offset < dataLen {
		end := offset + blockSize
		if end > dataLen {
			end = dataLen
		}
		if err := c.memData(data[offset:end], seq); err != nil {
			return fmt.Errorf("block %d: %w", seq, err)
		}
		offset = end
		seq++
	}
	return nil
}

// waitForOHAI reads raw bytes from the serial port until the stub's "OHAI"
// startup greeting is received, confirming the stub is running.
func (c *conn) waitForOHAI() error {
	const maxBytes = 512
	var buf []byte
	single := make([]byte, 32)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// Check for context cancellation
		if c.ctx != nil {
			select {
			case <-c.ctx.Done():
				return fmt.Errorf("stub load cancelled: %w", c.ctx.Err())
			default:
			}
		}

		c.port.SetReadTimeout(100 * time.Millisecond) //nolint:errcheck
		n, err := c.port.Read(single)
		if n > 0 {
			buf = append(buf, single[:n]...)
			if bytes.Contains(buf, []byte("OHAI")) {
				return nil
			}
			// Avoid unbounded growth; keep only the last maxBytes.
			if len(buf) > maxBytes {
				buf = buf[len(buf)-maxBytes:]
			}
		}
		if err != nil && err != io.EOF {
			continue
		}
	}
	return fmt.Errorf("timeout waiting for stub greeting (OHAI)")
}
