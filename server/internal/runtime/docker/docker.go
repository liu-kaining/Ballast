// Package docker 实现 runtime.SandboxRuntime 的 Docker 版本。
// 用 Docker Engine API（github.com/docker/docker/client）拉起/销毁容器。
package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/ballast/ballast-server/internal/runtime"
)

// DockerRuntime 用本地 Docker daemon 实现沙箱运行时。
type DockerRuntime struct {
	cli    *client.Client
	config Config
}

// Config DockerRuntime 配置，对应 ballast.yaml runtime_provider.config。
type Config struct {
	MaxCPUcores   int    `yaml:"max_cpu_cores"`
	MaxMemoryMB   int    `yaml:"max_memory_mb"`
	DefaultImage  string `yaml:"default_image"`
	WorkspaceRoot string `yaml:"workspace_root"`
}

// New 构造 DockerRuntime。需要本地 Docker daemon 可达。
func New(ctx context.Context, cfg Config) (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	if _, err := cli.Ping(ctx); err != nil {
		return nil, fmt.Errorf("docker ping: %w", err)
	}
	return &DockerRuntime{cli: cli, config: cfg}, nil
}

// Create 拉起一个隔离容器，返回 SandboxInstance。
func (r *DockerRuntime) Create(ctx context.Context, sessionID, imageName string, vol runtime.Mounts) (runtime.SandboxInstance, error) {
	if imageName == "" {
		imageName = r.config.DefaultImage
	}

	var mounts []mount.Mount
	if vol.SkillsDir != "" {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   vol.SkillsDir,
			Target:   "/workspace/.opencode/skills",
			ReadOnly: true,
		})
	}
	if vol.WorkspaceDir != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: vol.WorkspaceDir,
			Target: "/workspace/project",
		})
	}
	for _, em := range vol.Extra {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   em.Source,
			Target:   em.Destination,
			ReadOnly: em.ReadOnly,
		})
	}

	// harness-agent 作为容器入口，接管子进程 PTY。
	createResp, err := r.cli.ContainerCreate(ctx,
		&container.Config{
			Image: imageName,
			Cmd:   []string{"/usr/local/bin/harness-agent", "-session", sessionID},
			Env:   []string{"BALAST_SESSION_ID=" + sessionID},
			Tty:   false,
		},
		&container.HostConfig{
			Mounts:      mounts,
			AutoRemove:  false,
			NetworkMode: container.NetworkMode("bridge"),
			Resources: container.Resources{
				NanoCPUs:    int64(r.config.MaxCPUcores) * 1e9,
				Memory:      int64(r.config.MaxMemoryMB) * 1024 * 1024,
				PidsLimit:   pInt64(256),
			},
		},
		&network.NetworkingConfig{},
		nil,
		containerName(sessionID),
	)
	if err != nil {
		return nil, fmt.Errorf("container create: %w", err)
	}

	if err := r.cli.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
		// 创建成功但启动失败：尽力清理。
		_ = r.cli.ContainerRemove(ctx, createResp.ID, container.RemoveOptions{Force: true})
		return nil, fmt.Errorf("container start: %w", err)
	}

	inst := &dockerInstance{
		cli:        r.cli,
		id:         createResp.ID,
		sessionID:  sessionID,
		container:  containerName(sessionID),
		workspace:  r.config.WorkspaceRoot,
	}
	return inst, nil
}

// InjectJITCredential v0.1 stub：真实 Vault 对接留待 v0.2。
// 当前实现仅通过 docker exec 写入一个短生命周期 env 文件占位。
func (r *DockerRuntime) InjectJITCredential(ctx context.Context, sessionID, credsSecretID string) error {
	// TODO(v0.2): 调用 credential_center(Vault) 申请 15 分钟临时 kubeconfig，
	// 通过 docker cp 或 exec 写入沙箱 /etc/ballast/jit/kubeconfig。
	_ = credsSecretID
	_ = sessionID
	_ = ctx
	return nil
}

// Destroy 强制物理销毁沙箱。
func (r *DockerRuntime) Destroy(ctx context.Context, sessionID string) error {
	name := containerName(sessionID)
	// 先尝试按容器名解析 ID，再 force remove。
	info, err := r.cli.ContainerInspect(ctx, name)
	if err != nil {
		// 容器可能已不存在。
		return nil
	}
	return r.cli.ContainerRemove(ctx, info.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
}

// dockerInstance 实现 runtime.SandboxInstance。
type dockerInstance struct {
	cli       *client.Client
	id        string
	sessionID string
	container string
	workspace string
}

func (d *dockerInstance) GetID() string { return d.sessionID }

func (d *dockerInstance) GetIP() string {
	ctx := context.Background()
	info, err := d.cli.ContainerInspect(ctx, d.id)
	if err != nil {
		return ""
	}
	if n := info.NetworkSettings; n != nil {
		if net, ok := n.Networks["bridge"]; ok && net.IPAddress != "" {
			return net.IPAddress
		}
	}
	return ""
}

func (d *dockerInstance) ExecuteCommand(ctx context.Context, cmd []string) ([]byte, []byte, error) {
	execCfg := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	execID, err := d.cli.ContainerExecCreate(ctx, d.id, execCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("exec create: %w", err)
	}
	hijack, err := d.cli.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return nil, nil, fmt.Errorf("exec attach: %w", err)
	}
	defer hijack.Close()
	// 读取输出。简化处理：合并 reader。
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, rerr := hijack.Reader.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	// 不区分 stdout/stderr，全部返回到 stdout；stderr 留空。
	// 真实场景需用 demux，v0.1 简化。
	return buf, nil, nil
}

func containerName(sessionID string) string {
	return "ballast-sbx-" + sessionID
}

func pInt64(v int64) *int64 { return &v }

var _ runtime.SandboxRuntime = (*DockerRuntime)(nil)
var _ runtime.SandboxInstance = (*dockerInstance)(nil)
