package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

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
	v := viper.New()

	// 设置默认值
	v.SetDefault("port", "8080")
	v.SetDefault("jwt_secret", "your-secret-key-change-in-production")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "data/netdisk.db")
	v.SetDefault("storage.path", "data/uploads")

	// 设置配置文件
	v.SetConfigName("config")
	v.SetConfigType("toml")
	v.AddConfigPath(".")
	v.AddConfigPath("./")

	// 设置环境变量前缀
	v.SetEnvPrefix("ndisk")
	v.AutomaticEnv()

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// 配置文件不存在时使用默认值
		fmt.Println("Config file not found, using default values")
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// 检查是否使用了默认的 JWT 密钥
	if cfg.JWTSecret == "your-secret-key-change-in-production" || cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET must be set and cannot use default value")
	}

	return &cfg, nil
}

// GenerateExampleConfig 生成示例配置文件
func GenerateExampleConfig(path string) error {
	content := `# NDisk 配置文件

# 服务端口
port = "8080"

# JWT 密钥
jwt_secret = "your-secret-key-change-in-production"

[database]
# 数据库驱动 (sqlite, mysql, postgres)
driver = "sqlite"
# 数据库连接字符串
dsn = "data/netdisk.db"

[storage]
# 文件存储路径
path = "data/uploads"
`
	return os.WriteFile(path, []byte(content), 0644)
}
