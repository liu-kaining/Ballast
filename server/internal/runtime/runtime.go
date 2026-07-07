// Package runtime 定义沙箱运行时的 SPI（Service Provider Interface）。
// 接口契约对齐 spec/INIT.md 第 200-218 行。
//
// 默认实现为 DockerRuntime（docker 子包）。企业二开可基于 E2B MicroVM
// 实现 SandboxRuntime，上层控制调度逻辑无需改动。
package runtime

import "context"

// SandboxInstance 表示一个已拉起的隔离沙箱实例。
type SandboxInstance interface {
	// GetID 返回沙箱唯一标识（与 sessionID 一致）。
	GetID() string
	// GetIP 返回沙箱在控制面可达的网络地址（容器 IP 或 VM 地址）。
	GetIP() string
	// ExecuteCommand 在沙箱内执行一条命令，返回 stdout/stderr。
	ExecuteCommand(ctx context.Context, cmd []string) (stdout []byte, stderr []byte, err error)
}

// Mounts 描述挂载到沙箱的卷。
type Mounts struct {
	// SkillsDir 宿主机上 Skill 集合目录，挂载到沙箱 /workspace/.opencode/skills/。
	SkillsDir string
	// WorkspaceDir 可选：宿主机工作目录挂载点。
	WorkspaceDir string
	// Extra 额外挂载（宿主机路径 -> 容器路径，只读标记）。
	Extra []ExtraMount
}

type ExtraMount struct {
	Source      string
	Destination string
	ReadOnly    bool
}

// SandboxRuntime 是沙箱运行时的抽象。实现方须保证：
//   - Create 拉起一个完全隔离的环境
//   - InjectJITCredential 注入生命周期极短的临时凭证；未配置真实凭证提供方时必须拒绝
//   - Destroy 强行物理销毁沙箱，抹除所有痕迹
type SandboxRuntime interface {
	Create(ctx context.Context, sessionID string, imageName string, volume Mounts) (SandboxInstance, error)
	InjectJITCredential(ctx context.Context, sessionID string, credsSecretID string) error
	Destroy(ctx context.Context, sessionID string) error
}
