package cluster

import (
	"testing"
)

func TestReadFlashRequest(t *testing.T) {
	req := ReadFlashRequest{
		DevicePath: "/dev/ttyUSB0",
		Address:    0x10000,
		Size:       0x1000,
	}

	if req.DevicePath != "/dev/ttyUSB0" {
		t.Errorf("Expected DevicePath /dev/ttyUSB0, got %s", req.DevicePath)
	}
	if req.Address != 0x10000 {
		t.Errorf("Expected Address 0x10000, got 0x%x", req.Address)
	}
	if req.Size != 0x1000 {
		t.Errorf("Expected Size 0x1000, got 0x%x", req.Size)
	}
}

func TestReadFlashResponse(t *testing.T) {
	resp := ReadFlashResponse{
		JobID:       "test-job-123",
		Status:      "completed",
		Size:        4096,
		DownloadURL: "http://example.com/download/test-job-123",
	}

	if resp.JobID != "test-job-123" {
		t.Errorf("Expected JobID test-job-123, got %s", resp.JobID)
	}
	if resp.Status != "completed" {
		t.Errorf("Expected Status completed, got %s", resp.Status)
	}
	if resp.Size != 4096 {
		t.Errorf("Expected Size 4096, got %d", resp.Size)
	}
	if resp.DownloadURL != "http://example.com/download/test-job-123" {
		t.Errorf("Expected DownloadURL http://example.com/download/test-job-123, got %s", resp.DownloadURL)
	}
}

func TestReadFlashResponse_Error(t *testing.T) {
	resp := ReadFlashResponse{
		JobID:  "test-job-456",
		Status: "failed",
		Error:  "device not found",
	}

	if resp.Status != "failed" {
		t.Errorf("Expected Status failed, got %s", resp.Status)
	}
	if resp.Error != "device not found" {
		t.Errorf("Expected Error 'device not found', got %s", resp.Error)
	}
}
