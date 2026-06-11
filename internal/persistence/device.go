package persistence

import (
	"bytes"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"

	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

// BackendConfigData wraps backend-specific configuration for JSON storage
type BackendConfigData struct {
	Wokwi *WokwiConfigData `json:"wokwi,omitempty"`
	QEMU  *QEMUConfigData  `json:"qemu,omitempty"`
}

// WokwiConfigData contains Wokwi simulator configuration for storage
type WokwiConfigData struct {
	ChipType    string `json:"chip_type"`
	DiagramJSON string `json:"diagram_json"`
}

// QEMUConfigData contains QEMU emulator configuration for storage (future)
type QEMUConfigData struct {
	MachineType string `json:"machine_type"`
	MemorySize  int    `json:"memory_size"`
}

type DeviceRecord struct {
	DeviceID        string             `json:"device_id"`
	MACAddress      string             `json:"mac_address"`
	ChipType        string             `json:"chip_type"`
	ChipRev         string             `json:"chip_rev"`
	FlashSize       uint32             `json:"flash_size"`
	PSRAMSize       uint32             `json:"psram_size"`
	PSRAMType       string             `json:"psram_type"`
	BoardModel      string             `json:"board_model"`
	Description     string             `json:"description"`
	Aliases         []string           `json:"aliases"`
	Tags            []string           `json:"tags"`
	FirstSeen       time.Time          `json:"first_seen"`
	LastSeen        time.Time          `json:"last_seen"`
	LastPath        string             `json:"last_path"`
	NodeID          string             `json:"node_id"`
	Disabled        bool               `json:"disabled"`
	DisabledReason  string             `json:"disabled_reason,omitempty"`
	DisabledBy      string             `json:"disabled_by,omitempty"`
	DisabledAt      time.Time          `json:"disabled_at,omitempty"`
	Protected       bool               `json:"protected"`
	ProtectedReason string             `json:"protected_reason,omitempty"`
	ProtectedBy     string             `json:"protected_by,omitempty"`
	ProtectedAt     time.Time          `json:"protected_at,omitempty"`
	Backend         string             `json:"backend,omitempty"`        // Backend type: physical, wokwi, qemu
	BackendConfig   *BackendConfigData `json:"backend_config,omitempty"` // Backend-specific configuration
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

// SetDeviceDisabled sets the disabled state of a device
func (s *Store) SetDeviceDisabled(deviceID string, disabled bool, reason, by string) error {
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

		dev.Disabled = disabled
		if disabled {
			dev.DisabledReason = reason
			dev.DisabledBy = by
			dev.DisabledAt = time.Now()
		} else {
			dev.DisabledReason = ""
			dev.DisabledBy = ""
			dev.DisabledAt = time.Time{}
		}

		encoded, err := codec.Encode(dev)
		if err != nil {
			return err
		}

		return b.Put(key, encoded)
	})
}

// SetDeviceProtected sets the protected state of a device
func (s *Store) SetDeviceProtected(deviceID string, protected bool, reason, by string) error {
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

		dev.Protected = protected
		if protected {
			dev.ProtectedReason = reason
			dev.ProtectedBy = by
			dev.ProtectedAt = time.Now()
		} else {
			dev.ProtectedReason = ""
			dev.ProtectedBy = ""
			dev.ProtectedAt = time.Time{}
		}

		encoded, err := codec.Encode(dev)
		if err != nil {
			return err
		}

		return b.Put(key, encoded)
	})
}

// GetDeviceByPath finds a device by its last known connection path
func (s *Store) GetDeviceByPath(path string) (*DeviceRecord, error) {
	devices, err := s.ListDevices()
	if err != nil {
		return nil, err
	}

	for _, dev := range devices {
		if dev.LastPath == path {
			return dev, nil
		}
	}

	return nil, fmt.Errorf("device not found with path: %s", path)
}

// ToDeviceInfo converts a DeviceRecord to protocol.DeviceInfo
func (d *DeviceRecord) ToDeviceInfo() *protocol.DeviceInfo {
	info := &protocol.DeviceInfo{
		Path:            d.LastPath,
		DeviceID:        d.DeviceID,
		ChipType:        d.ChipType,
		NodeID:          d.NodeID,
		Status:          "available",
		Disabled:        d.Disabled,
		DisabledReason:  d.DisabledReason,
		DisabledBy:      d.DisabledBy,
		DisabledAt:      d.DisabledAt,
		Protected:       d.Protected,
		ProtectedReason: d.ProtectedReason,
		ProtectedBy:     d.ProtectedBy,
		ProtectedAt:     d.ProtectedAt,
		Backend:         protocol.BackendType(d.Backend),
	}

	// Convert backend config
	if d.BackendConfig != nil {
		switch d.Backend {
		case "wokwi":
			if d.BackendConfig.Wokwi != nil {
				info.BackendConfig = &protocol.WokwiConfig{
					ChipType:    d.BackendConfig.Wokwi.ChipType,
					DiagramJSON: d.BackendConfig.Wokwi.DiagramJSON,
				}
			}
		case "qemu":
			if d.BackendConfig.QEMU != nil {
				info.BackendConfig = &protocol.QEMUConfig{
					MachineType: d.BackendConfig.QEMU.MachineType,
					MemorySize:  d.BackendConfig.QEMU.MemorySize,
				}
			}
		}
	}

	// Set disabled status
	if d.Disabled {
		info.Status = "disabled"
	}

	// Set protected status
	if d.Protected {
		info.Status = "protected"
	}

	return info
}
