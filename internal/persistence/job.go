package persistence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

type JobRecord struct {
	ID           string            `json:"id"`
	FirmwarePath string            `json:"firmware_path"`
	DevicePath   string            `json:"device_path"`
	DeviceID     string            `json:"device_id,omitempty"`
	Status       JobStatus         `json:"status"`
	Progress     int               `json:"progress"`
	Error        string            `json:"error,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	StartedAt    *time.Time        `json:"started_at,omitempty"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

func (s *Store) SaveJob(job *JobRecord) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}
	if job.ID == "" {
		return fmt.Errorf("job_id required")
	}

	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}

	codec := &codec{}
	data, err := codec.Encode(job)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketJobs))
		if b == nil {
			return fmt.Errorf("jobs bucket not found")
		}

		key := jobKey(job.ID)
		oldData := b.Get(key)

		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("put job: %w", err)
		}

		pendingKey := []byte(prefixIndexPending + job.ID)
		if job.Status == JobStatusPending {
			if err := b.Put(pendingKey, []byte(job.ID)); err != nil {
				return fmt.Errorf("put pending index: %w", err)
			}
		} else {
			if err := b.Delete(pendingKey); err != nil {
				return fmt.Errorf("delete pending index: %w", err)
			}
		}

		if job.DevicePath != "" {
			var oldPath string
			if oldData != nil {
				if oldJob, err := codec.DecodeJob(oldData); err == nil {
					oldPath = oldJob.DevicePath
				}
			}

			if oldPath != "" && oldPath != job.DevicePath {
				devKey := jobDeviceIndexKey(oldPath)
				if err := removeJobIDFromIndex(b, devKey, job.ID); err != nil {
					return err
				}
			}

			devKey := jobDeviceIndexKey(job.DevicePath)
			if err := addJobIDToIndex(b, devKey, job.ID); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Store) GetJob(jobID string) (*JobRecord, error) {
	var job *JobRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketJobs))
		if b == nil {
			return fmt.Errorf("jobs bucket not found")
		}

		data := b.Get(jobKey(jobID))
		if data == nil {
			return fmt.Errorf("job not found: %s", jobID)
		}

		codec := &codec{}
		var err error
		job, err = codec.DecodeJob(data)
		return err
	})

	return job, err
}

func (s *Store) ListPendingJobs() ([]*JobRecord, error) {
	var jobs []*JobRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketJobs))
		if b == nil {
			return fmt.Errorf("jobs bucket not found")
		}

		codec := &codec{}
		prefix := []byte(prefixIndexPending)

		c := b.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			jobID := string(k[len(prefixIndexPending):])

			data := b.Get(jobKey(jobID))
			if data == nil {
				continue
			}

			job, err := codec.DecodeJob(data)
			if err != nil {
				return err
			}
			jobs = append(jobs, job)
		}

		return nil
	})

	return jobs, err
}

func (s *Store) ListJobsByDevice(devicePath string) ([]*JobRecord, error) {
	var jobIDs []string

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketJobs))
		if b == nil {
			return fmt.Errorf("jobs bucket not found")
		}

		data := b.Get(jobDeviceIndexKey(devicePath))
		if data == nil {
			return nil
		}

		return json.Unmarshal(data, &jobIDs)
	})

	if err != nil {
		return nil, err
	}

	jobs := make([]*JobRecord, 0, len(jobIDs))
	for _, id := range jobIDs {
		job, err := s.GetJob(id)
		if err != nil {
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

func (s *Store) ListJobs() ([]*JobRecord, error) {
	var jobs []*JobRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketJobs))
		if b == nil {
			return fmt.Errorf("jobs bucket not found")
		}

		codec := &codec{}
		c := b.Cursor()
		prefix := []byte(prefixJob)

		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			job, err := codec.DecodeJob(v)
			if err != nil {
				return fmt.Errorf("decode job %s: %w", k, err)
			}
			jobs = append(jobs, job)
		}

		return nil
	})

	return jobs, err
}

func (s *Store) DeleteJob(jobID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketJobs))
		if b == nil {
			return fmt.Errorf("jobs bucket not found")
		}

		key := jobKey(jobID)
		data := b.Get(key)
		if data == nil {
			return fmt.Errorf("job not found: %s", jobID)
		}

		codec := &codec{}
		job, err := codec.DecodeJob(data)
		if err != nil {
			return err
		}

		if err := b.Delete(key); err != nil {
			return err
		}

		if err := b.Delete([]byte(prefixIndexPending + jobID)); err != nil {
			return err
		}

		if job.DevicePath != "" {
			devKey := jobDeviceIndexKey(job.DevicePath)
			if err := removeJobIDFromIndex(b, devKey, jobID); err != nil {
				return err
			}
		}

		return nil
	})
}

func addJobIDToIndex(b *bolt.Bucket, key []byte, jobID string) error {
	data := b.Get(key)
	var jobIDs []string
	if data != nil {
		json.Unmarshal(data, &jobIDs)
	}

	for _, id := range jobIDs {
		if id == jobID {
			return nil
		}
	}

	jobIDs = append(jobIDs, jobID)
	newData, err := json.Marshal(jobIDs)
	if err != nil {
		return err
	}

	return b.Put(key, newData)
}

func removeJobIDFromIndex(b *bolt.Bucket, key []byte, jobID string) error {
	data := b.Get(key)
	if data == nil {
		return nil
	}

	var jobIDs []string
	if err := json.Unmarshal(data, &jobIDs); err != nil {
		return err
	}

	newIDs := make([]string, 0, len(jobIDs))
	for _, id := range jobIDs {
		if id != jobID {
			newIDs = append(newIDs, id)
		}
	}

	if len(newIDs) == 0 {
		return b.Delete(key)
	}

	newData, err := json.Marshal(newIDs)
	if err != nil {
		return err
	}

	return b.Put(key, newData)
}
