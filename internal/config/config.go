package config

import "github.com/spf13/viper"

type Config struct {
	Port    string `mapstructure:"port"`
	Storage struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"storage"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Port: "8080",  // 直接默认值
	}
	viper.SetDefault("storage.path", "./data/uploads")
	
	// 暂时先不用配置文件，直接默认值
	return cfg, nil
}