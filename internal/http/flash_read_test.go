package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/gorilla/mux"
)

// mockNode for testing
type mockNode struct {
	devices []*protocol.DeviceInfo
}

func (m *mockNode) State() *cluster.ClusterState {
	devs := make(map[string]*protocol.DeviceInfo, len(m.devices))
	for _, d := range m.devices {
		devs[d.Path] = d
	}
	return &cluster.ClusterState{
		Devices: devs,
	}
}

func (m *mockNode) Start(ctx context.Context) error {
	return nil
}

func (m *mockNode) Stop() error {
	return nil
}

func (m *mockNode) ID() string {
	return "test-node"
}

func TestReadFlashHandlerSubmit(t *testing.T) {
	tmpDir := t.TempDir()
	node := &mockNode{
		devices: []*protocol.DeviceInfo{
			{
				Path:   "/dev/ttyUSB0",
				Status: "available",
			},
		},
	}
	handler := NewReadFlashHandler(node, nil, tmpDir)

	req := ReadFlashRequest{
		DevicePath: "/dev/ttyUSB0",
		Address:    0x10000,
		Size:       4096,
	}

	body, _ := json.Marshal(req)
	r := httptest.NewRequest("POST", "/api/v1/flash/read", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleSubmit(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp ReadFlashResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JobID == "" {
		t.Error("expected non-empty job ID")
	}
	if resp.Status != "pending" {
		t.Errorf("expected status pending, got %s", resp.Status)
	}
}

func TestReadFlashHandlerSubmitMissingDevice(t *testing.T) {
	tmpDir := t.TempDir()
	node := &mockNode{devices: []*protocol.DeviceInfo{}}
	handler := NewReadFlashHandler(node, nil, tmpDir)

	req := ReadFlashRequest{
		DevicePath: "/dev/ttyUSB0",
		Address:    0x10000,
		Size:       4096,
	}

	body, _ := json.Marshal(req)
	r := httptest.NewRequest("POST", "/api/v1/flash/read", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleSubmit(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestReadFlashHandlerSubmitInvalidSize(t *testing.T) {
	tmpDir := t.TempDir()
	node := &mockNode{
		devices: []*protocol.DeviceInfo{
			{Path: "/dev/ttyUSB0", Status: "available"},
		},
	}
	handler := NewReadFlashHandler(node, nil, tmpDir)

	tests := []struct {
		name       string
		size       uint32
		wantStatus int
	}{
		{"zero size", 0, http.StatusBadRequest},
		{"size too large", 17 * 1024 * 1024, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ReadFlashRequest{
				DevicePath: "/dev/ttyUSB0",
				Address:    0x10000,
				Size:       tt.size,
			}

			body, _ := json.Marshal(req)
			r := httptest.NewRequest("POST", "/api/v1/flash/read", strings.NewReader(string(body)))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.handleSubmit(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestReadFlashHandlerStatus(t *testing.T) {
	tmpDir := t.TempDir()
	node := &mockNode{
		devices: []*protocol.DeviceInfo{
			{Path: "/dev/ttyUSB0", Status: "available"},
		},
	}
	handler := NewReadFlashHandler(node, nil, tmpDir)

	// Create a job directly
	jobID := "test-job-123"
	job := &ReadJob{
		ID:         jobID,
		DevicePath: "/dev/ttyUSB0",
		Address:    0x10000,
		Size:       4096,
		Status:     "completed",
		CreatedAt:  time.Now(),
	}
	job.DataPath = filepath.Join(tmpDir, job.ID+".bin")
	os.WriteFile(job.DataPath, []byte("test data"), 0644)

	handler.mu.Lock()
	handler.jobs[jobID] = job
	handler.mu.Unlock()

	r := httptest.NewRequest("GET", "/api/v1/flash/read/"+jobID, nil)
	w := httptest.NewRecorder()

	// Use mux to extract the job ID
	router := mux.NewRouter()
	router.HandleFunc("/api/v1/flash/read/{id}", handler.handleStatus)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp ReadFlashResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "completed" {
		t.Errorf("expected status completed, got %s", resp.Status)
	}
	if resp.DownloadURL == "" {
		t.Error("expected download URL for completed job")
	}
}

func TestReadFlashHandlerStatusNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	node := &mockNode{devices: []*protocol.DeviceInfo{}}
	handler := NewReadFlashHandler(node, nil, tmpDir)

	r := httptest.NewRequest("GET", "/api/v1/flash/read/nonexistent", nil)
	w := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/flash/read/{id}", handler.handleStatus)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestReadFlashHandlerDownload(t *testing.T) {
	tmpDir := t.TempDir()
	node := &mockNode{devices: []*protocol.DeviceInfo{}}
	handler := NewReadFlashHandler(node, nil, tmpDir)

	// Create a completed job with data
	jobID := "test-job-download"
	job := &ReadJob{
		ID:         jobID,
		DevicePath: "/dev/ttyUSB0",
		Status:     "completed",
		CreatedAt:  time.Now(),
	}
	dataPath := filepath.Join(tmpDir, job.ID+".bin")
	testData := []byte{0x01, 0x02, 0x03, 0x04}
	os.WriteFile(dataPath, testData, 0644)
	job.DataPath = dataPath

	handler.mu.Lock()
	handler.jobs[jobID] = job
	handler.mu.Unlock()

	r := httptest.NewRequest("GET", "/api/v1/flash/download/"+jobID, nil)
	w := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/flash/download/{id}", handler.handleDownload)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/octet-stream" {
		t.Errorf("expected content-type application/octet-stream, got %s", contentType)
	}

	responseBody := w.Body.Bytes()
	if len(responseBody) != len(testData) {
		t.Errorf("expected %d bytes, got %d", len(testData), len(responseBody))
	}
}

func TestReadFlashHandlerDownloadNotReady(t *testing.T) {
	tmpDir := t.TempDir()
	node := &mockNode{devices: []*protocol.DeviceInfo{}}
	handler := NewReadFlashHandler(node, nil, tmpDir)

	// Create a pending job
	jobID := "test-job-pending"
	job := &ReadJob{
		ID:         jobID,
		DevicePath: "/dev/ttyUSB0",
		Status:     "pending",
		CreatedAt:  time.Now(),
	}

	handler.mu.Lock()
	handler.jobs[jobID] = job
	handler.mu.Unlock()

	r := httptest.NewRequest("GET", "/api/v1/flash/download/"+jobID, nil)
	w := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/flash/download/{id}", handler.handleDownload)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestReadFlashHandlerCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	node := &mockNode{devices: []*protocol.DeviceInfo{}}
	handler := NewReadFlashHandler(node, nil, tmpDir)

	// Create an old job
	oldJob := &ReadJob{
		ID:         "old-job",
		DevicePath: "/dev/ttyUSB0",
		Status:     "completed",
		CreatedAt:  time.Now().Add(-15 * time.Minute),
	}
	dataPath := filepath.Join(tmpDir, oldJob.ID+".bin")
	os.WriteFile(dataPath, []byte("old data"), 0644)
	oldJob.DataPath = dataPath

	// Create a recent job
	recentJob := &ReadJob{
		ID:         "recent-job",
		DevicePath: "/dev/ttyUSB1",
		Status:     "completed",
		CreatedAt:  time.Now().Add(-5 * time.Minute),
	}

	handler.mu.Lock()
	handler.jobs["old-job"] = oldJob
	handler.jobs["recent-job"] = recentJob
	handler.mu.Unlock()

	// Run cleanup
	handler.cleanup()

	// Verify old job removed
	handler.mu.RLock()
	_, exists := handler.jobs["old-job"]
	handler.mu.RUnlock()
	if exists {
		t.Error("expected old job to be cleaned up")
	}

	// Verify recent job still exists
	handler.mu.RLock()
	_, exists = handler.jobs["recent-job"]
	handler.mu.RUnlock()
	if !exists {
		t.Error("expected recent job to still exist")
	}

	// Verify old data file removed
	if _, err := os.Stat(dataPath); err == nil {
		t.Error("expected old data file to be removed")
	}
}
