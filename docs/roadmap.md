# Ballast Roadmap

## v0.1（已交付）— 控制面骨架与底层基建

- ✅ Go 控制面后端（REST + WebSocket）
- ✅ Next.js 15 前端工作区（Reason Tree / Xterm.js / Approve 三栏）
- ✅ PostgreSQL 数据层（4 张核心表 + pgx store）
- ✅ SandboxRuntime SPI + Docker 实现（E2B 留扩展点）
- ✅ Harness-Agent（PTY 劫持 + 指令拦截 + gRPC proto 契约）
- ✅ OPA 策略引擎（Rego 热加载 + 三决策路径单测）
- ✅ Skill 资产基础 API + 手工会话只读挂载
- ✅ Git/IaC 工作区挂载（`workspace_dir` → `/workspace/project`）
- ✅ Trigger Rule 资产基础 API
- ✅ 会话审计 API + 工作区 Audit Trail
- ✅ Mock OpenCode 引擎（K8s CrashLoopBackOff 排障剧本）
- ✅ 端到端闭环：创建 → 排障 → 拦截 → 审批 → Resume → 销毁

## v0.2 — 真实引擎接入与自动化

- [x] **OpenCode HTTP/SSE client adapter**：`server/internal/opencode/client.HTTPEngine` 支持 session / prompt / event / MCP 注入；沙箱镜像安装真实 CLI 后可替换 mock-opencode
- [x] **真实 K8s 场景 runner**：`ballast-real-k8s-runner` 使用真实 kubectl + 只读 kubeconfig 访问真实 Kubernetes API，审批后执行 `kubectl apply` 并验证 rollout
- [ ] **gRPC 通信切换**：基于 `harness-agent/internal/proto/harness.proto` 生成 pb 代码，控制面 ↔ harness-agent 由 HTTP JSON 升级为 gRPC 双向流
- [x] **Webhook / Cron 自动执行**：基于 `ballast_trigger_rules` 资产，内部 Webhook 与 Cron scheduler 可自动拉起沙箱
- [ ] **Vault JIT 凭证真实对接**：`InjectJITCredential` 调用 HashiCorp Vault 申请 15 分钟临时 kubeconfig，注入沙箱
- [x] **飞书/钉钉审批卡片推送**：SUSPEND 时可通过 `generic/feishu/dingtalk` webhook 向值班 SRE 推送审批通知
- [x] **Web-TTY 双向接管**：工作区支持人工接管命令在当前沙箱执行，命令仍经 OPA 判定并进入审计/事件回放

## v0.3 — 资产中枢与生态

- [x] **Skill IDE**：资产中心提供 Skill Markdown、Trigger Rule JSON、MCP Plugin JSON 编辑与发布
- [x] **MCP 插件中心基础能力**：注册 MCP server，任务拉起时生成 `mcp_config.json` 并只读挂载进沙箱；真实 OpenCode 下可通过 client `POST /mcp` 注入
- [x] **审计事件回放**：`ballast_session_events` 持久化 Reason/TTY/Policy 事件，历史会话可回放；对象存储原始录像仍可作为生产增强
- [x] **Git PR 自动提交**：`ballast-git-pr-runner` 在真实挂载 Git 工作区内读取变更；branch/add/commit/push 均经 Ballast 审批，远端 PR/MR 链接通过模板生成

## v0.4 — 执行面强化

- [ ] **E2B Firecracker MicroVM Runtime**：实现 `SandboxRuntime` 接口的 E2B 版，更强隔离
- [ ] **ClickHouse 审计下沉**：高并发 TTY 日志迁出 PostgreSQL
- [ ] **多模型分发**：`model_router` 按 Plan/Act 阶段路由强推理 vs 快速模型
- [ ] **配置漂移自愈巡检**：Cron 任务 + 真实 Terraform/OpenTofu plan 漂移检测 + 自动生成回滚 PR

## 待评估

- 跨集群多租户隔离与 RBAC
- 离线/私有化部署的镜像与模型路由
- OPA 策略版本化与灰度发布
