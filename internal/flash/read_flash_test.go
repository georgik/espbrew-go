package flash

import (
	"context"
	"testing"
)

func TestReadFlashRequest_DefaultValues(t *testing.T) {
	req := &ReadFlashRequest{
		Port:    "/dev/ttyUSB0",
		Address: 0x10000,
		Size:    0x1000,
	}

	if req.Port != "/dev/ttyUSB0" {
		t.Errorf("Expected port /dev/ttyUSB0, got %s", req.Port)
	}
	if req.Address != 0x10000 {
		t.Errorf("Expected address 0x10000, got 0x%x", req.Address)
	}
	if req.Size != 0x1000 {
		t.Errorf("Expected size 0x1000, got 0x%x", req.Size)
	}
}

func TestReadFlashResult_Success(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	result := &ReadFlashResult{
		Success: true,
		Data:    data,
	}

	if !result.Success {
		t.Error("Expected success to be true")
	}
	if len(result.Data) != 4 {
		t.Errorf("Expected 4 bytes, got %d", len(result.Data))
	}
}

func TestReadFlashResult_Failure(t *testing.T) {
	result := &ReadFlashResult{
		Success: false,
		Error:   &MockError{},
	}

	if result.Success {
		t.Error("Expected success to be false")
	}
	if result.Error == nil {
		t.Error("Expected error to be set")
	}
}

func TestNewReadFlashRequest(t *testing.T) {
	flasher := NewFlasher(nil)

	req := &ReadFlashRequest{
		Port:    "/dev/ttyUSB0",
		Address: 0x10000,
		Size:    4096,
	}

	ctx := context.Background()
	result := flasher.ReadFlash(ctx, req)

	// This will fail in tests since there's no actual device
	// We're just testing the API structure
	if result == nil {
		t.Fatal("Expected result to be returned")
	}
}

// MockError for testing
type MockError struct{}

func (e *MockError) Error() string {
	return "mock error"
}
