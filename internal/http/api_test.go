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
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestAPIHandler_HandleStatus(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	ctx := context.Background()
	master.Start(ctx)
	defer master.Stop()

	handler := NewAPIHandler(master, store)
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()

	handler.handleStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	assert.Equal(t, 1.0, resp["nodes_count"])   // Leader node counts itself
	assert.Equal(t, 4.0, resp["devices_count"]) // 4 virtual devices auto-registered
	assert.Equal(t, "leader", resp["role"])
	assert.Equal(t, 0.0, resp["queue_size"])
}

func TestAPIHandler_HandleNodes(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	handler := NewAPIHandler(master, store)
	req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	w := httptest.NewRecorder()

	handler.handleNodes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	assert.NotNil(t, resp)
}

func TestAPIHandler_HandleDevices(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	// Register some test devices
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0027,
		Status: "available",
	})
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB1",
		VID:    0x4348,
		PID:    0x0027,
		Status: "busy",
	})

	handler := NewAPIHandler(master, store)
	req := httptest.NewRequest("GET", "/api/v1/devices", nil)
	w := httptest.NewRecorder()

	handler.handleDevices(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	assert.True(t, len(resp) >= 2, "Should have at least 2 devices")
}

func TestAPIHandler_HandleReserveDevice(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	// Register a test device
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0027,
		Status: "available",
	})

	handler := NewAPIHandler(master, store)

	// Test reserve
	req := httptest.NewRequest("POST", "/api/v1/devices/ttyUSB0/reserve", strings.NewReader(`{"client_id":"test-client","ttl":300}`))
	req = mux.SetURLVars(req, map[string]string{"name": "ttyUSB0"})
	w := httptest.NewRecorder()

	handler.handleReserveDevice(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	assert.Equal(t, "reserved", resp["status"])

	// Test double reserve (should fail)
	req2 := httptest.NewRequest("POST", "/api/v1/devices/ttyUSB0/reserve", strings.NewReader(`{"client_id":"another-client","ttl":300}`))
	req2 = mux.SetURLVars(req2, map[string]string{"name": "ttyUSB0"})
	w2 := httptest.NewRecorder()

	handler.handleReserveDevice(w2, req2)

	assert.Equal(t, http.StatusConflict, w2.Code)

	// Test release
	req3 := httptest.NewRequest("DELETE", "/api/v1/devices/ttyUSB0/reserve", strings.NewReader(`{"client_id":"test-client"}`))
	req3 = mux.SetURLVars(req3, map[string]string{"name": "ttyUSB0"})
	w3 := httptest.NewRecorder()

	handler.handleReserveDevice(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
}

func TestAPIHandler_HandleReserveDevice_NotFound(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	handler := NewAPIHandler(master, store)

	// Test reserve on non-existent device
	req := httptest.NewRequest("POST", "/api/v1/devices/doesnotexist/reserve", strings.NewReader(`{"client_id":"test-client"}`))
	req = mux.SetURLVars(req, map[string]string{"name": "doesnotexist"})
	w := httptest.NewRecorder()

	handler.handleReserveDevice(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPIHandler_HandleNodeJobProgress(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	handler := NewAPIHandler(master, store)

	// Create a test job
	queue := master.GetJobQueue()
	job := queue.EnqueueFlash("/path/to/firmware.bin", "/dev/ttyUSB0", 0, false)
	job.DeviceNode = "test-peer-1"

	// Test progress update
	payload := `{
		"job_id": "` + job.ID + `",
		"status": "running",
		"progress": 50,
		"node_id": "test-peer-1"
	}`

	req := httptest.NewRequest("POST", "/api/v1/nodes/test-peer-1/jobs/"+job.ID+"/progress", strings.NewReader(payload))
	req = mux.SetURLVars(req, map[string]string{"id": "test-peer-1", "job_id": job.ID})
	w := httptest.NewRecorder()

	handler.handleNodeJobProgress(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "ok", resp["status"])

	// Verify progress was updated
	updatedJob := queue.Get(job.ID)
	assert.NotNil(t, updatedJob)
	assert.Equal(t, 50, updatedJob.Progress)
}

func TestAPIHandler_HandleNodeJobProgress_Complete(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	// Register a test device
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		NodeID: "test-peer-1",
		Status: "busy",
	})

	handler := NewAPIHandler(master, store)

	// Create a test job
	queue := master.GetJobQueue()
	job := queue.EnqueueFlash("/path/to/firmware.bin", "/dev/ttyUSB0", 0, false)
	job.DeviceNode = "test-peer-1"

	// Test completion
	payload := `{
		"job_id": "` + job.ID + `",
		"status": "completed",
		"progress": 100,
		"node_id": "test-peer-1"
	}`

	req := httptest.NewRequest("POST", "/api/v1/nodes/test-peer-1/jobs/"+job.ID+"/progress", strings.NewReader(payload))
	req = mux.SetURLVars(req, map[string]string{"id": "test-peer-1", "job_id": job.ID})
	w := httptest.NewRecorder()

	handler.handleNodeJobProgress(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify job completed
	updatedJob := queue.Get(job.ID)
	assert.NotNil(t, updatedJob)
	assert.Equal(t, cluster.JobStatus("completed"), updatedJob.Status)
	assert.Equal(t, 100, updatedJob.Progress)

	// Verify device released
	state := master.State()
	dev := state.Devices["/dev/ttyUSB0"]
	assert.Equal(t, "available", dev.Status)
}

func TestAPIHandler_HandleNodeJobProgress_Failed(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	// Register a test device
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		NodeID: "test-peer-1",
		Status: "busy",
	})

	handler := NewAPIHandler(master, store)

	// Create a test job
	queue := master.GetJobQueue()
	job := queue.EnqueueFlash("/path/to/firmware.bin", "/dev/ttyUSB0", 0, false)
	job.DeviceNode = "test-peer-1"

	// Test failure
	payload := `{
		"job_id": "` + job.ID + `",
		"status": "failed",
		"progress": 75,
		"node_id": "test-peer-1",
		"error": "flash operation failed: connection lost"
	}`

	req := httptest.NewRequest("POST", "/api/v1/nodes/test-peer-1/jobs/"+job.ID+"/progress", strings.NewReader(payload))
	req = mux.SetURLVars(req, map[string]string{"id": "test-peer-1", "job_id": job.ID})
	w := httptest.NewRecorder()

	handler.handleNodeJobProgress(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify job failed
	updatedJob := queue.Get(job.ID)
	assert.NotNil(t, updatedJob)
	assert.Equal(t, cluster.JobStatus("failed"), updatedJob.Status)
	assert.Contains(t, updatedJob.Error, "flash operation failed")

	// Verify device released
	state := master.State()
	dev := state.Devices["/dev/ttyUSB0"]
	assert.Equal(t, "available", dev.Status)
}

func TestAPIHandler_HandleNodeJobProgress_JobNotFound(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	handler := NewAPIHandler(master, store)

	payload := `{
		"job_id": "non-existent-job",
		"status": "running",
		"progress": 50,
		"node_id": "test-peer-1"
	}`

	req := httptest.NewRequest("POST", "/api/v1/nodes/test-peer-1/jobs/non-existent-job/progress", strings.NewReader(payload))
	req = mux.SetURLVars(req, map[string]string{"id": "test-peer-1", "job_id": "non-existent-job"})
	w := httptest.NewRecorder()

	handler.handleNodeJobProgress(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPIHandler_HandleNodeJobProgress_NodeMismatch(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	handler := NewAPIHandler(master, store)

	// Create a test job assigned to one peer
	queue := master.GetJobQueue()
	job := queue.EnqueueFlash("/path/to/firmware.bin", "/dev/ttyUSB0", 0, false)
	job.DeviceNode = "peer-1"

	// Try to send progress from a different peer
	payload := `{
		"job_id": "` + job.ID + `",
		"status": "running",
		"progress": 50,
		"node_id": "peer-2"
	}`

	req := httptest.NewRequest("POST", "/api/v1/nodes/peer-2/jobs/"+job.ID+"/progress", strings.NewReader(payload))
	req = mux.SetURLVars(req, map[string]string{"id": "peer-2", "job_id": job.ID})
	w := httptest.NewRecorder()

	handler.handleNodeJobProgress(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAPIHandler_HandleNodeJobProgress_InvalidPayload(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	master := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableMaintenance: true,
		DisableWatcher:     true,
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
	}, store)
	master.Start(context.Background())
	defer master.Stop()

	handler := NewAPIHandler(master, store)

	req := httptest.NewRequest("POST", "/api/v1/nodes/test-peer-1/jobs/some-job/progress", strings.NewReader("invalid json"))
	req = mux.SetURLVars(req, map[string]string{"id": "test-peer-1", "job_id": "some-job"})
	w := httptest.NewRecorder()

	handler.handleNodeJobProgress(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
