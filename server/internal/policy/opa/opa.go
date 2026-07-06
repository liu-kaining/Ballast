// Package opa 用 Open Policy Agent / Rego 实现 policy.PolicyEngine。
// Rego 规则从 policies/*.rego 热加载，无需重启。
package opa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"

	"github.com/ballast/ballast-server/internal/policy"
)

// Engine 是基于 OPA/Rego 的 PolicyEngine 实现。
type Engine struct {
	mu       sync.RWMutex
	modules  map[string]*ast.Module
	compiler *ast.Compiler
	pkg      string // Rego package，默认 ballast.security
}

// Option Engine 配置项。
type Option func(*Engine)

// WithPackage 指定 Rego package 路径（默认 ballast.security）。
func WithPackage(pkg string) Option {
	return func(e *Engine) { e.pkg = pkg }
}

// New 从 regoDir 加载所有 .rego 文件构造 Engine。
func New(regoDir string, opts ...Option) (*Engine, error) {
	e := &Engine{modules: map[string]*ast.Module{}, pkg: "ballast.security"}
	for _, o := range opts {
		o(e)
	}
	if err := e.loadDir(regoDir); err != nil {
		return nil, err
	}
	return e, nil
}

// Reload 重新加载目录，用于热更新。
func (e *Engine) Reload(regoDir string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.loadDir(regoDir)
}

func (e *Engine) loadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read rego dir %s: %w", dir, err)
	}
	modules := map[string]*ast.Module{}
	for _, f := range entries {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".rego") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return fmt.Errorf("read %s: %w", f.Name(), err)
		}
		m, err := ast.ParseModule(f.Name(), string(raw))
		if err != nil {
			return fmt.Errorf("parse %s: %w", f.Name(), err)
		}
		modules[f.Name()] = m
	}
	e.modules = modules
	e.compiler = ast.NewCompiler()
	if e.compiler.Compile(modules); e.compiler.Errors != nil {
		return fmt.Errorf("compile rego: %v", e.compiler.Errors)
	}
	return nil
}

// EvaluateCommand 实现 policy.PolicyEngine。
// 求值顺序：先看 action（DENY 优先于 SUSPEND 优先于 allow），
// 再回退 default allow/action。
func (e *Engine) EvaluateCommand(ctx context.Context, cmdCtx policy.CommandContext) (policy.Decision, error) {
	e.mu.RLock()
	compiler := e.compiler
	pkg := e.pkg
	e.mu.RUnlock()

	input := map[string]any{
		"session_id": cmdCtx.SessionID,
		"user":       cmdCtx.User,
		"agent_name": cmdCtx.AgentName,
		"command":    cmdCtx.Command,
		"args":       cmdCtx.Args,
		"env":        cmdCtx.Env,
	}

	// 先查 action（DENY / SUSPEND）
	actionQuery := pkg + ".action"
	res, err := rego.New(
		rego.Query(actionQuery),
		rego.Compiler(compiler),
		rego.Input(input),
	).Eval(ctx)
	if err != nil {
		return policy.Deny, fmt.Errorf("eval action: %w", err)
	}
	if d, ok := extractString(res); ok {
		switch policy.Decision(d) {
		case policy.Deny:
			return policy.Deny, nil
		case policy.Suspend:
			return policy.Suspend, nil
		case policy.Approve:
			return policy.Approve, nil
		}
	}

	// 再查 allow
	allowQuery := pkg + ".allow"
	res, err = rego.New(
		rego.Query(allowQuery),
		rego.Compiler(compiler),
		rego.Input(input),
	).Eval(ctx, rego.EvalRuleRootsOnly(true))
	if err != nil {
		return policy.Deny, fmt.Errorf("eval allow: %w", err)
	}
	if allowed, ok := extractBool(res); ok && allowed {
		return policy.Approve, nil
	}
	// 默认按 spec 的 default action=SUSPEND 处理
	return policy.Suspend, nil
}

func extractString(res rego.ResultSet) (string, bool) {
	if len(res) == 0 || len(res[0].Expressions) == 0 {
		return "", false
	}
	v := res[0].Expressions[0].Value
	s, ok := v.(string)
	return s, ok
}

func extractBool(res rego.ResultSet) (bool, bool) {
	if len(res) == 0 || len(res[0].Expressions) == 0 {
		return false, false
	}
	v := res[0].Expressions[0].Value
	b, ok := v.(bool)
	return b, ok
}

var _ policy.PolicyEngine = (*Engine)(nil)
