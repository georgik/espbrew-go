package config

import "time"

// DeviceConfig is a single entry from the espbrew.toml [[devices]] section.
// It is the explicit, user-authored source of truth for which physical port
// maps to which device identity. Auto-discovery is disabled by default; the
// watcher only reports whether a port is present, so the identity of a port
// must come from here or from an explicit, on-demand probe.
type DeviceConfig struct {
	Path        string `mapstructure:"path" toml:"path"`
	ID          string `mapstructure:"id" toml:"id"`
	ChipType    string `mapstructure:"chip" toml:"chip"`
	Alias       string `mapstructure:"alias" toml:"alias"`
	Description string `mapstructure:"description" toml:"description"`
}

// StringID is a stable key for a device config (path, since ports are the
// physical truth the watcher reports on).
func (d DeviceConfig) StringID() string { return d.Path }

type StaticPeerConfig struct {
	ID      string `mapstructure:"id"`
	Address string `mapstructure:"address"`
	Port    int    `mapstructure:"port"`
}

type ClusterConfig struct {
	ClusterName       string             `mapstructure:"cluster_name"`
	Role              string             `mapstructure:"role"` // leader, peer, standalone
	BindAddress       string             `mapstructure:"bind_address"`
	HTTPPort          int                `mapstructure:"http_port"`
	LeaderAddress     string             `mapstructure:"leader_address"` // For peers
	HeartbeatInterval time.Duration      `mapstructure:"heartbeat_interval"`
	NodeTimeout       time.Duration      `mapstructure:"node_timeout"`
	LogLevel          string             `mapstructure:"log_level"`
	StaticPeers       []StaticPeerConfig `mapstructure:"static_peers"`
	PeerDiscoveryMode string             `mapstructure:"peer_discovery"`         // "mdns", "static", "both"
	LeaderCandidates  []string           `mapstructure:"leader_candidates"`      // For HA
	Devices           []DeviceConfig     `mapstructure:"devices" toml:"devices"` // Explicit port->device mapping
}

// DeviceByPath returns the device config for a given port, or nil.
func (c *ClusterConfig) DeviceByPath(path string) *DeviceConfig {
	for i := range c.Devices {
		if c.Devices[i].Path == path {
			return &c.Devices[i]
		}
	}
	return nil
}

func Default() *ClusterConfig {
	return &ClusterConfig{
		ClusterName:       "espbrew-cluster",
		Role:              "standalone",
		BindAddress:       "0.0.0.0",
		HTTPPort:          8080,
		LeaderAddress:     "",
		HeartbeatInterval: 5 * time.Second,
		NodeTimeout:       30 * time.Second,
		LogLevel:          "info",
		PeerDiscoveryMode: "mdns",
	}
}
