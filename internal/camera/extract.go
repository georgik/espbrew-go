package camera

import (
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"github.com/rs/zerolog/log"
)

// Extractor handles device subimage extraction from full captures
type Extractor struct {
	store        *Store
	mappingStore *persistence.Store
}

// NewExtractor creates a new device extractor
func NewExtractor(store *Store, mappingStore *persistence.Store) *Extractor {
	return &Extractor{
		store:        store,
		mappingStore: mappingStore,
	}
}

// ExtractDevices extracts device subimages from a full capture
// Returns information about each extracted device subimage
func (e *Extractor) ExtractDevices(fullImage image.Image, cameraID, capturePath string) ([]DeviceCaptureInfo, error) {
	// Get all mappings for this camera
	mappings, err := e.mappingStore.ListBoundingBoxesForCamera(cameraID)
	if err != nil {
		return nil, fmt.Errorf("get mappings: %w", err)
	}

	if len(mappings) == 0 {
		log.Debug().Str("camera_id", cameraID).Msg("No device mappings found for camera")
		return nil, nil
	}

	log.Info().
		Str("camera_id", cameraID).
		Int("mappings", len(mappings)).
		Msg("Extracting device subimages")

	// Get capture directory for storing subimages
	captureDir := filepath.Dir(capturePath)
	captureBaseName := filepath.Base(capturePath)
	captureBaseName = captureBaseName[0 : len(captureBaseName)-len(filepath.Ext(captureBaseName))]

	// Create subdirectory for device subimages
	subimageDir := filepath.Join(captureDir, captureBaseName)
	if err := os.MkdirAll(subimageDir, 0755); err != nil {
		return nil, fmt.Errorf("create subimage directory: %w", err)
	}

	results := make([]DeviceCaptureInfo, 0, len(mappings))

	// Get image dimensions
	bounds := fullImage.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	// Extract subimage for each device mapping
	for _, mapping := range mappings {
		// Convert normalized bounds to pixel coordinates
		x, y, width, height := mapping.Bounds.ToPixels(imgWidth, imgHeight)

		log.Debug().
			Str("device_id", mapping.DeviceID).
			Int("x", x).Int("y", y).Int("width", width).Int("height", height).
			Msg("Extracting device region")

		// Create adjustment parameters if configured
		var adj *AdjustmentParams
		if !mapping.Adjustment.IsZero() {
			adj = &AdjustmentParams{
				Brightness: mapping.Adjustment.Brightness,
				Contrast:   mapping.Adjustment.Contrast,
				Saturation: mapping.Adjustment.Saturation,
			}
		}

		// Extract and optionally adjust the region
		subimage, err := ExtractAndAdjust(fullImage, x, y, width, height, adj)
		if err != nil {
			log.Warn().
				Err(err).
				Str("device_id", mapping.DeviceID).
				Msg("Failed to extract device region")
			continue
		}

		// Generate filename for device subimage
		// Sanitize device ID for filename
		safeDeviceID := mapping.DeviceID
		if len(safeDeviceID) > 30 {
			safeDeviceID = safeDeviceID[:30]
		}
		subimageName := fmt.Sprintf("%s.jpg", safeDeviceID)
		subimagePath := filepath.Join(subimageDir, subimageName)

		// Save subimage
		outFile, err := os.Create(subimagePath)
		if err != nil {
			log.Warn().
				Err(err).
				Str("device_id", mapping.DeviceID).
				Msg("Failed to create subimage file")
			continue
		}

		if err := jpeg.Encode(outFile, subimage, &jpeg.Options{Quality: 90}); err != nil {
			outFile.Close()
			log.Warn().
				Err(err).
				Str("device_id", mapping.DeviceID).
				Msg("Failed to encode subimage")
			continue
		}
		outFile.Close()

		// Get relative path for storage
		relPath, err := filepath.Rel(e.store.GetBaseDir(), subimagePath)
		if err != nil {
			log.Warn().
				Err(err).
				Str("device_id", mapping.DeviceID).
				Msg("Failed to get relative path")
			continue
		}

		results = append(results, DeviceCaptureInfo{
			DeviceID:    mapping.DeviceID,
			Bounds:      mapping.Bounds,
			Subimage:    relPath,
			Adjustment:  mapping.Adjustment,
			GeneratedAt: time.Now(),
		})

		log.Info().
			Str("device_id", mapping.DeviceID).
			Str("path", relPath).
			Msg("Device subimage extracted")
	}

	// Save device capture metadata
	if len(results) > 0 {
		if err := e.store.SaveDeviceCaptures(capturePath, results); err != nil {
			log.Warn().Err(err).Str("capture", capturePath).Msg("Failed to save device capture metadata")
		}
	}

	return results, nil
}

// ExtractFromCaptureFile loads a capture from file and extracts device subimages
func (e *Extractor) ExtractFromCaptureFile(capturePath, cameraID string) ([]DeviceCaptureInfo, error) {
	// Open the capture file
	captureFile, err := os.Open(capturePath)
	if err != nil {
		return nil, fmt.Errorf("open capture file: %w", err)
	}
	defer captureFile.Close()

	// Decode the image
	img, _, err := image.Decode(captureFile)
	if err != nil {
		return nil, fmt.Errorf("decode capture: %w", err)
	}

	return e.ExtractDevices(img, cameraID, capturePath)
}

// GetMappingStore returns the mapping store (for testing)
func (e *Extractor) GetMappingStore() *persistence.Store {
	return e.mappingStore
}
