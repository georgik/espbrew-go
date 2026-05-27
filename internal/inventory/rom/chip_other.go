package rom

import (
	"fmt"
)

// ESP32S2Chip implements Chip interface for ESP32-S2
type ESP32S2Chip struct{}

func (c *ESP32S2Chip) Name() string {
	return "ESP32-S2"
}

func (c *ESP32S2Chip) BaseAddress() uint32 {
	return 0x60007000 + 0x44
}

func (c *ESP32S2Chip) MACRegister() uint32 {
	return 0x44
}

func (c *ESP32S2Chip) ReadMAC(conn *Connection) (string, error) {
	// Similar to S3 but without PSRAM
	mac0, err := conn.ReadReg(c.BaseAddress() + 0x44)
	if err != nil {
		return "", fmt.Errorf("read MAC0: %w", err)
	}
	mac1, err := conn.ReadReg(c.BaseAddress() + 0x48)
	if err != nil {
		return "", fmt.Errorf("read MAC1: %w", err)
	}
	macBytes := make([]byte, 6)
	macBytes[0] = byte(mac0)
	macBytes[1] = byte(mac0 >> 8)
	macBytes[2] = byte(mac0 >> 16)
	macBytes[3] = byte(mac0 >> 24)
	macBytes[4] = byte(mac1)
	macBytes[5] = byte(mac1 >> 8)
	return formatMAC(macBytes), nil
}

func (c *ESP32S2Chip) ReadPSRAM(conn *Connection) (uint32, string, error) {
	return 0, "", nil
}

func (c *ESP32S2Chip) ReadFlash(conn *Connection) (uint32, error) {
	return 0, nil
}

func (c *ESP32S2Chip) ReadRevision(conn *Connection) (uint8, uint8, error) {
	return 0, 0, nil
}

// ESP32C3Chip implements Chip interface for ESP32-C3
type ESP32C3Chip struct{}

func (c *ESP32C3Chip) Name() string {
	return "ESP32-C3"
}

func (c *ESP32C3Chip) BaseAddress() uint32 {
	return 0x60008800 + 0x044
}

func (c *ESP32C3Chip) MACRegister() uint32 {
	return 0x44
}

func (c *ESP32C3Chip) ReadMAC(conn *Connection) (string, error) {
	mac0, err := conn.ReadReg(c.BaseAddress() + 0x44)
	if err != nil {
		return "", fmt.Errorf("read MAC0: %w", err)
	}
	mac1, err := conn.ReadReg(c.BaseAddress() + 0x48)
	if err != nil {
		return "", fmt.Errorf("read MAC1: %w", err)
	}
	macBytes := make([]byte, 6)
	macBytes[0] = byte(mac0)
	macBytes[1] = byte(mac0 >> 8)
	macBytes[2] = byte(mac0 >> 16)
	macBytes[3] = byte(mac0 >> 24)
	macBytes[4] = byte(mac1)
	macBytes[5] = byte(mac1 >> 8)
	return formatMAC(macBytes), nil
}

func (c *ESP32C3Chip) ReadPSRAM(conn *Connection) (uint32, string, error) {
	return 0, "", nil // ESP32-C3 doesn't have embedded PSRAM
}

func (c *ESP32C3Chip) ReadFlash(conn *Connection) (uint32, error) {
	word3, err := conn.ReadReg(c.BaseAddress() + 4*3)
	if err != nil {
		return 0, fmt.Errorf("read flash word3: %w", err)
	}
	cap := (word3 >> 27) & 0x07
	sizeMap := map[uint32]uint32{
		0: 0,
		1: 8 * 1024 * 1024,
		2: 4 * 1024 * 1024,
	}
	size, ok := sizeMap[cap]
	if !ok {
		size = 0
	}
	return size, nil
}

func (c *ESP32C3Chip) ReadRevision(conn *Connection) (uint8, uint8, error) {
	word5, err := conn.ReadReg(c.BaseAddress() + 4*5)
	if err != nil {
		return 0, 0, fmt.Errorf("read revision word5: %w", err)
	}
	word3, err := conn.ReadReg(c.BaseAddress() + 4*3)
	if err != nil {
		return 0, 0, fmt.Errorf("read revision word3: %w", err)
	}
	major := uint8((word5 >> 24) & 0x03)
	minorHi := (word5 >> 23) & 0x01
	minorLo := (word3 >> 18) & 0x07
	minor := uint8((minorHi << 3) | minorLo)
	return major, minor, nil
}

// ESP32C6Chip implements Chip interface for ESP32-C6
type ESP32C6Chip struct{}

func (c *ESP32C6Chip) Name() string {
	return "ESP32-C6"
}

func (c *ESP32C6Chip) BaseAddress() uint32 {
	return 0x600B0800 + 0x044
}

func (c *ESP32C6Chip) MACRegister() uint32 {
	return 0x44
}

func (c *ESP32C6Chip) ReadMAC(conn *Connection) (string, error) {
	mac0, err := conn.ReadReg(c.BaseAddress() + 0x44)
	if err != nil {
		return "", fmt.Errorf("read MAC0: %w", err)
	}
	mac1, err := conn.ReadReg(c.BaseAddress() + 0x48)
	if err != nil {
		return "", fmt.Errorf("read MAC1: %w", err)
	}
	macBytes := make([]byte, 6)
	macBytes[0] = byte(mac0)
	macBytes[1] = byte(mac0 >> 8)
	macBytes[2] = byte(mac0 >> 16)
	macBytes[3] = byte(mac0 >> 24)
	macBytes[4] = byte(mac1)
	macBytes[5] = byte(mac1 >> 8)
	return formatMAC(macBytes), nil
}

func (c *ESP32C6Chip) ReadPSRAM(conn *Connection) (uint32, string, error) {
	return 0, "", nil
}

func (c *ESP32C6Chip) ReadFlash(conn *Connection) (uint32, error) {
	word4, err := conn.ReadReg(c.BaseAddress() + 4*4)
	if err != nil {
		return 0, fmt.Errorf("read flash word4: %w", err)
	}
	cap := word4 & 0x07
	sizeMap := map[uint32]uint32{
		0: 0,
		1: 8 * 1024 * 1024,
		2: 4 * 1024 * 1024,
	}
	size, ok := sizeMap[cap]
	if !ok {
		size = 0
	}
	return size, nil
}

func (c *ESP32C6Chip) ReadRevision(conn *Connection) (uint8, uint8, error) {
	word3, err := conn.ReadReg(c.BaseAddress() + 4*3)
	if err != nil {
		return 0, 0, fmt.Errorf("read revision word3: %w", err)
	}
	major := uint8((word3 >> 22) & 0x03)
	minor := uint8((word3 >> 18) & 0x0F)
	return major, minor, nil
}

// ESP32H2Chip implements Chip interface for ESP32-H2
type ESP32H2Chip struct{}

func (c *ESP32H2Chip) Name() string {
	return "ESP32-H2"
}

func (c *ESP32H2Chip) BaseAddress() uint32 {
	return 0x6000E000 + 0x044
}

func (c *ESP32H2Chip) MACRegister() uint32 {
	return 0x44
}

func (c *ESP32H2Chip) ReadMAC(conn *Connection) (string, error) {
	return "", fmt.Errorf("MAC read not implemented for ESP32-H2")
}

func (c *ESP32H2Chip) ReadPSRAM(conn *Connection) (uint32, string, error) {
	return 0, "", nil
}

func (c *ESP32H2Chip) ReadFlash(conn *Connection) (uint32, error) {
	return 0, nil
}

func (c *ESP32H2Chip) ReadRevision(conn *Connection) (uint8, uint8, error) {
	return 0, 0, nil
}

// ESP32P4Chip implements Chip interface for ESP32-P4
type ESP32P4Chip struct{}

func (c *ESP32P4Chip) Name() string {
	return "ESP32-P4"
}

func (c *ESP32P4Chip) BaseAddress() uint32 {
	return 0x600C0000 + 0x044
}

func (c *ESP32P4Chip) MACRegister() uint32 {
	return 0x44
}

func (c *ESP32P4Chip) ReadMAC(conn *Connection) (string, error) {
	return "", fmt.Errorf("MAC read not implemented for ESP32-P4")
}

func (c *ESP32P4Chip) ReadPSRAM(conn *Connection) (uint32, string, error) {
	return 0, "", nil
}

func (c *ESP32P4Chip) ReadFlash(conn *Connection) (uint32, error) {
	return 0, nil
}

func (c *ESP32P4Chip) ReadRevision(conn *Connection) (uint8, uint8, error) {
	return 0, 0, nil
}
