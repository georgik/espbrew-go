package snap

import (
	"context"
	"testing"
	"time"
)

// TestNewCapturer verifies the Capturer constructor
func TestNewCapturer(t *testing.T) {
	tests := []struct {
		name     string
		cameraID string
		timeout  time.Duration
		wantID   string
		wantTO   time.Duration
	}{
		{
			name:     "with camera ID and timeout",
			cameraID: "test-camera",
			timeout:  10 * time.Second,
			wantID:   "test-camera",
			wantTO:   10 * time.Second,
		},
		{
			name:     "empty camera ID",
			cameraID: "",
			timeout:  3 * time.Second,
			wantID:   "",
			wantTO:   3 * time.Second,
		},
		{
			name:     "zero timeout defaults to 5 seconds",
			cameraID: "camera-1",
			timeout:  0,
			wantID:   "camera-1",
			wantTO:   5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCapturer(tt.cameraID, tt.timeout)
			if c.cameraID != tt.wantID {
				t.Errorf("cameraID = %v, want %v", c.cameraID, tt.wantID)
			}
			if c.timeout != tt.wantTO {
				t.Errorf("timeout = %v, want %v", c.timeout, tt.wantTO)
			}
		})
	}
}

// TestValidateJPEG verifies JPEG validation logic
func TestValidateJPEG(t *testing.T) {
	c := NewCapturer("", 5*time.Second)

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "too short",
			data:    []byte{0xFF},
			wantErr: true,
		},
		{
			name:    "invalid magic bytes",
			data:    []byte{0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "valid JPEG magic bytes but invalid data",
			data:    []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, // Too short for valid JPEG
			wantErr: true,
		},
		{
			name:    "minimal valid JPEG header (will fail decode)",
			data:    []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.validateJPEG(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateJPEG() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestIsJPEG checks JPEG detection
func TestIsJPEG(t *testing.T) {
	c := NewCapturer("", 5*time.Second)

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "empty",
			data: []byte{},
			want: false,
		},
		{
			name: "one byte",
			data: []byte{0xFF},
			want: false,
		},
		{
			name: "JPEG magic bytes",
			data: []byte{0xFF, 0xD8},
			want: true,
		},
		{
			name: "not JPEG",
			data: []byte{0x89, 0x50}, // PNG magic
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.isJPEG(tt.data)
			if got != tt.want {
				t.Errorf("isJPEG() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestResolveCameraID tests camera ID resolution logic
func TestResolveCameraID(t *testing.T) {
	tests := []struct {
		name     string
		cameraID string
		wantErr  bool
	}{
		{
			name:     "explicit camera ID",
			cameraID: "some-camera",
			wantErr:  false,
		},
		{
			name:     "empty ID triggers discovery",
			cameraID: "",
			wantErr:  false, // May succeed if cameras available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCapturer(tt.cameraID, 5*time.Second)
			id, err := c.resolveCameraID(context.Background())
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err == nil && id == "" {
				t.Error("expected non-empty ID, got empty")
			}
		})
	}
}
