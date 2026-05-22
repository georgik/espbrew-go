package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/stretchr/testify/assert"
)

func TestAPIHandler_HandleStatus(t *testing.T) {
	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:          8080,
		DisablemDNS:       true,
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
	})
	ctx := context.Background()
	master.Start(ctx)
	defer master.Stop()

	handler := NewAPIHandler(master)
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()

	handler.handleStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	assert.Equal(t, 0.0, resp["nodes_count"])
	assert.Equal(t, 0.0, resp["devices_count"])
	assert.Equal(t, "leader", resp["role"])
	assert.Equal(t, 0.0, resp["queue_size"])
}

func TestAPIHandler_HandleNodes(t *testing.T) {
	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:          8080,
		DisablemDNS:       true,
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
	})
	master.Start(context.Background())
	defer master.Stop()

	handler := NewAPIHandler(master)
	req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	w := httptest.NewRecorder()

	handler.handleNodes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	assert.NotNil(t, resp)
}

func TestAPIHandler_HandleDevices(t *testing.T) {
	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:          8080,
		DisablemDNS:       true,
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
	})
	master.Start(context.Background())
	defer master.Stop()

	// Register a test device
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0027,
		Status: "available",
	})

	handler := NewAPIHandler(master)
	req := httptest.NewRequest("GET", "/api/v1/devices", nil)
	w := httptest.NewRecorder()

	handler.handleDevices(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	assert.Equal(t, 1, len(resp))
}

func TestAPIHandler_HandleQueue(t *testing.T) {
	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:          8080,
		DisablemDNS:       true,
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
	})
	master.Start(context.Background())
	defer master.Stop()

	// Add a device
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0027,
		Status: "available",
	})

	handler := NewAPIHandler(master)
	req := httptest.NewRequest("GET", "/api/v1/queue", nil)
	w := httptest.NewRecorder()

	handler.handleQueue(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	assert.Equal(t, 0.0, resp["pending"])
}

func TestAPIHandler_CreateJob(t *testing.T) {
	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:          8080,
		DisablemDNS:       true,
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
	})
	master.Start(context.Background())
	defer master.Stop()

	// Add a device
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0027,
		Status: "available",
	})

	handler := NewAPIHandler(master)

	body := `{"firmware": "test.bin", "device_path": "/dev/ttyUSB0"}`
	req := httptest.NewRequest("POST", "/api/v1/jobs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreateJob(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	assert.NotEmpty(t, resp["id"])
	assert.Equal(t, "test.bin", resp["firmware"])
	assert.Equal(t, "pending", resp["status"])
}

func TestServer_Start(t *testing.T) {
	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:          8081,
		DisablemDNS:       true,
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
	})
	master.Start(context.Background())
	defer master.Stop()

	server := NewServer("127.0.0.1:18081", master)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	assert.NoError(t, err)

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	resp, err := http.Get("http://127.0.0.1:18081/health")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]string
	json.NewDecoder(resp.Body).Decode(&health)
	assert.Equal(t, "healthy", health["status"])

	cancel()
}
