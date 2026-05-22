package config

import (
	"github.com/spf13/viper"
)

func Load(cfgPath string) (*ClusterConfig, error) {
	v := viper.New()
	cfg := Default()

	if cfgPath != "" {
		v.SetConfigFile(cfgPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, err
		}
	}

	v.SetEnvPrefix("ESPBREW")
	v.AutomaticEnv()

	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
