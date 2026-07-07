package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandEnvSupportsDefaults(t *testing.T) {
	t.Setenv("BALLAST_SET", "configured")
	t.Setenv("BALLAST_EMPTY", "")

	got := expandEnv("${BALLAST_SET:-fallback}|${BALLAST_MISSING:-fallback}|${BALLAST_EMPTY:-fallback}|$BALLAST_SET")
	want := "configured|fallback|fallback|configured"
	if got != want {
		t.Fatalf("expandEnv() = %q, want %q", got, want)
	}
}

func TestLoadAppliesEnvironmentOverrides(t *testing.T) {
	t.Setenv("DATABASE_DSN", "postgres://db/override")
	t.Setenv("BALLAST_POLICY_DIR", "/tmp/policies")
	t.Setenv("BALLAST_CONTROL_PLANE_URL", "http://control:8080")
	t.Setenv("BALLAST_RUNNER_COMMAND", "/usr/local/bin/ballast-real-k8s-runner")
	t.Setenv("BALLAST_KUBECONFIG", "/tmp/ballast/real-k8s/kubeconfig.sandbox.yaml")

	raw, err := os.ReadFile(filepath.Join("..", "..", "configs", "ballast.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ballast.yaml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.DSN != "postgres://db/override" {
		t.Fatalf("database DSN = %q", cfg.Database.DSN)
	}
	if cfg.Policy.RegoDir != "/tmp/policies" {
		t.Fatalf("rego dir = %q", cfg.Policy.RegoDir)
	}
	if cfg.RuntimeProvider.Config.ControlPlaneURL != "http://control:8080" {
		t.Fatalf("control plane URL = %q", cfg.RuntimeProvider.Config.ControlPlaneURL)
	}
	if cfg.RuntimeProvider.Config.RunnerCommand != "/usr/local/bin/ballast-real-k8s-runner" {
		t.Fatalf("runner command = %q", cfg.RuntimeProvider.Config.RunnerCommand)
	}
	if cfg.RuntimeProvider.Config.KubeconfigPath != "/tmp/ballast/real-k8s/kubeconfig.sandbox.yaml" {
		t.Fatalf("kubeconfig path = %q", cfg.RuntimeProvider.Config.KubeconfigPath)
	}
	if !cfg.RuntimeProvider.Config.RewriteLocalhostKubeconfig {
		t.Fatal("rewrite localhost kubeconfig should default to true")
	}
}

func TestProductionRejectsDevelopmentSecrets(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Address:       ":8080",
			Environment:   "production",
			JWTSecret:     "ballast-dev-secret-change-me",
			AdminToken:    "ballast-dev-admin-token",
			InternalToken: "ballast-dev-internal-token",
			CookieSecure:  true,
		},
		RuntimeProvider: RuntimeProviderConfig{
			Type: "docker",
			Config: RuntimeConfig{
				DefaultImage:    "sandbox",
				ControlPlaneURL: "https://control.example.com",
			},
		},
		Database: DatabaseConfig{DSN: "postgres://db"},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("production config accepted development secrets")
	}
}
