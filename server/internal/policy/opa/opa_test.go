package opa

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ballast/ballast-server/internal/policy"
)

const sampleRego = `package ballast.security

default decision = "SUSPEND"

decision = "APPROVE" {
	not input.unsafe
	input.command == "kubectl"
	input.args[0] == "get"
}

decision = "DENY" {
	input.unsafe
}

decision = "DENY" {
	input.command == "rm"
	input.args[0] == "-rf"
	input.args[1] == "/"
}
`

func writeTempRego(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "k8s.rego"), []byte(sampleRego), 0o644); err != nil {
		t.Fatalf("write rego: %v", err)
	}
	return dir
}

func TestEngine_WhitelistApproved(t *testing.T) {
	e, err := New(writeTempRego(t))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	d, err := e.EvaluateCommand(context.Background(), policy.CommandContext{
		Command: "kubectl",
		Args:    []string{"get", "pods", "-n", "prod"},
	})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if d != policy.Approve {
		t.Fatalf("expected APPROVE, got %s", d)
	}
}

func TestEngine_BlacklistDenied(t *testing.T) {
	e, err := New(writeTempRego(t))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	d, err := e.EvaluateCommand(context.Background(), policy.CommandContext{
		Command: "rm",
		Args:    []string{"-rf", "/"},
	})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if d != policy.Deny {
		t.Fatalf("expected DENY, got %s", d)
	}
}

func TestEngine_GraylistSuspended(t *testing.T) {
	e, err := New(writeTempRego(t))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	d, err := e.EvaluateCommand(context.Background(), policy.CommandContext{
		Command: "kubectl",
		Args:    []string{"apply", "-f", "fixed.yaml"},
	})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if d != policy.Suspend {
		t.Fatalf("expected SUSPEND, got %s", d)
	}
}

func TestEngine_UnknownCommandSuspended(t *testing.T) {
	e, err := New(writeTempRego(t))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	d, err := e.EvaluateCommand(context.Background(), policy.CommandContext{Command: "custom-deployer"})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if d != policy.Suspend {
		t.Fatalf("expected SUSPEND, got %s", d)
	}
}

func TestEngine_UnsafeCompositionDenied(t *testing.T) {
	e, err := New(writeTempRego(t))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	d, err := e.EvaluateCommand(context.Background(), policy.CommandContext{
		Command: "kubectl",
		Args:    []string{"get", "pods", "&&", "rm", "-rf", "/"},
		Unsafe:  true,
	})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if d != policy.Deny {
		t.Fatalf("expected DENY, got %s", d)
	}
}
