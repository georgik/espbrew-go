package flash

import (
	"context"
	"os"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"codeberg.org/georgik/espbrew-go/internal/flash/testutil"
)

func TestVirtualFlasherELF(t *testing.T) {
	// Skip if no test ELF available
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	// Set temp dir for virtual device
	oldHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	// Flash to virtual device
	flasher := NewFlasher(nil)
	req := &FlashRequest{
		Port:     "wokwi-esp32s3",
		Firmware: data,
		Chip:     chips.ChipESP32S3,
	}

	result := flasher.Flash(context.Background(), req)

	if !result.Success {
		t.Fatalf("Flash failed: %v", result.Error)
	}

	if result.Bytes == 0 {
		t.Error("No bytes written")
	}

	t.Logf("Virtual flash successful: %d bytes written", result.Bytes)

	// TODO: Verify flash contents by reading back from virtual device
	// This would require adding ReadFlash support for virtual devices
}
