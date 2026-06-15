package camera

import (
	"testing"

	"github.com/pion/mediadevices"
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

func TestExtractV4L2Path(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		want     string
	}{
		{
			name:     "Standard pion format",
			deviceID: "usb-046d_HD_Webcam_C615_C574F460-video-index0",
			want:     "/dev/video0",
		},
		{
			name:     "Video index 1",
			deviceID: "usb-046d_HD_Webcam_C615_C574F460-video-index1",
			want:     "/dev/video1",
		},
		{
			name:     "Higher video index",
			deviceID: "usb-046d_HD_Webcam_C615_C574F460-video-index5",
			want:     "/dev/video5",
		},
		{
			name:     "UUID format (no parse pattern)",
			deviceID: "65177c62-d991-4900-9f90-c1fb8692e550",
			want:     "65177c62-d991-4900-9f90-c1fb8692e550",
		},
		{
			name:     "UUID with video index (mixed format)",
			deviceID: "65177c62-d991-4900-9f90-c1fb8692e550-video-index2",
			want:     "/dev/video2",
		},
		{
			name:     "Platform camera (macOS)",
			deviceID: "0x80200000005ac2",
			want:     "0x80200000005ac2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the label as the device identifier for stable ID generation
			// The test's deviceID becomes the label for V4L2 format devices
			label := tt.deviceID
			if !isV4L2Format(tt.deviceID) {
				label = "Test Camera"
			}

			info, err := deviceToCameraInfo(mediadevices.MediaDeviceInfo{
				DeviceID: tt.deviceID,
				Label:    label,
				Kind:     mediadevices.VideoInput,
			})

			if err != nil {
				t.Fatalf("deviceToCameraInfo failed: %v", err)
			}

			if info == nil {
				t.Fatal("deviceToCameraInfo returned nil camera info")
			}

			// Check that we got a camera info
			if info.ID == "" {
				t.Error("Camera ID is empty")
			}

			// For V4L2-format inputs, Path should contain /dev/videoN
			// ID should be a stable identifier generated from the label
			if tt.want != "" && info.Path != tt.want {
				t.Errorf("Camera Path = %q, want %q", info.Path, tt.want)
			}
			// For V4L2 format, ID should be stable (cam-usb-...)
			// For non-V4L2 format (UUID), ID should be the DeviceID
			expectedID := tt.deviceID
			if isV4L2Format(tt.deviceID) {
				expectedID = generateStableCameraID(tt.deviceID)
			}
			if info.ID != expectedID {
				t.Errorf("Camera ID = %q, want %q", info.ID, expectedID)
			}
		})
	}
}

// isV4L2Format checks if a device ID looks like V4L2 format
func isV4L2Format(deviceID string) bool {
	return containsVideoKeyword(deviceID)
}

func TestGenerateStableCameraID(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		expected string
	}{
		{
			name:     "Standard USB camera with video-index",
			label:    "usb-046d_HD_Webcam_C615_C574F460-video-index0",
			expected: "cam-usb-046d_HD_Webcam_C615_C574F460",
		},
		{
			name:     "USB camera with semicolon video suffix",
			label:    "usb-Hewlett_Packard_HP_Webcam_HD_2300-video-index0;video0",
			expected: "cam-usb-Hewlett_Packard_HP_Webcam_HD_2300",
		},
		{
			name:     "Platform camera",
			label:    "Facetime HD Camera",
			expected: "cam-Facetime HD Camera",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateStableCameraID(tt.label)
			if result != tt.expected {
				t.Errorf("generateStableCameraID(%q) = %q, want %q", tt.label, result, tt.expected)
			}
		})
	}
}

func TestIsPrimaryVideoDevice(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		want     bool
	}{
		{
			name:     "Primary video device index 0",
			deviceID: "usb-camera-video-index0",
			want:     true,
		},
		{
			name:     "Secondary video device index 1",
			deviceID: "usb-camera-video-index1",
			want:     false,
		},
		{
			name:     "UUID format (no video-index)",
			deviceID: "65177c62-d991-4900-9f90-c1fb8692e550",
			want:     true, // Non-V4L2 devices are considered primary
		},
		{
			name:     "Platform camera (macOS)",
			deviceID: "Facetime HD Camera",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPrimaryVideoDevice(tt.deviceID)
			if got != tt.want {
				t.Errorf("isPrimaryVideoDevice(%q) = %v, want %v", tt.deviceID, got, tt.want)
			}
		})
	}
}
