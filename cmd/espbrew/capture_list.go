package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var captureListCmd = &cobra.Command{
	Use:   "list",
	Short: "List captures",
	Long: `List camera captures or device-specific captures.

Examples:
  espbrew capture list                    # List all camera captures
  espbrew capture list --device-id <id>   # List device captures
  espbrew capture list --today             # List today's captures only`,
	RunE: runCaptureList,
}

var captureListOpts struct {
	deviceID string
	today    bool
}

func init() {
	captureCmd.AddCommand(captureListCmd)

	captureListCmd.Flags().StringVar(&captureListOpts.deviceID, "device-id", "", "Filter by device ID")
	captureListCmd.Flags().BoolVar(&captureListOpts.today, "today", false, "Show only today's captures")
}

func runCaptureList(cmd *cobra.Command, args []string) error {
	// If device-id specified, list device captures
	if captureListOpts.deviceID != "" {
		return listDeviceCaptures(captureListOpts.deviceID)
	}

	// Otherwise, list camera captures
	return listCameraCaptures()
}

// listCameraCaptures lists all camera captures
func listCameraCaptures() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	capturesDir := filepath.Join(homeDir, ".espbrew", "captures")

	// Check if captures directory exists
	if _, err := os.Stat(capturesDir); os.IsNotExist(err) {
		log.Info().Msg("No captures found")
		return nil
	}

	// Scan for captures
	var captures []camera.CaptureMetadata

	if captureListOpts.today {
		// Only scan today's directory
		store, err := camera.NewStore(capturesDir)
		if err != nil {
			return fmt.Errorf("create store: %w", err)
		}

		todayCaptures, err := store.ListCaptures(time.Now())
		if err != nil {
			return fmt.Errorf("list today's captures: %w", err)
		}
		captures = append(captures, todayCaptures...)
	} else {
		// Scan all date directories
		entries, err := os.ReadDir(capturesDir)
		if err != nil {
			return fmt.Errorf("read captures directory: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			// Parse date from directory name
			date, err := time.Parse("2006-01-02", entry.Name())
			if err != nil {
				continue
			}

			store, err := camera.NewStore(capturesDir)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to create store")
				continue
			}

			dayCaptures, err := store.ListCaptures(date)
			if err != nil {
				log.Warn().Err(err).Str("date", entry.Name()).Msg("Failed to list captures")
				continue
			}

			captures = append(captures, dayCaptures...)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(captures, func(i, j int) bool {
		return captures[i].Timestamp.After(captures[j].Timestamp)
	})

	if len(captures) == 0 {
		log.Info().Msg("No captures found")
		return nil
	}

	log.Info().Msgf("Found %d captures:", len(captures))
	for i, cap := range captures {
		log.Info().Msgf("  %d. %s", i+1, cap.Filename)
		log.Info().Msgf("     Camera: %s", cap.CameraID)
		log.Info().Msgf("     Time:   %s", cap.Timestamp.Format("2006-01-02 15:04:05"))
		log.Info().Msgf("     Size:   %d bytes", cap.SizeBytes)
	}

	return nil
}

// listDeviceCaptures lists all device-specific captures
func listDeviceCaptures(deviceID string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	capturesDir := filepath.Join(homeDir, ".espbrew", "captures")

	// Check if captures directory exists
	if _, err := os.Stat(capturesDir); os.IsNotExist(err) {
		log.Info().Msg("No captures found")
		return nil
	}

	// Scan for device captures
	type deviceCaptureEntry struct {
		CapturePath string
		DeviceID    string
		Bounds      map[string]interface{}
		Subimage    string
		GeneratedAt time.Time
	}

	var deviceCaptures []deviceCaptureEntry

	err = filepath.Walk(capturesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		// Check for device capture metadata files (JSON files next to images)
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" {
			return nil
		}

		// Skip metadata.json files (camera metadata)
		if strings.HasPrefix(filepath.Base(path), "metadata") {
			return nil
		}

		// Read device capture metadata
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var captures []camera.DeviceCaptureInfo
		if err := json.Unmarshal(data, &captures); err != nil {
			return nil
		}

		// Find captures for the requested device
		for _, cap := range captures {
			if cap.DeviceID == deviceID {
				relPath, _ := filepath.Rel(capturesDir, path)
				deviceCaptures = append(deviceCaptures, deviceCaptureEntry{
					CapturePath: relPath,
					DeviceID:    cap.DeviceID,
					Bounds: map[string]interface{}{
						"x":      cap.Bounds.X,
						"y":      cap.Bounds.Y,
						"width":  cap.Bounds.Width,
						"height": cap.Bounds.Height,
					},
					Subimage:    cap.Subimage,
					GeneratedAt: cap.GeneratedAt,
				})
			}
		}

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("scan captures: %w", err)
	}

	// Sort by generation time (newest first)
	sort.Slice(deviceCaptures, func(i, j int) bool {
		return deviceCaptures[i].GeneratedAt.After(deviceCaptures[j].GeneratedAt)
	})

	if len(deviceCaptures) == 0 {
		log.Info().Str("device_id", deviceID).Msg("No device captures found")
		return nil
	}

	log.Info().Msgf("Found %d device captures for %s:", len(deviceCaptures), deviceID)
	for i, entry := range deviceCaptures {
		log.Info().Msgf("  %d. %s", i+1, entry.Subimage)
		log.Info().Msgf("     Bounds: x=%.3f, y=%.3f, w=%.3f, h=%.3f",
			entry.Bounds["x"], entry.Bounds["y"], entry.Bounds["width"], entry.Bounds["height"])
		log.Info().Msgf("     Time:   %s", entry.GeneratedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}
