package persistence

import (
	"bytes"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

type DeviceRecord struct {
	DeviceID    string    `json:"device_id"`
	MACAddress  string    `json:"mac_address"`
	ChipType    string    `json:"chip_type"`
	ChipRev     string    `json:"chip_rev"`
	FlashSize   uint32    `json:"flash_size"`
	PSRAMSize   uint32    `json:"psram_size"`
	PSRAMType   string    `json:"psram_type"`
	BoardModel  string    `json:"board_model"`
	Description string    `json:"description"`
	Aliases     []string  `json:"aliases"`
	Tags        []string  `json:"tags"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	LastPath    string    `json:"last_path"`
	NodeID      string    `json:"node_id"`
}

func (s *Store) SaveDevice(dev *DeviceRecord) error {
	if dev == nil {
		return fmt.Errorf("device is nil")
	}
	if dev.DeviceID == "" {
		return fmt.Errorf("device_id required")
	}

	if dev.FirstSeen.IsZero() {
		dev.FirstSeen = time.Now()
	}
	dev.LastSeen = time.Now()

	codec := &codec{}
	data, err := codec.Encode(dev)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDevices))
		if b == nil {
			return fmt.Errorf("devices bucket not found")
		}

		key := deviceKey(dev.DeviceID)

		oldData := b.Get(key)
		if oldData != nil {
			oldDev, err := codec.DecodeDevice(oldData)
			if err == nil {
				if oldDev.MACAddress != "" && oldDev.MACAddress != dev.MACAddress {
					if err := b.Delete(macIndexKey(oldDev.MACAddress)); err != nil {
						return err
					}
				}
				for _, oldAlias := range oldDev.Aliases {
					found := false
					for _, newAlias := range dev.Aliases {
						if oldAlias == newAlias {
							found = true
							break
						}
					}
					if !found {
						if err := b.Delete(aliasIndexKey(oldAlias)); err != nil {
							return err
						}
					}
				}
			}
		}

		if dev.MACAddress != "" {
			macKey := macIndexKey(dev.MACAddress)
			existingID := b.Get(macKey)
			if existingID != nil && string(existingID) != dev.DeviceID {
				return fmt.Errorf("MAC %s already used by device %s", dev.MACAddress, string(existingID))
			}
			if err := b.Put(macKey, []byte(dev.DeviceID)); err != nil {
				return err
			}
		}

		for _, alias := range dev.Aliases {
			if alias == "" {
				continue
			}
			if err := b.Put(aliasIndexKey(alias), []byte(dev.DeviceID)); err != nil {
				return err
			}
		}

		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("put device: %w", err)
		}

		return nil
	})
}

func (s *Store) GetDevice(deviceID string) (*DeviceRecord, error) {
	var dev *DeviceRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDevices))
		if b == nil {
			return fmt.Errorf("devices bucket not found")
		}

		data := b.Get(deviceKey(deviceID))
		if data == nil {
			return fmt.Errorf("device not found: %s", deviceID)
		}

		codec := &codec{}
		var err error
		dev, err = codec.DecodeDevice(data)
		return err
	})

	if err != nil {
		return nil, err
	}
	return dev, nil
}

func (s *Store) GetDeviceByMAC(mac string) (*DeviceRecord, error) {
	var deviceID string

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDevices))
		if b == nil {
			return fmt.Errorf("devices bucket not found")
		}

		id := b.Get(macIndexKey(mac))
		if id == nil {
			return fmt.Errorf("device not found with MAC: %s", mac)
		}
		deviceID = string(id)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetDevice(deviceID)
}

func (s *Store) GetDeviceByAlias(alias string) (*DeviceRecord, error) {
	var deviceID string

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDevices))
		if b == nil {
			return fmt.Errorf("devices bucket not found")
		}

		id := b.Get(aliasIndexKey(alias))
		if id == nil {
			return fmt.Errorf("device not found with alias: %s", alias)
		}
		deviceID = string(id)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetDevice(deviceID)
}

func (s *Store) ListDevices() ([]*DeviceRecord, error) {
	var devices []*DeviceRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDevices))
		if b == nil {
			return fmt.Errorf("devices bucket not found")
		}

		codec := &codec{}
		c := b.Cursor()
		prefix := []byte(prefixDevice)

		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			dev, err := codec.DecodeDevice(v)
			if err != nil {
				return fmt.Errorf("decode device %s: %w", k, err)
			}
			devices = append(devices, dev)
		}

		return nil
	})

	return devices, err
}

func (s *Store) DeleteDevice(deviceID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDevices))
		if b == nil {
			return fmt.Errorf("devices bucket not found")
		}

		key := deviceKey(deviceID)
		data := b.Get(key)
		if data == nil {
			return fmt.Errorf("device not found: %s", deviceID)
		}

		codec := &codec{}
		dev, err := codec.DecodeDevice(data)
		if err != nil {
			return err
		}

		if err := b.Delete(key); err != nil {
			return fmt.Errorf("delete device: %w", err)
		}

		if dev.MACAddress != "" {
			if err := b.Delete(macIndexKey(dev.MACAddress)); err != nil {
				return fmt.Errorf("delete mac index: %w", err)
			}
		}

		for _, alias := range dev.Aliases {
			if alias == "" {
				continue
			}
			if err := b.Delete(aliasIndexKey(alias)); err != nil {
				return fmt.Errorf("delete alias index: %w", err)
			}
		}

		return nil
	})
}

func (s *Store) GenerateManualID(chipType string) (string, error) {
	var nextID int

	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMeta))
		if b == nil {
			return fmt.Errorf("meta bucket not found")
		}

		key := []byte(metaNextManualID)
		v := b.Get(key)
		if v != nil {
			nextID = btoi(v)
		}
		nextID++

		if err := b.Put(key, itob(nextID)); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("manual-%s-%d", chipType, nextID), nil
}
