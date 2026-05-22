package config

import "time"

type ClusterConfig struct {
	ClusterName       string        `mapstructure:"cluster_name"`
	Role              string        `mapstructure:"role"` // leader, peer, standalone
	BindAddress       string        `mapstructure:"bind_address"`
	HTTPPort          int           `mapstructure:"http_port"`
	LeaderAddress     string        `mapstructure:"leader_address"` // For peers
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	NodeTimeout       time.Duration `mapstructure:"node_timeout"`
	LogLevel          string        `mapstructure:"log_level"`
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
	}
}
