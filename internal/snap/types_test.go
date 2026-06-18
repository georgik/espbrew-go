package snap

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSnapMetadata_Validation tests that SnapMetadata fields are properly set and validated.
func TestSnapMetadata_Validation(t *testing.T) {
	tests := []struct {
		name     string
		metadata SnapMetadata
		wantErr  bool
	}{
		{
			name: "valid metadata with all fields",
			metadata: SnapMetadata{
				SnapID:          "snap-12345",
				Timestamp:       time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
				Duration:        5000,
				DevicePath:      "/dev/ttyUSB0",
				ChipID:          "0x0000C45",
				ChipName:        "ESP32-C3",
				FlashEnabled:    true,
				FlashFirmware:   "/path/to/firmware.bin",
				FlashOffset:     0x1000,
				FlashSkipped:    false,
				FlashHashBefore: "abc123",
				FlashHashAfter:  "def456",
				MonitorEnabled:  true,
				MonitorDuration: 30 * time.Second,
				MonitorBaudRate: 115200,
				LogEntryCount:   100,
				CaptureEnabled:  true,
				CameraID:        "/dev/video0",
				ImageFormat:     "png",
				ImageSize:       1024,
				Status:          SnapStatusSuccess,
			},
			wantErr: false,
		},
		{
			name: "valid minimal metadata",
			metadata: SnapMetadata{
				SnapID:    "snap-minimal",
				Timestamp: time.Now(),
				Duration:  100,
				Status:    SnapStatusSuccess,
			},
			wantErr: false,
		},
		{
			name: "metadata with partial status",
			metadata: SnapMetadata{
				SnapID:    "snap-partial",
				Timestamp: time.Now(),
				Duration:  2000,
				Status:    SnapStatusPartial,
				Error:     "camera capture failed",
			},
			wantErr: false,
		},
		{
			name: "metadata with failed status",
			metadata: SnapMetadata{
				SnapID:    "snap-failed",
				Timestamp: time.Now(),
				Duration:  500,
				Status:    SnapStatusFailed,
				Error:     "device not found",
			},
			wantErr: false,
		},
		{
			name: "metadata with flash skipped",
			metadata: SnapMetadata{
				SnapID:         "snap-flash-skipped",
				Timestamp:      time.Now(),
				Duration:       1000,
				FlashEnabled:   true,
				FlashSkipped:   true,
				FlashFirmware:  "/path/to/firmware.bin",
				MonitorEnabled: true,
				Status:         SnapStatusSuccess,
			},
			wantErr: false,
		},
		{
			name: "metadata with capture disabled",
			metadata: SnapMetadata{
				SnapID:         "snap-no-capture",
				Timestamp:      time.Now(),
				Duration:       1500,
				FlashEnabled:   true,
				MonitorEnabled: true,
				CaptureEnabled: false,
				ImageSize:      0,
				Status:         SnapStatusSuccess,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that metadata can be marshaled to JSON
			data, err := json.Marshal(tt.metadata)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Test that metadata can be unmarshaled from JSON
			var unmarshaled SnapMetadata
			if err := json.Unmarshal(data, &unmarshaled); err != nil && !tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v", err)
				return
			}

			// Verify key fields are preserved
			if unmarshaled.SnapID != tt.metadata.SnapID {
				t.Errorf("SnapID = %v, want %v", unmarshaled.SnapID, tt.metadata.SnapID)
			}
			if unmarshaled.Status != tt.metadata.Status {
				t.Errorf("Status = %v, want %v", unmarshaled.Status, tt.metadata.Status)
			}
		})
	}
}

// TestSerialLogEntry_Timestamp tests timestamp handling in SerialLogEntry.
func TestSerialLogEntry_Timestamp(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		entry   SerialLogEntry
		wantTS  time.Time
		wantMsg string
	}{
		{
			name: "entry with info level",
			entry: SerialLogEntry{
				Timestamp: now,
				Message:   "System starting",
				Level:     "info",
				Source:    "main",
			},
			wantTS:  now,
			wantMsg: "System starting",
		},
		{
			name: "entry with error level",
			entry: SerialLogEntry{
				Timestamp: now.Add(-1 * time.Hour),
				Message:   "Connection failed",
				Level:     "error",
				Source:    "network",
			},
			wantTS:  now.Add(-1 * time.Hour),
			wantMsg: "Connection failed",
		},
		{
			name: "entry with debug level",
			entry: SerialLogEntry{
				Timestamp: now.Add(-5 * time.Minute),
				Message:   "Variable x = 42",
				Level:     "debug",
				Source:    "logger",
			},
			wantTS:  now.Add(-5 * time.Minute),
			wantMsg: "Variable x = 42",
		},
		{
			name: "entry with warn level",
			entry: SerialLogEntry{
				Timestamp: now.Add(-10 * time.Second),
				Message:   "High memory usage",
				Level:     "warn",
			},
			wantTS:  now.Add(-10 * time.Second),
			wantMsg: "High memory usage",
		},
		{
			name: "entry without level",
			entry: SerialLogEntry{
				Timestamp: now,
				Message:   "Plain log message",
			},
			wantTS:  now,
			wantMsg: "Plain log message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test timestamp preservation
			if !tt.entry.Timestamp.Equal(tt.wantTS) {
				t.Errorf("Timestamp = %v, want %v", tt.entry.Timestamp, tt.wantTS)
			}

			// Test message preservation
			if tt.entry.Message != tt.wantMsg {
				t.Errorf("Message = %v, want %v", tt.entry.Message, tt.wantMsg)
			}

			// Test JSON serialization
			data, err := json.Marshal(tt.entry)
			if err != nil {
				t.Errorf("json.Marshal() error = %v", err)
				return
			}

			var unmarshaled SerialLogEntry
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Errorf("json.Unmarshal() error = %v", err)
				return
			}

			// Verify timestamp is preserved through JSON round-trip
			if !unmarshaled.Timestamp.Equal(tt.entry.Timestamp) {
				t.Errorf("After round-trip Timestamp = %v, want %v", unmarshaled.Timestamp, tt.entry.Timestamp)
			}
		})
	}
}

// TestSnapResult_Empty tests empty and default SnapResult values.
func TestSnapResult_Empty(t *testing.T) {
	tests := []struct {
		name   string
		result SnapResult
		check  func(*testing.T, SnapResult)
	}{
		{
			name:   "zero value result",
			result: SnapResult{},
			check: func(t *testing.T, r SnapResult) {
				if r.Metadata.SnapID != "" {
					t.Errorf("Metadata.SnapID = %v, want empty", r.Metadata.SnapID)
				}
				if r.Logs != nil {
					t.Errorf("Logs = %v, want nil", r.Logs)
				}
				if len(r.ImageData) != 0 {
					t.Errorf("ImageData length = %v, want 0", len(r.ImageData))
				}
				if r.ImageBase64 != "" {
					t.Errorf("ImageBase64 = %v, want empty", r.ImageBase64)
				}
			},
		},
		{
			name: "result with empty logs slice",
			result: SnapResult{
				Logs: []SerialLogEntry{},
			},
			check: func(t *testing.T, r SnapResult) {
				if len(r.Logs) != 0 {
					t.Errorf("Logs length = %v, want 0", len(r.Logs))
				}
			},
		},
		{
			name: "result with nil logs",
			result: SnapResult{
				Logs: nil,
			},
			check: func(t *testing.T, r SnapResult) {
				if r.Logs != nil {
					t.Errorf("Logs = %v, want nil", r.Logs)
				}
			},
		},
		{
			name: "result with empty image data",
			result: SnapResult{
				ImageData:   []byte{},
				ImageBase64: "",
			},
			check: func(t *testing.T, r SnapResult) {
				if len(r.ImageData) != 0 {
					t.Errorf("ImageData length = %v, want 0", len(r.ImageData))
				}
				if r.ImageBase64 != "" {
					t.Errorf("ImageBase64 = %v, want empty", r.ImageBase64)
				}
			},
		},
		{
			name:   "ToMap with empty result",
			result: SnapResult{},
			check: func(t *testing.T, r SnapResult) {
				m := r.ToMap(true)
				if len(m) != 2 {
					t.Errorf("ToMap() length = %v, want 2", len(m))
				}
				if _, ok := m["metadata"]; !ok {
					t.Error("ToMap() missing 'metadata' key")
				}
				if _, ok := m["logs"]; !ok {
					t.Error("ToMap() missing 'logs' key")
				}
			},
		},
		{
			name: "ToMap with image data",
			result: SnapResult{
				ImageBase64: "base64encodeddata",
			},
			check: func(t *testing.T, r SnapResult) {
				m := r.ToMap(true)
				if len(m) != 3 {
					t.Errorf("ToMap() length = %v, want 3", len(m))
				}
				if _, ok := m["image_base64"]; !ok {
					t.Error("ToMap() missing 'image_base64' key")
				}
				if m["image_base64"] != "base64encodeddata" {
					t.Errorf("image_base64 = %v, want 'base64encodeddata'", m["image_base64"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.check != nil {
				tt.check(t, tt.result)
			}
		})
	}
}

// TestSnapStatus_Values tests all valid SnapStatus values.
func TestSnapStatus_Values(t *testing.T) {
	tests := []struct {
		name   string
		status SnapStatus
		valid  bool
	}{
		{
			name:   "success status",
			status: SnapStatusSuccess,
			valid:  true,
		},
		{
			name:   "partial status",
			status: SnapStatusPartial,
			valid:  true,
		},
		{
			name:   "failed status",
			status: SnapStatusFailed,
			valid:  true,
		},
		{
			name:   "custom status",
			status: SnapStatus("custom"),
			valid:  false,
		},
		{
			name:   "empty status",
			status: SnapStatus(""),
			valid:  false,
		},
	}

	validStatuses := map[SnapStatus]bool{
		SnapStatusSuccess: true,
		SnapStatusPartial: true,
		SnapStatusFailed:  true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := validStatuses[tt.status]
			if isValid != tt.valid {
				t.Errorf("Status %v validity = %v, want %v", tt.status, isValid, tt.valid)
			}

			// Test that valid statuses can be used in metadata
			if tt.valid {
				metadata := SnapMetadata{
					SnapID:    "test-" + string(tt.status),
					Timestamp: time.Now(),
					Status:    tt.status,
				}

				data, err := json.Marshal(metadata)
				if err != nil {
					t.Errorf("json.Marshal() with status %v error = %v", tt.status, err)
				}

				var unmarshaled SnapMetadata
				if err := json.Unmarshal(data, &unmarshaled); err != nil {
					t.Errorf("json.Unmarshal() with status %v error = %v", tt.status, err)
				}

				if unmarshaled.Status != tt.status {
					t.Errorf("After round-trip Status = %v, want %v", unmarshaled.Status, tt.status)
				}
			}
		})
	}
}
