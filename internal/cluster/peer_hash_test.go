package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/flashhash"
)

func TestQueryFlashHash(t *testing.T) {
	tests := []struct {
		name           string
		deviceID       string
		regions        []flashhash.FlashRegionInfo
		responseStatus int
		responseBody   interface{}
		expectError    bool
		errorContains  string
	}{
		{
			name:     "successful full_flash response",
			deviceID: "device-123",
			regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123"},
			},
			responseStatus: 200,
			responseBody: flashhash.FlashStatusResponse{
				Status: "full_flash",
				RegionsNeeded: []flashhash.FlashRegionInfo{
					{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123"},
				},
			},
			expectError: false,
		},
		{
			name:     "successful partial_update response",
			deviceID: "device-456",
			regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123"},
				{Name: "application", Offset: 0x10000, Size: 0x400000, MD5: "def456"},
			},
			responseStatus: 200,
			responseBody: flashhash.FlashStatusResponse{
				Status: "partial_update",
				RegionsNeeded: []flashhash.FlashRegionInfo{
					{Name: "application", Offset: 0x10000, Size: 0x400000, MD5: "def456"},
				},
				RegionsCached: []flashhash.CachedRegionInfo{
					{Name: "bootloader", Reason: "hash_match"},
				},
			},
			expectError: false,
		},
		{
			name:     "successful skip_all response",
			deviceID: "device-789",
			regions: []flashhash.FlashRegionInfo{
				{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123"},
			},
			responseStatus: 200,
			responseBody: flashhash.FlashStatusResponse{
				Status: "skip_all",
				RegionsCached: []flashhash.CachedRegionInfo{
					{Name: "bootloader", Reason: "hash_match"},
				},
			},
			expectError: false,
		},
		{
			name:           "nil client returns error",
			deviceID:       "device-nil",
			regions:        []flashhash.FlashRegionInfo{},
			responseStatus: 0,
			responseBody:   nil,
			expectError:    true,
			errorContains:  "cluster client cannot be nil",
		},
		{
			name:           "server returns 500 error with retry",
			deviceID:       "device-error",
			regions:        []flashhash.FlashRegionInfo{},
			responseStatus: 500,
			responseBody: map[string]string{
				"error": "internal server error",
			},
			expectError:   true,
			errorContains: "max retries exceeded",
		},
		{
			name:           "server returns 404 not found",
			deviceID:       "device-missing",
			regions:        []flashhash.FlashRegionInfo{},
			responseStatus: 404,
			responseBody: map[string]string{
				"error": "device not found",
			},
			expectError:   true,
			errorContains: "status 404",
		},
		{
			name:           "invalid json response",
			deviceID:       "device-bad-json",
			regions:        []flashhash.FlashRegionInfo{},
			responseStatus: 200,
			responseBody:   "invalid json{{}",
			expectError:    true,
			errorContains:  "decode response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server

			if tt.responseStatus > 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify request method and path
					if r.Method != "POST" {
						t.Errorf("expected POST request, got %s", r.Method)
					}
					expectedPath := "/api/v1/devices/" + tt.deviceID + "/flash-status"
					if r.URL.Path != expectedPath {
						t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
					}

					// Verify content type
					if r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
					}

					// Verify request body
					var req flashhash.FlashStatusRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Errorf("failed to decode request: %v", err)
					}
					if req.DeviceID != tt.deviceID {
						t.Errorf("expected deviceID %s, got %s", tt.deviceID, req.DeviceID)
					}

					w.WriteHeader(tt.responseStatus)
					if strBody, ok := tt.responseBody.(string); ok {
						w.Write([]byte(strBody))
					} else {
						json.NewEncoder(w).Encode(tt.responseBody)
					}
				}))
				defer server.Close()
			}

			var client *Client
			if server != nil {
				client = NewClient(server.URL)
			}

			resp, err := QueryFlashHash(client, tt.deviceID, tt.regions)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				if resp != nil {
					t.Error("expected nil response on error, got non-nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				if resp == nil {
					t.Error("expected non-nil response, got nil")
				}
			}
		})
	}
}

func TestProcessFlashHashResponse(t *testing.T) {
	tests := []struct {
		name          string
		response      *flashhash.FlashStatusResponse
		expectError   bool
		errorContains string
		expectNeeded  int
		expectCached  int
		validateNames map[string]string // map index -> expected name
	}{
		{
			name: "full_flash status returns needed regions only",
			response: &flashhash.FlashStatusResponse{
				Status: "full_flash",
				RegionsNeeded: []flashhash.FlashRegionInfo{
					{Name: "bootloader", Offset: 0x1000, Size: 0x7000},
					{Name: "application", Offset: 0x10000, Size: 0x400000},
				},
			},
			expectError:   false,
			expectNeeded:  2,
			expectCached:  0,
			validateNames: map[string]string{"0": "bootloader", "1": "application"},
		},
		{
			name: "partial_update returns both needed and cached",
			response: &flashhash.FlashStatusResponse{
				Status: "partial_update",
				RegionsNeeded: []flashhash.FlashRegionInfo{
					{Name: "application", Offset: 0x10000, Size: 0x400000},
				},
				RegionsCached: []flashhash.CachedRegionInfo{
					{Name: "bootloader", Reason: "hash_match"},
					{Name: "partition-table", Reason: "hash_match"},
				},
			},
			expectError:   false,
			expectNeeded:  1,
			expectCached:  2,
			validateNames: map[string]string{"needed-0": "application", "cached-0": "bootloader", "cached-1": "partition-table"},
		},
		{
			name: "skip_all returns cached regions only",
			response: &flashhash.FlashStatusResponse{
				Status: "skip_all",
				RegionsCached: []flashhash.CachedRegionInfo{
					{Name: "bootloader", Reason: "hash_match"},
					{Name: "application", Reason: "hash_match"},
				},
			},
			expectError:  false,
			expectNeeded: 0,
			expectCached: 2,
		},
		{
			name:          "nil response returns error",
			response:      nil,
			expectError:   true,
			errorContains: "response cannot be nil",
		},
		{
			name: "unknown status returns error",
			response: &flashhash.FlashStatusResponse{
				Status: "unknown_status",
			},
			expectError:   true,
			errorContains: "unknown response status",
		},
		{
			name: "empty partial_update response",
			response: &flashhash.FlashStatusResponse{
				Status:        "partial_update",
				RegionsNeeded: []flashhash.FlashRegionInfo{},
				RegionsCached: []flashhash.CachedRegionInfo{},
			},
			expectError:  false,
			expectNeeded: 0,
			expectCached: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needed, cached, err := ProcessFlashHashResponse(tt.response)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}

				if len(needed) != tt.expectNeeded {
					t.Errorf("expected %d needed regions, got %d", tt.expectNeeded, len(needed))
				}

				if len(cached) != tt.expectCached {
					t.Errorf("expected %d cached regions, got %d", tt.expectCached, len(cached))
				}

				// Validate specific names if provided
				for key, expectedName := range tt.validateNames {
					var actualName string
					if strings.HasPrefix(key, "needed-") {
						idx := 0
						if key != "needed-0" {
							_, _ = fmt.Sscanf(key, "needed-%d", &idx)
						}
						if idx < len(needed) {
							actualName = needed[idx].Name
						}
					} else if strings.HasPrefix(key, "cached-") {
						idx := 0
						if key != "cached-0" {
							_, _ = fmt.Sscanf(key, "cached-%d", &idx)
						}
						if idx < len(cached) {
							actualName = cached[idx].Name
						}
					} else if idx, err := strconv.Atoi(key); err == nil {
						if idx < len(needed) {
							actualName = needed[idx].Name
						}
					}

					if actualName != expectedName {
						t.Errorf("expected name %q for key %s, got %q", expectedName, key, actualName)
					}
				}
			}
		})
	}
}

func TestComputeDeviceHashes(t *testing.T) {
	// Reset global cache before each test
	globalHashCache = &hashCacheStore{
		cache: make(map[string]*HashCache),
	}

	tests := []struct {
		name           string
		devicePath     string
		chip           string
		expectError    bool
		errorContains  string
		expectRegions  int
		expectedLayout []string // expected region names in order
	}{
		{
			name:          "esp32s3 returns standard layout",
			devicePath:    "/dev/ttyUSB0",
			chip:          "esp32s3",
			expectError:   false,
			expectRegions: 4,
			expectedLayout: []string{
				flashhash.RegionBootloader,
				flashhash.RegionPartitionTable,
				flashhash.RegionOTASelect,
				flashhash.RegionApplication,
			},
		},
		{
			name:          "esp32 returns standard layout",
			devicePath:    "/dev/ttyUSB1",
			chip:          "esp32",
			expectError:   false,
			expectRegions: 4,
			expectedLayout: []string{
				flashhash.RegionBootloader,
				flashhash.RegionPartitionTable,
				flashhash.RegionOTASelect,
				flashhash.RegionApplication,
			},
		},
		{
			name:          "esp32c3 returns standard layout",
			devicePath:    "/dev/ttyUSB2",
			chip:          "esp32c3",
			expectError:   false,
			expectRegions: 4,
			expectedLayout: []string{
				flashhash.RegionBootloader,
				flashhash.RegionPartitionTable,
				flashhash.RegionOTASelect,
				flashhash.RegionApplication,
			},
		},
		{
			name:           "unknown chip returns empty regions",
			devicePath:     "/dev/ttyUSB3",
			chip:           "unknown",
			expectError:    false,
			expectRegions:  0,
			expectedLayout: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regions, err := ComputeDeviceHashes(tt.devicePath, tt.chip)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}

				if len(regions) != tt.expectRegions {
					t.Errorf("expected %d regions, got %d", tt.expectRegions, len(regions))
				}

				// Verify region names and structure
				for i, expectedName := range tt.expectedLayout {
					if i >= len(regions) {
						t.Errorf("expected region %d to be %q, but got fewer regions than expected", i, expectedName)
						continue
					}
					if regions[i].Name != expectedName {
						t.Errorf("expected region %d name %q, got %q", i, expectedName, regions[i].Name)
					}
					// Verify MD5 is empty (placeholder implementation)
					if regions[i].MD5 != "" {
						t.Errorf("expected empty MD5 for placeholder, got %q", regions[i].MD5)
					}
				}
			}
		})
	}
}

func TestHashCache(t *testing.T) {
	// Reset global cache
	globalHashCache = &hashCacheStore{
		cache: make(map[string]*HashCache),
	}

	t.Run("cache stores and retrieves values", func(t *testing.T) {
		devicePath := "/dev/ttyUSB0"
		chip := "esp32s3"
		regions := []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: "abc123"},
		}

		// Cache miss initially
		_, found := globalHashCache.Get(devicePath, chip)
		if found {
			t.Error("expected cache miss, got hit")
		}

		// Set cache entry
		globalHashCache.Set(devicePath, chip, regions, time.Minute)

		// Cache hit after set
		retrieved, found := globalHashCache.Get(devicePath, chip)
		if !found {
			t.Error("expected cache hit, got miss")
		}
		if len(retrieved) != len(regions) {
			t.Errorf("expected %d regions, got %d", len(regions), len(retrieved))
		}
		if retrieved[0].Name != regions[0].Name {
			t.Errorf("expected name %q, got %q", regions[0].Name, retrieved[0].Name)
		}
	})

	t.Run("cache expires after ttl", func(t *testing.T) {
		devicePath := "/dev/ttyUSB1"
		chip := "esp32"
		regions := []flashhash.FlashRegionInfo{
			{Name: "application", Offset: 0x10000, Size: 0x400000},
		}

		// Set cache with very short TTL
		globalHashCache.Set(devicePath, chip, regions, 10*time.Millisecond)

		// Should be cached immediately
		_, found := globalHashCache.Get(devicePath, chip)
		if !found {
			t.Error("expected cache hit immediately after set, got miss")
		}

		// Wait for expiration
		time.Sleep(15 * time.Millisecond)

		// Should be expired
		_, found = globalHashCache.Get(devicePath, chip)
		if found {
			t.Error("expected cache miss after expiration, got hit")
		}
	})

	t.Run("cache invalidation removes entry", func(t *testing.T) {
		devicePath := "/dev/ttyUSB2"
		chip := "esp32c3"
		regions := []flashhash.FlashRegionInfo{
			{Name: "bootloader", Offset: 0x1000, Size: 0x7000},
		}

		// Set cache
		globalHashCache.Set(devicePath, chip, regions, time.Minute)

		// Verify cached
		_, found := globalHashCache.Get(devicePath, chip)
		if !found {
			t.Error("expected cache hit, got miss")
		}

		// Invalidate
		InvalidateHashCache(devicePath, chip)

		// Should be removed
		_, found = globalHashCache.Get(devicePath, chip)
		if found {
			t.Error("expected cache miss after invalidation, got hit")
		}
	})

	t.Run("cache key is unique per device and chip", func(t *testing.T) {
		regions1 := []flashhash.FlashRegionInfo{{Name: "bootloader"}}
		regions2 := []flashhash.FlashRegionInfo{{Name: "application"}}

		globalHashCache.Set("/dev/ttyUSB0", "esp32", regions1, time.Minute)
		globalHashCache.Set("/dev/ttyUSB0", "esp32s3", regions2, time.Minute)

		// Each combination should have its own entry
		r1, _ := globalHashCache.Get("/dev/ttyUSB0", "esp32")
		r2, _ := globalHashCache.Get("/dev/ttyUSB0", "esp32s3")

		if r1[0].Name != "bootloader" {
			t.Errorf("expected bootloader, got %s", r1[0].Name)
		}
		if r2[0].Name != "application" {
			t.Errorf("expected application, got %s", r2[0].Name)
		}
	})
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		devicePath string
		chip       string
		expected   string
	}{
		{
			devicePath: "/dev/ttyUSB0",
			chip:       "esp32",
			expected:   "/dev/ttyUSB0:esp32",
		},
		{
			devicePath: "COM1",
			chip:       "esp32s3",
			expected:   "COM1:esp32s3",
		},
		{
			devicePath: "",
			chip:       "",
			expected:   ":",
		},
		{
			devicePath: "/dev/with:colon",
			chip:       "esp32",
			expected:   "/dev/with:colon:esp32",
		},
	}

	for _, tt := range tests {
		t.Run(tt.devicePath+":"+tt.chip, func(t *testing.T) {
			result := cacheKey(tt.devicePath, tt.chip)
			if result != tt.expected {
				t.Errorf("expected key %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestQueryFlashHashWithInvalidRegions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(flashhash.FlashStatusResponse{
			Status: "full_flash",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)

	// Test with nil regions slice
	resp, err := QueryFlashHash(client, "device-123", nil)
	if err != nil {
		t.Errorf("expected no error with nil regions, got %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}

	// Test with empty regions slice
	resp, err = QueryFlashHash(client, "device-456", []flashhash.FlashRegionInfo{})
	if err != nil {
		t.Errorf("expected no error with empty regions, got %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestProcessFlashHashResponseWithJobID(t *testing.T) {
	response := &flashhash.FlashStatusResponse{
		Status: "full_flash",
		RegionsNeeded: []flashhash.FlashRegionInfo{
			{Name: "application", Offset: 0x10000, Size: 0x400000},
		},
		JobID:   "job-123",
		Message: "Processing flash job",
	}

	needed, cached, err := ProcessFlashHashResponse(response)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(needed) != 1 {
		t.Errorf("expected 1 needed region, got %d", len(needed))
	}
	if len(cached) != 0 {
		t.Errorf("expected 0 cached regions, got %d", len(cached))
	}
	if needed[0].Name != "application" {
		t.Errorf("expected application, got %s", needed[0].Name)
	}
}
