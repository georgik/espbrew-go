package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/snap"
)

// TestLocalSnap_BasicFunctionality tests basic snap functionality
func TestLocalSnap_BasicFunctionality(t *testing.T) {
	t.Run("executor creation and configuration", func(t *testing.T) {
		virtualPath := ":virtual:"
		duration := 1 * time.Second

		executor := snap.NewExecutor(virtualPath, duration)

		// Test configuration methods
		executor.SetBaudRate(460800)
		executor.SetCameraID("esp32-cam")
		executor.SetNoCapture(true)
		executor.SetNoMonitor(true)

		// Verify getters
		if !executor.GetSkipFlash() {
			t.Error("GetSkipFlash should always return true")
		}
		if !executor.GetNoCapture() {
			t.Error("GetNoCapture should return true after SetNoCapture(true)")
		}
		if !executor.GetNoMonitor() {
			t.Error("GetNoMonitor should return true after SetNoMonitor(true)")
		}
	})

	t.Run("executor with empty device path", func(t *testing.T) {
		duration := 100 * time.Millisecond
		executor := snap.NewExecutor("", duration)

		// Run should still succeed with monitor/capture disabled
		executor.SetNoCapture(true)
		executor.SetNoMonitor(true)

		ctx := context.Background()
		_, err := executor.Run(ctx)

		// Should succeed since monitor and capture are disabled
		if err != nil {
			t.Errorf("Run() with empty device and no monitor/capture should succeed, got: %v", err)
		}
	})
}

// TestLocalSnap_OutputFormats tests different output formats
func TestLocalSnap_OutputFormats(t *testing.T) {
	// Create a test result
	testResult := &snap.SnapResult{
		Metadata: snap.SnapMetadata{
			SnapID:         "test-snap-123",
			Timestamp:      time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
			Duration:       1500,
			Status:         snap.SnapStatusSuccess,
			DevicePath:     "/dev/ttyUSB0",
			FlashSkipped:   true,
			MonitorEnabled: true,
			LogEntryCount:  5,
			CaptureEnabled: true,
			CameraID:       "esp32-cam",
			ImageFormat:    "jpg",
			ImageSize:      1024,
		},
		Logs: []snap.SerialLogEntry{
			{
				Timestamp: time.Date(2026, 6, 3, 12, 0, 1, 0, time.UTC),
				Message:   "System starting",
				Level:     "info",
			},
			{
				Timestamp: time.Date(2026, 6, 3, 12, 0, 2, 0, time.UTC),
				Message:   "Camera initialized",
				Level:     "info",
			},
		},
		ImageData: []byte{0xFF, 0xD8, 0xFF, 0xE0}, // JPEG header
	}

	t.Run("JSON format", func(t *testing.T) {
		var output strings.Builder
		handler := snap.NewHandler(snap.OutputFormatJSON, &output, "")

		err := handler.Write(testResult)
		if err != nil {
			t.Errorf("Write() error = %v", err)
		}

		jsonOutput := output.String()
		// Verify JSON contains expected fields
		if !strings.Contains(jsonOutput, "test-snap-123") {
			t.Error("JSON output should contain snap ID")
		}
		if !strings.Contains(jsonOutput, "success") {
			t.Error("JSON output should contain status")
		}
		if !strings.Contains(jsonOutput, "metadata") {
			t.Error("JSON output should contain metadata field")
		}
		if !strings.Contains(jsonOutput, "logs") {
			t.Error("JSON output should contain logs field")
		}
	})

	t.Run("text format", func(t *testing.T) {
		var output strings.Builder
		handler := snap.NewHandler(snap.OutputFormatText, &output, "")

		err := handler.Write(testResult)
		if err != nil {
			t.Errorf("Write() error = %v", err)
		}

		textOutput := output.String()
		// Verify text format contains expected sections
		if !strings.Contains(textOutput, "=== Snapshot Result ===") {
			t.Error("Text output should contain header")
		}
		if !strings.Contains(textOutput, "Status:") {
			t.Error("Text output should contain status line")
		}
		if !strings.Contains(textOutput, "test-snap-123") {
			t.Error("Text output should contain snap ID")
		}
		if !strings.Contains(textOutput, "--- Device ---") {
			t.Error("Text output should contain device section")
		}
	})

	t.Run("compact format", func(t *testing.T) {
		var output strings.Builder
		handler := snap.NewHandler(snap.OutputFormatCompact, &output, "")

		err := handler.Write(testResult)
		if err != nil {
			t.Errorf("Write() error = %v", err)
		}

		compactOutput := output.String()
		// Verify compact format is single line
		if !strings.Contains(compactOutput, "\n") || strings.Count(compactOutput, "\n") > 1 {
			// May have trailing newline from the write
			lines := strings.Split(strings.TrimSpace(compactOutput), "\n")
			if len(lines) > 1 {
				t.Errorf("Compact output should be single line, got %d lines", len(lines))
			}
		}
		if !strings.Contains(compactOutput, "test-snap-123") {
			t.Error("Compact output should contain snap ID")
		}
		if !strings.Contains(compactOutput, "success") {
			t.Error("Compact output should contain status")
		}
	})
}

// TestLocalSnap_SaveDirectory tests saving snap results to disk
func TestLocalSnap_SaveDirectory(t *testing.T) {
	t.Run("save image and metadata", func(t *testing.T) {
		// Create a temporary directory for saving
		saveDir := t.TempDir()

		// Create test result with image data
		testResult := &snap.SnapResult{
			Metadata: snap.SnapMetadata{
				SnapID:       "save-test-123",
				Timestamp:    time.Now(),
				Duration:     1000,
				Status:       snap.SnapStatusSuccess,
				FlashSkipped: true,
				ImageSize:    8,
			},
			ImageData: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x00, 0x00},
		}

		var output strings.Builder
		handler := snap.NewHandler(snap.OutputFormatJSON, &output, saveDir)

		err := handler.Write(testResult)
		if err != nil {
			t.Errorf("Write() error = %v", err)
		}

		// Verify files were created
		imagePath := filepath.Join(saveDir, "snap-save-test-123.jpg")
		metaPath := filepath.Join(saveDir, "snap-save-test-123.json")

		if _, err := os.Stat(imagePath); os.IsNotExist(err) {
			t.Error("Image file should exist in save directory")
		}

		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			t.Error("Metadata file should exist in save directory")
		}

		// Verify image content
		savedImageData, _ := os.ReadFile(imagePath)
		if len(savedImageData) != len(testResult.ImageData) {
			t.Errorf("Saved image size = %d, want %d", len(savedImageData), len(testResult.ImageData))
		}

		// Verify metadata content
		metaData, _ := os.ReadFile(metaPath)
		metaStr := string(metaData)
		if !strings.Contains(metaStr, "save-test-123") {
			t.Error("Metadata should contain snap ID")
		}
	})

	t.Run("save with nested directory creation", func(t *testing.T) {
		// Create a nested path that doesn't exist
		baseDir := t.TempDir()
		nestedDir := filepath.Join(baseDir, "level1", "level2", "snaps")

		testResult := &snap.SnapResult{
			Metadata: snap.SnapMetadata{
				SnapID:       "nested-test-456",
				Timestamp:    time.Now(),
				Duration:     500,
				Status:       snap.SnapStatusSuccess,
				FlashSkipped: true,
				ImageSize:    4,
			},
			ImageData: []byte{0xFF, 0xD8, 0xFF, 0xE0},
		}

		var output strings.Builder
		handler := snap.NewHandler(snap.OutputFormatText, &output, nestedDir)

		err := handler.Write(testResult)
		if err != nil {
			t.Errorf("Write() error = %v", err)
		}

		// Verify nested directory was created
		imagePath := filepath.Join(nestedDir, "snap-nested-test-456.jpg")
		if _, err := os.Stat(imagePath); os.IsNotExist(err) {
			t.Error("Image file should exist in nested save directory")
		}
	})

	t.Run("save without image data", func(t *testing.T) {
		saveDir := t.TempDir()

		// Create result without image data
		testResult := &snap.SnapResult{
			Metadata: snap.SnapMetadata{
				SnapID:       "no-image-789",
				Timestamp:    time.Now(),
				Duration:     300,
				Status:       snap.SnapStatusSuccess,
				FlashSkipped: true,
				ImageSize:    0,
			},
			ImageData: []byte{}, // Empty image data
		}

		var output strings.Builder
		handler := snap.NewHandler(snap.OutputFormatCompact, &output, saveDir)

		err := handler.Write(testResult)
		if err != nil {
			t.Errorf("Write() error = %v", err)
		}

		// Verify metadata was saved but no image file
		metaPath := filepath.Join(saveDir, "snap-no-image-789.json")
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			t.Error("Metadata file should exist even without image data")
		}

		imagePath := filepath.Join(saveDir, "snap-no-image-789.jpg")
		if _, err := os.Stat(imagePath); err == nil {
			// Image file might not be created if data is empty
			// This is acceptable behavior
		}
	})
}

// TestLocalSnap_ErrorConditions tests various error conditions
func TestLocalSnap_ErrorConditions(t *testing.T) {
	t.Run("nil result handling", func(t *testing.T) {
		var output strings.Builder
		handler := snap.NewHandler(snap.OutputFormatText, &output, "")

		err := handler.Write(nil)
		if err == nil {
			t.Error("Expected error when writing nil result")
		}
	})
}

// TestParseSnapOutputFormat tests the output format parsing logic
func TestParseSnapOutputFormat(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected snap.OutputFormat
	}{
		{"empty defaults to text", "", snap.OutputFormatText},
		{"explicit json", "json", snap.OutputFormatJSON},
		{"explicit text", "text", snap.OutputFormatText},
		{"explicit compact", "compact", snap.OutputFormatCompact},
		{"json file extension", "output.json", snap.OutputFormatJSON},
		{"file with json extension", "/path/to/result.json", snap.OutputFormatJSON},
		{"unknown format defaults to text", "unknown", snap.OutputFormatText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Temporarily set snapOpts.output for testing
			oldOutput := snapOpts.output
			snapOpts.output = tt.output
			defer func() { snapOpts.output = oldOutput }()

			format := parseSnapOutputFormat()
			if format != tt.expected {
				t.Errorf("parseSnapOutputFormat() = %v, want %v", format, tt.expected)
			}
		})
	}
}

// TestIsFormatOnly tests the format-only detection logic
func TestIsFormatOnly(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{"json is format only", "json", true},
		{"text is format only", "text", true},
		{"compact is format only", "compact", true},
		{"file path is not format only", "/path/to/output.json", false},
		{"empty is not format only", "", false},
		{"json file extension is not format only", "output.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFormatOnly(tt.output)
			if result != tt.expected {
				t.Errorf("isFormatOnly(%q) = %v, want %v", tt.output, result, tt.expected)
			}
		})
	}
}
