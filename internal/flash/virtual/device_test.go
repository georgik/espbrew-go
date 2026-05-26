package virtual

import (
	"bytes"
	"os"
	"testing"
)

func TestVirtualFlashRoundtrip(t *testing.T) {
	// Use temp dir for test isolation
	oldHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	testData := []byte{0xE9, 0x05, 0x02, 0x20, 0xAA, 0xBB, 0xCC, 0xDD}

	// Open device (will be created in temp dir)
	device, err := OpenDevice("test-roundtrip")
	if err != nil {
		t.Fatalf("Failed to open device: %v", err)
	}
	defer device.Close()

	// Write test data at offset 0x10000
	err = device.Write(0x10000, testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Read back
	readBack, err := device.Read(0x10000, uint32(len(testData)))
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if !bytes.Equal(testData, readBack) {
		t.Errorf("Roundtrip failed: got % x, want % x", readBack, testData)
	}
}

func TestVirtualFlashErase(t *testing.T) {
	oldHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	device, err := OpenDevice("test-erase")
	if err != nil {
		t.Fatalf("Failed to open device: %v", err)
	}
	defer device.Close()

	// Write data
	err = device.Write(0x10000, []byte{0xAA, 0xBB, 0xCC, 0xDD})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Erase region
	err = device.EraseRegion(0x10000, 4)
	if err != nil {
		t.Fatalf("Failed to erase: %v", err)
	}

	// Verify erased to 0xFF
	verified, err := device.Read(0x10000, 4)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	for i, b := range verified {
		if b != 0xFF {
			t.Errorf("Erase failed at byte %d: got 0x%02x, want 0xFF", i, b)
		}
	}
}

func TestVirtualFlashMultipleParts(t *testing.T) {
	oldHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	device, err := OpenDevice("test-multipart")
	if err != nil {
		t.Fatalf("Failed to open device: %v", err)
	}
	defer device.Close()

	// Simulate ESP flash layout: bootloader, partition table, app
	bootloader := []byte{0xE9, 0x01, 0x02, 0x20}
	partitionTable := []byte{0xAA, 0xBB, 0x01, 0x02}
	app := []byte{0xE9, 0x03, 0x00, 0x20, 0x11, 0x22, 0x33, 0x44}

	if err := device.Write(0x1000, bootloader); err != nil {
		t.Fatalf("Write bootloader: %v", err)
	}
	if err := device.Write(0x8000, partitionTable); err != nil {
		t.Fatalf("Write partition table: %v", err)
	}
	if err := device.Write(0x10000, app); err != nil {
		t.Fatalf("Write app: %v", err)
	}

	// Verify bootloader
	readBootloader, _ := device.Read(0x1000, 4)
	if !bytes.Equal(bootloader, readBootloader) {
		t.Errorf("Bootloader mismatch: got % x, want % x", readBootloader, bootloader)
	}

	// Verify partition table
	readPT, _ := device.Read(0x8000, 4)
	if !bytes.Equal(partitionTable, readPT) {
		t.Errorf("Partition table mismatch: got % x, want % x", readPT, partitionTable)
	}

	// Verify app
	readApp, _ := device.Read(0x10000, 8)
	if !bytes.Equal(app, readApp) {
		t.Errorf("App mismatch: got % x, want % x", readApp, app)
	}
}

func TestIsVirtualPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{":virtual:", true},
		{"wokwi-esp32s3", true},
		{"wokwi-esp32", true},
		{"wokwi-esp32c3", true},
		{"/dev/ttyUSB0", false},
		{"/dev/cu.usbserial", false},
		{"COM1", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsVirtualPath(tt.path); got != tt.expected {
				t.Errorf("IsVirtualPath(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestChipFromVirtualPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"wokwi-esp32s3", "esp32s3"},
		{"wokwi-esp32", "esp32"},
		{"wokwi-esp32c3", "esp32c3"},
		{":virtual:", "esp32s3"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := ChipFromVirtualPath(tt.path); got != tt.expected {
				t.Errorf("ChipFromVirtualPath(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestVirtualDeviceDump(t *testing.T) {
	oldHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	device, err := OpenDevice("test-dump")
	if err != nil {
		t.Fatalf("Failed to open device: %v", err)
	}
	defer device.Close()

	// Write some data
	device.Write(0x1000, []byte{0xAA, 0xBB, 0xCC})

	// Dump entire flash
	dump, err := device.Dump()
	if err != nil {
		t.Fatalf("Failed to dump: %v", err)
	}

	if len(dump) != VirtualFlashSize {
		t.Errorf("Dump size = %d, want %d", len(dump), VirtualFlashSize)
	}

	// Verify data is in dump
	if dump[0x1000] != 0xAA || dump[0x1001] != 0xBB || dump[0x1002] != 0xCC {
		t.Errorf("Data not found in dump at offset 0x1000: % x", dump[0x1000:0x1003])
	}
}
