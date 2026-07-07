# Ballast

> AI 时代云原生基础设施的安全自愈与演进底座（Harness）。
> 让每一家企业都敢于、合规地、无风险地在生产环境释放大模型智能体的生产力。

Ballast 解决"AI 引擎高效率"与"生产环境高风险"之间的终极矛盾：
视 OpenCode 为最锋利的"矛"（极致执行与代码重构力），视 Ballast 为最坚固的"盾"
（物理沙箱与指令级熔断网关）。唯有盾足够坚固，矛才敢放手冲锋。

完整理念与产品需求见 [`spec/INIT.md`](spec/INIT.md)。

---

## 当前版本：v0.2 — 自动化与资产中枢闭环

当前版本打通"控制面 → 自动化触发 → 隔离沙箱 → Skill/MCP 资产注入 → 变更策略拦截 →
人工断点审批 → 全量审计"的闭环。默认执行引擎仍以可验证的 **mock-opencode** 剧本运行；
真实 `opencode serve` 的 HTTP/SSE client adapter 已落地，企业环境可在沙箱镜像中安装真实 CLI 后接入。

### 当前范围

| 模块 | 状态 | 说明 |
| --- | --- | --- |
| Go 控制面后端 | ✅ | REST + WebSocket，`server/` |
| Next.js 15 前端工作区 | ✅ | 会话列表 + 三栏布局（Reason Tree / Xterm.js / Approve），`web/` |
| PostgreSQL 数据层 | ✅ | sessions / trigger_rules / skills / audit_logs / mcp_plugins，`server/migrations/` |
| SandboxRuntime SPI + Docker | ✅ | 接口 + Docker 实现，E2B 留作扩展点，`server/internal/runtime/` |
| Harness-Agent（PTY 劫持 + 指令拦截） | ✅ | `harness-agent/`，gRPC proto 契约已定义，v0.1 通信走 HTTP JSON |
| OPA 策略引擎 | ✅ | Rego 热加载，三决策路径（APPROVE/DENY/SUSPEND）单测覆盖，`server/internal/policy/` |
| Skill 资产与挂载 | ✅ | REST 管理 `SKILL.md`，手工会话可按 `skill_ids` 只读挂载到沙箱 `/workspace/.opencode/skills/` |
| Trigger Rule 自动化 | ✅ | REST 管理路由资产；内部 Webhook 入口与 Cron 调度可按规则自动拉起沙箱 |
| MCP 插件中心基础能力 | ✅ | REST 管理 MCP 插件，生成 `mcp_config.json` 并只读挂载到沙箱 |
| OpenCode HTTP/SSE client | ✅ | `server/internal/opencode/client` 实现 `Engine`，支持 session / prompt / event / MCP 注入 |
| Mock OpenCode 引擎 | ✅ | 沙箱内 JSON-Line 剧本；每条工具调用须等待 Harness 决策，`sandbox-image/mock-opencode` |
| 沙箱基础镜像 | ✅ | 预装 mock-opencode + harness-agent + 占位 CLI，`sandbox-image/` |
| 认证与边界 | ✅ | HttpOnly 控制台会话、内部 Bearer Token、CORS/WS Origin 校验 |
| 审计中心基础能力 | ✅ | 审计落库 + 会话审计 API + 工作区 Audit Trail |
| 端到端闭环 | ✅ | 创建 / Webhook → Skill+MCP 挂载 → Docker 沙箱 → 写操作挂起 → 精确审批 → 审计 → 自动销毁 |

### 仍需外部生产系统接入的扩展点

Vault JIT 真实对接、控制面 ↔ Harness gRPC 双向流替换、真实 OpenCode CLI 镜像打包、完整 Skill IDE、
E2B Firecracker Runtime、Web-TTY 双向接管、飞书/钉钉审批推送、Git PR 自动提交。
详见 [`docs/roadmap.md`](docs/roadmap.md)。

---

## 架构

```mermaid
flowchart TB
    subgraph cp [控制面 Ballast Control Plane]
        WebUI[Next.js 15 前端<br/>会话列表 + 终端预览 + 审批按钮]
        GoSrv[Go Server Engine<br/>REST + WebSocket API]
        OPA[OPA Policy Decision Point<br/>Rego 策略加载与求值]
        DB[(PostgreSQL<br/>sessions/skills/audit_logs)]
    end

    subgraph sb [边缘执行面 Sandbox]
        Harness[Harness-Agent<br/>PTY Master + 指令拦截 Guard]
        MockProc[mock-opencode<br/>JSON-Line 阻断式剧本]
    end

    WebUI <-->|REST/WS| GoSrv
    GoSrv -->|Docker CLI / daemon| Harness
    GoSrv --> OPA
    GoSrv --> DB
    Harness <-->|PTY：事件上行 / 决策下行| MockProc
    Harness -->|事件 + 阻断式命令请求| GoSrv
    GoSrv -->|EvaluateCommand| OPA
```

---

## 目录结构

```
Ballast/
├── spec/INIT.md                       # 项目核心基石文档（理念/PRD/架构/详细设计/配置）
├── docs/
│   ├── opencode-protocol-research.md  # OpenCode 协议调研 spike 产出
│   └── roadmap.md                     # 后续路线图
├── server/                            # Go 控制面后端
│   ├── cmd/ballast-server/            # 入口
│   ├── internal/
│   │   ├── automation/                # Webhook/Cron 触发执行
│   │   ├── api/                       # REST + WebSocket handlers
│   │   ├── config/                    # ballast.yaml 解析
│   │   ├── domain/                    # 领域模型（Session/Skill/AuditLog）
│   │   ├── orchestrator/              # 会话生命周期编排 + WS Hub
│   │   ├── policy/                    # PolicyEngine 接口 + OPA 实现
│   │   ├── opencode/                  # OpenCode Engine 接口 + HTTP/SSE client
│   │   ├── runtime/                   # SandboxRuntime SPI + Docker 实现
│   │   ├── store/                     # PostgreSQL 数据访问层（pgx）
│   ├── migrations/                    # 嵌入式、有序 PostgreSQL migrations
│   └── configs/ballast.yaml           # 主配置样例
├── harness-agent/                     # 沙箱内侧插桩网关
│   ├── cmd/harness-agent/
│   ├── internal/pty/                  # PTY master 劫持（creack/pty）
│   ├── internal/guard/                # 指令拦截 + 控制面上报
│   └── internal/proto/harness.proto   # gRPC 契约（v0.2 启用）
├── sandbox-image/                     # 沙箱基础镜像
│   ├── Dockerfile                     # 预装 mock-opencode + harness-agent + 占位 CLI
│   ├── mock-opencode                  # PTY 阻断剧本 + 可选 HTTP/SSE 协议预览
│   └── .opencode/skills/              # Skill 挂载点样例
├── policies/                          # OPA Rego 策略文件
│   └── k8s_prod_write_intercept.rego  # spec 中的样例策略
├── web/                               # Next.js 15 前端
│   ├── app/
│   │   ├── sessions/page.tsx          # 会话列表
│   │   └── sessions/[id]/page.tsx     # 三栏工作区
│   ├── components/                    # ReasonTree / Terminal(xterm) / ApproveBar
│   └── lib/api.ts                     # API + WS 客户端
├── docker-compose.yml                 # PostgreSQL + Server + Web 本地一键起
├── scripts/e2e-smoke.sh               # 认证/策略/审批/审计/销毁端到端验证
└── Makefile                           # make backend / frontend / sandbox-image / dev / test
```

---

## 本地起服务

需要：Go 1.26.4+、Node 22+、Docker（用于沙箱镜像与 compose）。

```bash
# 1. 拉依赖
cd server && go mod tidy && cd ..
cd harness-agent && go mod tidy && cd ..
cd web && npm install && cd ..

# 2. 全栈起（同时构建沙箱镜像）
docker compose up -d --build
# 或仅起数据库，后端/前端本机直跑：
docker compose up -d postgres
make backend-run   # -> http://localhost:8080
make frontend-dev  # -> http://localhost:3000

# 3. 也可单独构建沙箱基础镜像
make sandbox-image
```

打开 http://localhost:3000，使用开发管理令牌 `ballast-dev-admin-token` 登录。
点击“拉起沙箱会话”后：Mock 引擎输出 Reason Tree → 两条只读命令自动放行 →
`kubectl apply` 触发 SUSPEND → 右侧 Approve 精确放行该命令 → 审计落库 →
会话成功并自动物理销毁沙箱。

会话列表页还提供轻量资产面板：可写入示例 Skill / Trigger Rule / MCP Plugin，并在拉起
手工会话时选择 Skill 与 MCP。选中的 `SKILL.md` 与 `mcp_config.json` 会由控制面物化到
`runtime_provider.config.workspace_root`，
并以只读目录挂载进沙箱。通过 Docker socket 运行 compose 时，该目录必须对控制面容器与宿主
Docker daemon 使用同一路径；默认 compose 已将 `/tmp/ballast/sandboxes` 按相同路径挂载。

默认令牌仅用于本地开发。部署前必须通过环境变量覆盖
`BALLAST_JWT_SECRET`、`BALLAST_ADMIN_TOKEN`、`BALLAST_INTERNAL_TOKEN`，
并在 TLS 入口后启用 `cookie_secure`。

---

## API 速览

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/healthz` | 健康检查 |
| `POST` | `/api/auth/login` | 管理令牌换取 HttpOnly 会话 Cookie |
| `POST` | `/api/auth/logout` | 清除控制台会话 |
| `GET` | `/api/sessions?status=&limit=&offset=` | 会话列表 |
| `POST` | `/api/sessions` | 创建会话（拉起沙箱 + 引擎） |
| `GET` | `/api/sessions/{id}` | 会话详情 |
| `GET` | `/api/sessions/{id}/audit?limit=100` | 会话审计日志 |
| `POST` | `/api/sessions/{id}/approve` | 人工放行被挂起的命令 |
| `POST` | `/api/sessions/{id}/resume` | 等价 approve |
| `POST` | `/api/sessions/{id}/destroy` | 销毁沙箱 |
| `WS` | `/api/sessions/{id}/ws` | 实时事件流（reason.step / tool.call / policy.decision / policy.resumed） |
| `GET` | `/api/skills` | 列出 Skill 资产 |
| `POST` | `/api/skills` | Upsert Skill 资产 |
| `GET` | `/api/skills/{id}` | Skill 详情 |
| `GET` | `/api/trigger-rules` | 列出触发路由资产 |
| `POST` | `/api/trigger-rules` | Upsert 触发路由资产 |
| `GET` | `/api/trigger-rules/{id}` | 触发路由详情 |
| `GET` | `/api/mcp-plugins` | 列出 MCP 插件资产 |
| `POST` | `/api/mcp-plugins` | Upsert MCP 插件资产 |
| `GET` | `/api/mcp-plugins/{id}` | MCP 插件详情 |
| `POST` | `/api/internal/harness/report` | harness-agent guard 上报命令，返回 OPA 决策 |
| `POST` | `/api/internal/harness/event` | harness-agent 上报 Reason/Tool/Result 事件 |
| `POST` | `/api/internal/triggers/webhook/{source}` | 内部 Webhook 触发入口，按 active Trigger Rule 自动创建会话 |

除健康检查与登录外，控制台 API 均要求会话 Cookie；内部 Harness API 要求独立 Bearer Token。

---

## 验证

```bash
# 单元测试、静态检查与前端类型/规则检查
make test
make vet
cd web && npm run build

# 全栈启动后执行真实容器闭环
make e2e-test
```

端到端脚本会验证：匿名访问被拒绝、内部接口鉴权、CORS、Skill/Trigger Rule/MCP 资产 API、
带 Skill+MCP 创建会话、沙箱内 SKILL.md 与 mcp_config.json 只读挂载、Docker 沙箱创建、
两条只读命令自动 APPROVE、写命令 SUSPEND、人工精确放行、DB 与 API 审计计数、Webhook
自动触发会话以及沙箱自动销毁。

---

## 安全红线（来自 spec）

- 无监督/无审批的生产写操作：**绝不**
- OpenCode 直接在管理端宿主机执行本地 Shell：**绝不**
- 全自动黑盒自愈：**绝不**，每一步变更必须可视、可卡口
- 替代人类承担最终法律责任：**绝不**，AI 永远是副驾驶

v0.1 的 Docker Runtime 适用于本地开发和闭环验证。控制面需要访问 Docker daemon，
不应被直接暴露到不可信网络；生产隔离仍应按路线图替换为受管容器平台或 MicroVM Runtime。

---

## License

见 [LICENSE](LICENSE)。
