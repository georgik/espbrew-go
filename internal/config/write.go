package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// ErrConfigPathRequired is returned when no config path is provided.
var ErrConfigPathRequired = errors.New("config path required")

// DefaultConfigPath returns espbrew.toml in the current working directory.
// This is the file the leader auto-loads, so device mappings placed next to
// the running espbrew process are picked up without extra flags.
func DefaultConfigPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, "espbrew.toml"), nil
}

// WriteDevices persists the given device mapping to the espbrew.toml file at
// path. It is the canonical way to register a device so the identity lives in
// the explicit, version-controlled config rather than in hidden runtime
// persistence that silently resurrects on restart.
func WriteDevices(path string, devices []DeviceConfig) error {
	if path == "" {
		return ErrConfigPathRequired
	}

	data, err := toml.Marshal(struct {
		Devices []DeviceConfig `toml:"devices"`
	}{Devices: devices})
	if err != nil {
		return err
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}
