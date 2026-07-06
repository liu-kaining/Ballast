# Ballast Roadmap

## v0.1（已交付）— 控制面骨架与底层基建

- ✅ Go 控制面后端（REST + WebSocket）
- ✅ Next.js 15 前端工作区（Reason Tree / Xterm.js / Approve 三栏）
- ✅ PostgreSQL 数据层（4 张核心表 + pgx store）
- ✅ SandboxRuntime SPI + Docker 实现（E2B 留扩展点）
- ✅ Harness-Agent（PTY 劫持 + 指令拦截 + gRPC proto 契约）
- ✅ OPA 策略引擎（Rego 热加载 + 三决策路径单测）
- ✅ Mock OpenCode 引擎（K8s CrashLoopBackOff 排障剧本）
- ✅ 端到端闭环：创建 → 排障 → 拦截 → 审批 → Resume → 销毁

## v0.2 — 真实引擎接入与自动化

- [ ] **真实 OpenCode 引擎接入**：沙箱内 `opencode serve`，控制面 `client.HTTPEngine` 替换 Mock，校准 `/doc` OpenAPI spec 与 SSE 事件 schema
- [ ] **gRPC 通信切换**：基于 `harness-agent/internal/proto/harness.proto` 生成 pb 代码，控制面 ↔ harness-agent 由 HTTP JSON 升级为 gRPC 双向流
- [ ] **Webhook / Cron 自动路由**：落地 `ballast_trigger_rules` 表与 `trigger_routes.yaml`，Prometheus Alertmanager / Cron 触发自动拉起沙箱
- [ ] **Vault JIT 凭证真实对接**：`InjectJITCredential` 调用 HashiCorp Vault 申请 15 分钟临时 kubeconfig，注入沙箱
- [ ] **飞书/钉钉审批卡片推送**：SUSPEND 时向值班 SRE 推送带 Approve 按钮的卡片
- [ ] **Web-TTY 双向接管**：前端【接管终端】按钮，AI 停止 → 人工手敲 → 【释放接管】AI 承接现场

## v0.3 — 资产中枢与生态

- [ ] **Skill IDE**：Web 端编辑带 Frontmatter 的 `SKILL.md`，存 PostgreSQL，按需热注入沙箱 `/workspace/.opencode/skills/`
- [ ] **MCP 插件中心**：注册 MCP server，任务拉起时在沙箱内以 stdio 拉起并通过 `POST /mcp` 注入 OpenCode
- [ ] **审计录像回放**：`ballast_audit_logs.raw_tty_output_path` 指向对象存储的原始流，前端时间轴回放
- [ ] **Git PR 自动提交**：场景 B 演进态，沙箱内 git 提交分支并向 GitLab 推送 PR

## v0.4 — 执行面强化

- [ ] **E2B Firecracker MicroVM Runtime**：实现 `SandboxRuntime` 接口的 E2B 版，更强隔离
- [ ] **ClickHouse 审计下沉**：高并发 TTY 日志迁出 PostgreSQL
- [ ] **多模型分发**：`model_router` 按 Plan/Act 阶段路由强推理 vs 快速模型
- [ ] **配置漂移自愈巡检**：Cron 任务 + terraform plan 漂移检测 + 自动生成回滚 PR

## 待评估

- 跨集群多租户隔离与 RBAC
- 离线/私有化部署的镜像与模型路由
- OPA 策略版本化与灰度发布
