package camera

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewCapturer(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	capturer := NewCapturer(store)
	if capturer == nil {
		t.Fatal("NewCapturer() returned nil")
	}

	if capturer.store != store {
		t.Error("Capturer store not set correctly")
	}
}

func TestNewCapturerWithStore(t *testing.T) {
	capturer, err := NewCapturerWithStore()
	if err != nil {
		t.Fatalf("NewCapturerWithStore() error = %v", err)
	}

	if capturer == nil {
		t.Fatal("NewCapturerWithStore() returned nil")
	}

	if capturer.store == nil {
		t.Error("Capturer store is nil")
	}
}

func TestCapturerCapture(t *testing.T) {
	// Skip unless explicitly enabled
	if os.Getenv("ESPBREW_TEST_CAMERA") == "" {
		t.Skip("Set ESPBREW_TEST_CAMERA=1 to enable camera capture tests")
	}

	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	capturer := NewCapturer(store)

	ctx := context.Background()
	req := &CaptureRequest{
		Width:   640,
		Height:  480,
		Format:  "jpg",
		Quality: 85,
		Timeout: 10 * time.Second,
	}

	result, err := capturer.Capture(ctx, req)
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if result == nil {
		t.Fatal("Capture() returned nil result")
	}

	// Check result fields
	if result.Path == "" {
		t.Error("Result Path is empty")
	}
	if result.Format != "jpg" {
		t.Errorf("Result Format = %v, want jpg", result.Format)
	}
	if result.Width <= 0 {
		t.Errorf("Result Width = %v, want > 0", result.Width)
	}
	if result.Height <= 0 {
		t.Errorf("Result Height = %v, want > 0", result.Height)
	}
	if result.Size <= 0 {
		t.Errorf("Result Size = %v, want > 0", result.Size)
	}
	if len(result.Data) == 0 {
		t.Error("Result Data is empty")
	}

	// Check file exists
	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("Capture file does not exist: %v", err)
	}
}

func TestCapture(t *testing.T) {
	// Skip unless explicitly enabled
	if os.Getenv("ESPBREW_TEST_CAMERA") == "" {
		t.Skip("Set ESPBREW_TEST_CAMERA=1 to enable camera capture tests")
	}

	ctx := context.Background()

	result, err := Capture(ctx, "", 640, 480)
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if result == nil {
		t.Fatal("Capture() returned nil result")
	}

	if result.Path == "" {
		t.Error("Result Path is empty")
	}
}

func TestCapturerCaptureTimeout(t *testing.T) {
	// This test verifies timeout handling
	// Skip unless explicitly enabled
	if os.Getenv("ESPBREW_TEST_CAMERA") == "" {
		t.Skip("Set ESPBREW_TEST_CAMERA=1 to enable camera capture tests")
	}

	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)
	capturer := NewCapturer(store)

	ctx := context.Background()
	req := &CaptureRequest{
		Width:   640,
		Height:  480,
		Format:  "jpg",
		Quality: 85,
		Timeout: 100 * time.Millisecond, // Very short timeout
	}

	start := time.Now()
	_, err := capturer.Capture(ctx, req)
	elapsed := time.Since(start)

	// Should either succeed quickly or timeout
	if err != nil {
		t.Logf("Capture with short timeout failed (expected): %v", err)
	}

	// Should not take longer than timeout + 1 second
	if elapsed > req.Timeout+time.Second {
		t.Errorf("Capture took %v, expected < %v", elapsed, req.Timeout+time.Second)
	}
}

func TestCaptureRequestDefaults(t *testing.T) {
	req := &CaptureRequest{}

	if req.Timeout != 0 {
		t.Errorf("Default Timeout = %v, want 0", req.Timeout)
	}
	if req.Quality != 0 {
		t.Errorf("Default Quality = %v, want 0", req.Quality)
	}
	if req.Format != "" {
		t.Errorf("Default Format = %v, want empty", req.Format)
	}
}
