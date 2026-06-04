package persistence

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// CameraSettings stores camera-specific control values
type CameraSettings struct {
	CameraID         string    `json:"camera_id"`          // Unique camera identifier
	Name             string    `json:"name"`               // Human-readable name
	Brightness       int32     `json:"brightness"`         // 0-255
	Contrast         int32     `json:"contrast"`           // 0-255
	Saturation       int32     `json:"saturation"`         // 0-255
	Sharpness        int32     `json:"sharpness"`          // 0-255
	Gain             int32     `json:"gain"`               // 0-255
	Focus            int32     `json:"focus"`              // 0-255 (if supported)
	Exposure         int32     `json:"exposure"`           // Exposure value (if manual)
	WhiteBalance     int32     `json:"white_balance"`      // Color temperature
	AutoExposure     bool      `json:"auto_exposure"`      // Auto exposure enabled
	AutoFocus        bool      `json:"auto_focus"`         // Auto focus enabled
	AutoWhiteBalance bool      `json:"auto_white_balance"` // Auto white balance
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Preset name if this is a named preset
	PresetName string `json:"preset_name,omitempty"`
}

// CameraSettingsFilter defines criteria for listing camera settings
type CameraSettingsFilter struct {
	CameraID   string // Filter by specific camera
	PresetName string // Filter by preset name
	Limit      int    // Maximum results to return
}

// StoreCameraSettings saves camera settings to the database
func (s *Store) StoreCameraSettings(settings *CameraSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketCameraSettings))
		if b == nil {
			return fmt.Errorf("camera settings bucket not found")
		}

		// Set timestamps
		now := time.Now()
		if settings.CreatedAt.IsZero() {
			settings.CreatedAt = now
		}
		settings.UpdatedAt = now

		data, err := json.Marshal(settings)
		if err != nil {
			return fmt.Errorf("marshal settings: %w", err)
		}

		key := cameraSettingsKey(settings.CameraID)
		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("put settings: %w", err)
		}

		return nil
	})
}

// GetCameraSettings retrieves camera settings by camera ID
func (s *Store) GetCameraSettings(cameraID string) (*CameraSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var settings CameraSettings

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketCameraSettings))
		if b == nil {
			return fmt.Errorf("camera settings bucket not found")
		}

		key := cameraSettingsKey(cameraID)
		data := b.Get(key)
		if data == nil {
			return fmt.Errorf("camera settings not found: %s", cameraID)
		}

		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("unmarshal settings: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &settings, nil
}

// ListCameraSettings retrieves all camera settings
func (s *Store) ListCameraSettings(filter *CameraSettingsFilter) ([]CameraSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []CameraSettings

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketCameraSettings))
		if b == nil {
			return fmt.Errorf("camera settings bucket not found")
		}

		return b.ForEach(func(k, v []byte) error {
			var settings CameraSettings
			if err := json.Unmarshal(v, &settings); err != nil {
				return fmt.Errorf("unmarshal settings: %w", err)
			}

			// Apply filters
			if filter != nil {
				if filter.CameraID != "" && settings.CameraID != filter.CameraID {
					return nil
				}
				if filter.PresetName != "" && settings.PresetName != filter.PresetName {
					return nil
				}
			}

			results = append(results, settings)

			// Apply limit
			if filter != nil && filter.Limit > 0 && len(results) >= filter.Limit {
				return fmt.Errorf("limit reached")
			}

			return nil
		})
	})

	if err != nil && err.Error() == "limit reached" {
		err = nil
	}

	return results, err
}

// DeleteCameraSettings removes camera settings by camera ID
func (s *Store) DeleteCameraSettings(cameraID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketCameraSettings))
		if b == nil {
			return fmt.Errorf("camera settings bucket not found")
		}

		key := cameraSettingsKey(cameraID)
		if err := b.Delete(key); err != nil {
			return fmt.Errorf("delete settings: %w", err)
		}

		return nil
	})
}

// GetCameraPreset retrieves a camera preset by name
func (s *Store) GetCameraPreset(name string) (*CameraSettings, error) {
	settings, err := s.ListCameraSettings(&CameraSettingsFilter{
		PresetName: name,
		Limit:      1,
	})
	if err != nil {
		return nil, err
	}
	if len(settings) == 0 {
		return nil, fmt.Errorf("preset not found: %s", name)
	}
	return &settings[0], nil
}

// ListCameraPresets retrieves all named camera presets
func (s *Store) ListCameraPresets() ([]CameraSettings, error) {
	return s.ListCameraSettings(&CameraSettingsFilter{
		PresetName: "",
	})
}

// cameraSettingsKey generates the storage key for camera settings
func cameraSettingsKey(cameraID string) []byte {
	return []byte("camsettings:" + cameraID)
}
