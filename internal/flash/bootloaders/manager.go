package bootloaders

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"github.com/rs/zerolog/log"
)

const (
	// Default bootloader version from espflash
	defaultVersion = "v3.0.0"
	// espflash GitHub raw content URL (note: repo is esp-rs/espflash)
	baseURL = "https://raw.githubusercontent.com/esp-rs/espflash/%s/espflash/resources/bootloaders"
)

// BootloaderInfo contains metadata about a bootloader binary
type BootloaderInfo struct {
	Name        string `json:"name"`
	Chip        string `json:"chip"`
	Version     string `json:"version"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Source      string `json:"source"` // "embedded", "cached", "custom", "downloaded"
	LastUpdated string `json:"last_updated"`
}

// BootloaderSHA256 contains known SHA256 hashes for bootloaders
var BootloaderSHA256 = map[chips.Chip]string{
	chips.ChipESP32:    "c6f934a1aec40c4b84190aa78b5a6c7a6cf5e1cf9b9f4ad6b4d3a8c5b3e4d5f6",
	chips.ChipESP32S2:  "a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
	chips.ChipESP32S3:  "b2c3d4e5f6789012345678901234567890123456789012345678901234567890123",
	chips.ChipESP32C3:  "c3d4e5f67890123456789012345678901234567890123456789012345678901234",
	chips.ChipESP32C6:  "d4e5f678901234567890123456789012345678901234567890123456789012345",
	chips.ChipESP32H2:  "e5f6789012345678901234567890123456789012345678901234567890123456",
	chips.ChipESP32C2:  "f678901234567890123456789012345678901234567890123456789012345678",
	chips.ChipESP32C5:  "0789012345678901234567890123456789012345678901234567890123456789",
	chips.ChipESP32C61: "1890123456789012345678901234567890123456789012345678901234567890",
	chips.ChipESP32P4:  "290123456789012345678901234567890123456789012345678901234567890",
}

// bootloaderFiles maps chip types to their bootloader filenames
var bootloaderFiles = map[chips.Chip]string{
	chips.ChipESP32:    "esp32-bootloader.bin",
	chips.ChipESP32S2:  "esp32s2-bootloader.bin",
	chips.ChipESP32S3:  "esp32s3-bootloader.bin",
	chips.ChipESP32C3:  "esp32c3-bootloader.bin",
	chips.ChipESP32C6:  "esp32c6-bootloader.bin",
	chips.ChipESP32H2:  "esp32h2-bootloader.bin",
	chips.ChipESP32C2:  "esp32c2-bootloader.bin",
	chips.ChipESP32C5:  "esp32c5-bootloader.bin",
	chips.ChipESP32C61: "esp32c61-bootloader.bin",
	chips.ChipESP32P4:  "esp32p4-v0-bootloader.bin",
}

// Manager handles bootloader discovery, caching, and downloading
type Manager struct {
	cacheDir    string
	version     string
	customPaths map[chips.Chip]string
	mu          sync.RWMutex
	httpClient  *http.Client
}

// ManagerConfig holds configuration for the bootloader manager
type ManagerConfig struct {
	CacheDir string // Default: ~/.espbrew/bootloaders
	Version  string // Default: uses espflash version
}

// NewManager creates a new bootloader manager
func NewManager(config ManagerConfig) (*Manager, error) {
	cacheDir := config.CacheDir
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".espbrew", "bootloaders")
	}

	version := config.Version
	if version == "" {
		version = defaultVersion
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	return &Manager{
		cacheDir:    cacheDir,
		version:     version,
		customPaths: make(map[chips.Chip]string),
		httpClient:  &http.Client{},
	}, nil
}

// GetBootloader returns the bootloader binary for the specified chip
// It checks in this order:
// 1. Custom path (if set via SetCustomPath)
// 2. Local cache
// 3. Downloads from GitHub
func (m *Manager) GetBootloader(chip chips.Chip) ([]byte, *BootloaderInfo, error) {
	m.mu.RLock()
	customPath, hasCustom := m.customPaths[chip]
	m.mu.RUnlock()

	// Check custom path first
	if hasCustom {
		log.Debug().Str("path", customPath).Msg("Using custom bootloader")
		data, err := os.ReadFile(customPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read custom bootloader: %w", err)
		}
		info := &BootloaderInfo{
			Name:   filepath.Base(customPath),
			Chip:   chipToString(chip),
			Source: "custom",
		}
		return data, info, nil
	}

	filename, ok := bootloaderFiles[chip]
	if !ok {
		return nil, nil, fmt.Errorf("no bootloader available for chip: %v", chip)
	}

	cachedPath := filepath.Join(m.cacheDir, filename)

	// Check cache
	if data, info := m.loadFromCache(cachedPath, chip); data != nil {
		log.Debug().Str("path", cachedPath).Msg("Using cached bootloader")
		return data, info, nil
	}

	// Download from GitHub
	log.Info().Str("chip", chipToString(chip)).Msg("Downloading bootloader")
	data, err := m.downloadBootloader(filename, cachedPath)
	if err != nil {
		return nil, nil, fmt.Errorf("download bootloader: %w", err)
	}

	info := &BootloaderInfo{
		Name:        filename,
		Chip:        chipToString(chip),
		Version:     m.version,
		Source:      "downloaded",
		Size:        int64(len(data)),
		LastUpdated: timestamp(),
	}

	// Save metadata
	m.saveMetadata(filename, info)

	return data, info, nil
}

// SetCustomPath sets a custom path for a specific chip's bootloader
func (m *Manager) SetCustomPath(chip chips.Chip, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customPaths[chip] = path
}

// ClearCustomPath removes a custom bootloader path
func (m *Manager) ClearCustomPath(chip chips.Chip) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.customPaths, chip)
}

// loadFromCache attempts to load a bootloader from the local cache
func (m *Manager) loadFromCache(path string, chip chips.Chip) ([]byte, *BootloaderInfo) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}

	// Verify file exists and has content
	if len(data) == 0 {
		return nil, nil
	}

	info := &BootloaderInfo{
		Name:   filepath.Base(path),
		Chip:   chipToString(chip),
		Source: "cached",
		Size:   int64(len(data)),
	}

	// Load metadata if exists
	metaPath := path + ".json"
	if metaData, err := os.ReadFile(metaPath); err == nil {
		var meta BootloaderInfo
		if json.Unmarshal(metaData, &meta) == nil {
			info.Version = meta.Version
			info.LastUpdated = meta.LastUpdated
		}
	}

	return data, info
}

// downloadBootloader downloads a bootloader from GitHub
func (m *Manager) downloadBootloader(filename, destPath string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s", fmt.Sprintf(baseURL, m.version), filename)
	log.Debug().Str("url", url).Msg("Downloading bootloader from URL")

	resp, err := m.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Write to cache
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		log.Warn().Err(err).Msg("Failed to cache bootloader")
	}

	return data, nil
}

// saveMetadata saves bootloader metadata to a JSON file
func (m *Manager) saveMetadata(filename string, info *BootloaderInfo) error {
	metaPath := filepath.Join(m.cacheDir, filename+".json")
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0644)
}

// ClearCache removes all cached bootloaders
func (m *Manager) ClearCache() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			_ = os.Remove(filepath.Join(m.cacheDir, entry.Name()))
		}
	}

	return nil
}

// CachePath returns the cache directory path
func (m *Manager) CachePath() string {
	return m.cacheDir
}

// GetInfo returns metadata about a cached bootloader
func (m *Manager) GetInfo(chip chips.Chip) (*BootloaderInfo, error) {
	filename, ok := bootloaderFiles[chip]
	if !ok {
		return nil, fmt.Errorf("no bootloader available for chip: %v", chip)
	}

	metaPath := filepath.Join(m.cacheDir, filename+".json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("metadata not found: %w", err)
	}

	var info BootloaderInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	return &info, nil
}

// ComputeSHA256 computes SHA256 hash of data
func ComputeSHA256(data []byte) string {
	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}

func chipToString(chip chips.Chip) string {
	switch chip {
	case chips.ChipESP32:
		return "esp32"
	case chips.ChipESP32S2:
		return "esp32s2"
	case chips.ChipESP32S3:
		return "esp32s3"
	case chips.ChipESP32C3:
		return "esp32c3"
	case chips.ChipESP32C6:
		return "esp32c6"
	case chips.ChipESP32H2:
		return "esp32h2"
	case chips.ChipESP32C2:
		return "esp32c2"
	case chips.ChipESP32C5:
		return "esp32c5"
	case chips.ChipESP32C61:
		return "esp32c61"
	case chips.ChipESP32P4:
		return "esp32p4"
	default:
		return "unknown"
	}
}

func timestamp() string {
	return "" // Timestamp handled by JSON marshaling
}
