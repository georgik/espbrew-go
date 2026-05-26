package flash

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/chips"
)

// TestHardwareFlashAndVerify tests flashing and reading back to verify
func TestHardwareFlashAndVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hardware test in short mode")
	}

	port := os.Getenv("ESPBREW_TEST_PORT")
	if port == "" {
		t.Skip("Set ESPBREW_TEST_PORT")
	}

	elfPath := os.Getenv("ESPBREW_TEST_ELF")
	if elfPath == "" {
		t.Skip("Set ESPBREW_TEST_ELF")
	}

	elfData, err := os.ReadFile(elfPath)
	if err != nil {
		t.Fatalf("Failed to read ELF: %v", err)
	}

	flasher := NewFlasher(&FlasherOptions{
		BaudRate:      115200,
		FlashBaudRate: 460800,
		Compress:      true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Flash the ELF
	req := &FlashRequest{
		Port:     port,
		Firmware: elfData,
		Chip:     chips.ChipESP32S3,
	}

	result := flasher.Flash(ctx, req)
	if !result.Success {
		t.Fatalf("Flash failed: %v", result.Error)
	}
	t.Logf("Flash successful: %d bytes", result.Bytes)

	// Convert ELF to get source data for MD5 comparison
	imgData, err := ConvertELFToESPImage(elfData, chips.ChipESP32S3)
	if err != nil {
		t.Fatalf("Failed to convert ELF: %v", err)
	}

	parts, _ := ParseMultiPartImage(imgData)
	var appData []byte
	for _, p := range parts {
		if p.Offset == 0x10000 {
			appData = p.Data
			break
		}
	}

	if len(appData) == 0 {
		t.Fatal("No app data found in converted image")
	}

	expectedMD5 := md5.Sum(appData)
	t.Logf("Expected MD5 of app data: %s", hex.EncodeToString(expectedMD5[:]))

	// Read back the app region (full size for accurate MD5 comparison)
	// Pad to 4KB boundary for flash read alignment
	readSize := ((len(appData) + 4095) / 4096) * 4096
	if readSize > 4*1024*1024 {
		readSize = 4 * 1024 * 1024 // Cap at 4MB for safety
	}

	readReq := &ReadFlashRequest{
		Port:    port,
		Address: 0x10000,
		Size:    uint32(readSize),
		Chip:    chips.ChipESP32S3,
	}

	readResult := flasher.ReadFlash(ctx, readReq)
	if !readResult.Success {
		t.Fatalf("Read flash failed: %v", readResult.Error)
	}

	t.Logf("Read %d bytes from 0x10000", len(readResult.Data))

	// Compare only the actual app data length
	compareLen := len(appData)
	if len(readResult.Data) < compareLen {
		compareLen = len(readResult.Data)
	}

	actualMD5 := md5.Sum(readResult.Data[:compareLen])
	t.Logf("Actual MD5 of read-back data (%d bytes): %s", compareLen, hex.EncodeToString(actualMD5[:]))

	if expectedMD5 != actualMD5 {
		t.Errorf("MD5 mismatch! Expected %s, got %s", hex.EncodeToString(expectedMD5[:]), hex.EncodeToString(actualMD5[:]))
	}

	// Check ESP image magic
	if len(readResult.Data) < 4 {
		t.Fatalf("Not enough data read: %d bytes", len(readResult.Data))
	}

	if readResult.Data[0] != 0xE9 {
		t.Errorf("ESP image magic not found: got 0x%02x, expected 0xE9", readResult.Data[0])
	}

	t.Logf("ESP Header: magic=0x%02x, segments=%d, mode=0x%02x",
		readResult.Data[0], readResult.Data[1], readResult.Data[2])

	// Parse segment headers from read-back data
	if len(readResult.Data) >= 24 {
		t.Logf("Segment headers from read-back:")
		offset := 24
		for i := 0; i < int(readResult.Data[1]) && offset+8 <= len(readResult.Data); i++ {
			addr := uint32(readResult.Data[offset]) | uint32(readResult.Data[offset+1])<<8 |
				uint32(readResult.Data[offset+2])<<16 | uint32(readResult.Data[offset+3])<<24
			length := uint32(readResult.Data[offset+4]) | uint32(readResult.Data[offset+5])<<8 |
				uint32(readResult.Data[offset+6])<<16 | uint32(readResult.Data[offset+7])<<24
			t.Logf("  Seg[%d]: addr=0x%08x, len=%d", i, addr, length)
			offset += 8 + int(length)
		}
	}

	// Count non-zero bytes
	nonZero := 0
	for _, b := range readResult.Data {
		if b != 0 {
			nonZero++
		}
	}
	t.Logf("Non-zero bytes in first %d: %d", len(readResult.Data), nonZero)

	if nonZero < 1000 {
		t.Errorf("Too few non-zero bytes: %d (data may be corrupted)", nonZero)
	}
}
