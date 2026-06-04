package cluster

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/devices" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		devices := []DeviceInfo{
			{Path: "/dev/ttyUSB0", VID: "0x4348", PID: "0x0028", State: "available"},
			{Path: "/dev/ttyUSB1", VID: "0x4348", PID: "0x0029", State: "busy"},
		}
		json.NewEncoder(w).Encode(devices)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	devices, err := client.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	if devices[0].Path != "/dev/ttyUSB0" {
		t.Errorf("expected /dev/ttyUSB0, got %s", devices[0].Path)
	}

	if devices[0].State != "available" {
		t.Errorf("expected available, got %s", devices[0].State)
	}
}

func TestClient_ListDevices_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.ListDevices()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_GetStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		status := ClusterStatus{
			NodesCount:   3,
			DevicesCount: 5,
			JobsCount:    2,
			Role:         "master",
			QueueSize:    1,
		}
		json.NewEncoder(w).Encode(status)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	status, err := client.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.NodesCount != 3 {
		t.Errorf("expected 3 nodes, got %d", status.NodesCount)
	}

	if status.Role != "master" {
		t.Errorf("expected master, got %s", status.Role)
	}
}

func TestClient_GetStatus_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetStatus()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestExecuteSnap_URL_Encoding verifies that device IDs with slashes
// are properly URL-encoded in the request query parameter.
func TestExecuteSnap_URL_Encoding(t *testing.T) {
	// Track the actual device_id received
	var receivedDeviceID string

	// Mock server that returns the path it received
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedDeviceID = r.URL.Query().Get("device_id")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"snap_id":"test-snap","status":"success"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	// Test with device path containing slashes
	tests := []struct {
		name         string
		deviceID     string
		wantDeviceID string
	}{
		{
			name:         "device path with slashes",
			deviceID:     "/dev/ttyACM0",
			wantDeviceID: "/dev/ttyACM0", // Query parameter should be decoded by server
		},
		{
			name:         "device path with COM port",
			deviceID:     "COM3",
			wantDeviceID: "COM3",
		},
		{
			name:         "device ID without slashes",
			deviceID:     "esp-aa:bb:cc:dd:ee:ff",
			wantDeviceID: "esp-aa:bb:cc:dd:ee:ff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := SnapRequest{
				DeviceID: tt.deviceID,
				Duration: 10,
			}

			_, err := client.ExecuteSnap(req)
			if err != nil {
				t.Fatalf("ExecuteSnap() error = %v", err)
			}

			if receivedDeviceID != tt.wantDeviceID {
				t.Errorf("ExecuteSnap() device_id = %s, want %s", receivedDeviceID, tt.wantDeviceID)
			}
		})
	}
}

// TestExecuteSnap_DeviceNotFound verifies proper error handling when
// the snap endpoint returns 404 for an unknown device.
func TestExecuteSnap_DeviceNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`404 page not found`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	req := SnapRequest{
		DeviceID: "unknown-device",
		Duration: 10,
	}

	_, err := client.ExecuteSnap(req)
	if err == nil {
		t.Fatal("ExecuteSnap() expected error for 404 response, got nil")
	}

	// Verify error message contains status info
	errStr := err.Error()
	if len(errStr) < 10 || !contains(errStr, "status 404") {
		t.Errorf("Error message should contain status info, got: %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
