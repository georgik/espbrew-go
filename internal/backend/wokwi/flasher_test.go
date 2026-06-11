package wokwi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

func TestNewFlasher(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1}`,
		},
	}

	flasher, err := NewFlasher(device)
	if err != nil {
		t.Fatalf("NewFlasher failed: %v", err)
	}

	if flasher == nil {
		t.Fatal("Expected flasher, got nil")
	}

	wokwiFlasher, ok := flasher.(*Flasher)
	if !ok {
		t.Fatal("Expected Wokwi Flasher, got different type")
	}

	if wokwiFlasher.config.ChipType != "ESP32" {
		t.Errorf("Expected ChipType ESP32, got %s", wokwiFlasher.config.ChipType)
	}
}

func TestNewFlasher_InvalidBackend(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-physical",
		Backend:  protocol.BackendPhysical,
	}

	_, err := NewFlasher(device)
	if err == nil {
		t.Error("Expected error for physical backend, got nil")
	}
}

func TestNewFlasher_MissingConfig(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		// No BackendConfig
	}

	_, err := NewFlasher(device)
	if err == nil {
		t.Error("Expected error for missing config, got nil")
	}
}

func TestFlasher_Flash(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1}`,
		},
	}

	flasher, err := NewFlasher(device)
	if err != nil {
		t.Fatalf("NewFlasher failed: %v", err)
	}

	// Create a temporary firmware file
	tmpDir := t.TempDir()
	firmwarePath := filepath.Join(tmpDir, "test.bin")
	testData := []byte("test firmware data")
	if err := os.WriteFile(firmwarePath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test firmware: %v", err)
	}

	ctx := context.Background()
	progress := make(chan int, 10)

	err = flasher.Flash(ctx, firmwarePath, progress)
	if err != nil {
		t.Errorf("Flash failed: %v", err)
	}

	// Check progress
	progressCount := 0
	for {
		select {
		case p := <-progress:
			if p < 0 || p > 100 {
				t.Errorf("Invalid progress value: %d", p)
			}
			progressCount++
		default:
			goto done
		}
	}
done:

	if progressCount == 0 {
		t.Error("Expected progress updates, got none")
	}

	// Verify firmware was stored
	wokwiFlasher, ok := flasher.(*Flasher)
	if !ok {
		t.Fatal("Expected Wokwi Flasher, got different type")
	}
	storedPath := wokwiFlasher.GetFirmwarePath(firmwarePath)
	if _, err := os.Stat(storedPath); os.IsNotExist(err) {
		t.Errorf("Firmware not stored at %s", storedPath)
	}
}

func TestFlasher_Flash_NonexistentFile(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1}`,
		},
	}

	flasher, err := NewFlasher(device)
	if err != nil {
		t.Fatalf("NewFlasher failed: %v", err)
	}

	ctx := context.Background()
	progress := make(chan int, 10)

	err = flasher.Flash(ctx, "/nonexistent/file.bin", progress)
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestFlasher_ReadFlash_NotSupported(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1}`,
		},
	}

	flasher, err := NewFlasher(device)
	if err != nil {
		t.Fatalf("NewFlasher failed: %v", err)
	}

	ctx := context.Background()
	_, err = flasher.ReadFlash(ctx, 0, 1024)
	if err == nil {
		t.Error("Expected error for ReadFlash (not supported), got nil")
	}
}

func TestFlasher_GetFirmwarePath(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1}`,
		},
	}

	flasher, err := NewFlasher(device)
	if err != nil {
		t.Fatalf("NewFlasher failed: %v", err)
	}

	wokwiFlasher, ok := flasher.(*Flasher)
	if !ok {
		t.Fatal("Expected Wokwi Flasher, got different type")
	}

	sourcePath := "/tmp/test.bin"
	firmwarePath := wokwiFlasher.GetFirmwarePath(sourcePath)

	if firmwarePath == "" {
		t.Error("Expected firmware path, got empty string")
	}

	if filepath.Base(firmwarePath) == filepath.Base(sourcePath) {
		// Path should include timestamp, not just base name
		t.Log("Firmware path:", firmwarePath)
	}
}

func TestFlasher_GetStoredFirmwares(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1}`,
		},
	}

	flasher, err := NewFlasher(device)
	if err != nil {
		t.Fatalf("NewFlasher failed: %v", err)
	}

	wokwiFlasher, ok := flasher.(*Flasher)
	if !ok {
		t.Fatal("Expected Wokwi Flasher, got different type")
	}

	// Create test firmware files
	tmpDir := t.TempDir()
	wokwiFlasher.firmwareDir = tmpDir

	// Create test files
	testFiles := []string{"test1.bin", "test2.elf", "test3.json"}
	for _, file := range testFiles {
		path := filepath.Join(tmpDir, file)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Create a non-firmware file
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("readme"), 0644); err != nil {
		t.Fatalf("Failed to create readme: %v", err)
	}

	firmwares, err := wokwiFlasher.GetStoredFirmwares()
	if err != nil {
		t.Errorf("GetStoredFirmwares failed: %v", err)
	}

	// Should only return firmware files (.bin, .elf, .json)
	if len(firmwares) != len(testFiles) {
		t.Errorf("Expected %d firmwares, got %d", len(testFiles), len(firmwares))
	}
}

func TestFlasher_Cleanup(t *testing.T) {
	device := &protocol.DeviceInfo{
		DeviceID: "test-wokwi",
		Backend:  protocol.BackendWokwi,
		BackendConfig: &protocol.WokwiConfig{
			ChipType:    "ESP32",
			DiagramJSON: `{"version":1}`,
		},
	}

	flasher, err := NewFlasher(device)
	if err != nil {
		t.Fatalf("NewFlasher failed: %v", err)
	}

	wokwiFlasher, ok := flasher.(*Flasher)
	if !ok {
		t.Fatal("Expected Wokwi Flasher, got different type")
	}

	// Cleanup should not error
	err = wokwiFlasher.Cleanup(0)
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}
}
