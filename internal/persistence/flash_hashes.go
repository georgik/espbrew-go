package persistence

import (
	"encoding/json"
	"fmt"

	"codeberg.org/georgik/espbrew-go/internal/flashhash"
	bolt "go.etcd.io/bbolt"
)

// SaveFlashHashes saves flash region hashes for a job
func (s *Store) SaveFlashHashes(hashes *flashhash.JobFlashHashes) error {
	if hashes == nil {
		return fmt.Errorf("hashes is nil")
	}
	if hashes.JobID == "" {
		return fmt.Errorf("job_id required")
	}

	data, err := json.Marshal(hashes)
	if err != nil {
		return fmt.Errorf("marshal hashes: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketFlashHashes))
		if b == nil {
			return fmt.Errorf("flash_hashes bucket not found")
		}

		key := flashHashKey(hashes.JobID)
		return b.Put(key, data)
	})
}

// GetFlashHashes retrieves flash hashes for a specific job
func (s *Store) GetFlashHashes(jobID string) (*flashhash.JobFlashHashes, error) {
	var hashes *flashhash.JobFlashHashes

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketFlashHashes))
		if b == nil {
			return fmt.Errorf("flash_hashes bucket not found")
		}

		data := b.Get(flashHashKey(jobID))
		if data == nil {
			return fmt.Errorf("flash hashes not found for job: %s", jobID)
		}

		var h flashhash.JobFlashHashes
		if err := json.Unmarshal(data, &h); err != nil {
			return fmt.Errorf("decode hashes: %w", err)
		}

		hashes = &h
		return nil
	})

	if err != nil {
		return nil, err
	}
	return hashes, nil
}

// DeleteFlashHashes removes flash hashes for a job
func (s *Store) DeleteFlashHashes(jobID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketFlashHashes))
		if b == nil {
			return fmt.Errorf("flash_hashes bucket not found")
		}

		key := flashHashKey(jobID)
		return b.Delete(key)
	})
}

// ListFlashHashesForDevice retrieves all flash hash records for a specific device
func (s *Store) ListFlashHashesForDevice(deviceID string) ([]*flashhash.JobFlashHashes, error) {
	var hashes []*flashhash.JobFlashHashes

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketFlashHashes))
		if b == nil {
			return fmt.Errorf("flash_hashes bucket not found")
		}

		c := b.Cursor()
		prefix := []byte("hash-")

		for k, v := c.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
			var h flashhash.JobFlashHashes
			if err := json.Unmarshal(v, &h); err != nil {
				continue
			}

			// Filter by device ID if specified
			if deviceID != "" && h.DeviceID != deviceID {
				continue
			}

			hashes = append(hashes, &h)
		}

		return nil
	})

	return hashes, err
}

// CreateFlashHashesBucket creates the flash_hashes bucket if it doesn't exist
func (s *Store) createFlashHashesBucket() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.Bucket([]byte(bucketFlashHashes)).CreateBucketIfNotExists([]byte(bucketFlashHashes))
		if err != nil {
			return fmt.Errorf("create flash_hashes bucket: %w", err)
		}
		_ = b
		return nil
	})
}

func flashHashKey(jobID string) []byte {
	return []byte("hash-" + jobID)
}

// EnsureFlashHashesBucket initializes the flash_hashes bucket on store open
func (s *Store) EnsureFlashHashesBucket() error {
	return s.createFlashHashesBucket()
}
