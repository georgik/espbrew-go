package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/inventory"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var mappingCmd = &cobra.Command{
	Use:   "mapping",
	Short: "Manage device bounding box mappings",
	Long:  `Manage device-to-camera bounding box mappings for automated device identification in captures.`,
}

var mappingListCmd = &cobra.Command{
	Use:   "list --device-id <id>",
	Short: "List all bounding boxes for a device",
	RunE:  runMappingList,
}

var mappingSetCmd = &cobra.Command{
	Use:   "set --device-id <id> --camera <cam> --bounds <x,y,w,h>",
	Short: "Create or update a bounding box mapping",
	Long: `Create or update a bounding box mapping for a device.

Bounds are specified as comma-separated normalized coordinates (0.0-1.0):
  x: Top-left X position (0.0-1.0)
  y: Top-left Y position (0.0-1.0)
  w: Width as fraction of image width (0.0-1.0)
  h: Height as fraction of image height (0.0-1.0)

Example:
  espbrew mapping set --device-id esp-aa:bb:cc:dd:ee:ff --camera cam-001 --bounds 0.1,0.2,0.3,0.4`,
	RunE: runMappingSet,
}

var mappingRemoveCmd = &cobra.Command{
	Use:   "remove --id <bbox-id>",
	Short: "Delete a bounding box mapping",
	RunE:  runMappingRemove,
}

var mappingExportCmd = &cobra.Command{
	Use:   "export --device-id <id> --output <file>",
	Short: "Export mappings as JSON",
	RunE:  runMappingExport,
}

var mappingImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import mappings from JSON",
	Args:  cobra.ExactArgs(1),
	RunE:  runMappingImport,
}

type mappingFlags struct {
	deviceID  string
	cameraID  string
	bounds    string
	bboxID    string
	output    string
	overwrite bool
}

var mappingOpts mappingFlags

func init() {
	rootCmd.AddCommand(mappingCmd)
	mappingCmd.AddCommand(mappingListCmd)
	mappingCmd.AddCommand(mappingSetCmd)
	mappingCmd.AddCommand(mappingRemoveCmd)
	mappingCmd.AddCommand(mappingExportCmd)
	mappingCmd.AddCommand(mappingImportCmd)

	// List flags
	mappingListCmd.Flags().StringVar(&mappingOpts.deviceID, "device-id", "", "Device ID (required)")

	// Set flags
	mappingSetCmd.Flags().StringVar(&mappingOpts.deviceID, "device-id", "", "Device ID (required)")
	mappingSetCmd.Flags().StringVar(&mappingOpts.cameraID, "camera", "", "Camera ID (required)")
	mappingSetCmd.Flags().StringVar(&mappingOpts.bounds, "bounds", "", "Bounding box as x,y,width,height (required)")
	mappingSetCmd.Flags().StringVar(&mappingOpts.bboxID, "id", "", "Bounding box ID (for updates)")
	mappingSetCmd.Flags().BoolVar(&mappingOpts.overwrite, "overwrite", false, "Overwrite existing mapping for this device/camera pair")

	// Remove flags
	mappingRemoveCmd.Flags().StringVar(&mappingOpts.bboxID, "id", "", "Bounding box ID (required)")

	// Export flags
	mappingExportCmd.Flags().StringVar(&mappingOpts.deviceID, "device-id", "", "Device ID (required)")
	mappingExportCmd.Flags().StringVarP(&mappingOpts.output, "output", "o", "", "Output file (required)")

	// Mark required flags
	mappingListCmd.MarkFlagRequired("device-id")
	mappingSetCmd.MarkFlagRequired("device-id")
	mappingSetCmd.MarkFlagRequired("camera")
	mappingSetCmd.MarkFlagRequired("bounds")
	mappingRemoveCmd.MarkFlagRequired("id")
	mappingExportCmd.MarkFlagRequired("device-id")
	mappingExportCmd.MarkFlagRequired("output")
}

func runMappingList(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	mappings, err := store.ListBoundingBoxesForDevice(mappingOpts.deviceID)
	if err != nil {
		return fmt.Errorf("list mappings: %w", err)
	}

	if len(mappings) == 0 {
		log.Info().Str("device_id", mappingOpts.deviceID).Msg("No bounding box mappings found")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCAMERA\tBOUNDS\t\tCALIBRATION\tCREATED\tUPDATED")
	for _, m := range mappings {
		boundsStr := fmt.Sprintf("%.3f,%.3f,%.3f,%.3f", m.Bounds.X, m.Bounds.Y, m.Bounds.Width, m.Bounds.Height)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n",
			m.ID,
			m.CameraID,
			boundsStr,
			m.CalibrationVersion,
			m.CreatedAt.Format("2006-01-02 15:04"),
			m.UpdatedAt.Format("2006-01-02 15:04"))
	}
	tw.Flush()

	return nil
}

func runMappingSet(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	// Parse bounds
	bounds, err := parseBounds(mappingOpts.bounds)
	if err != nil {
		return fmt.Errorf("invalid bounds: %w", err)
	}

	// Validate device exists in inventory
	inv, err := inventory.NewInventory()
	if err == nil {
		if _, err := inv.Get(mappingOpts.deviceID); err != nil {
			log.Warn().Str("device_id", mappingOpts.deviceID).Msg("Device not found in inventory (continuing anyway)")
		}
	}

	// Generate or use provided ID
	bboxID := mappingOpts.bboxID
	if bboxID == "" {
		bboxID = fmt.Sprintf("bbox-%s-%s", mappingOpts.deviceID, randomID(6))
	}

	mapping := &persistence.DeviceBoundingBoxMapping{
		ID:       bboxID,
		DeviceID: mappingOpts.deviceID,
		CameraID: mappingOpts.cameraID,
		Bounds:   *bounds,
	}

	if err := store.SaveBoundingBox(mapping); err != nil {
		return fmt.Errorf("save bounding box: %w", err)
	}

	log.Info().
		Str("bbox_id", bboxID).
		Str("device_id", mappingOpts.deviceID).
		Str("camera_id", mappingOpts.cameraID).
		Msg("Bounding box mapping saved")
	return nil
}

func runMappingRemove(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	bboxID := mappingOpts.bboxID

	// Confirm deletion
	fmt.Printf("Delete bounding box mapping %s? (y/N): ", bboxID)
	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		log.Info().Msg("Cancelled")
		return nil
	}

	if err := store.DeleteBoundingBox(bboxID); err != nil {
		return fmt.Errorf("delete bounding box: %w", err)
	}

	log.Info().Str("bbox_id", bboxID).Msg("Bounding box mapping deleted")
	return nil
}

func runMappingExport(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	mappings, err := store.ListBoundingBoxesForDevice(mappingOpts.deviceID)
	if err != nil {
		return fmt.Errorf("list mappings: %w", err)
	}

	if len(mappings) == 0 {
		log.Info().Str("device_id", mappingOpts.deviceID).Msg("No mappings to export")
		return nil
	}

	// Create export data
	exportData := struct {
		DeviceID   string                                  `json:"device_id"`
		ExportedAt time.Time                               `json:"exported_at"`
		Mappings   []*persistence.DeviceBoundingBoxMapping `json:"mappings"`
	}{
		DeviceID:   mappingOpts.deviceID,
		ExportedAt: time.Now(),
		Mappings:   mappings,
	}

	data, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(mappingOpts.output)
	if outputDir != "" && outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}

	if err := os.WriteFile(mappingOpts.output, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	log.Info().
		Str("file", mappingOpts.output).
		Int("count", len(mappings)).
		Msg("Mappings exported")
	return nil
}

func runMappingImport(cmd *cobra.Command, args []string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	filename := args[0]

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Parse export data
	var exportData struct {
		DeviceID   string                                  `json:"device_id"`
		ExportedAt time.Time                               `json:"exported_at"`
		Mappings   []*persistence.DeviceBoundingBoxMapping `json:"mappings"`
	}

	if err := json.Unmarshal(data, &exportData); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	if len(exportData.Mappings) == 0 {
		log.Info().Msg("No mappings found in file")
		return nil
	}

	// Import each mapping
	imported := 0
	for _, m := range exportData.Mappings {
		if m.ID == "" {
			// Generate new ID if missing
			m.ID = fmt.Sprintf("bbox-%s-%s", m.DeviceID, randomID(6))
		}
		if err := store.SaveBoundingBox(m); err != nil {
			log.Warn().Err(err).Str("bbox_id", m.ID).Msg("Failed to import mapping")
			continue
		}
		imported++
	}

	log.Info().
		Str("file", filename).
		Int("imported", imported).
		Int("total", len(exportData.Mappings)).
		Msg("Mappings imported")
	return nil
}

// parseBounds parses a comma-separated string of normalized coordinates
func parseBounds(s string) (*persistence.BoundingBox, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return nil, fmt.Errorf("expected 4 values (x,y,width,height), got %d", len(parts))
	}

	values := make([]float64, 4)
	for i, part := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number at position %d: %w", i+1, err)
		}
		values[i] = val
	}

	bbox := &persistence.BoundingBox{
		X:      values[0],
		Y:      values[1],
		Width:  values[2],
		Height: values[3],
	}

	if err := bbox.Validate(); err != nil {
		return nil, err
	}

	return bbox, nil
}

// openStore opens the persistence store using default configuration
func openStore() (*persistence.Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	espbrewDir := filepath.Join(homeDir, ".espbrew")
	dbPath := filepath.Join(espbrewDir, "espbrew.db")

	// Ensure espbrew directory exists
	if err := os.MkdirAll(espbrewDir, 0755); err != nil {
		return nil, fmt.Errorf("create espbrew directory: %w", err)
	}

	return persistence.Open(persistence.DefaultConfig(dbPath))
}
