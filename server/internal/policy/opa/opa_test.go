package opa

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ballast/ballast-server/internal/policy"
)

const sampleRego = `package ballast.security

default allow = false
default action = "SUSPEND"

allow {
	is_safe_command(input.command)
}

action = "DENY" {
	blacklist_commands[_] == input.command
}

action = "SUSPEND" {
	not is_safe_command(input.command)
	graylist_commands[_] == input.command
}

is_safe_command(cmd) {
	safe_commands[_] == cmd
}

safe_commands = [
	"kubectl get", "kubectl logs", "kubectl describe", "kubectl top",
	"git status", "git diff", "git log",
	"terraform plan", "terraform validate",
	"ls", "cat", "grep", "awk"
]

blacklist_commands = [
	"rm -rf /", "mkfs", "fdisk", "dd",
	"kubectl delete namespace", "kubectl delete clusterrolebinding",
	"shutdown", "reboot"
]

graylist_commands = [
	"kubectl apply", "kubectl delete", "kubectl patch", "kubectl edit",
	"terraform apply", "terraform destroy",
	"git push", "helm upgrade", "helm uninstall"
]
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
	d, err := e.EvaluateCommand(context.Background(), policy.CommandContext{Command: "kubectl get"})
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
	d, err := e.EvaluateCommand(context.Background(), policy.CommandContext{Command: "rm -rf /"})
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
	d, err := e.EvaluateCommand(context.Background(), policy.CommandContext{Command: "kubectl apply"})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if d != policy.Suspend {
		t.Fatalf("expected SUSPEND, got %s", d)
	}
}
