package config

import "time"

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
	PeerDiscoveryMode string             `mapstructure:"peer_discovery"`    // "mdns", "static", "both"
	LeaderCandidates  []string           `mapstructure:"leader_candidates"` // For HA
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
