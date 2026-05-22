package http

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/georgik/esp-ci-cluster/internal/cluster"
	"github.com/georgik/esp-ci-cluster/pkg/protocol"
)

func TestFlashHandler_handleUpload(t *testing.T) {
	// Create a mock master
	master := cluster.NewLeaderNode("test-master", &cluster.LeaderConfig{
		DisablemDNS: true,
	})

	handler := NewFlashHandler(master, os.TempDir())

	// Create test firmware
	testData := []byte("test firmware data")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("firmware", "test.bin")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(testData)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/flash/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	handler.handleUpload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var uploadResp FlashUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if uploadResp.FileID == "" {
		t.Fatal("expected file_id")
	}

	if uploadResp.Size != int64(len(testData)) {
		t.Errorf("expected size %d, got %d", len(testData), uploadResp.Size)
	}

	// Verify file exists
	filePath := filepath.Join(os.TempDir(), uploadResp.FileID+".bin")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("uploaded file not found: %s", filePath)
	} else {
		os.Remove(filePath) // cleanup
	}
}

func TestFlashHandler_handleUpload_NoFile(t *testing.T) {
	master := cluster.NewLeaderNode("test-master", &cluster.LeaderConfig{
		DisablemDNS: true,
	})

	handler := NewFlashHandler(master, os.TempDir())

	req := httptest.NewRequest("POST", "/api/v1/flash/upload", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data")

	w := httptest.NewRecorder()
	handler.handleUpload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestFlashHandler_handleFlashSubmit(t *testing.T) {
	master := cluster.NewLeaderNode("test-master", &cluster.LeaderConfig{
		DisablemDNS: true,
	})

	// Register a test device
	master.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0028,
		Status: "available",
	})

	handler := NewFlashHandler(master, os.TempDir())

	// Create test firmware file
	testData := []byte("test firmware")
	testFile := filepath.Join(os.TempDir(), "test-firmware.bin")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	// Upload firmware first
	var uploadBody bytes.Buffer
	uploadWriter := multipart.NewWriter(&uploadBody)
	part, _ := uploadWriter.CreateFormFile("firmware", "test.bin")
	part.Write(testData)
	uploadWriter.Close()

	uploadReq := httptest.NewRequest("POST", "/api/v1/flash/upload", &uploadBody)
	uploadReq.Header.Set("Content-Type", uploadWriter.FormDataContentType())

	uploadW := httptest.NewRecorder()
	handler.handleUpload(uploadW, uploadReq)

	var uploadResp FlashUploadResponse
	json.NewDecoder(uploadW.Body).Decode(&uploadResp)

	// Now submit flash job
	submitReq := FlashSubmitRequest{
		DevicePath: "/dev/ttyUSB0",
		FileID:     uploadResp.FileID,
		ClientID:   "test-client",
	}

	body, _ := json.Marshal(submitReq)
	req := httptest.NewRequest("POST", "/api/v1/flash", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.handleFlashSubmit(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var submitResp FlashSubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if submitResp.JobID == "" {
		t.Fatal("expected job_id")
	}

	if submitResp.DevicePath != "/dev/ttyUSB0" {
		t.Errorf("expected /dev/ttyUSB0, got %s", submitResp.DevicePath)
	}
}

func TestFlashHandler_handleFlashSubmit_DeviceNotFound(t *testing.T) {
	master := cluster.NewLeaderNode("test-master", &cluster.LeaderConfig{
		DisablemDNS: true,
	})

	handler := NewFlashHandler(master, os.TempDir())

	// First upload a file so we get past that check
	testData := []byte("test firmware")
	var uploadBody bytes.Buffer
	uploadWriter := multipart.NewWriter(&uploadBody)
	part, _ := uploadWriter.CreateFormFile("firmware", "test.bin")
	part.Write(testData)
	uploadWriter.Close()

	uploadReq := httptest.NewRequest("POST", "/api/v1/flash/upload", &uploadBody)
	uploadReq.Header.Set("Content-Type", uploadWriter.FormDataContentType())

	uploadW := httptest.NewRecorder()
	handler.handleUpload(uploadW, uploadReq)

	var uploadResp FlashUploadResponse
	json.NewDecoder(uploadW.Body).Decode(&uploadResp)

	// Now try to flash to non-existent device
	req := FlashSubmitRequest{
		DevicePath: "/dev/nonexistent",
		FileID:     uploadResp.FileID,
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/flash", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.handleFlashSubmit(w, httpReq)

	resp := w.Result()
	// Device not found returns 409 Conflict from EnqueueJob error
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}
