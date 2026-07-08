package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ballast/ballast-server/internal/runtime"
)

type recordedCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls []recordedCall
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, recordedCall{name: name, args: append([]string(nil), args...)})
	switch args[0] {
	case "run":
		return []byte("container-123\n"), nil, nil
	case "inspect":
		return []byte("true\n"), nil, nil
	case "rm":
		return nil, nil, nil
	default:
		return nil, nil, errors.New("unexpected command")
	}
}

func TestCreateBuildsHardenedContainerCommand(t *testing.T) {
	runner := &fakeRunner{}
	r := &DockerRuntime{
		config: Config{
			MaxCPUCores:     2,
			MaxMemoryMB:     512,
			DefaultImage:    "ballast-runner-base:test",
			ControlPlaneURL: "http://host.docker.internal:8080",
			InternalToken:   "internal-secret",
			RunnerEnv: map[string]string{
				"BALLAST_GIT_PUSH":            "1",
				"BALLAST_GIT_PR_URL_TEMPLATE": "https://gitlab.example/new?branch={branch}",
				"bad-name":                    "ignored",
			},
			DockerBinary: "docker",
		},
		runner: runner,
	}

	instance, err := r.Create(context.Background(), "sess-123", "", runtime.Mounts{})
	if err != nil {
		t.Fatal(err)
	}
	if instance.GetID() != "sess-123" {
		t.Fatalf("instance id = %q", instance.GetID())
	}
	command := strings.Join(runner.calls[0].args, " ")
	for _, required := range []string{
		"--read-only",
		"--cap-drop ALL",
		"--security-opt no-new-privileges",
		"BALLAST_SESSION_ID=sess-123",
		"BALLAST_INTERNAL_TOKEN=internal-secret",
		"BALLAST_CHILD=/usr/local/bin/mock-opencode",
		"ballast-runner-base:test",
		"BALLAST_GIT_PUSH=1",
		"BALLAST_GIT_PR_URL_TEMPLATE=https://gitlab.example/new?branch={branch}",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("docker run command missing %q: %s", required, command)
		}
	}
	if strings.Contains(command, "bad-name") {
		t.Fatalf("docker run command contains invalid env name: %s", command)
	}
}

func TestCreateMountsRewrittenKubeconfig(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "kubeconfig.yaml")
	if err := os.WriteFile(source, []byte("server: https://127.0.0.1:6445\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	r := &DockerRuntime{
		config: Config{
			MaxCPUCores:                2,
			MaxMemoryMB:                512,
			DefaultImage:               "ballast-runner-base:test",
			WorkspaceRoot:              root,
			ControlPlaneURL:            "http://host.docker.internal:8080",
			InternalToken:              "internal-secret",
			RunnerCommand:              "/usr/local/bin/ballast-real-k8s-runner",
			KubeconfigPath:             source,
			RewriteLocalhostKubeconfig: true,
			KubeNamespace:              "ballast-demo",
			KubeTargetSelector:         "app=crashloop-demo",
			KubeTargetDeployment:       "crashloop-demo",
			KubeFixConfigMap:           "crashloop-demo-config",
			DockerBinary:               "docker",
		},
		runner: runner,
	}

	if _, err := r.Create(context.Background(), "sess-kube", "", runtime.Mounts{}); err != nil {
		t.Fatal(err)
	}
	command := strings.Join(runner.calls[0].args, " ")
	for _, required := range []string{
		"BALLAST_CHILD=/usr/local/bin/ballast-real-k8s-runner",
		"KUBECONFIG=/workspace/.kube/config",
		"BALLAST_TARGET_NAMESPACE=ballast-demo",
		"dst=/workspace/.kube/config,readonly",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("docker run command missing %q: %s", required, command)
		}
	}
	rewritten := filepath.Join(root, "sess-kube", "runtime", "kubeconfig.yaml")
	content, err := os.ReadFile(rewritten)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "https://host.docker.internal:6445") {
		t.Fatalf("kubeconfig was not rewritten: %s", string(content))
	}
	if err := r.Destroy(context.Background(), "sess-kube"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rewritten); !os.IsNotExist(err) {
		t.Fatalf("runtime kubeconfig was not cleaned up: %v", err)
	}
}

func TestCreateMountsWorkspaceDir(t *testing.T) {
	workspaceDir := t.TempDir()
	runner := &fakeRunner{}
	r := &DockerRuntime{
		config: Config{
			MaxCPUCores:     2,
			MaxMemoryMB:     512,
			DefaultImage:    "ballast-runner-base:test",
			ControlPlaneURL: "http://host.docker.internal:8080",
			InternalToken:   "internal-secret",
			DockerBinary:    "docker",
		},
		runner: runner,
	}

	if _, err := r.Create(context.Background(), "sess-workspace", "", runtime.Mounts{WorkspaceDir: workspaceDir}); err != nil {
		t.Fatal(err)
	}
	command := strings.Join(runner.calls[0].args, " ")
	want := "src=" + workspaceDir + ",dst=/workspace/project"
	if !strings.Contains(command, want) {
		t.Fatalf("docker run command missing %q: %s", want, command)
	}
}

func TestCreateRejectsUnsafeSessionID(t *testing.T) {
	r := &DockerRuntime{config: Config{DefaultImage: "image"}, runner: &fakeRunner{}}
	if _, err := r.Create(context.Background(), "../escape", "", runtime.Mounts{}); err == nil {
		t.Fatal("expected invalid session id error")
	}
}
