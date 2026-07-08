// Package docker implements runtime.SandboxRuntime with the Docker CLI.
package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ballast/ballast-server/internal/runtime"
)

// Config controls Docker sandbox resource and control-plane settings.
type Config struct {
	MaxCPUCores                int
	MaxMemoryMB                int
	DefaultImage               string
	WorkspaceRoot              string
	ControlPlaneURL            string
	InternalToken              string
	RunnerCommand              string
	RunnerArgs                 string
	RunnerEnv                  map[string]string
	KubeconfigPath             string
	RewriteLocalhostKubeconfig bool
	KubeNamespace              string
	KubeTargetSelector         string
	KubeTargetDeployment       string
	KubeFixConfigMap           string
	DockerBinary               string
}

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// DockerRuntime uses a Docker daemon reachable through the configured CLI.
type DockerRuntime struct {
	config Config
	runner commandRunner
}

// New validates configuration and verifies that the Docker daemon is reachable.
func New(ctx context.Context, cfg Config) (*DockerRuntime, error) {
	if cfg.DockerBinary == "" {
		cfg.DockerBinary = "docker"
	}
	if cfg.MaxCPUCores <= 0 {
		cfg.MaxCPUCores = 2
	}
	if cfg.MaxMemoryMB <= 0 {
		cfg.MaxMemoryMB = 2048
	}
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = filepath.Join(os.TempDir(), "ballast", "sandboxes")
	}
	if cfg.RunnerCommand == "" {
		cfg.RunnerCommand = "/usr/local/bin/mock-opencode"
	}
	if cfg.DefaultImage == "" {
		return nil, errors.New("docker default image is required")
	}
	if cfg.InternalToken == "" {
		return nil, errors.New("docker internal token is required")
	}
	parsed, err := url.Parse(cfg.ControlPlaneURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid control plane URL %q", cfg.ControlPlaneURL)
	}

	r := &DockerRuntime{config: cfg, runner: execRunner{}}
	if _, stderr, err := r.runner.Run(ctx, cfg.DockerBinary, "version", "--format", "{{.Server.Version}}"); err != nil {
		return nil, fmt.Errorf("docker daemon unavailable: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return r, nil
}

var safeSessionID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// Create starts a hardened, disposable sandbox container.
func (r *DockerRuntime) Create(ctx context.Context, sessionID, imageName string, vol runtime.Mounts) (runtime.SandboxInstance, error) {
	if !safeSessionID.MatchString(sessionID) {
		return nil, fmt.Errorf("invalid session id %q", sessionID)
	}
	if imageName == "" {
		imageName = r.config.DefaultImage
	}
	runnerCommand := r.runnerCommand()

	name := containerName(sessionID)
	controlURL := strings.TrimRight(r.config.ControlPlaneURL, "/")
	args := []string{
		"run", "-d",
		"--name", name,
		"--label", "ballast.session_id=" + sessionID,
		"--cpus", strconv.Itoa(r.config.MaxCPUCores),
		"--memory", strconv.Itoa(r.config.MaxMemoryMB) + "m",
		"--pids-limit", "256",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--add-host", "host.docker.internal:host-gateway",
		"-e", "BALLAST_SESSION_ID=" + sessionID,
		"-e", "BALLAST_REPORT_URL=" + controlURL + "/api/internal/harness/report",
		"-e", "BALLAST_EVENT_URL=" + controlURL + "/api/internal/harness/event",
		"-e", "BALLAST_INTERNAL_TOKEN=" + r.config.InternalToken,
		"-e", "BALLAST_CHILD=" + runnerCommand,
		"-e", "BALLAST_CHILD_ARGS=" + r.config.RunnerArgs,
		"-e", "BALLAST_AGENT_NAME=" + filepath.Base(runnerCommand),
	}
	args = append(args, r.realK8sEnv()...)
	args = append(args, r.runnerEnv()...)

	mountArgs, err := buildMountArgs(vol)
	if err != nil {
		return nil, err
	}
	kubeMountArgs, err := r.kubeconfigMountArgs(sessionID)
	if err != nil {
		r.cleanupRuntimeFiles(sessionID)
		return nil, err
	}
	args = append(args, mountArgs...)
	args = append(args, kubeMountArgs...)
	args = append(args, imageName)

	stdout, stderr, err := r.runner.Run(ctx, r.config.DockerBinary, args...)
	if err != nil {
		return nil, fmt.Errorf("docker run: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	containerID := strings.TrimSpace(string(stdout))
	if containerID == "" {
		return nil, errors.New("docker run returned an empty container id")
	}

	running, inspectErr, err := r.runner.Run(ctx, r.config.DockerBinary, "inspect", "-f", "{{.State.Running}}", containerID)
	if err != nil || strings.TrimSpace(string(running)) != "true" {
		logs, _, _ := r.runner.Run(ctx, r.config.DockerBinary, "logs", containerID)
		_, _, _ = r.runner.Run(ctx, r.config.DockerBinary, "rm", "-f", "-v", containerID)
		r.cleanupRuntimeFiles(sessionID)
		return nil, fmt.Errorf("sandbox failed to stay running: %s %s", strings.TrimSpace(string(inspectErr)), strings.TrimSpace(string(logs)))
	}

	return &dockerInstance{
		runtime:   r,
		id:        containerID,
		sessionID: sessionID,
	}, nil
}

func (r *DockerRuntime) realK8sEnv() []string {
	var out []string
	add := func(key, value string) {
		if value != "" {
			out = append(out, "-e", key+"="+value)
		}
	}
	if r.config.KubeconfigPath != "" {
		add("KUBECONFIG", "/workspace/.kube/config")
	}
	add("BALLAST_TARGET_NAMESPACE", r.config.KubeNamespace)
	add("BALLAST_TARGET_SELECTOR", r.config.KubeTargetSelector)
	add("BALLAST_TARGET_DEPLOYMENT", r.config.KubeTargetDeployment)
	add("BALLAST_FIX_CONFIGMAP", r.config.KubeFixConfigMap)
	return out
}

var safeEnvName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func (r *DockerRuntime) runnerEnv() []string {
	if len(r.config.RunnerEnv) == 0 {
		return nil
	}
	keys := make([]string, 0, len(r.config.RunnerEnv))
	for key := range r.config.RunnerEnv {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		if !safeEnvName.MatchString(trimmedKey) {
			continue
		}
		value := strings.TrimSpace(r.config.RunnerEnv[key])
		if value == "" {
			continue
		}
		out = append(out, "-e", trimmedKey+"="+value)
	}
	return out
}

func (r *DockerRuntime) runnerCommand() string {
	if r.config.RunnerCommand != "" {
		return r.config.RunnerCommand
	}
	return "/usr/local/bin/mock-opencode"
}

func (r *DockerRuntime) kubeconfigMountArgs(sessionID string) ([]string, error) {
	if strings.TrimSpace(r.config.KubeconfigPath) == "" {
		return nil, nil
	}
	source := r.config.KubeconfigPath
	if !filepath.IsAbs(source) {
		return nil, fmt.Errorf("kubeconfig path must be absolute: %s", source)
	}
	if _, err := os.Stat(source); err != nil {
		return nil, fmt.Errorf("kubeconfig %s is not readable: %w", source, err)
	}
	if r.config.RewriteLocalhostKubeconfig {
		rewritten, err := r.rewriteKubeconfigForSandbox(sessionID, source)
		if err != nil {
			return nil, err
		}
		source = rewritten
	}
	return []string{
		"--mount",
		"type=bind,src=" + source + ",dst=/workspace/.kube/config,readonly",
	}, nil
}

func (r *DockerRuntime) rewriteKubeconfigForSandbox(sessionID, source string) (string, error) {
	raw, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read kubeconfig: %w", err)
	}
	content := string(raw)
	replacements := []struct {
		from string
		to   string
	}{
		{"https://127.0.0.1:", "https://host.docker.internal:"},
		{"https://localhost:", "https://host.docker.internal:"},
		{"http://127.0.0.1:", "http://host.docker.internal:"},
		{"http://localhost:", "http://host.docker.internal:"},
	}
	for _, replacement := range replacements {
		content = strings.ReplaceAll(content, replacement.from, replacement.to)
	}
	root := filepath.Join(r.workspaceRoot(), sessionID, "runtime")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create runtime workspace: %w", err)
	}
	target := filepath.Join(root, "kubeconfig.yaml")
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write sandbox kubeconfig: %w", err)
	}
	return target, nil
}

func (r *DockerRuntime) workspaceRoot() string {
	if r.config.WorkspaceRoot != "" {
		return r.config.WorkspaceRoot
	}
	return filepath.Join(os.TempDir(), "ballast", "sandboxes")
}

func (r *DockerRuntime) cleanupRuntimeFiles(sessionID string) {
	sessionRoot := filepath.Join(r.workspaceRoot(), sessionID)
	_ = os.RemoveAll(filepath.Join(sessionRoot, "runtime"))
	_ = os.Remove(sessionRoot)
}

func buildMountArgs(vol runtime.Mounts) ([]string, error) {
	var out []string
	add := func(source, destination string, readOnly bool) error {
		if !filepath.IsAbs(source) || !filepath.IsAbs(destination) {
			return fmt.Errorf("mount paths must be absolute: %q -> %q", source, destination)
		}
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("mount source %s: %w", source, err)
		}
		spec := "type=bind,src=" + source + ",dst=" + destination
		if readOnly {
			spec += ",readonly"
		}
		out = append(out, "--mount", spec)
		return nil
	}
	if vol.SkillsDir != "" {
		if err := add(vol.SkillsDir, "/workspace/.opencode/skills", true); err != nil {
			return nil, err
		}
	}
	if vol.WorkspaceDir != "" {
		if err := add(vol.WorkspaceDir, "/workspace/project", false); err != nil {
			return nil, err
		}
	}
	for _, extra := range vol.Extra {
		if err := add(extra.Source, extra.Destination, extra.ReadOnly); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// InjectJITCredential is deliberately unsupported until a real credential
// provider is connected; silently pretending to inject a credential is unsafe.
func (r *DockerRuntime) InjectJITCredential(context.Context, string, string) error {
	return errors.New("JIT credential injection is not configured")
}

// Destroy force-removes a sandbox and its anonymous volumes.
func (r *DockerRuntime) Destroy(ctx context.Context, sessionID string) error {
	if !safeSessionID.MatchString(sessionID) {
		return fmt.Errorf("invalid session id %q", sessionID)
	}
	_, stderr, err := r.runner.Run(ctx, r.config.DockerBinary, "rm", "-f", "-v", containerName(sessionID))
	if err != nil && !strings.Contains(string(stderr), "No such container") {
		return fmt.Errorf("docker rm: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	r.cleanupRuntimeFiles(sessionID)
	return nil
}

type dockerInstance struct {
	runtime   *DockerRuntime
	id        string
	sessionID string
}

func (d *dockerInstance) GetID() string { return d.sessionID }

func (d *dockerInstance) GetIP() string {
	stdout, _, err := d.runtime.runner.Run(
		context.Background(),
		d.runtime.config.DockerBinary,
		"inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", d.id,
	)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(stdout))
}

func (d *dockerInstance) ExecuteCommand(ctx context.Context, cmd []string) ([]byte, []byte, error) {
	if len(cmd) == 0 {
		return nil, nil, errors.New("command is empty")
	}
	args := append([]string{"exec", d.id}, cmd...)
	stdout, stderr, err := d.runtime.runner.Run(ctx, d.runtime.config.DockerBinary, args...)
	if err != nil {
		return stdout, stderr, fmt.Errorf("docker exec: %w", err)
	}
	return stdout, stderr, nil
}

func containerName(sessionID string) string {
	return "ballast-sbx-" + sessionID
}

var _ runtime.SandboxRuntime = (*DockerRuntime)(nil)
var _ runtime.SandboxInstance = (*dockerInstance)(nil)
