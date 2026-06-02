package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/flashhash"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/gorilla/mux"
)

func TestFlashStatusHandler_handleFlashStatus(t *testing.T) {
	// Create a test store
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewFlashStatusHandler(store)

	// Test 1: No job hashes exist - should return full_flash
	t.Run("NoJobHashes", func(t *testing.T) {
		req := flashhash.FlashStatusRequest{
			DeviceID: "test-device-1",
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123def456abc123def456abc123de"},
			},
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/api/v1/devices/test-device-1/flash-status", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq = mux.SetURLVars(httpReq, map[string]string{"deviceId": "test-device-1"})

		w := httptest.NewRecorder()
		handler.handleFlashStatus(w, httpReq)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var statusResp flashhash.FlashStatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if statusResp.Status != "full_flash" {
			t.Errorf("expected status 'full_flash', got '%s'", statusResp.Status)
		}
	})

	// Test 2: With matching hashes - should return skip_all
	t.Run("MatchingHashes", func(t *testing.T) {
		// First, save job hashes
		jobHashes := &flashhash.JobFlashHashes{
			JobID:    "test-job-1",
			DeviceID: "test-device-2",
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123def456abc123def456abc123de"},
				{Name: "application", Offset: 0x10000, Size: 0x400000, MD5: "fed456abc123def456abc123def456ab"},
			},
		}

		if err := store.SaveFlashHashes(jobHashes); err != nil {
			t.Fatalf("save job hashes: %v", err)
		}

		// Now query with matching regions
		req := flashhash.FlashStatusRequest{
			DeviceID: "test-device-2",
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123def456abc123def456abc123de"},
				{Name: "application", Offset: 0x10000, Size: 0x400000, MD5: "fed456abc123def456abc123def456ab"},
			},
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/api/v1/devices/test-device-2/flash-status", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq = mux.SetURLVars(httpReq, map[string]string{"deviceId": "test-device-2"})

		w := httptest.NewRecorder()
		handler.handleFlashStatus(w, httpReq)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var statusResp flashhash.FlashStatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if statusResp.Status != "skip_all" {
			t.Errorf("expected status 'skip_all', got '%s'", statusResp.Status)
		}

		if len(statusResp.RegionsCached) != 2 {
			t.Errorf("expected 2 cached regions, got %d", len(statusResp.RegionsCached))
		}

		if len(statusResp.RegionsNeeded) != 0 {
			t.Errorf("expected 0 needed regions, got %d", len(statusResp.RegionsNeeded))
		}

		if statusResp.JobID != "test-job-1" {
			t.Errorf("expected job_id 'test-job-1', got '%s'", statusResp.JobID)
		}
	})

	// Test 3: With mismatched hashes - should return partial_update
	t.Run("MismatchedHashes", func(t *testing.T) {
		// Save job hashes with different MD5
		jobHashes := &flashhash.JobFlashHashes{
			JobID:    "test-job-2",
			DeviceID: "test-device-3",
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
				{Name: "application", Offset: 0x10000, Size: 0x400000, MD5: "22222222222222222222222222222222"},
			},
		}

		if err := store.SaveFlashHashes(jobHashes); err != nil {
			t.Fatalf("save job hashes: %v", err)
		}

		// Query with different client hashes (bootloader matches, application doesn't)
		req := flashhash.FlashStatusRequest{
			DeviceID: "test-device-3",
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},     // matches
				{Name: "application", Offset: 0x10000, Size: 0x400000, MD5: "33333333333333333333333333333333"}, // mismatch
			},
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/api/v1/devices/test-device-3/flash-status", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq = mux.SetURLVars(httpReq, map[string]string{"deviceId": "test-device-3"})

		w := httptest.NewRecorder()
		handler.handleFlashStatus(w, httpReq)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var statusResp flashhash.FlashStatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if statusResp.Status != "partial_update" {
			t.Errorf("expected status 'partial_update', got '%s'", statusResp.Status)
		}

		if len(statusResp.RegionsNeeded) != 1 {
			t.Errorf("expected 1 needed region, got %d", len(statusResp.RegionsNeeded))
		}

		if len(statusResp.RegionsCached) != 1 {
			t.Errorf("expected 1 cached region, got %d", len(statusResp.RegionsCached))
		}
	})

	// Test 4: All regions mismatched - should return full_flash
	t.Run("AllMismatched", func(t *testing.T) {
		jobHashes := &flashhash.JobFlashHashes{
			JobID:    "test-job-3",
			DeviceID: "test-device-4",
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "11111111111111111111111111111111"},
				{Name: "application", Offset: 0x10000, Size: 0x400000, MD5: "22222222222222222222222222222222"},
			},
		}

		if err := store.SaveFlashHashes(jobHashes); err != nil {
			t.Fatalf("save job hashes: %v", err)
		}

		// Query with completely different hashes
		req := flashhash.FlashStatusRequest{
			DeviceID: "test-device-4",
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				{Name: "application", Offset: 0x10000, Size: 0x400000, MD5: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
			},
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/api/v1/devices/test-device-4/flash-status", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq = mux.SetURLVars(httpReq, map[string]string{"deviceId": "test-device-4"})

		w := httptest.NewRecorder()
		handler.handleFlashStatus(w, httpReq)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var statusResp flashhash.FlashStatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if statusResp.Status != "full_flash" {
			t.Errorf("expected status 'full_flash', got '%s'", statusResp.Status)
		}
	})

	// Test 5: Device ID mismatch - should return error
	t.Run("DeviceIDMismatch", func(t *testing.T) {
		req := flashhash.FlashStatusRequest{
			DeviceID: "different-device",
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123def456abc123def456abc123de"},
			},
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/api/v1/devices/test-device-5/flash-status", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq = mux.SetURLVars(httpReq, map[string]string{"deviceId": "test-device-5"})

		w := httptest.NewRecorder()
		handler.handleFlashStatus(w, httpReq)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	// Test 6: Invalid region - should return error
	t.Run("InvalidRegion", func(t *testing.T) {
		req := flashhash.FlashStatusRequest{
			DeviceID: "test-device-6",
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "invalid"}, // invalid MD5
			},
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/api/v1/devices/test-device-6/flash-status", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq = mux.SetURLVars(httpReq, map[string]string{"deviceId": "test-device-6"})

		w := httptest.NewRecorder()
		handler.handleFlashStatus(w, httpReq)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})
}

func TestFlashStatusHandler_handleGetJobHashes(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewFlashStatusHandler(store)

	// Save test hashes
	jobHashes := &flashhash.JobFlashHashes{
		JobID:    "test-job-get",
		DeviceID: "test-device",
		Regions: []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123def456abc123def456abc123de"},
		},
	}

	if err := store.SaveFlashHashes(jobHashes); err != nil {
		t.Fatalf("save job hashes: %v", err)
	}

	// Test getting existing hashes
	t.Run("GetExisting", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/jobs/test-job-get/hashes", nil)
		req = mux.SetURLVars(req, map[string]string{"jobId": "test-job-get"})
		w := httptest.NewRecorder()

		handler.handleGetJobHashes(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var respHashes flashhash.JobFlashHashes
		if err := json.NewDecoder(resp.Body).Decode(&respHashes); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if respHashes.JobID != "test-job-get" {
			t.Errorf("expected job_id 'test-job-get', got '%s'", respHashes.JobID)
		}

		if len(respHashes.Regions) != 1 {
			t.Errorf("expected 1 region, got %d", len(respHashes.Regions))
		}
	})

	// Test getting non-existent hashes
	t.Run("GetNonExistent", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/jobs/non-existent/hashes", nil)
		req = mux.SetURLVars(req, map[string]string{"jobId": "non-existent"})
		w := httptest.NewRecorder()

		handler.handleGetJobHashes(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusNotFound {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(bodyBytes))
		}
	})
}

func TestFlashStatusHandler_handlePutJobHashes(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewFlashStatusHandler(store)

	// Test saving valid hashes
	t.Run("SaveValid", func(t *testing.T) {
		hashes := flashhash.JobFlashHashes{
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123def456abc123def456abc123de"},
			},
		}

		body, _ := json.Marshal(hashes)
		req := httptest.NewRequest("PUT", "/api/v1/jobs/test-job-put/hashes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = mux.SetURLVars(req, map[string]string{"jobId": "test-job-put"})

		w := httptest.NewRecorder()
		handler.handlePutJobHashes(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var respHashes flashhash.JobFlashHashes
		if err := json.NewDecoder(resp.Body).Decode(&respHashes); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if respHashes.JobID != "test-job-put" {
			t.Errorf("expected job_id 'test-job-put', got '%s'", respHashes.JobID)
		}
	})

	// Test saving invalid region
	t.Run("SaveInvalidRegion", func(t *testing.T) {
		hashes := flashhash.JobFlashHashes{
			Regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "invalid"}, // invalid MD5
			},
		}

		body, _ := json.Marshal(hashes)
		req := httptest.NewRequest("PUT", "/api/v1/jobs/test-job-invalid/hashes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = mux.SetURLVars(req, map[string]string{"jobId": "test-job-invalid"})

		w := httptest.NewRecorder()
		handler.handlePutJobHashes(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})
}

func TestFlashStatusHandler_handleDeleteJobHashes(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewFlashStatusHandler(store)

	// First save some hashes
	jobHashes := &flashhash.JobFlashHashes{
		JobID:    "test-job-delete",
		DeviceID: "test-device",
		Regions: []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123def456abc123def456abc123de"},
		},
	}

	if err := store.SaveFlashHashes(jobHashes); err != nil {
		t.Fatalf("save job hashes: %v", err)
	}

	// Test deleting existing hashes
	t.Run("DeleteExisting", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/jobs/test-job-delete/hashes", nil)
		req = mux.SetURLVars(req, map[string]string{"jobId": "test-job-delete"})
		w := httptest.NewRecorder()

		handler.handleDeleteJobHashes(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusNoContent {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 204, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		// Verify deletion
		_, err := store.GetFlashHashes("test-job-delete")
		if err == nil {
			t.Error("expected error when getting deleted hashes")
		}
	})
}
