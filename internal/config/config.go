package config

type Config struct {
	Port      string `mapstructure:"port"`
	JWTSecret string `mapstructure:"jwt_secret"`
	Database  struct {
		Driver string `mapstructure:"driver"`
		DSN    string `mapstructure:"dsn"`
	} `mapstructure:"database"`
	Storage struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"storage"`
}

func Load() (*Config, error) {
	// 直接硬编码默认值（简单可靠）
	return &Config{
		Port:      "8080",
		JWTSecret: "your-secret-key-change-in-production",
		Database: struct {
			Driver string `mapstructure:"driver"`
			DSN    string `mapstructure:"dsn"`
		}{
			Driver: "sqlite",
			DSN:    "data/netdisk.db",
		},
		Storage: struct {
			Path string `mapstructure:"path"`
		}{
			Path: "data/uploads",
		},
	}, nil
}
