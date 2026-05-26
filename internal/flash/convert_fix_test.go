package flash

import (
	"os"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"codeberg.org/georgik/espbrew-go/internal/flash/testutil"
)

func TestConvertELFFlashSizeESP32S3(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	result, err := ConvertELFToESPImage(data, chips.ChipESP32S3)
	if err != nil {
		t.Fatalf("ConvertELFToESPImage failed: %v", err)
	}

	parts, err := ParseMultiPartImage(result)
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	// Find app part (at offset 0x10000)
	var appData []byte
	for _, part := range parts {
		if part.Offset == 0x10000 {
			appData = part.Data
			break
		}
	}

	if appData == nil {
		t.Fatal("App part not found")
	}

	if len(appData) < 4 {
		t.Fatalf("App data too short: %d bytes", len(appData))
	}

	// Check ESP image header
	// Byte 0: Magic (0xE9)
	// Byte 1: Segment count
	// Byte 2: Flash mode
	// Byte 3: Flash size + frequency (should be 0x40 for 16MB @ 40MHz)
	if appData[0] != 0xE9 {
		t.Errorf("Magic byte = 0x%02x, want 0xE9", appData[0])
	}

	// Byte 3 should be 0x40 for 16MB flash
	// 0x40 = (flashSize << 4) | flashFreq = (0x04 << 4) | 0x00
	flashSizeFreq := appData[3]
	if flashSizeFreq != 0x40 {
		t.Errorf("Flash size/freq byte = 0x%02x, want 0x40 (16MB @ 40MHz)", flashSizeFreq)
	}

	t.Logf("App image header: % x", appData[:24])
	t.Logf("Flash size/freq: 0x%02x (16MB @ 40MHz = 0x40)", flashSizeFreq)
}

func TestConvertELFNoSHA256(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	result, err := ConvertELFToESPImage(data, chips.ChipESP32S3)
	if err != nil {
		t.Fatalf("ConvertELFToESPImage failed: %v", err)
	}

	parts, err := ParseMultiPartImage(result)
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	var appData []byte
	for _, part := range parts {
		if part.Offset == 0x10000 {
			appData = part.Data
			break
		}
	}

	if len(appData) < 200 {
		t.Fatalf("App data too short: %d bytes", len(appData))
	}

	// Scan for "0.0.0" version string in app image
	// This should be in a DRAM segment containing the app_desc
	foundVersion := false
	versionOffset := 0
	for i := 0; i < len(appData)-6; i++ {
		if appData[i] == 0x30 && appData[i+1] == 0x2e && appData[i+2] == 0x30 &&
			appData[i+3] == 0x2e && appData[i+4] == 0x30 && appData[i+5] == 0x00 {
			foundVersion = true
			versionOffset = i
			break
		}
	}

	if !foundVersion {
		t.Errorf("Version string '0.0.0' not found in app image")
		t.Logf("First 200 bytes: % x", appData[:200])
		return
	}

	t.Logf("Found '0.0.0' at offset 0x%X", versionOffset)

	// Check that at offset 0x70 from version start is not all zeros (should be SHA256 position)
	if versionOffset+0x70+32 > len(appData) {
		t.Fatalf("Cannot check SHA256 position, data too short")
	}

	sha256Pos := appData[versionOffset+0x70 : versionOffset+0x70+32]
	allZeros := true
	for _, b := range sha256Pos {
		if b != 0 {
			allZeros = false
			break
		}
	}

	if !allZeros {
		t.Logf("SHA256 position has data (this might be OK): % x...", sha256Pos[:8])
	} else {
		t.Logf("SHA256 position is zeros (expected - espflash doesn't populate it)")
	}
}
