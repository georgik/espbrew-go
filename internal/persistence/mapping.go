package persistence

import (
	"bytes"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// DeviceBoundingBoxMapping maps a device to its location in a camera view
type DeviceBoundingBoxMapping struct {
	ID                 string          `json:"id"`
	DeviceID           string          `json:"device_id"`           // Device reference
	CameraID           string          `json:"camera_id"`           // Camera reference (can change, see CameraName)
	CameraName         string          `json:"camera_name"`         // Stable camera identifier
	Bounds             BoundingBox     `json:"bounds"`              // Normalized box
	CalibrationVersion int             `json:"calibration_version"` // Camera position version
	Adjustment         ImageAdjustment `json:"adjustment"`          // Per-region image enhancement
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// CameraCalibration stores camera position data
type CameraCalibration struct {
	ID             string    `json:"id"`
	CameraID       string    `json:"camera_id"`
	Version        int       `json:"version"`         // Increment on position change
	Description    string    `json:"description"`     // Human-readable position name
	ReferenceImage string    `json:"reference_image"` // Optional reference path
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SaveBoundingBox saves a bounding box mapping to the database
func (s *Store) SaveBoundingBox(mapping *DeviceBoundingBoxMapping) error {
	if mapping == nil {
		return fmt.Errorf("mapping is nil")
	}
	if mapping.DeviceID == "" {
		return fmt.Errorf("device_id required")
	}
	if mapping.CameraID == "" {
		return fmt.Errorf("camera_id required")
	}
	if mapping.ID == "" {
		return fmt.Errorf("id required")
	}

	// Validate bounds
	if err := mapping.Bounds.Validate(); err != nil {
		return fmt.Errorf("invalid bounds: %w", err)
	}

	now := time.Now()
	if mapping.CreatedAt.IsZero() {
		mapping.CreatedAt = now
	}
	mapping.UpdatedAt = now

	// If calibration version is not set, try to get current version
	if mapping.CalibrationVersion == 0 {
		calib, err := s.GetCalibration(mapping.CameraID)
		if err == nil && calib != nil {
			mapping.CalibrationVersion = calib.Version
		}
	}

	codec := &codec{}
	data, err := codec.Encode(mapping)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketBoundingBoxes))
		if b == nil {
			return fmt.Errorf("bounding_boxes bucket not found")
		}

		key := boundingBoxKey(mapping.ID)

		// Get old data to update indexes if needed
		oldData := b.Get(key)
		if oldData != nil {
			oldMapping, err := codec.DecodeBoundingBoxMapping(oldData)
			if err == nil {
				// Remove old camera index entry
				oldCamKey := cameraIndexKey(oldMapping.CameraID, oldMapping.ID)
				if err := b.Delete(oldCamKey); err != nil {
					return err
				}
				// Remove old device index entry
				oldDevKey := deviceIndexKey(oldMapping.DeviceID, oldMapping.ID)
				if err := b.Delete(oldDevKey); err != nil {
					return err
				}
			}
		}

		// Save mapping
		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("put bounding box: %w", err)
		}

		// Update camera index
		camKey := cameraIndexKey(mapping.CameraID, mapping.ID)
		if err := b.Put(camKey, []byte(mapping.ID)); err != nil {
			return err
		}

		// Update device index
		devKey := deviceIndexKey(mapping.DeviceID, mapping.ID)
		if err := b.Put(devKey, []byte(mapping.ID)); err != nil {
			return err
		}

		return nil
	})
}

// GetBoundingBox retrieves a bounding box mapping by ID
func (s *Store) GetBoundingBox(id string) (*DeviceBoundingBoxMapping, error) {
	var mapping *DeviceBoundingBoxMapping

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketBoundingBoxes))
		if b == nil {
			return fmt.Errorf("bounding_boxes bucket not found")
		}

		data := b.Get(boundingBoxKey(id))
		if data == nil {
			return fmt.Errorf("bounding box not found: %s", id)
		}

		codec := &codec{}
		var err error
		mapping, err = codec.DecodeBoundingBoxMapping(data)
		return err
	})

	if err != nil {
		return nil, err
	}
	return mapping, nil
}

// GetBoundingBoxForDeviceAndCamera finds a bounding box mapping for a specific device and camera combination.
// Returns the first matching mapping or nil if none exists.
func (s *Store) GetBoundingBoxForDeviceAndCamera(deviceID, cameraID string) (*DeviceBoundingBoxMapping, error) {
	var mapping *DeviceBoundingBoxMapping

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketBoundingBoxes))
		if b == nil {
			return fmt.Errorf("bounding_boxes bucket not found")
		}

		codec := &codec{}
		prefix := cameraIndexPrefix(cameraID)
		c := b.Cursor()

		// Iterate through all mappings for this camera
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			bboxID := string(v)
			data := b.Get(boundingBoxKey(bboxID))
			if data == nil {
				continue
			}
			m, err := codec.DecodeBoundingBoxMapping(data)
			if err != nil {
				continue
			}
			// Check if this is the device we're looking for
			if m.DeviceID == deviceID {
				mapping = m
				return nil
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return mapping, nil
}

// ListBoundingBoxesForCamera retrieves all bounding boxes for a specific camera
// First tries camera_id lookup, then falls back to camera_name lookup for stability
func (s *Store) ListBoundingBoxesForCamera(cameraID string) ([]*DeviceBoundingBoxMapping, error) {
	var mappings []*DeviceBoundingBoxMapping

	// First try direct camera ID lookup (fast path using index)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketBoundingBoxes))
		if b == nil {
			return fmt.Errorf("bounding_boxes bucket not found")
		}

		codec := &codec{}
		prefix := cameraIndexPrefix(cameraID)
		c := b.Cursor()

		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			bboxID := string(v)
			data := b.Get(boundingBoxKey(bboxID))
			if data == nil {
				continue
			}
			mapping, err := codec.DecodeBoundingBoxMapping(data)
			if err != nil {
				return fmt.Errorf("decode bounding box %s: %w", bboxID, err)
			}
			mappings = append(mappings, mapping)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// If we found mappings by camera ID, return them
	if len(mappings) > 0 {
		return mappings, nil
	}

	// Fallback: Search by camera_name for stability across restarts
	// This handles cases where camera DeviceID changes but name stays the same
	var nameBasedMappings []*DeviceBoundingBoxMapping
	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketBoundingBoxes))
		if b == nil {
			return nil
		}

		codec := &codec{}
		c := b.Cursor()

		// Scan all bounding boxes
		for k, v := c.First(); k != nil; k, v = c.Next() {
			// Skip index keys (they start with "idx-")
			if bytes.HasPrefix(k, []byte("idx-")) {
				continue
			}

			data := v
			mapping, err := codec.DecodeBoundingBoxMapping(data)
			if err != nil {
				continue
			}

			// Match by camera name
			if mapping.CameraName == cameraID {
				nameBasedMappings = append(nameBasedMappings, mapping)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// If we found mappings by name, update their camera_id to the new one
	if len(nameBasedMappings) > 0 {
		// Update the mappings with the new camera ID for future lookups
		for _, m := range nameBasedMappings {
			m.CameraID = cameraID
			if err := s.SaveBoundingBox(m); err != nil {
				// Log but don't fail - we'll return the name-based results
				continue
			}
		}
		return nameBasedMappings, nil
	}

	return mappings, nil
}

// ListBoundingBoxesForDevice retrieves all bounding boxes for a specific device
func (s *Store) ListBoundingBoxesForDevice(deviceID string) ([]*DeviceBoundingBoxMapping, error) {
	var mappings []*DeviceBoundingBoxMapping

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketBoundingBoxes))
		if b == nil {
			return fmt.Errorf("bounding_boxes bucket not found")
		}

		codec := &codec{}
		prefix := deviceIndexPrefix(deviceID)
		c := b.Cursor()

		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			bboxID := string(v)
			data := b.Get(boundingBoxKey(bboxID))
			if data == nil {
				continue
			}
			mapping, err := codec.DecodeBoundingBoxMapping(data)
			if err != nil {
				return fmt.Errorf("decode bounding box %s: %w", bboxID, err)
			}
			mappings = append(mappings, mapping)
		}

		return nil
	})

	return mappings, err
}

// DeleteBoundingBox removes a bounding box mapping from the database
func (s *Store) DeleteBoundingBox(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketBoundingBoxes))
		if b == nil {
			return fmt.Errorf("bounding_boxes bucket not found")
		}

		key := boundingBoxKey(id)
		data := b.Get(key)
		if data == nil {
			return fmt.Errorf("bounding box not found: %s", id)
		}

		codec := &codec{}
		mapping, err := codec.DecodeBoundingBoxMapping(data)
		if err != nil {
			return err
		}

		// Delete the mapping
		if err := b.Delete(key); err != nil {
			return fmt.Errorf("delete bounding box: %w", err)
		}

		// Delete camera index entry
		camKey := cameraIndexKey(mapping.CameraID, id)
		if err := b.Delete(camKey); err != nil {
			return fmt.Errorf("delete camera index: %w", err)
		}

		// Delete device index entry
		devKey := deviceIndexKey(mapping.DeviceID, id)
		if err := b.Delete(devKey); err != nil {
			return fmt.Errorf("delete device index: %w", err)
		}

		return nil
	})
}

// SaveCalibration saves or updates a camera calibration
func (s *Store) SaveCalibration(calib *CameraCalibration) error {
	if calib == nil {
		return fmt.Errorf("calibration is nil")
	}
	if calib.CameraID == "" {
		return fmt.Errorf("camera_id required")
	}
	if calib.ID == "" {
		return fmt.Errorf("id required")
	}

	now := time.Now()
	if calib.CreatedAt.IsZero() {
		calib.CreatedAt = now
	}
	calib.UpdatedAt = now

	codec := &codec{}
	data, err := codec.Encode(calib)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketCalibrations))
		if b == nil {
			return fmt.Errorf("calibrations bucket not found")
		}

		key := calibrationKey(calib.ID)
		camIndexKey := calibrationCameraIndexKey(calib.CameraID)

		// Save calibration
		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("put calibration: %w", err)
		}

		// Update camera index (points to current calibration)
		if err := b.Put(camIndexKey, []byte(calib.ID)); err != nil {
			return err
		}

		return nil
	})
}

// GetCalibration retrieves the current calibration for a camera
func (s *Store) GetCalibration(cameraID string) (*CameraCalibration, error) {
	var calib *CameraCalibration

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketCalibrations))
		if b == nil {
			return fmt.Errorf("calibrations bucket not found")
		}

		// Get the current calibration ID for this camera
		camIndexKey := calibrationCameraIndexKey(cameraID)
		calibID := b.Get(camIndexKey)
		if calibID == nil {
			return fmt.Errorf("no calibration found for camera: %s", cameraID)
		}

		// Get the calibration data
		data := b.Get(calibrationKey(string(calibID)))
		if data == nil {
			return fmt.Errorf("calibration data not found: %s", calibID)
		}

		codec := &codec{}
		var err error
		calib, err = codec.DecodeCameraCalibration(data)
		return err
	})

	if err != nil {
		return nil, err
	}
	return calib, nil
}

// IncrementCalibrationVersion creates a new calibration version for a camera
func (s *Store) IncrementCalibrationVersion(cameraID, description string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketCalibrations))
		if b == nil {
			return fmt.Errorf("calibrations bucket not found")
		}

		codec := &codec{}
		var newVersion int

		// Get current calibration to determine next version
		camIndexKey := calibrationCameraIndexKey(cameraID)
		currentCalibID := b.Get(camIndexKey)
		if currentCalibID != nil {
			currentData := b.Get(calibrationKey(string(currentCalibID)))
			if currentData != nil {
				currentCalib, err := codec.DecodeCameraCalibration(currentData)
				if err == nil {
					newVersion = currentCalib.Version + 1
				}
			}
		}
		if newVersion == 0 {
			newVersion = 1
		}

		// Create new calibration record
		now := time.Now()
		newCalib := &CameraCalibration{
			ID:          fmt.Sprintf("calib-%s-%d", cameraID, newVersion),
			CameraID:    cameraID,
			Version:     newVersion,
			Description: description,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		data, err := codec.Encode(newCalib)
		if err != nil {
			return err
		}

		// Save new calibration
		key := calibrationKey(newCalib.ID)
		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("put calibration: %w", err)
		}

		// Update camera index to point to new calibration
		if err := b.Put(camIndexKey, []byte(newCalib.ID)); err != nil {
			return err
		}

		return nil
	})
}
