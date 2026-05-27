package rom

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"go.bug.st/serial"
)

// ROM protocol commands
const (
	CMD_SYNC              = 0x08
	CMD_READ_REG          = 0x0A
	CMD_GET_SECURITY_INFO = 0x14
)

// ROM protocol response values
const (
	ROM_INVALID_RECV_MSG = 0x05
	ROM_OK_RET           = 0x07
)

var (
	ErrSyncFailed      = errors.New("sync failed")
	ErrReadRegFailed   = errors.New("read register failed")
	ErrTimeout         = errors.New("timeout")
	ErrInvalidResponse = errors.New("invalid response")
)

// Chip represents an ESP chip with readable eFuse registers
type Chip interface {
	// Name returns the chip name
	Name() string
	// BaseAddress returns the BLOCK1 base address for this chip
	BaseAddress() uint32
	// MACRegister returns the offset of the MAC register
	MACRegister() uint32
	// ReadMAC reads the factory MAC address
	ReadMAC(conn *Connection) (string, error)
	// ReadPSRAM reads PSRAM size and type (returns 0, "" if not supported)
	ReadPSRAM(conn *Connection) (uint32, string, error)
	// ReadFlash reads embedded flash size (returns 0 if not supported)
	ReadFlash(conn *Connection) (uint32, error)
	// ReadRevision reads chip major and minor revision
	ReadRevision(conn *Connection) (uint8, uint8, error)
}

// Connection wraps a serial port with ESP ROM protocol
type Connection struct {
	port     serial.Port
	chip     Chip
	chipType string
	timeout  time.Duration
	debug    bool
}

// Config holds connection configuration
type Config struct {
	BaudRate int
	Timeout  time.Duration
	Debug    bool
}

// DefaultConfig returns default connection config
func DefaultConfig() *Config {
	return &Config{
		BaudRate: 115200,
		Timeout:  3 * time.Second,
		Debug:    false,
	}
}

// Open opens a serial connection for ROM communication
func Open(portPath string, cfg *Config) (*Connection, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	mode := &serial.Mode{
		BaudRate: cfg.BaudRate,
	}
	port, err := serial.Open(portPath, mode)
	if err != nil {
		return nil, fmt.Errorf("open port: %w", err)
	}

	return &Connection{
		port:    port,
		timeout: cfg.Timeout,
		debug:   cfg.Debug,
	}, nil
}

// Close closes the serial connection
func (c *Connection) Close() error {
	return c.port.Close()
}

// SetReadTimeout sets the read timeout for subsequent operations
func (c *Connection) SetReadTimeout(d time.Duration) {
	c.timeout = d
}

// read reads from the serial port with timeout
func (c *Connection) read(buf []byte) (int, error) {
	if err := c.port.SetReadTimeout(c.timeout); err != nil {
		return 0, err
	}
	return c.port.Read(buf)
}

// write writes to the serial port
func (c *Connection) write(data []byte) error {
	_, err := c.port.Write(data)
	return err
}

// flush reads any pending data from the port
func (c *Connection) flush() error {
	buf := make([]byte, 1024)
	c.port.SetReadTimeout(100 * time.Millisecond)
	for {
		n, err := c.port.Read(buf)
		if err != nil || n == 0 {
			break
		}
	}
	return nil
}

// Sync synchronizes with the ROM bootloader
func (c *Connection) Sync() error {
	// Flush any pending data
	c.flush()

	// Build sync command
	cmd := []byte{0x07, 0x07, 0x12, 0x20}
	sync := make([]byte, 36)
	copy(sync, cmd)

	for i := 0; i < 8; i++ {
		encoded := EncodeSLIP(sync)
		if c.debug {
			fmt.Printf("[SYNC] Sending %d bytes\n", len(encoded))
		}
		if err := c.write(encoded); err != nil {
			return fmt.Errorf("write sync: %w", err)
		}

		// Try to read response
		resp := make([]byte, 128)
		n, err := c.read(resp)
		if err != nil && !errors.Is(err, io.EOF) {
			continue
		}

		if n > 0 {
			decoded, err := DecodeSLIP(resp[:n])
			if err != nil {
				continue
			}
			if len(decoded) > 0 && decoded[0] == ROM_OK_RET {
				if c.debug {
					fmt.Printf("[SYNC] Sync successful\n")
				}
				return nil
			}
		}
	}

	return ErrSyncFailed
}

// ReadReg reads a 32-bit register value via ROM protocol
func (c *Connection) ReadReg(addr uint32) (uint32, error) {
	// Command: READ_REG (0x0A) + address (4 bytes)
	cmd := make([]byte, 8)
	cmd[0] = CMD_READ_REG
	binary.BigEndian.PutUint32(cmd[4:], addr)

	encoded := EncodeSLIP(cmd)
	if c.debug {
		fmt.Printf("[READ_REG] addr=0x%08x\n", addr)
	}
	if err := c.write(encoded); err != nil {
		return 0, fmt.Errorf("write read_reg: %w", err)
	}

	// Read response: command echo + value
	resp := make([]byte, 128)
	n, err := c.read(resp)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}

	decoded, err := DecodeSLIP(resp[:n])
	if err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	if len(decoded) < 8 {
		return 0, ErrInvalidResponse
	}

	// Check for error response
	if decoded[0] == ROM_INVALID_RECV_MSG || decoded[0] != CMD_READ_REG {
		return 0, ErrReadRegFailed
	}

	value := binary.BigEndian.Uint32(decoded[4:8])
	if c.debug {
		fmt.Printf("[READ_REG] addr=0x%08x value=0x%08x\n", addr, value)
	}
	return value, nil
}

// GetSecurityInfo reads security info (for chip detection on newer chips)
func (c *Connection) GetSecurityInfo() (uint32, error) {
	cmd := []byte{CMD_GET_SECURITY_INFO, 0x00, 0x00, 0x00}
	encoded := EncodeSLIP(cmd)
	if c.debug {
		fmt.Printf("[GET_SECURITY_INFO] Sending\n")
	}
	if err := c.write(encoded); err != nil {
		return 0, fmt.Errorf("write get_security_info: %w", err)
	}

	resp := make([]byte, 128)
	n, err := c.read(resp)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}

	decoded, err := DecodeSLIP(resp[:n])
	if err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	if len(decoded) < 12 {
		return 0, ErrInvalidResponse
	}

	// Response format: flags(4) + chip_id(4) + api_version(4)
	chipID := binary.BigEndian.Uint32(decoded[4:8])
	if c.debug {
		fmt.Printf("[GET_SECURITY_INFO] chip_id=0x%08x\n", chipID)
	}
	return chipID, nil
}

// DetectChip detects the chip type using GET_SECURITY_INFO or magic register
func (c *Connection) DetectChip() error {
	// First try GET_SECURITY_INFO (works on ESP32-S2, S3, C3, P4, etc.)
	chipID, err := c.GetSecurityInfo()
	if err == nil && chipID != 0 {
		chip := DetectBySecurityID(chipID)
		if chip != nil {
			c.chip = chip
			c.chipType = chip.Name()
			if c.debug {
				fmt.Printf("[DETECT] Chip: %s (security_id=0x%08x)\n", c.chipType, chipID)
			}
			return nil
		}
	}

	// Fall back to magic register read
	// Try common magic register locations
	for _, addr := range []uint32{0x40001000, 0x3FF000E4, 0x60007000} {
		magic, err := c.ReadReg(addr)
		if err == nil {
			chip := DetectByMagic(magic)
			if chip != nil {
				c.chip = chip
				c.chipType = chip.Name()
				if c.debug {
					fmt.Printf("[DETECT] Chip: %s (magic=0x%08x @ 0x%08x)\n", c.chipType, magic, addr)
				}
				return nil
			}
		}
	}

	return fmt.Errorf("unable to detect chip")
}

// Chip returns the detected chip implementation
func (c *Connection) Chip() Chip {
	return c.chip
}

// ChipType returns the detected chip type name
func (c *Connection) ChipType() string {
	return c.chipType
}

// ReadMAC reads the factory MAC address
func (c *Connection) ReadMAC() (string, error) {
	if c.chip == nil {
		return "", fmt.Errorf("chip not detected")
	}
	return c.chip.ReadMAC(c)
}

// ReadPSRAM reads PSRAM size and type
func (c *Connection) ReadPSRAM() (uint32, string, error) {
	if c.chip == nil {
		return 0, "", fmt.Errorf("chip not detected")
	}
	return c.chip.ReadPSRAM(c)
}

// ReadFlash reads embedded flash size
func (c *Connection) ReadFlash() (uint32, error) {
	if c.chip == nil {
		return 0, fmt.Errorf("chip not detected")
	}
	return c.chip.ReadFlash(c)
}

// ReadRevision reads chip revision
func (c *Connection) ReadRevision() (uint8, uint8, error) {
	if c.chip == nil {
		return 0, 0, fmt.Errorf("chip not detected")
	}
	return c.chip.ReadRevision(c)
}
