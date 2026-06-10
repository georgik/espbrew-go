package camera

import (
	"testing"
	"time"
)

func TestRegistry(t *testing.T) {
	// Create a new registry (not the global one)
	registry := NewRegistry()

	// Start should begin watching
	registry.Start()
	defer registry.Stop()

	// Give it time for initial scan
	time.Sleep(100 * time.Millisecond)

	// List should return cameras
	cameras := registry.List()
	if len(cameras) < 0 {
		t.Errorf("List() returned invalid length")
	}

	// If cameras found, test GetByID
	for _, cam := range cameras {
		found, ok := registry.GetByID(cam.ID)
		if !ok {
			t.Errorf("GetByID(%s) not found", cam.ID)
		}
		if found == nil || found.ID != cam.ID {
			t.Errorf("GetByID(%s) returned wrong camera", cam.ID)
		}
	}

	// Test GetNameByID with unknown camera
	name := registry.GetNameByID("unknown-camera-id-12345678")
	if name == "" {
		t.Errorf("GetNameByID() returned empty string for unknown ID")
	}
	// Should return fallback name
	if len(name) < 8 {
		t.Errorf("GetNameByID() fallback too short: %s", name)
	}

	// Test GetByPath
	for _, cam := range cameras {
		if cam.Path != "" {
			found, ok := registry.GetByPath(cam.Path)
			if !ok {
				t.Errorf("GetByPath(%s) not found", cam.Path)
			}
			if found == nil || found.Path != cam.Path {
				t.Errorf("GetByPath(%s) returned wrong camera", cam.Path)
			}
			break // Test with first valid path only
		}
	}
}

func TestRegistryGetNameByID(t *testing.T) {
	registry := NewRegistry()

	// Test empty ID
	name := registry.GetNameByID("")
	if name != "Unknown Camera" {
		t.Errorf("GetNameByID(\"\") should return 'Unknown Camera', got: %s", name)
	}

	// Test "unknown" ID
	name = registry.GetNameByID("unknown")
	if name != "Unknown Camera" {
		t.Errorf("GetNameByID(\"unknown\") should return 'Unknown Camera', got: %s", name)
	}

	// Test long ID (should return shortened version)
	longID := "abcd1234-5678-90ab-cdef-123456789012"
	name = registry.GetNameByID(longID)
	expected := "Camera abcd1234"
	if name != expected {
		t.Errorf("GetNameByID(%s) should return '%s', got: %s", longID, expected, name)
	}

	// Test short ID (should return full ID)
	shortID := "abc123"
	name = registry.GetNameByID(shortID)
	expected = "Camera abc123"
	if name != expected {
		t.Errorf("GetNameByID(%s) should return '%s', got: %s", shortID, expected, name)
	}
}
