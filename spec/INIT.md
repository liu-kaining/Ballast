# 📑 Ballast 项目核心基石文档合集 (Foundation Documents)

---

## 🧭 文档一：Ballast 整体理念 (Core Philosophy)

### 1. 我们的初衷 (Our Origin)

在云原生与微服务铺天盖地的今天，分布式系统的复杂度和变更频率已经彻底超越了人类大脑的认知带宽。SRE（站点可靠性工程师）团队陷入了一个死循环：**系统越复杂，线上故障越多；线上故障越多，堆砌的“死板自动化脚本”和 Runbook 就越多；而这些脆而不坚的脚本，又在下一次系统变更中成为引发二次故障的温床。**

大模型（LLM）与智能体（Agent）的爆发，让我们看到了“意图驱动”弹性运维的曙光。像 OpenCode 这样具备极致终端控制力与代码重构力的开源引擎，理论上能像人类专家一样去排障、改配置。然而，大模型的幻觉天性和不可控的终端破坏力，成为它进入生产环境之间一条不可逾越的“信任鸿沟”。没有任何一家公司的基础架构负责人，敢放开一个赤裸的 AI 接口去对生产集群执行写操作。

**Ballast 的诞生，就是为了解决“AI 引擎高效率”与“生产环境高风险”之间的终极矛盾。**

### 2. 我们的理念 (Our Vision)

* **矛与盾的哲学：** 视 OpenCode 为最锋利的“矛”（极致的执行与代码重构力），视 Ballast 为最坚固的“盾”（物理沙箱与指令级熔断网关）。唯有盾足够坚固，矛才敢放手冲锋。
* **解耦人的认知带宽：** 将基础设施的“故障止损时间（MTTR）”和“长周期演进成本（IaC）”，从“依赖人类肉体的生理反应与认知极限”中彻底解耦。
* **不当消防员，当总指挥：** 改变 SRE 的工作形态。SRE 不应是凌晨三点满头大汗、在黑乎乎的终端里盲目敲命令的消防员，而应该是坐在 Ballast 仪表盘前，审阅 AI 诊断证据、按下“变更批准”按钮的总指挥。

### 3. 我们的理想 (我们的终极目标)

成为 AI 时代云原生基础设施的标准“安全自愈与演进底座（Harness）”。让每一家企业都敢于、合规地、无风险地在生产环境释放大模型智能体的生产力。

### 4. 能力边界矩阵 (The Four Quadrants)

```
                       【 擅长解决 (Excels) 】
                       * IaC 基础架构代码演进
                       * 复杂故障的多源联动初筛诊断
                       * 持续合规审计与配置漂移自愈
                                  ▲
                                  │
【 能解决 (Solves) 】               │               【 不适合解决 (Unsuitable) 】
* 3 AM 认知盲区与手速瓶颈          ├───────────────► * 长期容量规划与时序趋势预测
* 传统自动化脚本的“维护地狱”        │                 * 跨团队、跨领域业务应用 Bug 修复
* 生产环境的“AI 信任鸿沟”         │                 * 组织流程改进与事故复盘总结
                                  │
                                  ▼
                     【 绝对不碰 (Hard Red Lines) 】
                     * 无监督/无审批的生产写操作
                     * 通用低代码全家桶大模型庶务
                     * 替代人类承担安全与法律最终责任

```

#### 🟩 能解决什么问题？(Solves)

* **3 AM 故障认知致盲：** 消除故障发生初期，人类由于生理疲劳和信息过载导致的定位延迟。
* **传统的“死剧本”维护地狱：** 用大模型的“意图驱动”和自主工具调用循环，替代死板易碎的死写 Shell/Ansible 脚本。
* **AI 落地生产的合规死锁：** 解决企业对大模型幻觉、误操作、删库跑路的恐惧，为企业架设一条绝对安全的物理红线。

#### 🚀 极其擅长解决什么问题？(Excels)

* **IaC 与 GitOps 基础设施演进：** 如 Kubernetes 升级导致几百个 YAML 的 `apiVersion` 过期需要精密批量重构，或者云厂商 Terraform Provider 升级带来的破坏性语法变更。
* **复杂故障自愈诊断（只读态高并发）：** 告警触发后，秒级进沙箱，并发执行数十条诸如 `kubectl describe`、日志捞取、监控指标 MCP 查询动作，在人醒来前组装好证据链。
* **持续配置漂移回滚：** 自动化抓出开发在线上用 Web UI 绕过 Git 做的“暗箱变更”，生成回滚代码并悬挂等待一键审批恢复。

#### ⚠️ 不适合解决什么问题？(Unsuitable)

* **长期容量规划与时序趋势预测：** 这是时序数据统计学和传统 ML 模型的地盘（如 Prophecy 算法），本平台不进行此类计算。
* **业务应用代码（Java/Go 等）的 Bug 修复：** 这涉及复杂的业务逻辑和上下游领域设计。本平台只专注于基础设施（Infra）和运行环境。
* **故障复盘文档（Post-Mortem）的无中生有：** 平台提供 100% 还原的终端审计录像和时间线，但写不出涉及公司组织架构改进的制度性报告。

#### 🟥 绝对不碰什么问题？(Hard Red Lines)

* **无监督的生产写操作：** 坚决不允许 AI 在没有“人类点击 Approve 批准”或“Git 预发环境 CI 自动化测试通过”的前提下，直接向生产集群写入变更。
* **通用大模型庶务：** 拒绝写周报、拒绝查天气、拒绝充当办公客服。Ballast 保持微内核的纯粹性，只接 SRE 领域的组件。
* **替代人类承担最终法律责任：** AI 永远是副驾驶，最终变更确认权永远属于人类，平台绝不提供任何规避人类签字确认责任的设计。

---

## 📋 文档二：产品需求文档 (PRD)

### 1. 产品核心能力定义

Ballast 旨在打造一个闭环的 AI 运维安全台架。产品核心能力聚焦于：**告警安全接入 -> 隔离沙箱拉起 -> 意图驱动排障/演进 -> 变更策略拦截 -> 人工断点审批 -> 终端接管控制 -> 全量行为审计**。

### 2. “有所为，有所不为”约束表

| 平台有所为 (Must Do) | 平台有所不为 (Must NOT Do) |
| --- | --- |
| **必须** 为每一次任务分配物理隔离、即用即弃的沙箱。 | **绝不** 允许 OpenCode 直接在平台管理端宿主机执行本地 Shell。 |
| **必须** 支持指令级别的内省拦截与 OPA 动态策略匹配。 | **绝不** 做死硬的代码黑名单过滤，所有拦截必须基于动态可调策略。 |
| **必须** 提供实时 Web-TTY（Xterm.js）和人类随时接管功能。 | **绝不** 做全自动的“黑盒自愈”，每一步变更必须可视、可卡口。 |
| **必须** 支持将 SRE 剧本固化为本地可插拔挂载的 Markdown Skill。 | **绝不** 自研大模型、不自研聊天监控，只消费开源标准生态。 |

### 3. 功能模块规划

```
+-----------------------------------------------------------------------------------+
|                                 【 Ballast 产品功能矩阵 】                          |
+--------------------------+--------------------------+-----------------------------+
| 1. 自主运维工作区 (Workspace) | 2. 自动化与审计中心 (Task)   | 3. 资产与扩展中枢 (Asset)   |
+--------------------------+--------------------------+-----------------------------+
| * AI 思考步骤树 (Reason Tree) | * 自动化任务路由表 (Router) | * 剧本资产编辑器 (Skill IDE) |
| * 实时 WebTTY (Xterm.js)  | * 定时/Webhook 触发器配置 | * MCP 标准插件中心          |
| * 人工控制熔断接管按钮     | * 100% TTY 变更历史录像审计 | * OPA 安全策略管理控制台    |
+--------------------------+--------------------------+-----------------------------+

```

### 4. 用户动线旅程设计 (User Journey Map)

#### 场景 A：无人值守的告警初筛与安全阻断自愈（排障态）

1. **触发阶段：** 凌晨 3:00，Prometheus 触发 `K8s_Pod_CrashLoopBackOff` 高危告警，推送到 Ballast 自动化中心。
2. **准备阶段：** 平台识别路由，自动调用 Vault 注入一个有效期 15 分钟的临时只读 Kubeconfig，秒级拉起一个“K8s 诊断特种兵沙箱”。
3. **诊断阶段：** 沙箱内 OpenCode 启动，自动加载预设的 `k8s_debug_skill`。OpenCode 连续执行 `kubectl get`、`kubectl logs` 抓出根因是某个 ConfigMap 挂载路径写错，导致死循环。
4. **修复/拦截阶段：** OpenCode 尝试执行 `kubectl apply -f fixed_cm.yaml` 实施自愈。
5. **卡口阶段：** Ballast Harness 网关捕获到 `apply` 动作，触发 OPA 灰名单规则。沙箱进程瞬间冻结。Ballast 向值班 SRE 的手机飞书/钉钉推送一条带卡片的通知，并同时将本会话在控制台标红。
6. **介入阶段：** SRE 被唤醒，打开电脑进入【自主运维工作区】。左边看到 AI 完整的“思考步骤树”和错误日志证据，右边看到被冻结的终端现场。
7. **闭环阶段：** SRE 确认修改无误，在网页端点击【Approve（放行）】。状态机恢复，OpenCode 执行完变更，跑完健康检查，沙箱自动物理销毁。

#### 场景 B：人工下发长周期基础架构升级任务（演进态）

1. **输入阶段：** SRE 登录控制区，在【自主运维工作区】输入意图：“帮我把当前项目的 Terraform 脚本中，所有过期的 AWS 安全组语法重构为最新的 v5 规范”。
2. **拉起阶段：** 平台启动长任务会话。拉起配置了 Terraform 工具链的沙箱，挂载 Git 仓库。
3. **开发阶段：** 用户坐在屏幕前，左侧看着 OpenCode 利用本地 LSP 进行语法扫描并生成 File Diff 的思考树；右侧 Xterm.js 实时滚动滚动着代码修改的细节。
4. **接管阶段（可选）：** 过程中，用户发现 AI 漏掉了一个特定环境的命名规则，点击【接管终端】，AI 停止，用户直接在右侧终端手敲了两行命令补全。点【释放接管】，AI 承接当前终端现场继续干活。
5. **提单阶段：** 任务完成，Ballast 自动驱动沙箱内 Git 插件，向公司 GitLab 提交分支并推送一个 PR，并在工作区向用户呈现 PR 链接，提示等待团队 Code Review。沙箱销毁。

---

## 🏗️ 文档三：技术全局架构 (Technical Architecture)

Ballast 的核心技术哲学是“微内核 + 开放扩展”。控制面与执行面彻底物理解耦，Ballast 负责提供严密的 Harness（外骨骼控制层），而将 OpenCode 引擎作为其可插拔的中央执行动力。

### 1. 系统全景架构蓝图 (ASCII Blueprint)

```
==================================【 1. 控制面 (Ballast Control Plane) 】==================================
|                                                                                                       |
|   +----------------------------------+  WebSocket   +---------------------------------------------+   |
|   | 浏览器 Web 前端 (Next.js 15)      | <──────────> | 平台后端中枢 (Ballast Server Engine in Go)   |   |
|   | - Reason Tree View (步骤树)       |              | - REST / WebSocket API 网关                  |   |
|   | - Xterm.js Terminal (流式终端)   |              | - OPA Policy Decision Point (策略决策)       |   |
|   | - Config & Asset Manager (资产)  |              | - JIT Credential Manager (凭证动态签发)      |   |
|   +----------------------------------+              +----------------------┬----------------------+   |
|                                                                            │                          |
=============================================================================│===========================
                                                                             │ gRPC / Docker API
                                                                             ▼
==================================【 2. 边缘执行面 (Isolated Runtime) 】===================================
|                                                                                                       |
|   +-----------------------------------------------------------------------------------------------+   |
|   | 动态隔离沙箱实例 (Docker Container / E2B Firecracker MicroVM)                                  |   |
|   |                                                                                               |   |
|   |   +---------------------------------------------------------------------------------------+   |   |
|   |   | Ballast-Harness-Agent (本地侧插桩网关)                                                 |   |   |
|   |   | - PTY Master Pseudo-Terminal Device (伪终端主设备劫持)                                  |   |   |
|   |   | - Interception Guard Filter (指令过滤器)                                              |   |   |
|   |   +------------------------------------------┬--------------------------------------------+   |   |
|   |                                              │ PTY Slave / Stdio                              |   |
|   |                                              ▼                                                |   |
|   |   +---------------------------------------------------------------------------------------+   |   |
|   |   | OpenCode 核心执行引擎 (Run as `opencode serve --format json`)                           |   |   |
|   |   | - 内置 DAG 编排状态机                                                                  |   |   |
|   |   | - 动态加载动态挂载目录: `/workspace/.opencode/skills/` (由控制面热注入)                |   |   |
|   |   +----------------──────────────────────────┬────────────────────────────────────────----+   |   |
|   |                                              │                                                |   |
|   |                    ┌─────────────────────────┴─────────────────────────┐                      |   |
|   |                    ▼                                                   ▼                      |   |
|   |   +---------------------------------------+           +---------------------------------------+   |   |
|   |   | 开放插件集: MCP Servers (标准协议)     |           | SRE 物理工具箱: OS CLIs               |   |   |
|   |   | - mcp-server-k8sgpt (诊断先锋)         |           | - kubectl, terraform, helm            |   |   |
|   |   | - mcp-server-prometheus (指标调用)    |           | - aws-cli, gcloud, git                |   |   |
|   |   +---------------------------------------+           +---------------------------------------+   |   |
|   +-----------------------------------------------------------------------------------------------+   |
=========================================================================================================

```

### 2. 开放共荣与生态兼容设计（Plugin & SPI）

为了保证平台极致的兼容性与二次开发便利，Ballast 将三大运维资产抽象为标准的**开放服务接口（SPI）**：

#### 开放点 A：能力兼容层（MCP 规范标准对接）

平台坚决不为 Prometheus、Jira 或公司自研 CMDB 写死任何对接代码。Ballast 全面拥抱 Anthropic 提出的 **Model Context Protocol (MCP)**。

* **兼容机制：** 二开人员只需使用任何语言实现标准的 MCP 协议（提供 `tools/list` 和 `tools/call`）。Ballast 在【资产中枢】注册该 MCP 后，任务拉起时会自动在沙箱内以 Stdio 形式拉起该 MCP 守护进程，并在生成的 `mcp_config.json` 中宣告。OpenCode 引擎在启动时通过原生 MCP 机制自动吃下这些工具，瞬间获得外部系统的读写能力。

#### 开放点 B：经验沉淀层（Markdown Skill 动态挂载）

* **兼容机制：** 平台将 SRE 专家的排障 Runbook 规范化为 OpenCode 原生的 `SKILL.md`（带 Frontmatter 声明）。用户在 Web IDE 编写的 Skill 统一存放在 PostgreSQL 中。当某个特定的自动化规则被触发时，平台在拉起沙箱的瞬间，会通过 Docker Volume 或文件写入的方式，将勾选的 Skill 集合动态推入沙箱的 `/workspace/.opencode/skills/` 目录下。OpenCode 启动时自动进行扫描和热加载，实现经验的按需即插即用。

#### 开放点 C：基础设施工具层（镜像工具箱）

* **兼容机制：** 平台将“沙箱环境”彻底接口化（`SandboxRuntime`）。平台默认开源提供基础的 `DockerRuntime`。企业二开团队可以根据自身安全性要求，轻松修改 Dockerfile 预装其特定的 CLI 工具（如自研的内部部署工具），或者通过实现 `SandboxRuntime` 接口将底层替换为 E2B 虚拟机，而上层的控制调度逻辑完全不需改动。

---

## 🛠️ 文档四：技术模块详细设计 (Detailed Technical Design)

### 1. 核心接口定义 (Go Sample Code)

#### 沙箱运行时接口 (`SandboxRuntime`)

```go
package runtime

import "context"

type SandboxInstance interface {
	GetID() string
	GetIP() string
	ExecuteCommand(ctx context.Context, cmd []string) (stdout []byte, stderr []byte, err error)
}

type SandboxRuntime interface {
	// Create 拉起一个完全隔离的沙箱环境
	Create(ctx context.Context, sessionID string, imageName string, volume Mounts) (SandboxInstance, error)
	// InjectJITCredential 注入生命周期极短的临时运维凭证
	InjectJITCredential(ctx context.Context, sessionID string, credsSecretID string) error
	// Destroy 强行物理销毁沙箱，抹除所有痕迹
	Destroy(ctx context.Context, sessionID string) error
}

```

#### 安全策略拦截引擎接口 (`PolicyEngine`)

```go
package policy

import "context"

type Decision string

const (
	Approve Decision = "APPROVE"
	Deny    Decision = "DENY"
	Suspend Decision = "SUSPEND" // 触发人工审批断点
)

type CommandContext struct {
	SessionID string   `json:"session_id"`
	User      string   `json:"user"`
	AgentName string   `json:"agent_name"`
	Command   string   `json:"command"`   // OpenCode 尝试执行的 Bash 原始命令
	Args      []string `json:"args"`      // 命令参数
	Env       []string `json:"env"`       // 容器当前环境变量上下文
}

type PolicyEngine interface {
	// EvaluateCommand 输入尝试执行的命令上下文，输出决策
	EvaluateCommand(ctx context.Context, cmdCtx CommandContext) (Decision, error)
}

```

### 2. 核心数据表设计 (PostgreSQL)

```sql
-- 1. 会话主表 (管理任务与Chat全局生命周期)
CREATE TABLE ballast_sessions (
    session_id VARCHAR(64) PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    trigger_type VARCHAR(32) NOT NULL, -- 'WEBHOOK', 'CRON', 'MANUAL_CHAT'
    status VARCHAR(32) NOT NULL,       -- 'RUNNING', 'SUSPENDED', 'SUCCESS', 'FAILED'
    agent_image VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_session_status ON ballast_sessions(status);

-- 2. 自动化路由规则表
CREATE TABLE ballast_trigger_rules (
    rule_id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    trigger_source VARCHAR(64) NOT NULL, -- e.g., 'Prometheus_Alertmanager'
    match_expression JSONB NOT NULL,       -- e.g., '{"alertname": "PodCrashLoop"}'
    bind_skills VARCHAR(64)[] NOT NULL,   -- 绑定的 Skill ID 数组
    agent_image VARCHAR(255) NOT NULL,
    policy_group VARCHAR(64) NOT NULL     -- 关联的 OPA 策略组
);

-- 3. SRE 剧本 Skill 资产表
CREATE TABLE ballast_skills (
    skill_id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    trigger_words TEXT[] NOT NULL,
    markdown_content TEXT NOT NULL,       -- 包含 Frontmatter 的标准 OpenCode SKILL.md
    version INT DEFAULT 1,
    updated_by VARCHAR(64) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 4. 全量 TTY 变更审计日志表 (高并发写入场景，生产环境建议挂载到 ClickHouse，此处展示核心结构)
CREATE TABLE ballast_audit_logs (
    audit_id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(64) NOT NULL,
    loop_index INT NOT NULL,              -- OpenCode 第几次思考循环
    model_name VARCHAR(64),
    prompt_tokens INT DEFAULT 0,
    completion_tokens INT DEFAULT 0,
    executed_command TEXT,                -- 拦截或执行的命令
    policy_decision VARCHAR(32),          -- 'APPROVE', 'SUSPEND', 'DENY'
    approver VARCHAR(64),                 -- 放行审批人
    raw_tty_output_path TEXT,             -- 指向对象存储的本轮 stdout/stderr 原始流路径
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_audit_session_id ON ballast_audit_logs(session_id);

```

### 3. 核心流转序列图 (System Flow Sequence Diagram)

下图详细梳理了一个告警进来后，系统的 **请求流、数据流、大模型流、配置流** 是如何在 Ballast 与 OpenCode 之间实现安全闭环的：

```
+------------+     +----------------+     +---------------+     +-----------------------+     +------------------+
| Alert/User |     | Ballast Server |     | Vault / JIT   |     | Sandbox (Harness Agent|     | OpenCode Engine  |
+-----┬------+     +-------┬--------+     +-------┬-------+     +-----------┬-----------+     +--------┬---------+
      │                    │                      │                         │                          │
      │ 1. 触发 Webhook     │                      │                         │                          │
      ├───────────────────>│                      │                         │                          │
      │                    │ 2. 查询路由规则表与   │                         │                          │
      │                    │    OPA 策略配置      │                         │                          │
      │                    │──┐                   │                         │                          │
      │                    │  │ [配置流]           │                         │                          │
      │                    │<─┘                   │                         │                          │
      │                    │                      │                         │                          │
      │                    │ 3. 申请15分钟临时凭证 │                         │                          │
      │                    ├─────────────────────>│                         │                          │
      │                    │ 4. 返回加密 Kubeconfig│                         │                          │
      │                    |<─────────────────────│                         │                          │
      │                    │                                                │                          │
      │                    │ 5. 拉起隔离沙箱并挂载 Skill 目录 & 注入凭证      │                          │
      │                    ├───────────────────────────────────────────────>│                          │
      │                    │                                                │                          │
      │                    │ 6. 在沙箱内部拉起 OpenCode Engine (`serve`)    │                          │
      │                    ├──────────────────────────────────────────────────────────────────────────>│
      │                    │                                                │                          │
      │                    │=================== [ 大模型思考循环开始 ] ===================              │
      │                    │                                                │                          │
      │                    │                                                │ 7. 自动执行 Skill 排障    │
      │                    │                                                │    只读命令 (捞日志/指标) │
      │                    │                                                |<─────────────────────────│
      │                    │ 8. 实时 TTY 字符流 & 大模型 Token 消耗事件      │                          │
      │                    |<───────────────────────────────────────────────┤                          │
      │                    │    (数据流：送往前端 Xterm & 落库 ClickHouse)  │                          │
      │                    │                                                │                          │
      │                    │                                                │ 9. 尝试执行高危写命令:   │
      │                    │                                                │    `kubectl apply`       │
      │                    │                                                |<─────────────────────────│
      │                    │ 10. 拦截信号：调用 OPA 判定发现命中灰名单       │                          │
      │                    |<───────────────────────────────────────────────┤                          │
      │                    │                                                │                          │
      │                    │ 11. [安全断点] 强行挂起(Suspend)沙箱内执行进程  │                          │
      │                    ├───────────────────────────────────────────────>│                          │
      │                    │                                                │                          │
      │ 12. 推送审批卡片    │                                                │                          │
      │<───────────────────┤                                                │                          │
      │ 13. 人类点击同意    │                                                │                          │
      ├───────────────────>│                                                │                          │
      │                    │                                                │                          │
      │                    │ 14. 发送 Resume 信号放行命令                   │                          │
      │                    ├───────────────────────────────────────────────>│                          │
      │                    │                                                │                          │
      │                    │                                                │ 15. 放行并向集群执行命令  │
      │                    │                                                │─────────────────────────>│
      │                    │                                                │                          │
      │                    │ 16. 执行完成，通知 Server 任务结束             │                          │
      │                    |<──────────────────────────────────────────────────────────────────────────│
      │                    │                                                │                          │
      │                    │ 17. 强行物理销毁沙箱，抹除所有临时凭证与残留数据 │                          │
      │                    ├───────────────────────────────────────────────>│                          │
+-----┴------+     +-------┴--------+     +-------┴-------+     +-----------┴-----------+     +--------┴---------+

```

---

## ⚙️ 文档五：配置设计说明 (Configuration Design)

Ballast 的核心原则是“一切皆资产，一切皆可配”。我们坚决不在核心微内核中写死任何特定的运维组件和安全规则。所有的路由、沙箱、MCP、以及安全拦截红线，全部通过极其优雅、可读的 YAML 和标准 Rego 语言向 SRE 团队全面开放。

### 1. 主配置文件示例 (`/etc/ballast/ballast.yaml`)

该文件定义了管理端的全局环境，包括基础架构组件对接和全局大模型默认路由。

```yaml
server:
  address: "0.0.0.0:8080"
  environment: "production"
  jwt_secret: "ballast-crypto-sign-key-change-me"

# 任务隔离运行时配置 (可扩展实现 SPI 接口)
runtime_provider:
  type: "docker" # 可选: 'docker', 'podman', 'e2b_microvm'
  config:
    max_cpu_cores: 2
    max_memory_mb: 2048
    default_image: "harbor.internal.com/sre/ballast-runner-base:v1.0"
    workspace_root: "/tmp/ballast/sandboxes"

# 外部凭证中心对接 (用于 JIT 凭证分发)
credential_center:
  provider: "hashicorp_vault"
  endpoint: "https://vault.internal.com:8200"
  auth_token_env: "VAULT_TOKEN"

# 统一大模型分发适配层 (Model Runtime Router)
model_router:
  default_provider: "deepseek"
  providers:
    deepseek:
      api_base: "https://api.deepseek.com/v1"
      api_key_env: "DEEPSEEK_API_KEY"
      default_model: "deepseek-reasoner" # 强推理模型用于 Plan 阶段
    openai:
      api_base: "https://api.openai.com/v1"
      api_key_env: "OPENAI_API_KEY"
      default_model: "gpt-4o"

```

### 2. 自动化任务路由与规则配置文件 (`/etc/ballast/rules/trigger_routes.yaml`)

该文件用于配置 Webhook 告警或定时 Cron 如何绑定 Agent 镜像和本地固化的 SRE 剧本（Skills）。

```yaml
version: "v1alpha"
trigger_routes:
  # 路由规则一：Prometheus K8s 告警自愈初筛
  - name: "k8s-pod-crash-auto-triage"
    active: true
    source: "prometheus_alertmanager"
    match_labels:
      alertname: "K8sPodCrashLooping"
      severity: "critical"
    action:
      agent_image: "harbor.internal.com/sre/ballast-agent-k8s:v1.0"
      # 动态注入该任务需要的本地 Markdown Skill 剧本资产
      inject_skills:
        - "k8s-log-harvester"
        - "k8s-event-analyzer"
      # 挂载的开放 MCP 能力插件
      mcp_plugins:
        - "k8sgpt-mcp-server"
        - "prometheus-metric-reader"
      security_policy_group: "k8s_prod_write_intercept"

  # 路由规则二：每日凌晨 IaC 配置漂移自动巡检
  - name: "daily-iac-drift-inspection"
    active: true
    source: "cron"
    cron_expression: "0 2 * * *" # 每晚凌晨两点
    action:
      agent_image: "harbor.internal.com/sre/ballast-agent-terraform:v1.0"
      inject_skills:
        - "tf-drift-detector"
      mcp_plugins: []
      security_policy_group: "git_pr_only_policy"

```

### 3. OPA 安全策略配置文件 (`/etc/ballast/policies/k8s_prod_write_intercept.rego`)

Ballast 采用 CNCF 顶级开源项目 **Open Policy Agent (OPA)** 语法进行命令级安全内省。二开团队只需更新 Rego 规则，即可实现零重启、免编译的代码变更卡口定义。

```rego
package ballast.security

# 默认状态：所有行为拒绝，除非命中放行白名单
default allow = false
default action = "SUSPEND" # 触发安全策略时的默认首选行为：挂起进程进入人工审批断点

# 规则一：绝对允许放行名单 (只读和安全的 Git 操作)
allow {
    is_safe_command(input.command)
}

# 规则二：绝对阻断黑名单 (任何环境下 AI 引擎绝对不能碰的红线命令)
action = "DENY" {
    blacklist_commands[_] == input.command
}

# 规则三：触发人工审批断点的灰名单 (允许 AI 在沙箱内 Plan，但执行必须截获并悬挂)
action = "SUSPEND" {
    not is_safe_command(input.command)
    graylist_commands[_] == input.command
}

# --- 策略底座元数据声明 ---

is_safe_command(cmd) {
    safe_commands[_] == cmd
}

# 只读命令白名单 (Auto-Run)
safe_commands = [
    "kubectl get", "kubectl logs", "kubectl describe", "kubectl top",
    "git status", "git diff", "git log",
    "terraform plan", "terraform validate",
    "ls", "cat", "grep", "awk"
]

# 危险命令黑名单 (Blocked)
blacklist_commands = [
    "rm -rf /", "mkfs", "fdisk", "dd",
    "kubectl delete namespace", "kubectl delete clusterrolebinding",
    "shutdown", "reboot"
]

# 高危变更灰名单 (Human-in-the-loop Intercept)
graylist_commands = [
    "kubectl apply", "kubectl delete", "kubectl patch", "kubectl edit",
    "terraform apply", "terraform destroy",
    "git push", "helm upgrade", "helm uninstall"
]

```

