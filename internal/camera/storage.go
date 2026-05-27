package camera

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Store manages captured images storage
type Store struct {
	baseDir string
	mu      sync.Mutex
}

// NewStore creates a new image storage manager
func NewStore(baseDir string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	return &Store{baseDir: baseDir}, nil
}

// DefaultStore creates a store in ~/.espbrew/captures
func DefaultStore() (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".espbrew", "captures")
	return NewStore(baseDir)
}

// GetDateDir returns the directory for today's captures
func (s *Store) GetDateDir() (string, error) {
	dateStr := time.Now().Format("2006-01-02")
	dateDir := filepath.Join(s.baseDir, dateStr)

	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return "", fmt.Errorf("create date directory: %w", err)
	}

	return dateDir, nil
}

// GenerateFilename creates a unique filename for a capture
func (s *Store) GenerateFilename(cameraID, format string) (string, error) {
	dateDir, err := s.GetDateDir()
	if err != nil {
		return "", err
	}

	// Create a shorter camera ID for filename
	shortID := cameraID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	timestamp := time.Now().Format("20060102-150405")
	ext := format
	if ext == "" || ext == "jpeg" {
		ext = "jpg"
	}

	filename := fmt.Sprintf("cam-%s-%s.%s", shortID, timestamp, ext)
	return filepath.Join(dateDir, filename), nil
}

// Save saves image data and updates metadata
func (s *Store) Save(cameraID, format string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.GenerateFilename(cameraID, format)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write image file: %w", err)
	}

	// Update metadata
	if err := s.updateMetadata(cameraID, path, format, len(data)); err != nil {
		log.Warn().Err(err).Msg("Failed to update metadata")
	}

	log.Info().Str("path", path).Int("size", len(data)).Msg("Image saved")
	return path, nil
}

// CaptureMetadata holds information about a single capture
type CaptureMetadata struct {
	Filename  string    `json:"filename"`
	Timestamp time.Time `json:"timestamp"`
	CameraID  string    `json:"camera_id"`
	Format    string    `json:"format"`
	SizeBytes int64     `json:"size_bytes"`
}

// CameraMetadata holds all captures for a camera
type CameraMetadata struct {
	CameraID   string            `json:"camera_id"`
	CameraName string            `json:"camera_name"`
	CreatedAt  time.Time         `json:"created_at"`
	Captures   []CaptureMetadata `json:"captures"`
}

// getMetadataPath returns the metadata file path for a date directory
func (s *Store) getMetadataPath(dateDir string) string {
	return filepath.Join(dateDir, "metadata.json")
}

// loadMetadata loads metadata from a date directory
func (s *Store) loadMetadata(dateDir string) (map[string]*CameraMetadata, error) {
	path := s.getMetadataPath(dateDir)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]*CameraMetadata), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}

	var metadata map[string]*CameraMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	if metadata == nil {
		metadata = make(map[string]*CameraMetadata)
	}

	return metadata, nil
}

// saveMetadata writes metadata to a date directory
func (s *Store) saveMetadata(dateDir string, metadata map[string]*CameraMetadata) error {
	path := s.getMetadataPath(dateDir)

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	return nil
}

// updateMetadata adds a capture record to the metadata file
func (s *Store) updateMetadata(cameraID, path, format string, size int) error {
	dateDir, err := s.GetDateDir()
	if err != nil {
		return err
	}

	metadata, err := s.loadMetadata(dateDir)
	if err != nil {
		return err
	}

	// Get or create camera metadata
	camMeta, ok := metadata[cameraID]
	if !ok {
		camMeta = &CameraMetadata{
			CameraID:  cameraID,
			CreatedAt: time.Now(),
			Captures:  make([]CaptureMetadata, 0),
		}
		metadata[cameraID] = camMeta
	}

	// Add capture record
	capture := CaptureMetadata{
		Filename:  filepath.Base(path),
		Timestamp: time.Now(),
		CameraID:  cameraID,
		Format:    format,
		SizeBytes: int64(size),
	}
	camMeta.Captures = append(camMeta.Captures, capture)

	return s.saveMetadata(dateDir, metadata)
}

// ListCaptures returns all captures for a specific date
func (s *Store) ListCaptures(date time.Time) ([]CaptureMetadata, error) {
	dateDir := filepath.Join(s.baseDir, date.Format("2006-01-02"))

	metadata, err := s.loadMetadata(dateDir)
	if err != nil {
		return nil, err
	}

	var all []CaptureMetadata
	for _, camMeta := range metadata {
		all = append(all, camMeta.Captures...)
	}

	return all, nil
}

// CleanupOld removes captures older than the specified duration
func (s *Store) CleanupOld(olderThan time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	removed := 0

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return fmt.Errorf("list storage directory: %w", err)
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

		if date.Before(cutoff) {
			dirPath := filepath.Join(s.baseDir, entry.Name())
			if err := os.RemoveAll(dirPath); err != nil {
				log.Warn().Err(err).Str("dir", entry.Name()).Msg("Failed to remove old captures")
			} else {
				removed++
				log.Debug().Str("dir", entry.Name()).Msg("Removed old captures")
			}
		}
	}

	if removed > 0 {
		log.Info().Int("removed", removed).Msg("Cleaned up old captures")
	}

	return nil
}

// GetBaseDir returns the base storage directory
func (s *Store) GetBaseDir() string {
	return s.baseDir
}
