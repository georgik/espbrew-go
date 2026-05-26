package bootloaders

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/chips"
)

// TestHTTPDirectAccess verifies URL is accessible via HTTP
// Run with: go test -v ./internal/flash/bootloaders -run TestHTTPDirectAccess
func TestHTTPDirectAccess(t *testing.T) {
	url := "https://raw.githubusercontent.com/esp-rs/espflash/v3.0.0/espflash/resources/bootloaders/esp32s3-bootloader.bin"

	// Test 1: Default http.Get
	t.Run("default_get", func(t *testing.T) {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("HTTP GET failed: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("URL: %s", url)
		t.Logf("Status: %s", resp.Status)
		t.Logf("StatusCode: %d", resp.StatusCode)
		t.Logf("ContentLength: %d bytes", resp.ContentLength)

		if resp.StatusCode != 200 {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestDownloadBootloader(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Use temp cache dir for testing
	tmpDir := t.TempDir()
	mgr, err := NewManager(ManagerConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Test downloading ESP32-S3 bootloader
	chip := chips.ChipESP32S3
	data, info, err := mgr.GetBootloader(chip)
	if err != nil {
		t.Fatalf("Failed to download bootloader for %s: %v", chip, err)
	}

	// Verify data was downloaded
	if len(data) == 0 {
		t.Fatal("Downloaded bootloader is empty")
	}

	t.Logf("Downloaded bootloader for %s: %d bytes", chip, len(data))
	t.Logf("Info: %+v", info)

	// Verify file was cached
	expectedFile := "esp32s3-bootloader.bin"
	cachedPath := filepath.Join(tmpDir, expectedFile)
	if _, err := os.Stat(cachedPath); os.IsNotExist(err) {
		t.Errorf("Bootloader not cached at %s", cachedPath)
	}

	// Verify magic bytes (ESP32 binary starts with 0xE9)
	if data[0] != 0xE9 {
		t.Errorf("Invalid bootloader magic: expected 0xE9, got 0x%02X", data[0])
	}
}

func TestDownloadAllBootloaders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	tmpDir := t.TempDir()
	mgr, err := NewManager(ManagerConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	testChips := []chips.Chip{
		chips.ChipESP32,
		chips.ChipESP32S2,
		chips.ChipESP32S3,
		chips.ChipESP32C3,
		chips.ChipESP32C6,
	}

	for _, chip := range testChips {
		t.Run(chip.String(), func(t *testing.T) {
			data, info, err := mgr.GetBootloader(chip)
			if err != nil {
				t.Fatalf("Failed to download bootloader for %s: %v", chip, err)
			}

			if len(data) < 10000 {
				t.Errorf("Bootloader too small for %s: %d bytes", chip, len(data))
			}

			if data[0] != 0xE9 {
				t.Errorf("Invalid magic for %s: expected 0xE9, got 0x%02X", chip, data[0])
			}

			t.Logf("%s: %d bytes, source=%s", chip, len(data), info.Source)
		})
	}
}

func TestBootloaderCacheHit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	tmpDir := t.TempDir()
	mgr, err := NewManager(ManagerConfig{CacheDir: tmpDir})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	chip := chips.ChipESP32S3

	// First download
	data1, info1, err := mgr.GetBootloader(chip)
	if err != nil {
		t.Fatalf("First download failed: %v", err)
	}

	if info1.Source != "downloaded" {
		t.Errorf("Expected source='downloaded', got '%s'", info1.Source)
	}

	// Second download should hit cache
	data2, info2, err := mgr.GetBootloader(chip)
	if err != nil {
		t.Fatalf("Second download failed: %v", err)
	}

	if info2.Source != "cached" {
		t.Errorf("Expected source='cached', got '%s'", info2.Source)
	}

	if len(data1) != len(data2) {
		t.Error("Cached data size differs from original")
	}
}
