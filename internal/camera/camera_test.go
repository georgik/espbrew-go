package camera

import (
	"testing"
)

func TestDetectBackend(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		want     Backend
	}{
		{
			name:     "Linux V4L2 device",
			deviceID: "/dev/video0",
			want:     BackendV4L2,
		},
		{
			name:     "Linux V4L2 with video in name",
			deviceID: "usb-0000:00:14.0-1",
			want:     BackendUnknown,
		},
		{
			name:     "macOS Facetime",
			deviceID: "0x8020000005ac8514",
			want:     BackendAVFoundation,
		},
		{
			name:     "Generic USB device (unknown platform)",
			deviceID: "usb-1234567890",
			want:     BackendUnknown,
		},
		{
			name:     "Windows DirectShow GUID",
			deviceID: "\\\\?\\usb#vid_1234&pid_5678",
			want:     BackendDirectShow,
		},
		{
			name:     "Windows device path",
			deviceID: "@device:pnp:\\\\?\\usb#vid_1234",
			want:     BackendDirectShow,
		},
		{
			name:     "Unknown device",
			deviceID: "unknown-device-id",
			want:     BackendUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectBackend(tt.deviceID)
			if got != tt.want {
				t.Errorf("DetectBackend() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVideoFormatString(t *testing.T) {
	f := VideoFormat{
		Width:       1920,
		Height:      1080,
		PixelFormat: "MJPG",
	}

	want := "1920x1080/MJPG"
	if got := f.String(); got != want {
		t.Errorf("VideoFormat.String() = %v, want %v", got, want)
	}
}

func TestCameraInfoIsAvailable(t *testing.T) {
	tests := []struct {
		name string
		cam  *CameraInfo
		want bool
	}{
		{
			name: "Available camera",
			cam: &CameraInfo{
				ID:   "cam-001",
				Name: "Test Camera",
			},
			want: true,
		},
		{
			name: "Missing ID",
			cam: &CameraInfo{
				Name: "Test Camera",
			},
			want: false,
		},
		{
			name: "Missing name",
			cam: &CameraInfo{
				ID: "cam-001",
			},
			want: false,
		},
		{
			name: "Empty camera",
			cam:  &CameraInfo{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cam.IsAvailable(); got != tt.want {
				t.Errorf("CameraInfo.IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCameraInfoGetBestFormat(t *testing.T) {
	cam := &CameraInfo{
		ID:   "cam-001",
		Name: "Test Camera",
		Formats: []VideoFormat{
			{Width: 640, Height: 480, PixelFormat: "YUYV"},
			{Width: 1920, Height: 1080, PixelFormat: "MJPG"},
			{Width: 1280, Height: 720, PixelFormat: "YUYV"},
		},
	}

	tests := []struct {
		name    string
		width   uint32
		height  uint32
		wantStr string
	}{
		{
			name:    "Exact match",
			width:   640,
			height:  480,
			wantStr: "640x480/YUYV",
		},
		{
			name:    "Another exact match",
			width:   1920,
			height:  1080,
			wantStr: "1920x1080/MJPG",
		},
		{
			name:    "Closest match",
			width:   800,
			height:  600,
			wantStr: "1920x1080/MJPG", // Highest resolution
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cam.GetBestFormat(tt.width, tt.height)
			if got == nil {
				t.Fatal("GetBestFormat() returned nil")
			}
			if gotStr := got.String(); gotStr != tt.wantStr {
				t.Errorf("GetBestFormat() = %v, want %v", gotStr, tt.wantStr)
			}
		})
	}
}

func TestCameraInfoGetBestFormatEmpty(t *testing.T) {
	cam := &CameraInfo{
		ID:      "cam-001",
		Name:    "Test Camera",
		Formats: []VideoFormat{},
	}

	got := cam.GetBestFormat(1920, 1080)
	if got != nil {
		t.Errorf("GetBestFormat() = %v, want nil", got)
	}
}
