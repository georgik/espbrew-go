package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/georgik/esp-ci-cluster/internal/cluster"
	"github.com/georgik/esp-ci-cluster/pkg/protocol"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

func TestProgressHub_Broadcast(t *testing.T) {
	hub := NewProgressHub()
	jobID := "test-job"

	// Create mock subscribers
	sub1 := &ProgressSubscriber{
		jobID:  jobID,
		send:   make(chan ProgressMessage, 10),
		cancel: make(chan struct{}),
	}
	sub2 := &ProgressSubscriber{
		jobID:  jobID,
		send:   make(chan ProgressMessage, 10),
		cancel: make(chan struct{}),
	}

	hub.subscribers[jobID] = []*ProgressSubscriber{sub1, sub2}

	// Broadcast progress
	hub.BroadcastProgress(jobID, 50, "running")

	// Check both subscribers received
	select {
	case msg := <-sub1.send:
		if msg.Progress != 50 {
			t.Errorf("expected progress 50, got %d", msg.Progress)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub1 did not receive message")
	}

	select {
	case msg := <-sub2.send:
		if msg.Progress != 50 {
			t.Errorf("expected progress 50, got %d", msg.Progress)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub2 did not receive message")
	}
}

func TestProgressHandler_WebSocket(t *testing.T) {
	master := cluster.NewMasterNode("test-master", &cluster.MasterConfig{
		DisablemDNS: true,
	})

	hub := NewProgressHub()
	handler := NewProgressHandler(master, hub)

	// Register a test device
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0028,
		Status: "available",
	})

	// Create a test job
	job, err := master.EnqueueJob("test.bin", "/dev/ttyUSB0")
	if err != nil {
		t.Fatal(err)
	}

	// Create test server with gorilla/mux
	router := mux.NewRouter()
	router.HandleFunc("/api/v1/flash/{job_id}/progress", handler.handleProgressWebSocket).Methods("GET")

	server := httptest.NewServer(router)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + server.URL[4:] + "/api/v1/flash/" + job.ID + "/progress"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	// Should receive init message
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var msg ProgressMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read init message: %v", err)
	}

	if msg.Type != "init" {
		t.Errorf("expected type init, got %s", msg.Type)
	}
	if msg.JobID != job.ID {
		t.Errorf("expected job_id %s, got %s", job.ID, msg.JobID)
	}

	// Broadcast progress
	hub.BroadcastProgress(job.ID, 75, "running")

	// Should receive progress message
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read progress message: %v", err)
	}

	if msg.Type != "progress" {
		t.Errorf("expected type progress, got %s", msg.Type)
	}
	if msg.Progress != 75 {
		t.Errorf("expected progress 75, got %d", msg.Progress)
	}
}

func TestProgressHandler_JobNotFound(t *testing.T) {
	master := cluster.NewMasterNode("test-master", &cluster.MasterConfig{
		DisablemDNS: true,
	})

	hub := NewProgressHub()
	handler := NewProgressHandler(master, hub)

	req := httptest.NewRequest("GET", "/api/v1/flash/nonexistent/progress", nil)
	w := httptest.NewRecorder()

	handler.handleProgressWebSocket(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestProgressHandler_MultipleSubscribers(t *testing.T) {
	master := cluster.NewMasterNode("test-master", &cluster.MasterConfig{
		DisablemDNS: true,
	})

	hub := NewProgressHub()
	handler := NewProgressHandler(master, hub)

	// Register a test device and create job
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0028,
		Status: "available",
	})

	job, _ := master.EnqueueJob("test.bin", "/dev/ttyUSB0")

	// Create test server with gorilla/mux
	router := mux.NewRouter()
	router.HandleFunc("/api/v1/flash/{job_id}/progress", handler.handleProgressWebSocket).Methods("GET")

	server := httptest.NewServer(router)
	defer server.Close()

	// Connect multiple subscribers
	var conns []*websocket.Conn
	for i := 0; i < 3; i++ {
		wsURL := "ws" + server.URL[4:] + "/api/v1/flash/" + job.ID + "/progress"
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial websocket %d: %v", i, err)
		}
		defer conn.Close()
		conns = append(conns, conn)

		// Read and discard init message
		var msg ProgressMessage
		conn.ReadJSON(&msg)
	}

	// Broadcast progress
	hub.BroadcastProgress(job.ID, 30, "running")

	// All subscribers should receive progress
	for i, conn := range conns {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		var msg ProgressMessage
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("conn %d did not receive progress: %v", i, err)
		}
		if msg.Progress != 30 {
			t.Errorf("conn %d: expected progress 30, got %d", i, msg.Progress)
		}
	}
}

func TestClient_ProgressStream(t *testing.T) {
	// Start a real test server
	master := cluster.NewMasterNode("test-master", &cluster.MasterConfig{
		DisablemDNS: true,
	})

	hub := NewProgressHub()
	progressHandler := NewProgressHandler(master, hub)
	flashHandler := NewFlashHandler(master, os.TempDir())

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/flash/upload", flashHandler.handleUpload).Methods("POST")
	router.HandleFunc("/api/v1/flash", flashHandler.handleFlashSubmit).Methods("POST")
	router.HandleFunc("/api/v1/flash/{job_id}/progress", progressHandler.handleProgressWebSocket).Methods("GET")

	server := httptest.NewServer(router)
	defer server.Close()

	// Register device
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0028,
		Status: "available",
	})

	// Create client
	client := cluster.NewClient(server.URL)

	// Upload firmware
	testData := []byte("test firmware data")
	testFile := os.TempDir() + "/test-firmware.bin"
	os.WriteFile(testFile, testData, 0644)
	defer os.Remove(testFile)

	uploadResp, err := client.UploadFirmware(testFile)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	// Submit job
	flashResp, err := client.SubmitFlash(cluster.FlashSubmitRequest{
		DevicePath: "/dev/ttyUSB0",
		FileID:     uploadResp.FileID,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Connect to progress
	progressClient, err := client.ConnectProgress(flashResp.JobID)
	if err != nil {
		t.Fatalf("connect progress: %v", err)
	}
	defer progressClient.Close()

	// Simulate some progress
	go func() {
		for i := 0; i <= 100; i += 25 {
			time.Sleep(10 * time.Millisecond)
			hub.BroadcastProgress(flashResp.JobID, i, "running")
		}
		hub.BroadcastComplete(flashResp.JobID, "completed", "")
	}()

	// Stream progress
	progressCount := 0
	err = progressClient.Stream(func(msg cluster.ProgressMessage) {
		if msg.Type == "progress" {
			progressCount++
		}
	})

	if err != nil {
		t.Errorf("stream error: %v", err)
	}

	if progressCount == 0 {
		t.Error("expected at least one progress message")
	}

	fmt.Printf("Received %d progress messages\n", progressCount)
}
