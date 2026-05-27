package rom

import (
	"encoding/binary"
	"fmt"
)

const (
	ESP32S3_BLOCK1_BASE = 0x60007000 + 0x44
)

// ESP32S3Chip implements Chip interface for ESP32-S3
type ESP32S3Chip struct{}

// Name returns the chip name
func (c *ESP32S3Chip) Name() string {
	return "ESP32-S3"
}

// BaseAddress returns the BLOCK1 base address
func (c *ESP32S3Chip) BaseAddress() uint32 {
	return ESP32S3_BLOCK1_BASE
}

// MACRegister returns the MAC register offset
func (c *ESP32S3Chip) MACRegister() uint32 {
	return 0x0044 // EFUSE_RD_MAC_SPI_SYS_0_REG
}

// ReadMAC reads the factory MAC address from eFuse
func (c *ESP32S3Chip) ReadMAC(conn *Connection) (string, error) {
	mac0, err := conn.ReadReg(c.BaseAddress() + 0x44)
	if err != nil {
		return "", fmt.Errorf("read MAC0: %w", err)
	}
	mac1, err := conn.ReadReg(c.BaseAddress() + 0x48)
	if err != nil {
		return "", fmt.Errorf("read MAC1: %w", err)
	}

	// MAC format: mac1[15:0] + mac0[31:0]
	// Pack as Big Endian: mac0 (32 bits) + mac1[15:0] (16 bits)
	macBytes := make([]byte, 6)
	binary.BigEndian.PutUint32(macBytes[0:4], mac0)
	macBytes[4] = byte(mac1 >> 8)
	macBytes[5] = byte(mac1)

	return formatMAC(macBytes), nil
}

// ReadPSRAM reads PSRAM size and type from eFuse
func (c *ESP32S3Chip) ReadPSRAM(conn *Connection) (uint32, string, error) {
	// PSRAM capacity from BLOCK1 word 4 & 5
	word4, err := conn.ReadReg(c.BaseAddress() + 4*4)
	if err != nil {
		return 0, "", fmt.Errorf("read PSRAM word4: %w", err)
	}
	word5, err := conn.ReadReg(c.BaseAddress() + 4*5)
	if err != nil {
		return 0, "", fmt.Errorf("read PSRAM word5: %w", err)
	}

	// PSRAM capacity: (word5[19] << 2) | word4[3:4]
	capLo := (word4 >> 3) & 0x03
	capHi := (word5 >> 19) & 0x01
	cap := (capHi << 2) | capLo

	// PSRAM vendor: word4[7:8]
	vendor := (word4 >> 7) & 0x03

	// Map capacity to bytes
	sizeMap := map[uint32]uint32{
		0: 0, // None
		1: 8 * 1024 * 1024,
		2: 2 * 1024 * 1024,
		3: 16 * 1024 * 1024,
		4: 4 * 1024 * 1024,
	}
	size, ok := sizeMap[cap]
	if !ok {
		size = 0
	}

	// Map vendor to string
	vendorMap := map[uint32]string{
		0: "",
		1: "AP_3v3",
		2: "AP_1v8",
	}
	vendorStr, ok := vendorMap[vendor]
	if !ok {
		vendorStr = ""
	}

	return size, vendorStr, nil
}

// ReadFlash reads embedded flash size from eFuse
func (c *ESP32S3Chip) ReadFlash(conn *Connection) (uint32, error) {
	// Flash capacity from BLOCK1 word 3
	word3, err := conn.ReadReg(c.BaseAddress() + 4*3)
	if err != nil {
		return 0, fmt.Errorf("read flash word3: %w", err)
	}

	// Flash capacity: word3[27:29]
	cap := (word3 >> 27) & 0x07

	// Map to bytes
	sizeMap := map[uint32]uint32{
		0: 0, // None
		1: 8 * 1024 * 1024,
		2: 4 * 1024 * 1024,
	}
	size, ok := sizeMap[cap]
	if !ok {
		size = 0
	}

	return size, nil
}

// ReadRevision reads chip major and minor revision from eFuse
func (c *ESP32S3Chip) ReadRevision(conn *Connection) (uint8, uint8, error) {
	// Revision from BLOCK1 word 3 & 5
	word5, err := conn.ReadReg(c.BaseAddress() + 4*5)
	if err != nil {
		return 0, 0, fmt.Errorf("read revision word5: %w", err)
	}
	word3, err := conn.ReadReg(c.BaseAddress() + 4*3)
	if err != nil {
		return 0, 0, fmt.Errorf("read revision word3: %w", err)
	}

	// Major: word5[24:25]
	major := uint8((word5 >> 24) & 0x03)

	// Minor: (word5[23] << 3) | word3[18:20]
	minorHi := (word5 >> 23) & 0x01
	minorLo := (word3 >> 18) & 0x07
	minor := uint8((minorHi << 3) | minorLo)

	return major, minor, nil
}
