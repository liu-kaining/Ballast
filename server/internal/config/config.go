package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 对应 /etc/ballast/ballast.yaml 的全局配置。
type Config struct {
	Server            ServerConfig            `yaml:"server"`
	RuntimeProvider   RuntimeProviderConfig   `yaml:"runtime_provider"`
	CredentialCenter  CredentialCenterConfig  `yaml:"credential_center"`
	ModelRouter       ModelRouterConfig        `yaml:"model_router"`
	Database          DatabaseConfig          `yaml:"database"`
	Policy            PolicyConfig            `yaml:"policy"`
}

type ServerConfig struct {
	Address     string `yaml:"address"`
	Environment string `yaml:"environment"`
	JWTSecret   string `yaml:"jwt_secret"`
}

type RuntimeProviderConfig struct {
	Type   string            `yaml:"type"`
	Config map[string]any    `yaml:"config"`
}

type CredentialCenterConfig struct {
	Provider    string `yaml:"provider"`
	Endpoint    string `yaml:"endpoint"`
	AuthTokenEnv string `yaml:"auth_token_env"`
}

type ModelRouterConfig struct {
	DefaultProvider string                       `yaml:"default_provider"`
	Providers       map[string]ModelProviderConfig `yaml:"providers"`
}

type ModelProviderConfig struct {
	APIBase     string `yaml:"api_base"`
	APIKeyEnv   string `yaml:"api_key_env"`
	DefaultModel string `yaml:"default_model"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type PolicyConfig struct {
	RegoDir string `yaml:"rego_dir"`
}

// Load 从给定路径读取并解析 ballast.yaml。
// ${ENV_VAR} 形式的占位符会被环境变量替换。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	expanded := expandEnv(string(raw))
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Server.Address == "" {
		return fmt.Errorf("server.address is required")
	}
	if c.RuntimeProvider.Type == "" {
		return fmt.Errorf("runtime_provider.type is required")
	}
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}
	return nil
}

// expandEnv 替换 ${VAR} 占位符。未设置的变量替换为空串。
func expandEnv(s string) string {
	return os.Expand(s, func(key string) string {
		// os.Expand 默认处理 $VAR 与 ${VAR}；这里统一处理两种形式。
		return os.Getenv(key)
	})
}

// EnvOrPlaceholder 供配置值引用环境变量时使用的小工具。
func EnvOrPlaceholder(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
