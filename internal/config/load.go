package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ResolveConfigPath returns the explicit path, or espbrew.toml in the current
// working directory. The leader loads and writes the mapping at this path.
func ResolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "espbrew.toml")
	}
	return "espbrew.toml"
}

func Load(cfgPath string) (*ClusterConfig, error) {
	v := viper.New()
	v.SetConfigType("toml")
	cfg := Default()

	// Resolve the config file path. An explicit path wins; otherwise fall
	// back to espbrew.toml in the current working directory so the tool picks
	// up the nearest explicit device mapping.
	path := cfgPath
	if path == "" {
		if wd, err := os.Getwd(); err == nil {
			path = filepath.Join(wd, "espbrew.toml")
		}
	}
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("read config %q: %w", path, err)
			}
		}
	}

	v.SetEnvPrefix("ESPBREW")
	v.AutomaticEnv()

	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
