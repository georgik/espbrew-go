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
