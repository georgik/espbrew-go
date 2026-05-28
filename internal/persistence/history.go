package persistence

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

type FlashStatus string

const (
	FlashStatusPending   FlashStatus = "pending"
	FlashStatusRunning   FlashStatus = "running"
	FlashStatusCompleted FlashStatus = "completed"
	FlashStatusFailed    FlashStatus = "failed"
)

type FlashRecord struct {
	ID           string            `json:"id"`
	DeviceID     string            `json:"device_id"`
	DevicePath   string            `json:"device_path"`
	FirmwarePath string            `json:"firmware_path"`
	Status       FlashStatus       `json:"status"`
	Progress     int               `json:"progress"`
	Error        string            `json:"error,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
	Duration     int64             `json:"duration_ms,omitempty"`
	Success      bool              `json:"success"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

func (s *Store) SaveFlashRecord(rec *FlashRecord) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	if rec.ID == "" {
		return fmt.Errorf("record_id required")
	}

	if rec.StartedAt.IsZero() {
		rec.StartedAt = time.Now()
	}

	yearMonth := rec.StartedAt.Format("2006-01")
	dateKey := rec.StartedAt.Format("2006-01-02")

	codec := &codec{}
	data, err := codec.Encode(rec)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHistory))
		if b == nil {
			return fmt.Errorf("history bucket not found")
		}

		deviceHistKey := historyDeviceKey(rec.DeviceID, yearMonth)
		if err := appendToHistoryList(b, deviceHistKey, rec.ID, data); err != nil {
			return err
		}

		dateHistKey := historyDateKey(dateKey)
		if err := appendToHistoryList(b, dateHistKey, rec.ID, data); err != nil {
			return err
		}

		return nil
	})
}

func appendToHistoryList(b *bolt.Bucket, key []byte, id string, data []byte) error {
	existing := b.Get(key)
	if existing == nil {
		list := []map[string]string{
			{"id": id, "data": base64.StdEncoding.EncodeToString(data)},
		}
		encoded, err := json.Marshal(list)
		if err != nil {
			return err
		}
		return b.Put(key, encoded)
	}

	var list []map[string]string
	if err := json.Unmarshal(existing, &list); err != nil {
		return err
	}

	list = append(list, map[string]string{
		"id":   id,
		"data": base64.StdEncoding.EncodeToString(data),
	})

	encoded, err := json.Marshal(list)
	if err != nil {
		return err
	}

	return b.Put(key, encoded)
}

func (s *Store) GetFlashHistory(deviceID string, yearMonth string) ([]*FlashRecord, error) {
	var records []*FlashRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHistory))
		if b == nil {
			return fmt.Errorf("history bucket not found")
		}

		data := b.Get(historyDeviceKey(deviceID, yearMonth))
		if data == nil {
			return nil
		}

		var list []map[string]string
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}

		codec := &codec{}
		for _, item := range list {
			recordData, err := base64.StdEncoding.DecodeString(item["data"])
			if err != nil {
				return err
			}
			rec, err := codec.DecodeFlashRecord(recordData)
			if err != nil {
				return err
			}
			records = append(records, rec)
		}

		return nil
	})

	return records, err
}

func (s *Store) GetFlashHistoryByDate(date string) ([]*FlashRecord, error) {
	var records []*FlashRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHistory))
		if b == nil {
			return fmt.Errorf("history bucket not found")
		}

		data := b.Get(historyDateKey(date))
		if data == nil {
			return nil
		}

		var list []map[string]string
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}

		codec := &codec{}
		for _, item := range list {
			recordData, err := base64.StdEncoding.DecodeString(item["data"])
			if err != nil {
				return err
			}
			rec, err := codec.DecodeFlashRecord(recordData)
			if err != nil {
				return err
			}
			records = append(records, rec)
		}

		return nil
	})

	return records, err
}

func (s *Store) ListRecentFlashes(limit int) ([]*FlashRecord, error) {
	var records []*FlashRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHistory))
		if b == nil {
			return fmt.Errorf("history bucket not found")
		}

		codec := &codec{}
		prefix := []byte(prefixHistDate)

		seen := make(map[string]bool)

		c := b.Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			if len(records) >= limit {
				break
			}

			var list []map[string]string
			if err := json.Unmarshal(v, &list); err != nil {
				continue
			}

			for _, item := range list {
				if len(records) >= limit {
					break
				}

				recordData, err := base64.StdEncoding.DecodeString(item["data"])
				if err != nil {
					continue
				}

				id := item["id"]
				if seen[id] {
					continue
				}
				seen[id] = true

				rec, err := codec.DecodeFlashRecord(recordData)
				if err != nil {
					continue
				}
				records = append(records, rec)
			}
		}

		return nil
	})

	return records, err
}
