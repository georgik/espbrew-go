package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
	"codeberg.org/georgik/espbrew-go/internal/flashhash"
	"github.com/rs/zerolog/log"
)

// checkFlashStatusOptimization performs hash-based flash detection
// It queries the server for flash status and logs optimization opportunities
func checkFlashStatusOptimization(client *cluster.Client, devicePath, firmwarePath string) error {
	log.Info().Msg("Checking flash status for optimization opportunities...")

	// 1. Determine chip type and get appropriate flash layout
	chip := flashOpts.chip
	if chip == "auto" {
		// Try to detect from device or default to ESP32-S3
		chip = "esp32s3"
	}

	var regions []flashhash.FlashRegionInfo
	switch chip {
	case "esp32s3":
		regions = flashhash.StandardESP32S3Layout4MB()
	case "esp32", "esp32-s2":
		regions = flashhash.StandardESP32Layout4MB()
	case "esp32c3":
		regions = flashhash.StandardESP32C3Layout4MB()
	default:
		// Default to ESP32-S3 layout for unknown chips
		regions = flashhash.StandardESP32S3Layout4MB()
		log.Debug().Str("chip", chip).Msg("Using default ESP32-S3 layout")
	}

	// 2. Compute hashes for firmware regions
	log.Debug().Int("regions", len(regions)).Msg("Computing firmware hashes")

	// Read firmware file
	firmwareData, err := os.ReadFile(firmwarePath)
	if err != nil {
		return fmt.Errorf("read firmware: %w", err)
	}

	// Compute hashes for regions we're about to flash
	firmwareHashes, err := flashhash.ComputeAllRegionsMD5(firmwareData, regions)
	if err != nil {
		return fmt.Errorf("compute firmware hashes: %w", err)
	}

	// 3. For single image mode, we only have the application region
	// Filter to only regions we're actually flashing (based on preset)
	var regionsToCheck []flashhash.FlashRegionInfo
	if flashOpts.preset == "app" {
		// Only checking application region
		for _, r := range firmwareHashes {
			if r.Name == flashhash.RegionApplication {
				regionsToCheck = append(regionsToCheck, r)
				break
			}
		}
	} else {
		regionsToCheck = firmwareHashes
	}

	if len(regionsToCheck) == 0 {
		log.Debug().Msg("No regions to check for optimization")
		return nil
	}

	// 4. Query server flash status
	statusReq := flashhash.FlashStatusRequest{
		DeviceID: devicePath, // Use device path as ID for now
		Regions:  regionsToCheck,
	}

	reqBody, err := json.Marshal(statusReq)
	if err != nil {
		return fmt.Errorf("marshal status request: %w", err)
	}

	// Build URL for flash status endpoint
	statusURL := client.BaseURL() + "/api/v1/devices/" + devicePath + "/flash-status"

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := httpClient.Post(statusURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("query flash status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Server might not support flash status endpoint (older version)
		if resp.StatusCode == http.StatusNotFound {
			log.Debug().Msg("Flash status endpoint not available (server may need update)")
			return nil
		}
		return fmt.Errorf("flash status query failed: status %d", resp.StatusCode)
	}

	var statusResp flashhash.FlashStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return fmt.Errorf("decode status response: %w", err)
	}

	// 5. Log optimization results
	switch statusResp.Status {
	case "skip_all":
		log.Info().Msg("Hash-based optimization: All regions match hashes - flash can be skipped!")
		log.Info().Str("job_id", statusResp.JobID).Msg("This firmware is already on the device")
		fmt.Printf("\nOptimization: Flash skipped - firmware already on device\n")
		return fmt.Errorf("flash skipped: firmware already on device")

	case "partial_update":
		log.Info().
			Int("cached", len(statusResp.RegionsCached)).
			Int("needed", len(statusResp.RegionsNeeded)).
			Msg("Hash-based optimization: Partial update available")

		for _, cached := range statusResp.RegionsCached {
			log.Info().Str("region", cached.Name).Str("reason", cached.Reason).Msg("Cached")
		}
		fmt.Printf("\nOptimization: %d region(s) cached, %d need flashing\n",
			len(statusResp.RegionsCached), len(statusResp.RegionsNeeded))

	case "full_flash":
		log.Info().Msg("Hash-based optimization: No regions match hashes - full flash required")
	}

	return nil
}

// computeDeviceFlashHashes reads flash from device and computes region hashes
// This is a placeholder for future implementation when we add device flash reading
func computeDeviceFlashHashes(devicePath string, regions []flashhash.FlashRegionInfo) ([]flashhash.FlashRegionInfo, error) {
	// TODO: Implement device flash reading
	// For now, return empty hashes (will trigger full flash)
	log.Debug().Msg("Device flash hash computation not yet implemented for local mode")
	return nil, fmt.Errorf("device flash reading not implemented")
}
