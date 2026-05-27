package camera

import (
	"testing"
)

func TestNewDiscoverer(t *testing.T) {
	d := NewDiscoverer()
	if d == nil {
		t.Fatal("NewDiscoverer() returned nil")
	}

	if d.cameras == nil {
		t.Error("Discoverer cameras map is nil")
	}
}

func TestDiscovererDiscover(t *testing.T) {
	d := NewDiscoverer()

	cameras, err := d.Discover()
	if err != nil {
		t.Logf("Discover() error (may be expected in CI): %v", err)
	}

	if cameras == nil {
		t.Error("Discover() returned nil cameras slice")
	}

	// If cameras were found, validate them
	for _, cam := range cameras {
		if cam.ID == "" {
			t.Error("Camera has empty ID")
		}
		if cam.Name == "" {
			t.Error("Camera has empty Name")
		}
	}
}

func TestDiscover(t *testing.T) {
	cameras, err := Discover()
	if err != nil {
		t.Logf("Discover() error (may be expected in CI): %v", err)
	}

	if cameras == nil {
		t.Error("Discover() returned nil cameras slice")
	}
}

func TestDiscovererGetByID(t *testing.T) {
	d := NewDiscoverer()

	// Try to discover cameras first
	cameras, _ := d.Discover()

	if len(cameras) == 0 {
		t.Skip("No cameras found for GetByID test")
	}

	// Test getting first camera by ID
	cam, ok := d.GetByID(cameras[0].ID)
	if !ok {
		t.Errorf("GetByID(%q) returned ok=false", cameras[0].ID)
	}
	if cam == nil {
		t.Fatal("GetByID() returned nil camera")
	}
	if cam.ID != cameras[0].ID {
		t.Errorf("GetByID() returned ID %q, want %q", cam.ID, cameras[0].ID)
	}
}

func TestDiscovererGetByIDNotFound(t *testing.T) {
	d := NewDiscoverer()

	_, ok := d.GetByID("nonexistent-camera-id")
	if ok {
		t.Error("GetByID() returned ok=true for nonexistent ID")
	}
}

func TestDiscovererList(t *testing.T) {
	d := NewDiscoverer()

	// Discover cameras
	cameras, _ := d.Discover()

	// List should return same cameras
	listed := d.List()

	if len(listed) != len(cameras) {
		t.Errorf("List() returned %d cameras, want %d", len(listed), len(cameras))
	}
}

func TestDiscovererRefresh(t *testing.T) {
	d := NewDiscoverer()

	// First discovery
	cameras1, _ := d.Discover()
	count1 := len(cameras1)

	// Refresh
	cameras2, err := d.Refresh()
	if err != nil {
		t.Logf("Refresh() error: %v", err)
	}

	if len(cameras2) != count1 {
		t.Logf("Refresh() returned %d cameras, first discovery had %d", len(cameras2), count1)
	}
}

func TestCameraInfoStringRepresentation(t *testing.T) {
	cam := &CameraInfo{
		ID:      "cam-001",
		Name:    "Test Camera",
		Path:    "/dev/video0",
		Backend: BackendV4L2,
		Formats: []VideoFormat{
			{Width: 1920, Height: 1080, PixelFormat: "MJPG"},
			{Width: 640, Height: 480, PixelFormat: "YUYV"},
		},
	}

	if cam.ID != "cam-001" {
		t.Errorf("Camera ID = %v, want cam-001", cam.ID)
	}
	if cam.Name != "Test Camera" {
		t.Errorf("Camera Name = %v, want Test Camera", cam.Name)
	}
	if cam.Backend != BackendV4L2 {
		t.Errorf("Camera Backend = %v, want V4L2", cam.Backend)
	}
	if len(cam.Formats) != 2 {
		t.Errorf("Camera has %d formats, want 2", len(cam.Formats))
	}
}

func TestBackendValues(t *testing.T) {
	tests := []struct {
		name  string
		value Backend
	}{
		{"V4L2", BackendV4L2},
		{"AVFoundation", BackendAVFoundation},
		{"DirectShow", BackendDirectShow},
		{"Unknown", BackendUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				t.Errorf("Backend %s has empty string value", tt.name)
			}
		})
	}
}

func TestVideoFormatFields(t *testing.T) {
	f := VideoFormat{
		Width:       1920,
		Height:      1080,
		PixelFormat: "MJPG",
	}

	if f.Width != 1920 {
		t.Errorf("Width = %v, want 1920", f.Width)
	}
	if f.Height != 1080 {
		t.Errorf("Height = %v, want 1080", f.Height)
	}
	if f.PixelFormat != "MJPG" {
		t.Errorf("PixelFormat = %v, want MJPG", f.PixelFormat)
	}
}
