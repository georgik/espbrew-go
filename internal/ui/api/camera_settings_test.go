//go:build js
// +build js

package api

import (
	"testing"
)

func TestCameraControlsResponse(t *testing.T) {
	resp := &CameraControlsResponse{
		Current:      make(map[string]int32),
		DisplayPreset: make(map[string]int32),
		FocusPresets:  make(map[string]int32),
		Ranges:        make(map[string]ControlRange),
	}

	resp.Current["brightness"] = 50
	resp.Current["contrast"] = 60
	resp.Available = true
	resp.Platform = "linux"

	if resp.Platform != "linux" {
		t.Errorf("Expected platform 'linux', got '%s'", resp.Platform)
	}

	if !resp.Available {
		t.Error("Expected Available to be true")
	}

	if resp.Current["brightness"] != 50 {
		t.Errorf("Expected brightness 50, got %d", resp.Current["brightness"])
	}
}

func TestControlRange(t *testing.T) {
	rangeInfo := ControlRange{
		Min:     0,
		Max:     100,
		Current: 50,
	}

	if rangeInfo.Min != 0 {
		t.Errorf("Expected Min 0, got %d", rangeInfo.Min)
	}

	if rangeInfo.Max != 100 {
		t.Errorf("Expected Max 100, got %d", rangeInfo.Max)
	}

	if rangeInfo.Current != 50 {
		t.Errorf("Expected Current 50, got %d", rangeInfo.Current)
	}
}

func TestCameraSettingsRequest(t *testing.T) {
	req := &CameraSettingsRequest{
		CameraID:         "camera-1",
		Brightness:       50,
		Contrast:         60,
		Saturation:       50,
		Sharpness:        50,
		Gain:             0,
		Focus:            0,
		Exposure:         100,
		WhiteBalance:     4000,
		AutoExposure:     true,
		AutoFocus:        false,
		AutoWhiteBalance: true,
	}

	if req.CameraID != "camera-1" {
		t.Errorf("Expected CameraID 'camera-1', got '%s'", req.CameraID)
	}

	if req.Brightness != 50 {
		t.Errorf("Expected Brightness 50, got %d", req.Brightness)
	}

	if !req.AutoExposure {
		t.Error("Expected AutoExposure to be true")
	}

	if req.AutoFocus {
		t.Error("Expected AutoFocus to be false")
	}
}

func TestCameraSettings(t *testing.T) {
	settings := &CameraSettings{
		CameraID:         "camera-1",
		Brightness:       50,
		Contrast:         60,
		Saturation:       50,
		Sharpness:        50,
		Gain:             0,
		Focus:            0,
		Exposure:         100,
		WhiteBalance:     4000,
		AutoExposure:     true,
		AutoFocus:        false,
		AutoWhiteBalance: true,
	}

	if settings.CameraID != "camera-1" {
		t.Errorf("Expected CameraID 'camera-1', got '%s'", settings.CameraID)
	}

	if settings.WhiteBalance != 4000 {
		t.Errorf("Expected WhiteBalance 4000, got %d", settings.WhiteBalance)
	}
}
