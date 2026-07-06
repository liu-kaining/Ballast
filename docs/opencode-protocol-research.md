# OpenCode 协议调研 Spike

> Phase 0 产出。本文件固化 Ballast 与 OpenCode 引擎之间的集成契约，作为 v0.1 Mock 引擎与 v0.2 真实 client 的共同接口基准。

## 1. 调研方式

- 目标命令：`opencode serve [--port 4096] [--hostname 127.0.0.1] [--cors <origin>] [--mdns]`
- 协议形态：headless HTTP server，暴露 OpenAPI 3.1 spec，实时事件通过 SSE 推送
- 运行时：Bun + Hono（轻量 HTTP 框架），原生 WebSocket/SSE 支持
- 调研手段：本地未安装 opencode CLI（v0.1 环境约束），结论基于官方文档 https://opencode.ai/docs/server/ 与 https://opencode.ai/docs/sdk/ 及社区集成案例。v0.2 接入真实引擎时需用 `curl http://localhost:4096/doc` 抓取最新 OpenAPI 3.1 spec 校准。

## 2. 关键端点

| 方法 | 路径 | 用途 | 响应 |
| --- | --- | --- | --- |
| `GET` | `/doc` | OpenAPI 3.1 规范 | HTML/JSON spec，可导入 Postman 或用 openapi-generator 生成客户端 |
| `POST` | `/session` | 创建会话 | 会话对象（含 `id`、`title`） |
| `GET` | `/session` | 列出会话 | 会话数组 |
| `POST` | `/session/{id}/message` | 向会话发送消息 | `AssistantMessage`，支持 `format` 字段请求结构化 JSON 输出 |
| `GET` | `/event` | SSE 实时事件流 | 首个事件 `server.connected`，随后是 bus 事件（token 消耗、工具调用、reason step 等） |
| `GET` | `/lsp` | LSP 服务状态 | `LSPStatus[]` |
| `GET` | `/formatter` | 格式化器状态 | `FormatterStatus[]` |
| `GET` | `/mcp` | MCP 服务状态 | `{ [name]: MCPStatus }` |
| `POST` | `/mcp` | 动态注入 MCP server | body `{ name, config }`，返回 MCPStatus |

## 3. 事件流（SSE）

`GET /event` 返回 `text/event-stream`：

- 首事件：`server.connected`（携带 server 元信息）
- 随后：bus 事件，类型包括：
  - 会话/消息生命周期事件（message 开始、token 增量、message 完成）
  - 工具调用事件（tool_call 开始、参数、stdout/stderr、结束）
  - reason step 事件（AI 思考步骤，对应 Ballast Reason Tree）
  - error 事件

事件 schema 由 OpenAPI 3.1 spec 在 `/doc` 中定义；v0.2 需用 `curl http://localhost:4096/doc -H "Accept: application/json" > opencode-api.json` 拉取后用 openapi-generator 生成 Go 客户端。

## 4. 消息体样例

创建会话：

```http
POST /session HTTP/1.1
Content-Type: application/json

{"title": "k8s-pod-crash-triage"}
```

发送消息（支持结构化输出）：

```http
POST /session/{id}/message HTTP/1.1
Content-Type: application/json

{
  "parts": [{"type": "text", "text": "诊断 CrashLoopBackOff 根因"}],
  "model": {"providerID": "deepseek", "modelID": "deepseek-reasoner"}
}
```

动态注入 MCP（Ballast 沙箱拉起后用此注入 k8sgpt/prometheus MCP）：

```http
POST /mcp HTTP/1.1
Content-Type: application/json

{"name": "k8sgpt-mcp-server", "config": {"command": "k8sgpt-mcp", "args": []}}
```

## 5. Ballast 侧抽象接口（Go）

v0.1 在 `server/internal/opencode` 定义如下接口，Mock 与真实 client 共同实现：

```go
package opencode

import "context"

// Engine 抽象 OpenCode 引擎的控制面。
// v0.1 由 mock.MockEngine 实现；v0.2 由 client.HTTPEngine 实现，
// 通过 opencode serve 暴露的 OpenAPI 3.1 端点对接。
type Engine interface {
    // StartSession 创建一个 OpenCode 会话，返回会话 ID。
    StartSession(ctx context.Context, title string, opts SessionOpts) (string, error)
    // Prompt 向会话发送一条用户消息，返回 AssistantMessage ID。
    // 事件通过 Events() 流异步推送。
    Prompt(ctx context.Context, sessionID string, text string) (string, error)
    // Events 返回 SSE 事件流的只读 channel。
    // 事件类型见 docs/opencode-protocol-research.md §3。
    Events(ctx context.Context, sessionID string) (<-chan Event, error)
    // InjectMCP 动态挂载一个 MCP server 到引擎。
    InjectMCP(ctx context.Context, name string, config MCPConfig) error
    // Stop 终止引擎进程。
    Stop(ctx context.Context) error
}

type SessionOpts struct {
    ModelProvider string
    ModelID       string
    WorkingDir    string
    SkillDir      string // /workspace/.opencode/skills/
}

type Event struct {
    Type      string          // "server.connected", "message.token", "tool.call", "reason.step", ...
    Payload   json.RawMessage // 原始事件负载
    Timestamp time.Time
}

type MCPConfig struct {
    Command string
    Args    []string
    Env     map[string]string
}
```

## 6. 与 spec 架构的对齐澄清

spec 同时提到"PTY Master 伪终端劫持"与"`opencode serve --format json`"，二者并不矛盾：

- **OpenCode 引擎本体** 通过 HTTP/SSE（`opencode serve`）控制 —— Ballast Server 是它的 HTTP 客户端
- **PTY 劫持** 作用于 OpenCode 在沙箱内 spawn 的 shell 子进程（工具执行层）—— 由 `harness-agent` 在 PTY master 端拦截 `kubectl`/`terraform` 等命令，组装 `CommandContext` 上报控制面 `PolicyEngine`

因此 Ballast 控制面到沙箱有两条通信路径：
1. **HTTP/SSE → OpenCode 引擎**：下发意图、订阅 Reason Tree 与 token 事件
2. **gRPC → Harness-Agent**：下发 Suspend/Resume/Terminate 控制信号、上报被拦截的命令上下文

v0.1 Mock 引擎模拟路径 1；harness-agent 实现路径 2 的 PTY 拦截与控制信号。

## 7. v0.2 接入清单

- [ ] 在沙箱镜像内安装 opencode CLI（`npm i -g opencode-ai` 或 Bun 安装）
- [ ] `opencode serve --port 4096 --hostname 0.0.0.0` 作为沙箱 entrypoint 之一
- [ ] 用 `curl http://<sandbox-ip>:4096/doc -H "Accept: application/json"` 抓取真实 OpenAPI spec，对比本文件校准
- [ ] 用 openapi-generator 生成 Go client，替换 `opencode/mock.MockEngine` 为 `opencode/client.HTTPEngine`
- [ ] 校验 SSE 事件 schema 与 `Event.Type` 常量集
- [ ] 验证 `POST /mcp` 动态注入 k8sgpt/prometheus MCP 的实际行为
