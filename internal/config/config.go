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
	NFS struct {
		Enabled bool   `mapstructure:"enabled"`
		Port    string `mapstructure:"port"`
		HMACKey string `mapstructure:"hmac_key"`
	} `mapstructure:"nfs"`
}

func Load() (*Config, error) {
	v := viper.New()

	// 设置默认值
	v.SetDefault("port", "8080")
	v.SetDefault("jwt_secret", "your-secret-key-change-in-production")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "data/netdisk.db")
	v.SetDefault("storage.path", "data/uploads")
	v.SetDefault("nfs.enabled", false)
	v.SetDefault("nfs.port", "2049")

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

	// 检查是否使用了默认的数据库驱动但配置了 postgres DSN
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
	}

	// 检查是否使用了默认的 JWT 密钥
	if cfg.JWTSecret == "your-secret-key-change-in-production" || cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET must be set and cannot use default value")
	}

	return &cfg, nil
}

// GenerateExampleConfig 生成示例配置文件
func GenerateExampleConfig(path string) error {
	content := `# NDisk 配置文件示例
# 复制此文件为 config.toml 并修改配置

# 服务端口
port = "8080"

# JWT 密钥
jwt_secret = "your-secret-key-change-in-production"

[database]
# 数据库驱动 (sqlite, postgres)
driver = "sqlite"
# SQLite 连接字符串
dsn = "data/netdisk.db"
# PostgreSQL 连接字符串示例（取消注释并修改后使用）：
# driver = "postgres"
# dsn = "host=127.0.0.1 user=ndisk password=mysecretpassword dbname=ndisk port=5432 sslmode=disable TimeZone=Asia/Shanghai"

[storage]
# 文件存储路径
path = "data/uploads"

[nfs]
# 是否启用 NFS 协议支持
enabled = false
# NFS 服务端口
port = "2049"
# NFS 句柄签名密钥（生产环境请更换为随机字符串）
hmac_key = "change-this-to-a-random-secret"
`
	return os.WriteFile(path, []byte(content), 0644)
}
