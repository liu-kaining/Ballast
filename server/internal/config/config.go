package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 对应 /etc/ballast/ballast.yaml 的全局配置。
type Config struct {
	Server           ServerConfig           `yaml:"server"`
	RuntimeProvider  RuntimeProviderConfig  `yaml:"runtime_provider"`
	CredentialCenter CredentialCenterConfig `yaml:"credential_center"`
	Notifications    NotificationConfig     `yaml:"notifications"`
	ModelRouter      ModelRouterConfig      `yaml:"model_router"`
	Database         DatabaseConfig         `yaml:"database"`
	Policy           PolicyConfig           `yaml:"policy"`
}

type ServerConfig struct {
	Address            string   `yaml:"address"`
	Environment        string   `yaml:"environment"`
	JWTSecret          string   `yaml:"jwt_secret"`
	AdminToken         string   `yaml:"admin_token"`
	InternalToken      string   `yaml:"internal_token"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
	CookieSecure       bool     `yaml:"cookie_secure"`
}

type RuntimeProviderConfig struct {
	Type   string        `yaml:"type"`
	Config RuntimeConfig `yaml:"config"`
}

type RuntimeConfig struct {
	MaxCPUCores                int    `yaml:"max_cpu_cores"`
	MaxMemoryMB                int    `yaml:"max_memory_mb"`
	DefaultImage               string `yaml:"default_image"`
	WorkspaceRoot              string `yaml:"workspace_root"`
	ControlPlaneURL            string `yaml:"control_plane_url"`
	RunnerCommand              string `yaml:"runner_command"`
	RunnerArgs                 string `yaml:"runner_args"`
	KubeconfigPath             string `yaml:"kubeconfig_path"`
	RewriteLocalhostKubeconfig bool   `yaml:"rewrite_localhost_kubeconfig"`
	KubeNamespace              string `yaml:"kube_namespace"`
	KubeTargetSelector         string `yaml:"kube_target_selector"`
	KubeTargetDeployment       string `yaml:"kube_target_deployment"`
	KubeFixConfigMap           string `yaml:"kube_fix_configmap"`
}

type CredentialCenterConfig struct {
	Provider     string `yaml:"provider"`
	Endpoint     string `yaml:"endpoint"`
	AuthTokenEnv string `yaml:"auth_token_env"`
}

type NotificationConfig struct {
	ApprovalWebhookURL  string `yaml:"approval_webhook_url"`
	ApprovalWebhookKind string `yaml:"approval_webhook_kind"`
	ConsoleBaseURL      string `yaml:"console_base_url"`
}

type ModelRouterConfig struct {
	DefaultProvider string                         `yaml:"default_provider"`
	Providers       map[string]ModelProviderConfig `yaml:"providers"`
}

type ModelProviderConfig struct {
	APIBase      string `yaml:"api_base"`
	APIKeyEnv    string `yaml:"api_key_env"`
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
	if c.Server.JWTSecret == "" {
		return fmt.Errorf("server.jwt_secret is required")
	}
	if c.Server.AdminToken == "" {
		return fmt.Errorf("server.admin_token is required")
	}
	if c.Server.InternalToken == "" {
		return fmt.Errorf("server.internal_token is required")
	}
	if c.RuntimeProvider.Type != "docker" {
		return fmt.Errorf("runtime_provider.type %q is not supported", c.RuntimeProvider.Type)
	}
	if c.RuntimeProvider.Config.DefaultImage == "" {
		return fmt.Errorf("runtime_provider.config.default_image is required")
	}
	if c.RuntimeProvider.Config.ControlPlaneURL == "" {
		return fmt.Errorf("runtime_provider.config.control_plane_url is required")
	}
	if c.Server.Environment == "production" {
		if !c.Server.CookieSecure {
			return fmt.Errorf("server.cookie_secure must be true in production")
		}
		for name, value := range map[string]string{
			"server.jwt_secret":     c.Server.JWTSecret,
			"server.admin_token":    c.Server.AdminToken,
			"server.internal_token": c.Server.InternalToken,
		} {
			if strings.Contains(strings.ToLower(value), "dev") || len(value) < 24 {
				return fmt.Errorf("%s must be a non-development secret of at least 24 characters in production", name)
			}
		}
	}
	return nil
}

var bracedEnvPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// expandEnv 替换 $VAR、${VAR} 和 ${VAR:-fallback}。
// 带 :- 的表达式在变量未设置或为空时使用 fallback。
func expandEnv(s string) string {
	withDefaults := bracedEnvPattern.ReplaceAllStringFunc(s, func(expr string) string {
		match := bracedEnvPattern.FindStringSubmatch(expr)
		if value := strings.TrimSpace(os.Getenv(match[1])); value != "" {
			return value
		}
		if match[2] != "" {
			return match[3]
		}
		return ""
	})
	return os.ExpandEnv(withDefaults)
}

// EnvOrPlaceholder 供配置值引用环境变量时使用的小工具。
func EnvOrPlaceholder(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
