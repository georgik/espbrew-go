package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/inventory/rom"
	"github.com/rs/zerolog/log"
)

const (
	// DevicesFileName is the name of the inventory database file
	DevicesFileName = "devices.json"
	// ESPBrewDir is the directory name in user home
	ESPBrewDir = ".espbrew"
)

// Inventory manages device inventory database
type Inventory struct {
	mu      sync.RWMutex
	dbPath  string
	devices map[string]*DeviceInventory
}

// NewInventory creates a new inventory manager
func NewInventory() (*Inventory, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	inventoryDir := filepath.Join(homeDir, ESPBrewDir)
	dbPath := filepath.Join(inventoryDir, DevicesFileName)

	inv := &Inventory{
		dbPath:  dbPath,
		devices: make(map[string]*DeviceInventory),
	}

	if err := inv.load(); err != nil {
		log.Warn().Err(err).Msg("Failed to load inventory, starting fresh")
		// Don't fail if file doesn't exist yet
	}

	return inv, nil
}

// load loads the inventory from disk
func (i *Inventory) load() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	data, err := os.ReadFile(i.dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet
		}
		return err
	}

	var devices []*DeviceInventory
	if err := json.Unmarshal(data, &devices); err != nil {
		return err
	}

	i.devices = make(map[string]*DeviceInventory)
	for _, dev := range devices {
		i.devices[dev.DeviceID] = dev
	}

	log.Debug().Int("count", len(i.devices)).Msg("Loaded device inventory")
	return nil
}

// save saves the inventory to disk
func (i *Inventory) save() error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(i.dbPath), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	devices := make([]*DeviceInventory, 0, len(i.devices))
	for _, dev := range i.devices {
		devices = append(devices, dev)
	}

	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(i.dbPath, data, 0644); err != nil {
		return err
	}

	return nil
}

// Get retrieves a device by ID
func (i *Inventory) Get(deviceID string) (*DeviceInventory, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	dev, ok := i.devices[deviceID]
	if !ok {
		return nil, fmt.Errorf("device not found: %s", deviceID)
	}
	return dev, nil
}

// FindByMAC finds a device by MAC address
func (i *Inventory) FindByMAC(mac string) (*DeviceInventory, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, dev := range i.devices {
		if dev.MACAddress == mac {
			return dev, nil
		}
	}
	return nil, fmt.Errorf("device not found with MAC: %s", mac)
}

// FindByAlias finds a device by alias
func (i *Inventory) FindByAlias(alias string) (*DeviceInventory, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, dev := range i.devices {
		for _, a := range dev.Aliases {
			if a == alias {
				return dev, nil
			}
		}
	}
	return nil, fmt.Errorf("device not found with alias: %s", alias)
}

// FindByPath finds a device by its last known path
func (i *Inventory) FindByPath(path string) (*DeviceInventory, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, dev := range i.devices {
		if dev.LastPath == path {
			return dev, nil
		}
	}
	return nil, fmt.Errorf("device not found with path: %s", path)
}

// FindByRequirements finds all devices matching the given requirements
func (i *Inventory) FindByRequirements(req *FlashRequirement) ([]*DeviceInventory, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	var results []*DeviceInventory
	for _, dev := range i.devices {
		if dev.Matches(req) {
			results = append(results, dev)
		}
	}
	return results, nil
}

// List returns all devices
func (i *Inventory) List() []*DeviceInventory {
	i.mu.RLock()
	defer i.mu.RUnlock()

	devices := make([]*DeviceInventory, 0, len(i.devices))
	for _, dev := range i.devices {
		devices = append(devices, dev)
	}
	return devices
}

// Save saves or updates a device in the inventory
func (i *Inventory) Save(device *DeviceInventory) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	device.LastSeen = time.Now()
	i.devices[device.DeviceID] = device

	return i.save()
}

// Delete removes a device from the inventory
func (i *Inventory) Delete(deviceID string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, ok := i.devices[deviceID]; !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	delete(i.devices, deviceID)
	return i.save()
}

// GetOrCreate gets a device by identity, creating it if it doesn't exist
func (i *Inventory) GetOrCreate(identity *DeviceIdentity, path, nodeID string) (*DeviceInventory, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	deviceID := rom.DeviceID(identity.MAC)

	// Check if device already exists
	if dev, ok := i.devices[deviceID]; ok {
		// Update last seen and path
		dev.LastSeen = time.Now()
		dev.LastPath = path
		if nodeID != "" {
			dev.NodeID = nodeID
		}
		return dev, i.save()
	}

	// Create new device entry
	now := time.Now()
	dev := &DeviceInventory{
		DeviceID:   deviceID,
		MACAddress: identity.MAC,
		ChipType:   identity.Chip,
		ChipRev:    formatRevision(identity.ChipMajor, identity.ChipMinor),
		FlashSize:  identity.FlashSize,
		PSRAMSize:  identity.PSRAMSize,
		PSRAMType:  identity.PSRAMType,
		Aliases:    []string{},
		Tags:       []string{},
		FirstSeen:  now,
		LastSeen:   now,
		LastPath:   path,
		NodeID:     nodeID,
	}

	i.devices[deviceID] = dev
	return dev, i.save()
}

// UpdateTags updates the tags for a device
func (i *Inventory) UpdateTags(deviceID string, tags []string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	dev, ok := i.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	dev.Tags = tags
	return i.save()
}

// AddTag adds a tag to a device
func (i *Inventory) AddTag(deviceID, tag string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	dev, ok := i.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	// Check if tag already exists
	for _, t := range dev.Tags {
		if t == tag {
			return nil // Already has this tag
		}
	}

	dev.Tags = append(dev.Tags, tag)
	return i.save()
}

// RemoveTag removes a tag from a device
func (i *Inventory) RemoveTag(deviceID, tag string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	dev, ok := i.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	// Filter out the tag
	newTags := make([]string, 0, len(dev.Tags))
	for _, t := range dev.Tags {
		if t != tag {
			newTags = append(newTags, t)
		}
	}

	dev.Tags = newTags
	return i.save()
}

// UpdateAliases updates the aliases for a device
func (i *Inventory) UpdateAliases(deviceID string, aliases []string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	dev, ok := i.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	dev.Aliases = aliases
	return i.save()
}

// AddAlias adds an alias to a device
func (i *Inventory) AddAlias(deviceID, alias string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	dev, ok := i.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	// Check if alias already exists
	for _, a := range dev.Aliases {
		if a == alias {
			return nil // Already has this alias
		}
	}

	dev.Aliases = append(dev.Aliases, alias)
	return i.save()
}

// RemoveAlias removes an alias from a device
func (i *Inventory) RemoveAlias(deviceID, alias string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	dev, ok := i.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	// Filter out the alias
	newAliases := make([]string, 0, len(dev.Aliases))
	for _, a := range dev.Aliases {
		if a != alias {
			newAliases = append(newAliases, a)
		}
	}

	dev.Aliases = newAliases
	return i.save()
}

// SetBoardModel sets the board model for a device
func (i *Inventory) SetBoardModel(deviceID, model string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	dev, ok := i.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	dev.BoardModel = model
	return i.save()
}

// SetDescription sets the description for a device
func (i *Inventory) SetDescription(deviceID, desc string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	dev, ok := i.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	dev.Description = desc
	return i.save()
}
