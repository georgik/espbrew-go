package snap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewHandler(t *testing.T) {
	handler := NewHandler(OutputFormatJSON, &bytes.Buffer{}, "")
	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
	if handler.format != OutputFormatJSON {
		t.Errorf("expected format %s, got %s", OutputFormatJSON, handler.format)
	}
}

func TestHandler_WriteNilResult(t *testing.T) {
	handler := NewHandler(OutputFormatJSON, &bytes.Buffer{}, "")
	err := handler.Write(nil)
	if err == nil {
		t.Error("expected error for nil result, got nil")
	}
	if !strings.Contains(err.Error(), "nil result") {
		t.Errorf("expected 'nil result' error, got: %v", err)
	}
}

func TestHandler_WriteJSON(t *testing.T) {
	result := &SnapResult{
		Metadata: SnapMetadata{
			SnapID:    "test-123",
			Timestamp: time.Date(2024, 6, 3, 12, 0, 0, 0, time.UTC),
			Duration:  1500,
			Status:    SnapStatusSuccess,
		},
		ImageData:   []byte{0x89, 0x50, 0x4e, 0x47}, // PNG header
		ImageBase64: "",
	}

	var buf bytes.Buffer
	handler := NewHandler(OutputFormatJSON, &buf, "")

	err := handler.Write(result)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"snap_id": "test-123"`) {
		t.Errorf("JSON output missing snap_id: %s", output)
	}
	if !strings.Contains(output, `"image_base64"`) {
		t.Errorf("JSON output missing image_base64 field")
	}
	if !strings.Contains(output, `"status": "success"`) {
		t.Errorf("JSON output missing status: %s", output)
	}
}

func TestHandler_WriteText(t *testing.T) {
	result := &SnapResult{
		Metadata: SnapMetadata{
			SnapID:          "text-test",
			Timestamp:       time.Date(2024, 6, 3, 12, 0, 0, 0, time.UTC),
			Duration:        500,
			Status:          SnapStatusSuccess,
			DevicePath:      "/dev/ttyUSB0",
			ChipName:        "ESP32-S3",
			MonitorEnabled:  true,
			MonitorDuration: 2 * time.Second,
			MonitorBaudRate: 115200,
			LogEntryCount:   5,
		},
		Logs: []SerialLogEntry{
			{
				Timestamp: time.Date(2024, 6, 3, 12, 0, 1, 0, time.UTC),
				Message:   "Boot complete",
				Level:     "info",
			},
		},
	}

	var buf bytes.Buffer
	handler := NewHandler(OutputFormatText, &buf, "")

	err := handler.Write(result)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "=== Snapshot Result ===") {
		t.Error("Text output missing header")
	}
	if !strings.Contains(output, "Status:") {
		t.Error("Text output missing status line")
	}
	if !strings.Contains(output, "/dev/ttyUSB0") {
		t.Error("Text output missing device path")
	}
	if !strings.Contains(output, "ESP32-S3") {
		t.Error("Text output missing chip name")
	}
	if !strings.Contains(output, "Serial Monitor") {
		t.Error("Text output missing monitor section")
	}
}

func TestHandler_WriteCompact(t *testing.T) {
	result := &SnapResult{
		Metadata: SnapMetadata{
			SnapID:         "compact-123",
			Timestamp:      time.Date(2024, 6, 3, 12, 0, 0, 0, time.UTC),
			Duration:       300,
			Status:         SnapStatusSuccess,
			DeviceNode:     "node-1",
			ChipName:       "ESP8266",
			FlashEnabled:   true,
			FlashFirmware:  "/path/to/firmware.bin",
			MonitorEnabled: true,
			LogEntryCount:  10,
			CaptureEnabled: true,
			ImageSize:      12345,
		},
	}

	var buf bytes.Buffer
	handler := NewHandler(OutputFormatCompact, &buf, "")

	err := handler.Write(result)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "compact-123") {
		t.Errorf("Compact output missing snap_id: %s", output)
	}
	if !strings.Contains(output, "success") {
		t.Errorf("Compact output missing status: %s", output)
	}
	if !strings.Contains(output, "300ms") {
		t.Errorf("Compact output missing duration: %s", output)
	}
	if !strings.Contains(output, "node-1") {
		t.Errorf("Compact output missing device node: %s", output)
	}
	if !strings.Contains(output, "ESP8266") {
		t.Errorf("Compact output missing chip name: %s", output)
	}
	if !strings.Contains(output, "flash") {
		t.Errorf("Compact output missing flash indicator: %s", output)
	}
	if !strings.Contains(output, "firmware.bin") {
		t.Errorf("Compact output missing firmware name: %s", output)
	}
	if !strings.Contains(output, "monitor:10") {
		t.Errorf("Compact output missing monitor count: %s", output)
	}
	if !strings.Contains(output, "capture:12345") {
		t.Errorf("Compact output missing capture size: %s", output)
	}
}

func TestHandler_SaveFiles(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()

	result := &SnapResult{
		Metadata: SnapMetadata{
			SnapID:     "save-test",
			Timestamp:  time.Now(),
			Duration:   100,
			Status:     SnapStatusSuccess,
			DevicePath: "/dev/ttyUSB0",
			ChipName:   "ESP32",
			ImageSize:  4,
		},
		ImageData: []byte{0xFF, 0xD8, 0xFF, 0xE0}, // JPEG header
	}

	handler := NewHandler(OutputFormatJSON, &bytes.Buffer{}, tmpDir)

	err := handler.Write(result)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Check that image file was created
	imagePath := filepath.Join(tmpDir, "snap-save-test.jpg")
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		t.Errorf("Image file not created: %s", imagePath)
	}

	// Check that metadata file was created
	metaPath := filepath.Join(tmpDir, "snap-save-test.json")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Errorf("Metadata file not created: %s", metaPath)
	}

	// Verify metadata content
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("Failed to read metadata file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "save-test") {
		t.Errorf("Metadata missing snap_id: %s", content)
	}
	if !strings.Contains(content, "ESP32") {
		t.Errorf("Metadata missing chip_name: %s", content)
	}
}

func TestHandler_SaveFilesNoImage(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()

	result := &SnapResult{
		Metadata: SnapMetadata{
			SnapID:    "no-img",
			Timestamp: time.Now(),
			Duration:  100,
			Status:    SnapStatusSuccess,
		},
		ImageData: []byte{},
	}

	handler := NewHandler(OutputFormatJSON, &bytes.Buffer{}, tmpDir)

	err := handler.Write(result)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Check that metadata file was created even without image
	metaPath := filepath.Join(tmpDir, "snap-no-img.json")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Errorf("Metadata file not created: %s", metaPath)
	}

	// Check that image file was NOT created
	imagePath := filepath.Join(tmpDir, "snap-no-img.jpg")
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Errorf("Image file should not exist: %s", imagePath)
	}
}

func TestHandler_UnknownFormat(t *testing.T) {
	result := &SnapResult{
		Metadata: SnapMetadata{
			SnapID:    "unknown-fmt",
			Timestamp: time.Now(),
			Status:    SnapStatusSuccess,
		},
	}

	handler := NewHandler("unknown", &bytes.Buffer{}, "")
	err := handler.Write(result)
	if err == nil {
		t.Error("expected error for unknown format, got nil")
	}
	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("expected 'unknown output format' error, got: %v", err)
	}
}

// TestOutput_WriteJSON tests the JSON output format with comprehensive cases.
func TestOutput_WriteJSON(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		result  *SnapResult
		wantErr bool
		check   func(*testing.T, string)
	}{
		{
			name: "valid result with image",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:         "json-test-001",
					Timestamp:      now,
					Duration:       1234,
					Status:         SnapStatusSuccess,
					DevicePath:     "/dev/ttyUSB0",
					DeviceNode:     "node-1",
					ChipID:         "ESP32-D0WDQ5",
					ChipName:       "ESP32",
					FlashEnabled:   true,
					FlashFirmware:  "/path/to/firmware.bin",
					FlashOffset:    0x1000,
					MonitorEnabled: true,
					LogEntryCount:  2,
					CaptureEnabled: true,
					CameraID:       "camera-0",
					ImageFormat:    "jpeg",
					ImageSize:      1024,
				},
				Logs: []SerialLogEntry{
					{
						Timestamp: now.Add(-time.Second),
						Message:   "Boot complete",
						Level:     "info",
						Source:    "boot",
					},
					{
						Timestamp: now,
						Message:   "WiFi connected",
						Level:     "info",
						Source:    "wifi",
					},
				},
				ImageData: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				// Check for required fields
				requiredFields := []string{
					`"snap_id":`,
					`"json-test-001"`,
					`"status":`,
					`"success"`,
					`"image_base64":`,
					`"logs":`,
					`"metadata":`,
				}
				for _, field := range requiredFields {
					if !strings.Contains(output, field) {
						t.Errorf("JSON missing field: %s", field)
					}
				}
			},
		},
		{
			name: "valid result without image",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:    "json-test-002",
					Timestamp: now,
					Duration:  100,
					Status:    SnapStatusSuccess,
				},
				Logs: []SerialLogEntry{},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				if strings.Contains(output, `"image_base64"`) {
					t.Error("JSON should not have image_base64 when no image data")
				}
			},
		},
		{
			name: "result with pre-encoded base64",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:    "json-test-003",
					Timestamp: now,
					Duration:  50,
					Status:    SnapStatusSuccess,
				},
				ImageData:   []byte{1, 2, 3},
				ImageBase64: "AQID",
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, `"image_base64": "AQID"`) {
					t.Error("JSON should preserve pre-encoded base64")
				}
			},
		},
		{
			name:    "nil result",
			result:  nil,
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := NewHandler(OutputFormatJSON, &buf, "")

			err := handler.Write(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}

// TestOutput_WriteText tests the text output format with comprehensive cases.
func TestOutput_WriteText(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		result  *SnapResult
		wantErr bool
		check   func(*testing.T, string)
	}{
		{
			name: "full snapshot with all sections",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:          "text-test-001",
					Timestamp:       now,
					Duration:        5678,
					Status:          SnapStatusSuccess,
					DevicePath:      "/dev/ttyACM0",
					DeviceNode:      "node-2",
					ChipID:          "ESP32-C3",
					ChipName:        "ESP32-C3",
					FlashEnabled:    true,
					FlashFirmware:   "/firmware/app.bin",
					FlashOffset:     0x8000,
					FlashHashBefore: "abc123",
					FlashHashAfter:  "def456",
					FlashSkipped:    false,
					MonitorEnabled:  true,
					MonitorDuration: 10 * time.Second,
					MonitorBaudRate: 115200,
					LogEntryCount:   3,
					CaptureEnabled:  true,
					CameraID:        "cam-1",
					ImageFormat:     "png",
					ImageSize:       2048,
				},
				Logs: []SerialLogEntry{
					{
						Timestamp: now.Add(-2 * time.Second),
						Message:   "System start",
						Level:     "info",
						Source:    "system",
					},
					{
						Timestamp: now.Add(-time.Second),
						Message:   "Warning: low memory",
						Level:     "warn",
						Source:    "memory",
					},
					{
						Timestamp: now,
						Message:   "Error: timeout",
						Level:     "error",
						Source:    "network",
					},
				},
				ImageData: []byte{0x89, 0x50, 0x4E, 0x47},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				expectedSections := []string{
					"=== Snapshot Result ===",
					"Status:",
					"Snap ID:",
					"Timestamp:",
					"Duration:",
					"--- Device ---",
					"--- Flash ---",
					"--- Serial Monitor ---",
					"Log entries:",
					"--- Camera Capture ---",
					"========================",
				}
				for _, section := range expectedSections {
					if !strings.Contains(output, section) {
						t.Errorf("Text output missing section: %s", section)
					}
				}
				// Check log level formatting
				if !strings.Contains(output, "[info]") || !strings.Contains(output, "[warn]") || !strings.Contains(output, "[error]") {
					t.Error("Text output missing log level formatting")
				}
			},
		},
		{
			name: "snapshot with error status",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:    "text-test-002",
					Timestamp: now,
					Duration:  100,
					Status:    SnapStatusFailed,
					Error:     "device not found",
				},
				Logs: []SerialLogEntry{},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "Error:") {
					t.Error("Text output missing error field")
				}
				if !strings.Contains(output, "device not found") {
					t.Error("Text output missing error message")
				}
			},
		},
		{
			name: "snapshot with flash skipped",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:        "text-test-003",
					Timestamp:     now,
					Duration:      200,
					Status:        SnapStatusPartial,
					FlashEnabled:  true,
					FlashSkipped:  true,
					FlashFirmware: "/path/to/firmware.bin",
					FlashOffset:   0x1000,
				},
				Logs: []SerialLogEntry{},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "Skipped:") {
					t.Error("Text output missing flash skipped field")
				}
				if !strings.Contains(output, "true") {
					t.Error("Text output should show flash skipped as true")
				}
			},
		},
		{
			name: "minimal snapshot",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:    "text-test-004",
					Timestamp: now,
					Duration:  50,
					Status:    SnapStatusSuccess,
				},
				Logs: []SerialLogEntry{},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "=== Snapshot Result ===") {
					t.Error("Text output missing header")
				}
				if !strings.Contains(output, "Snap ID:") {
					t.Error("Text output missing snap ID")
				}
				if !strings.Contains(output, "Status:") {
					t.Error("Text output missing status")
				}
				// Device section header is always present even when empty
				if !strings.Contains(output, "--- Device ---") {
					t.Error("Text output missing device section header")
				}
				// But should not have optional conditional sections
				if strings.Contains(output, "--- Flash ---") {
					t.Error("Minimal snapshot should not have flash section")
				}
				if strings.Contains(output, "--- Serial Monitor ---") {
					t.Error("Minimal snapshot should not have monitor section")
				}
				if strings.Contains(output, "--- Camera Capture ---") {
					t.Error("Minimal snapshot should not have capture section")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := NewHandler(OutputFormatText, &buf, "")

			err := handler.Write(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}

// TestOutput_WriteCompact tests the compact output format with comprehensive cases.
func TestOutput_WriteCompact(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		result  *SnapResult
		wantErr bool
		check   func(*testing.T, string)
	}{
		{
			name: "full snapshot",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:         "compact-001",
					Timestamp:      now,
					Duration:       1234,
					Status:         SnapStatusSuccess,
					DeviceNode:     "node-1",
					ChipName:       "ESP32",
					FlashEnabled:   true,
					FlashFirmware:  "/path/to/my-firmware.bin",
					MonitorEnabled: true,
					LogEntryCount:  42,
					CaptureEnabled: true,
					ImageSize:      8192,
				},
				Logs: []SerialLogEntry{},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				// Should be a single line
				lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
				if len(lines) != 1 {
					t.Errorf("Expected single line, got %d lines", len(lines))
				}
				// Check components
				components := []string{"compact-001", "success", "1234ms", "node-1", "ESP32", "flash", "my-firmware.bin", "monitor:42", "capture:8192"}
				for _, comp := range components {
					if !strings.Contains(output, comp) {
						t.Errorf("Compact output missing component: %s", comp)
					}
				}
			},
		},
		{
			name: "minimal snapshot",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:    "compact-002",
					Timestamp: now,
					Duration:  100,
					Status:    SnapStatusSuccess,
				},
				Logs: []SerialLogEntry{},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "compact-002") {
					t.Error("Compact output missing snap ID")
				}
				if !strings.Contains(output, "success") {
					t.Error("Compact output missing status")
				}
				if !strings.Contains(output, "100ms") {
					t.Error("Compact output missing duration")
				}
			},
		},
		{
			name: "failed snapshot",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:    "compact-003",
					Timestamp: now,
					Duration:  50,
					Status:    SnapStatusFailed,
					Error:     "timeout",
				},
				Logs: []SerialLogEntry{},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "failed") {
					t.Error("Compact output should show failed status")
				}
			},
		},
		{
			name: "partial status",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:         "compact-004",
					Timestamp:      now,
					Duration:       300,
					Status:         SnapStatusPartial,
					FlashEnabled:   true,
					MonitorEnabled: true,
					LogEntryCount:  5,
				},
				Logs: []SerialLogEntry{},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "partial") {
					t.Error("Compact output should show partial status")
				}
				if !strings.Contains(output, "flash") {
					t.Error("Compact output should show flash enabled")
				}
				if !strings.Contains(output, "monitor:5") {
					t.Error("Compact output should show monitor count")
				}
			},
		},
		{
			name: "flash without firmware path",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:       "compact-005",
					Timestamp:    now,
					Duration:     200,
					Status:       SnapStatusSuccess,
					FlashEnabled: true,
				},
				Logs: []SerialLogEntry{},
			},
			wantErr: false,
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "flash") {
					t.Error("Compact output should show flash enabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := NewHandler(OutputFormatCompact, &buf, "")

			err := handler.Write(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}

// TestOutput_SaveImage tests saving images to disk with comprehensive cases.
func TestOutput_SaveImage(t *testing.T) {
	now := time.Now()
	testImageData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	tests := []struct {
		name       string
		result     *SnapResult
		saveDir    string
		wantErr    bool
		checkFiles func(*testing.T, string)
	}{
		{
			name: "save with image data",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:         "save-img-001",
					Timestamp:      now,
					Duration:       100,
					Status:         SnapStatusSuccess,
					CaptureEnabled: true,
					ImageSize:      len(testImageData),
				},
				ImageData: testImageData,
				Logs:      []SerialLogEntry{},
			},
			saveDir: func() string {
				dir, _ := os.MkdirTemp("", "snap-save-test-*")
				return dir
			}(),
			wantErr: false,
			checkFiles: func(t *testing.T, dir string) {
				imgFile := filepath.Join(dir, "snap-save-img-001.jpg")
				metaFile := filepath.Join(dir, "snap-save-img-001.json")
				if _, err := os.Stat(imgFile); os.IsNotExist(err) {
					t.Errorf("Image file not created: %s", imgFile)
				}
				if _, err := os.Stat(metaFile); os.IsNotExist(err) {
					t.Errorf("Metadata file not created: %s", metaFile)
				}
			},
		},
		{
			name: "save without image data",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:    "save-img-002",
					Timestamp: now,
					Duration:  50,
					Status:    SnapStatusSuccess,
				},
				ImageData: []byte{},
				Logs:      []SerialLogEntry{},
			},
			saveDir: func() string {
				dir, _ := os.MkdirTemp("", "snap-save-test-*")
				return dir
			}(),
			wantErr: false,
			checkFiles: func(t *testing.T, dir string) {
				metaFile := filepath.Join(dir, "snap-save-img-002.json")
				if _, err := os.Stat(metaFile); os.IsNotExist(err) {
					t.Errorf("Metadata file not created: %s", metaFile)
				}
				imgFile := filepath.Join(dir, "snap-save-img-002.jpg")
				if _, err := os.Stat(imgFile); !os.IsNotExist(err) {
					t.Errorf("Image file should not exist: %s", imgFile)
				}
			},
		},
		{
			name: "save with nested directory creation",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:         "save-img-003",
					Timestamp:      now,
					Duration:       100,
					Status:         SnapStatusSuccess,
					CaptureEnabled: true,
					ImageSize:      len(testImageData),
				},
				ImageData: testImageData,
				Logs:      []SerialLogEntry{},
			},
			saveDir: func() string {
				baseDir, _ := os.MkdirTemp("", "snap-save-test-*")
				return filepath.Join(baseDir, "nested", "path")
			}(),
			wantErr: false,
			checkFiles: func(t *testing.T, dir string) {
				imgFile := filepath.Join(dir, "snap-save-img-003.jpg")
				metaFile := filepath.Join(dir, "snap-save-img-003.json")
				if _, err := os.Stat(imgFile); os.IsNotExist(err) {
					t.Errorf("Image file not created in nested path: %s", imgFile)
				}
				if _, err := os.Stat(metaFile); os.IsNotExist(err) {
					t.Errorf("Metadata file not created in nested path: %s", metaFile)
				}
			},
		},
		{
			name: "verify image content integrity",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:         "save-img-004",
					Timestamp:      now,
					Duration:       100,
					Status:         SnapStatusSuccess,
					CaptureEnabled: true,
					ImageSize:      len(testImageData),
				},
				ImageData: testImageData,
				Logs:      []SerialLogEntry{},
			},
			saveDir: func() string {
				dir, _ := os.MkdirTemp("", "snap-save-test-*")
				return dir
			}(),
			wantErr: false,
			checkFiles: func(t *testing.T, dir string) {
				imgFile := filepath.Join(dir, "snap-save-img-004.jpg")
				data, err := os.ReadFile(imgFile)
				if err != nil {
					t.Fatalf("Failed to read saved image: %v", err)
				}
				if len(data) != len(testImageData) {
					t.Errorf("Image size = %d, want %d", len(data), len(testImageData))
				}
				for i := range testImageData {
					if data[i] != testImageData[i] {
						t.Errorf("Image data mismatch at byte %d", i)
					}
				}
			},
		},
		{
			name: "verify metadata content",
			result: &SnapResult{
				Metadata: SnapMetadata{
					SnapID:         "save-img-005",
					Timestamp:      now,
					Duration:       250,
					Status:         SnapStatusSuccess,
					DevicePath:     "/dev/ttyUSB0",
					ChipName:       "ESP32",
					FlashEnabled:   true,
					FlashFirmware:  "/firmware.bin",
					FlashOffset:    0x10000,
					MonitorEnabled: true,
					LogEntryCount:  10,
					CaptureEnabled: true,
					ImageSize:      len(testImageData),
				},
				ImageData: testImageData,
				Logs: []SerialLogEntry{
					{
						Timestamp: now,
						Message:   "Test log",
						Level:     "info",
					},
				},
			},
			saveDir: func() string {
				dir, _ := os.MkdirTemp("", "snap-save-test-*")
				return dir
			}(),
			wantErr: false,
			checkFiles: func(t *testing.T, dir string) {
				metaFile := filepath.Join(dir, "snap-save-img-005.json")
				data, err := os.ReadFile(metaFile)
				if err != nil {
					t.Fatalf("Failed to read metadata: %v", err)
				}
				if !strings.Contains(string(data), "save-img-005") {
					t.Error("Metadata missing snap_id")
				}
				if !strings.Contains(string(data), "ESP32") {
					t.Error("Metadata missing chip_name")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.saveDir != "" && strings.Contains(tt.saveDir, "snap-save-test") {
				defer os.RemoveAll(tt.saveDir)
			}

			var buf bytes.Buffer
			handler := NewHandler(OutputFormatJSON, &buf, tt.saveDir)

			err := handler.Write(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFiles != nil {
				tt.checkFiles(t, tt.saveDir)
			}
		})
	}
}
