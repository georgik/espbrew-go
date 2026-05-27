package inventory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInventoryLoadSave(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "devices.json")

	inv := &Inventory{
		dbPath:  dbPath,
		devices: make(map[string]*DeviceInventory),
	}

	// Create test device
	now := time.Now()
	dev := &DeviceInventory{
		DeviceID:   "esp-84:f7:03:12:34:56",
		MACAddress: "84:f7:03:12:34:56",
		ChipType:   "ESP32-S3",
		ChipRev:    "1.0",
		FlashSize:  8 * 1024 * 1024,
		PSRAMSize:  8 * 1024 * 1024,
		PSRAMType:  "AP_3v3",
		Aliases:    []string{"test-station-1"},
		Tags:       []string{"production", "psram-8mb"},
		BoardModel: "ESP32-S3-BOX-3",
		FirstSeen:  now,
		LastSeen:   now,
		LastPath:   "/dev/ttyUSB0",
		NodeID:     "node-1",
	}

	// Save device
	inv.devices[dev.DeviceID] = dev
	if err := inv.save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}

	// Create new inventory and load
	inv2 := &Inventory{
		dbPath:  dbPath,
		devices: make(map[string]*DeviceInventory),
	}
	if err := inv2.load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Verify loaded data
	if len(inv2.devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(inv2.devices))
	}

	loaded := inv2.devices[dev.DeviceID]
	if loaded == nil {
		t.Fatal("device not found after load")
	}

	if loaded.DeviceID != dev.DeviceID {
		t.Errorf("DeviceID: got %s, want %s", loaded.DeviceID, dev.DeviceID)
	}
	if loaded.MACAddress != dev.MACAddress {
		t.Errorf("MACAddress: got %s, want %s", loaded.MACAddress, dev.MACAddress)
	}
	if loaded.ChipType != dev.ChipType {
		t.Errorf("ChipType: got %s, want %s", loaded.ChipType, dev.ChipType)
	}
	if loaded.FlashSize != dev.FlashSize {
		t.Errorf("FlashSize: got %d, want %d", loaded.FlashSize, dev.FlashSize)
	}
	if len(loaded.Aliases) != 1 || loaded.Aliases[0] != "test-station-1" {
		t.Errorf("Aliases: got %v, want [test-station-1]", loaded.Aliases)
	}
	if len(loaded.Tags) != 2 {
		t.Errorf("Tags length: got %d, want 2", len(loaded.Tags))
	}
}

func TestInventoryGetNotFound(t *testing.T) {
	inv := &Inventory{
		dbPath:  "/tmp/test_nonexistent.json",
		devices: make(map[string]*DeviceInventory),
	}

	_, err := inv.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent device, got nil")
	}
}

func TestInventorySaveAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "devices.json")

	inv, err := NewInventoryWithPath(dbPath)
	if err != nil {
		t.Fatalf("NewInventoryWithPath failed: %v", err)
	}

	dev := &DeviceInventory{
		DeviceID:   "esp-test",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S3",
		ChipRev:    "1.0",
		Aliases:    []string{},
		Tags:       []string{},
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}

	if err := inv.Save(dev); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	retrieved, err := inv.Get("esp-test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.DeviceID != dev.DeviceID {
		t.Errorf("DeviceID mismatch: got %s, want %s", retrieved.DeviceID, dev.DeviceID)
	}
}

func TestInventoryTags(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "devices.json")

	inv, err := NewInventoryWithPath(dbPath)
	if err != nil {
		t.Fatalf("NewInventoryWithPath failed: %v", err)
	}

	dev := &DeviceInventory{
		DeviceID:   "esp-test",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S3",
		Aliases:    []string{},
		Tags:       []string{},
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}
	inv.devices[dev.DeviceID] = dev

	// Add tags
	if err := inv.AddTag("esp-test", "production"); err != nil {
		t.Fatalf("AddTag failed: %v", err)
	}
	if err := inv.AddTag("esp-test", "psram-8mb"); err != nil {
		t.Fatalf("AddTag failed: %v", err)
	}

	// Check tags
	if len(dev.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(dev.Tags))
	}

	// Try adding duplicate (should be no-op)
	if err := inv.AddTag("esp-test", "production"); err != nil {
		t.Fatalf("AddTag duplicate failed: %v", err)
	}
	if len(dev.Tags) != 2 {
		t.Errorf("duplicate tag should not be added, got %d tags", len(dev.Tags))
	}

	// Remove tag
	if err := inv.RemoveTag("esp-test", "production"); err != nil {
		t.Fatalf("RemoveTag failed: %v", err)
	}
	if len(dev.Tags) != 1 || dev.Tags[0] != "psram-8mb" {
		t.Errorf("after removal, expected [psram-8mb], got %v", dev.Tags)
	}
}

func TestInventoryAliases(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "devices.json")

	inv, err := NewInventoryWithPath(dbPath)
	if err != nil {
		t.Fatalf("NewInventoryWithPath failed: %v", err)
	}

	dev := &DeviceInventory{
		DeviceID:   "esp-test",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S3",
		Aliases:    []string{},
		Tags:       []string{},
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}
	inv.devices[dev.DeviceID] = dev

	// Add aliases
	if err := inv.AddAlias("esp-test", "station-1"); err != nil {
		t.Fatalf("AddAlias failed: %v", err)
	}
	if err := inv.AddAlias("esp-test", "station-2"); err != nil {
		t.Fatalf("AddAlias failed: %v", err)
	}

	if len(dev.Aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(dev.Aliases))
	}

	// Find by alias
	found, err := inv.FindByAlias("station-1")
	if err != nil {
		t.Fatalf("FindByAlias failed: %v", err)
	}
	if found.DeviceID != "esp-test" {
		t.Errorf("found wrong device: got %s, want esp-test", found.DeviceID)
	}
}

func TestFlashRequirementMatching(t *testing.T) {
	dev := &DeviceInventory{
		DeviceID:   "esp-test",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		ChipType:   "ESP32-S3",
		ChipRev:    "1.0",
		FlashSize:  8 * 1024 * 1024,
		PSRAMSize:  8 * 1024 * 1024,
		PSRAMType:  "AP_3v3",
		Tags:       []string{"production", "psram-8mb"},
		BoardModel: "ESP32-S3-BOX-3",
	}

	tests := []struct {
		name     string
		req      *FlashRequirement
		expected bool
	}{
		{
			name:     "match all",
			req:      &FlashRequirement{ChipType: "ESP32-S3"},
			expected: true,
		},
		{
			name:     "wrong chip",
			req:      &FlashRequirement{ChipType: "ESP32-C3"},
			expected: false,
		},
		{
			name:     "PSRAM requirement met",
			req:      &FlashRequirement{MinPSRAM: 4 * 1024 * 1024},
			expected: true,
		},
		{
			name:     "PSRAM requirement not met",
			req:      &FlashRequirement{MinPSRAM: 16 * 1024 * 1024},
			expected: false,
		},
		{
			name:     "tag match",
			req:      &FlashRequirement{Tags: []string{"production"}},
			expected: true,
		},
		{
			name:     "tag mismatch",
			req:      &FlashRequirement{Tags: []string{"dev"}},
			expected: false,
		},
		{
			name:     "multiple tags match",
			req:      &FlashRequirement{Tags: []string{"production", "psram-8mb"}},
			expected: true,
		},
		{
			name:     "board model match",
			req:      &FlashRequirement{BoardModel: "ESP32-S3-BOX-3"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dev.Matches(tt.req)
			if result != tt.expected {
				t.Errorf("Matches() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// NewInventoryWithPath creates an inventory with a specific path (for testing)
func NewInventoryWithPath(dbPath string) (*Inventory, error) {
	inv := &Inventory{
		dbPath:  dbPath,
		devices: make(map[string]*DeviceInventory),
	}
	if err := inv.load(); err != nil {
		return nil, err
	}
	return inv, nil
}
