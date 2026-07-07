package docker

import (
	"context"
	"errors"
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
			DockerBinary:    "docker",
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
		"ballast-runner-base:test",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("docker run command missing %q: %s", required, command)
		}
	}
}

func TestCreateRejectsUnsafeSessionID(t *testing.T) {
	r := &DockerRuntime{config: Config{DefaultImage: "image"}, runner: &fakeRunner{}}
	if _, err := r.Create(context.Background(), "../escape", "", runtime.Mounts{}); err == nil {
		t.Fatal("expected invalid session id error")
	}
}
