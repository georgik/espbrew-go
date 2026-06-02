package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/flashhash"
)

// HashCache stores computed flash hashes with expiration
type HashCache struct {
	hashes     []flashhash.FlashRegionInfo
	devicePath string
	chip       string
	expiry     time.Time
}

// hashCacheStore manages cached hash entries
type hashCacheStore struct {
	mu    sync.RWMutex
	cache map[string]*HashCache
}

// global hash cache instance
var globalHashCache = &hashCacheStore{
	cache: make(map[string]*HashCache),
}

const (
	defaultCacheTTL = 5 * time.Minute
)

// cacheKey generates a unique key for the cache entry
func cacheKey(devicePath, chip string) string {
	return devicePath + ":" + chip
}

// Get retrieves cached hashes if still valid
func (s *hashCacheStore) Get(devicePath, chip string) ([]flashhash.FlashRegionInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := cacheKey(devicePath, chip)
	entry, exists := s.cache[key]
	if !exists {
		return nil, false
	}

	// Check if cache has expired
	if time.Now().After(entry.expiry) {
		delete(s.cache, key)
		return nil, false
	}

	return entry.hashes, true
}

// Set stores hashes in the cache
func (s *hashCacheStore) Set(devicePath, chip string, hashes []flashhash.FlashRegionInfo, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := cacheKey(devicePath, chip)
	s.cache[key] = &HashCache{
		hashes:     hashes,
		devicePath: devicePath,
		chip:       chip,
		expiry:     time.Now().Add(ttl),
	}
}

// Invalidate removes a cache entry
func (s *hashCacheStore) Invalidate(devicePath, chip string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := cacheKey(devicePath, chip)
	delete(s.cache, key)
}

// QueryFlashHash sends a hash query to the cluster leader
// It returns the server's response with optimization recommendations
func QueryFlashHash(client *Client, deviceID string, regions []flashhash.FlashRegionInfo) (*flashhash.FlashStatusResponse, error) {
	if client == nil {
		return nil, fmt.Errorf("cluster client cannot be nil")
	}

	// Build the request
	req := flashhash.FlashStatusRequest{
		DeviceID: deviceID,
		Regions:  regions,
	}

	// Marshal request to JSON
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/devices/%s/flash-status", client.BaseURL(), deviceID)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request with retry logic
	resp, err := client.doWithRetry(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Decode response
	var response flashhash.FlashStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &response, nil
}

// ProcessFlashHashResponse handles the server response from a hash query
// It returns regions that need to be flashed and regions that can be skipped
func ProcessFlashHashResponse(response *flashhash.FlashStatusResponse) (needed, cached []flashhash.FlashRegionInfo, err error) {
	if response == nil {
		return nil, nil, fmt.Errorf("response cannot be nil")
	}

	switch response.Status {
	case "full_flash":
		// All regions need to be flashed
		return response.RegionsNeeded, nil, nil

	case "partial_update":
		// Some regions can be skipped - regions_cached contains the cached ones
		// Convert CachedRegionInfo to FlashRegionInfo for the cached regions
		cachedRegions := make([]flashhash.FlashRegionInfo, len(response.RegionsCached))
		for i, cr := range response.RegionsCached {
			cachedRegions[i] = flashhash.FlashRegionInfo{
				Name: cr.Name,
			}
		}
		return response.RegionsNeeded, cachedRegions, nil

	case "skip_all":
		// No regions need flashing
		cachedRegions := make([]flashhash.FlashRegionInfo, len(response.RegionsCached))
		for i, cr := range response.RegionsCached {
			cachedRegions[i] = flashhash.FlashRegionInfo{
				Name: cr.Name,
			}
		}
		return nil, cachedRegions, nil

	default:
		return nil, nil, fmt.Errorf("unknown response status: %s", response.Status)
	}
}

// ComputeDeviceHashes computes hashes from device flash memory
// This is a placeholder implementation that will be filled in when
// device reading functionality is available
func ComputeDeviceHashes(devicePath, chip string) ([]flashhash.FlashRegionInfo, error) {
	// Check cache first
	if hashes, found := globalHashCache.Get(devicePath, chip); found {
		return hashes, nil
	}

	// Determine regions based on chip type
	var regions []flashhash.FlashRegionInfo
	switch chip {
	case "esp32s3":
		regions = flashhash.StandardESP32S3Layout4MB()
	case "esp32":
		regions = flashhash.StandardESP32Layout4MB()
	case "esp32c3":
		regions = flashhash.StandardESP32C3Layout4MB()
	default:
		// For unknown chips, return empty regions
		// TODO: Implement actual device reading when available
		return []flashhash.FlashRegionInfo{}, nil
	}

	// TODO: Implement actual flash reading from device
	// For now, return regions without computed hashes
	// The actual implementation would:
	// 1. Open serial connection to device
	// 2. Put device in download mode
	// 3. Read each region using appropriate protocol
	// 4. Compute MD5 hash for each region
	// 5. Return regions with computed hashes

	result := make([]flashhash.FlashRegionInfo, len(regions))
	for i, region := range regions {
		result[i] = flashhash.FlashRegionInfo{
			Name:   region.Name,
			Offset: region.Offset,
			Size:   region.Size,
			MD5:    "", // Placeholder: no hash computed yet
		}
	}

	// Cache the result
	globalHashCache.Set(devicePath, chip, result, defaultCacheTTL)

	return result, nil
}

// InvalidateHashCache removes cached hashes for a device
func InvalidateHashCache(devicePath, chip string) {
	globalHashCache.Invalidate(devicePath, chip)
}
