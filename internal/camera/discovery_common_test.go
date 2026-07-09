package camera

import (
	"testing"
)

// TestContainsVideoKeyword tests the helper function across all platforms
func TestContainsVideoKeyword(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"Contains video keyword", "usb-camera-video0", true},
		{"Contains video-index pattern", "usb-camera-video-index0", true},
		{"No video keyword", "EAB7A68F-EC2B-4487-AADF-D8A91C1CB782", false},
		{"UUID format", "619d9715-877a-4155-83ad-4c1675a2deae", false},
		{"Empty string", "", false},
		{"Platform UID", "0x1134000303a8000", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsVideoKeyword(tt.s)
			if got != tt.want {
				t.Errorf("containsVideoKeyword(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

// TestExtractV4L2PathCrossPlatform tests path extraction across all platforms
func TestExtractV4L2PathCrossPlatform(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		want     string
	}{
		{
			name:     "Standard pion format with video-index0",
			deviceID: "usb-046d_HD_Webcam_C615_C574F460-video-index0",
			want:     "/dev/video0",
		},
		{
			name:     "Semicolon video suffix (actual device number)",
			deviceID: "usb-046d_HD_Webcam_C615_C574F460-video-index0;video4",
			want:     "/dev/video4",
		},
		{
			name:     "Higher video index",
			deviceID: "usb-046d_HD_Webcam_C615_C574F460-video-index5",
			want:     "/dev/video5",
		},
		{
			name:     "Only semicolon suffix",
			deviceID: "some-device;video2",
			want:     "/dev/video2",
		},
		{
			name:     "UUID format (no parse pattern)",
			deviceID: "65177c62-d991-4900-9f90-c1fb8692e550",
			want:     "65177c62-d991-4900-9f90-c1fb8692e550",
		},
		{
			name:     "Platform camera (macOS UID)",
			deviceID: "0x1134000303a8000",
			want:     "0x1134000303a8000",
		},
		{
			name:     "macOS UUID format",
			deviceID: "EAB7A68F-EC2B-4487-AADF-D8A91C1CB782",
			want:     "EAB7A68F-EC2B-4487-AADF-D8A91C1CB782",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractV4L2Path(tt.deviceID)
			if got != tt.want {
				t.Errorf("extractV4L2Path(%q) = %q, want %q", tt.deviceID, got, tt.want)
			}
		})
	}
}

// TestGenerateStableCameraIDCrossPlatform tests ID generation across all platforms
func TestGenerateStableCameraIDCrossPlatform(t *testing.T) {
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
			name:     "No video-index suffix",
			label:    "usb-046d_HD_Webcam_C615_C574F460",
			expected: "cam-usb-046d_HD_Webcam_C615_C574F460",
		},
		{
			name:     "Only video-index in suffix",
			label:    "camera-video-index2",
			expected: "cam-camera",
		},
		{
			name:     "Empty string",
			label:    "",
			expected: "cam-",
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

// TestPlatform tests platform function
func TestPlatform(t *testing.T) {
	got := Platform()
	if got == "" {
		t.Error("Platform() returned empty string")
	}
	if got == "unknown" {
		t.Logf("Platform() returned 'unknown', current platform may not be explicitly supported")
	}
}
