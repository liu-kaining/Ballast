"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import {
  listSessions,
  createSession,
  listSkills,
  listTriggerRules,
  listMCPPlugins,
  upsertSkill,
  upsertTriggerRule,
  upsertMCPPlugin,
  login,
  logout,
  APIError,
  errorMessage,
  type Session,
  type SessionStatus,
  type Skill,
  type TriggerRule,
  type MCPPlugin,
} from "@/lib/api";

const STATUS_COLORS: Record<SessionStatus, string> = {
  RUNNING: "var(--ok)",
  SUSPENDED: "var(--warn)",
  SUCCESS: "var(--accent)",
  FAILED: "var(--danger)",
};

export default function SessionsPage() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState("K8s CrashLoopBackOff 排障");
  const [authRequired, setAuthRequired] = useState(false);
  const [token, setToken] = useState("");
  const [skills, setSkills] = useState<Skill[]>([]);
  const [triggerRules, setTriggerRules] = useState<TriggerRule[]>([]);
  const [mcpPlugins, setMCPPlugins] = useState<MCPPlugin[]>([]);
  const [selectedSkillIDs, setSelectedSkillIDs] = useState<string[]>([]);
  const [selectedMCPPluginIDs, setSelectedMCPPluginIDs] = useState<string[]>([]);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [list, skillList, ruleList, mcpList] = await Promise.all([
        listSessions(),
        listSkills(),
        listTriggerRules(),
        listMCPPlugins(),
      ]);
      setSessions(list);
      setSkills(skillList);
      setTriggerRules(ruleList);
      setMCPPlugins(mcpList);
      setSelectedSkillIDs((current) =>
        current.filter((id) => skillList.some((skill) => skill.skill_id === id))
      );
      setSelectedMCPPluginIDs((current) =>
        current.filter((id) => mcpList.some((plugin) => plugin.plugin_id === id))
      );
    } catch (e: unknown) {
      if (e instanceof APIError && e.status === 401) {
        setAuthRequired(true);
      }
      setError(errorMessage(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  async function handleCreate() {
    setCreating(true);
    setError(null);
    try {
      await createSession(
        title,
        "ballast-runner-base:dev",
        selectedSkillIDs,
        selectedMCPPluginIDs
      );
      await refresh();
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setCreating(false);
    }
  }

  async function handleLogin() {
    setCreating(true);
    setError(null);
    try {
      await login(token);
      setToken("");
      setAuthRequired(false);
      await refresh();
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setCreating(false);
    }
  }

  async function handleLogout() {
    await logout();
    setSessions([]);
    setAuthRequired(true);
  }

  async function handleSeedSkill() {
    setCreating(true);
    setError(null);
    try {
      await upsertSkill({
        skill_id: "k8s-debug",
        name: "K8s CrashLoop Debug",
        description: "用于 CrashLoopBackOff 初筛的本地 Markdown Skill 示例。",
        trigger_words: ["k8s", "pod", "crashloop"],
        markdown_content:
          "---\nname: k8s-debug\nsummary: Diagnose Kubernetes CrashLoopBackOff safely\n---\n# K8s CrashLoop Debug\n\n先执行只读 kubectl get/describe/logs，任何 apply/delete 操作必须等待 Ballast 审批。\n",
        version: 1,
        updated_by: "operator",
      });
      await refresh();
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setCreating(false);
    }
  }

  async function handleSeedRule() {
    setCreating(true);
    setError(null);
    try {
      await upsertTriggerRule({
        rule_id: "k8s-crashloop",
        name: "K8s CrashLoopBackOff triage",
        is_active: true,
        trigger_source: "prometheus_alertmanager",
        match_expression: { alertname: "K8sPodCrashLooping", severity: "critical" },
        bind_skills: ["k8s-debug"],
        agent_image: "ballast-runner-base:dev",
        policy_group: "k8s_prod_write_intercept",
      });
      await refresh();
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setCreating(false);
    }
  }

  async function handleSeedMCPPlugin() {
    setCreating(true);
    setError(null);
    try {
      await upsertMCPPlugin({
        plugin_id: "prometheus",
        name: "Prometheus MCP",
        command: "prometheus-mcp",
        args: ["--stdio"],
        env: { PROM_URL: "http://prometheus:9090" },
        is_active: true,
        updated_by: "operator",
      });
      await refresh();
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setCreating(false);
    }
  }

  function toggleSkill(id: string) {
    setSelectedSkillIDs((current) =>
      current.includes(id) ? current.filter((item) => item !== id) : [...current, id]
    );
  }

  function toggleMCPPlugin(id: string) {
    setSelectedMCPPluginIDs((current) =>
      current.includes(id) ? current.filter((item) => item !== id) : [...current, id]
    );
  }

  return (
    <main style={{ maxWidth: 1100, margin: "0 auto", padding: "48px 24px" }}>
      <header
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: 24,
        }}
      >
        <h1 style={{ fontSize: 24, margin: 0 }}>会话列表</h1>
        <div style={{ display: "flex", gap: 8 }}>
          {!authRequired && <Link href="/assets" style={btnStyle()}>资产中心</Link>}
          {!authRequired && <button onClick={handleLogout} style={btnStyle()}>退出</button>}
          <button onClick={refresh} style={btnStyle()} disabled={loading}>刷新</button>
        </div>
      </header>

      {authRequired && (
        <section
          style={{
            background: "var(--panel)",
            border: "1px solid var(--border)",
            borderRadius: 8,
            padding: 20,
            marginBottom: 24,
          }}
        >
          <h2 style={{ marginTop: 0, fontSize: 18 }}>控制台认证</h2>
          <p style={{ color: "var(--muted)", fontSize: 13 }}>
            输入 Ballast 管理令牌。令牌仅用于登录请求，后续由 HttpOnly 会话 Cookie 认证。
          </p>
          <div style={{ display: "flex", gap: 12 }}>
            <input
              type="password"
              value={token}
              onChange={(event) => setToken(event.target.value)}
              onKeyDown={(event) => event.key === "Enter" && handleLogin()}
              placeholder="管理令牌"
              style={{
                flex: 1,
                background: "var(--panel-2)",
                border: "1px solid var(--border)",
                borderRadius: 6,
                color: "var(--text)",
                padding: "10px 12px",
              }}
            />
            <button onClick={handleLogin} disabled={!token || creating} style={btnStyle(true)}>
              登录
            </button>
          </div>
        </section>
      )}

      {!authRequired && (
      <section
        style={{
          background: "var(--panel)",
          border: "1px solid var(--border)",
          borderRadius: 8,
          padding: 16,
          marginBottom: 24,
          display: "flex",
          gap: 12,
          alignItems: "center",
        }}
      >
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="会话标题 / 意图描述"
          style={{
            flex: 1,
            background: "var(--panel-2)",
            border: "1px solid var(--border)",
            borderRadius: 6,
            color: "var(--text)",
            padding: "10px 12px",
            fontFamily: "inherit",
          }}
        />
        <button onClick={handleCreate} disabled={creating} style={btnStyle(true)}>
          {creating ? "拉起中..." : "拉起沙箱会话"}
        </button>
      </section>
      )}

      {!authRequired && (
        <section
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
            gap: 16,
            marginBottom: 24,
          }}
        >
          <AssetPanel
            title="Skill 资产"
            actionLabel="写入示例 Skill"
            onAction={handleSeedSkill}
            busy={creating}
          >
            {skills.length === 0 ? (
              <p style={mutedStyle()}>暂无 Skill。可写入示例后随会话只读挂载到沙箱。</p>
            ) : (
              skills.map((skill) => (
                <label key={skill.skill_id} style={assetRowStyle()}>
                  <input
                    type="checkbox"
                    checked={selectedSkillIDs.includes(skill.skill_id)}
                    onChange={() => toggleSkill(skill.skill_id)}
                  />
                  <span>
                    <strong>{skill.name}</strong>
                    <code style={{ display: "block", color: "var(--muted)" }}>
                      {skill.skill_id} · v{skill.version}
                    </code>
                  </span>
                </label>
              ))
            )}
          </AssetPanel>
          <AssetPanel
            title="MCP 插件"
            actionLabel="写入示例 MCP"
            onAction={handleSeedMCPPlugin}
            busy={creating}
          >
            {mcpPlugins.length === 0 ? (
              <p style={mutedStyle()}>暂无 MCP 插件。可写入示例后生成 mcp_config.json 挂载进沙箱。</p>
            ) : (
              mcpPlugins.map((plugin) => (
                <label key={plugin.plugin_id} style={assetRowStyle()}>
                  <input
                    type="checkbox"
                    checked={selectedMCPPluginIDs.includes(plugin.plugin_id)}
                    onChange={() => toggleMCPPlugin(plugin.plugin_id)}
                    disabled={!plugin.is_active}
                  />
                  <span>
                    <strong>{plugin.name}</strong>
                    <code style={{ display: "block", color: "var(--muted)" }}>
                      {plugin.plugin_id} · {plugin.command}
                    </code>
                  </span>
                </label>
              ))
            )}
          </AssetPanel>
          <AssetPanel
            title="触发路由"
            actionLabel="写入示例路由"
            onAction={handleSeedRule}
            busy={creating}
          >
            {triggerRules.length === 0 ? (
              <p style={mutedStyle()}>暂无自动化路由。Webhook 入口与 Cron 调度会读取这里的规则。</p>
            ) : (
              triggerRules.map((rule) => (
                <div key={rule.rule_id} style={assetRowStyle()}>
                  <span style={{ color: rule.is_active ? "var(--ok)" : "var(--muted)" }}>
                    ●
                  </span>
                  <span>
                    <strong>{rule.name}</strong>
                    <code style={{ display: "block", color: "var(--muted)" }}>
                      {rule.trigger_source} → {rule.agent_image}
                    </code>
                  </span>
                </div>
              ))
            )}
          </AssetPanel>
        </section>
      )}

      {error && (
        <div
          style={{
            color: "var(--danger)",
            background: "var(--panel)",
            border: "1px solid var(--danger)",
            borderRadius: 8,
            padding: 12,
            marginBottom: 24,
            fontFamily: "var(--mono)",
          }}
        >
          {error}
        </div>
      )}

      {!authRequired && <section
        style={{
          background: "var(--panel)",
          border: "1px solid var(--border)",
          borderRadius: 8,
          overflow: "hidden",
        }}
      >
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ textAlign: "left", color: "var(--muted)" }}>
              <th style={thStyle()}>会话</th>
              <th style={thStyle()}>状态</th>
              <th style={thStyle()}>触发</th>
              <th style={thStyle()}>Agent 镜像</th>
              <th style={thStyle()}>创建时间</th>
            </tr>
          </thead>
          <tbody>
            {sessions.length === 0 && !loading && (
              <tr>
                <td colSpan={5} style={{ ...tdStyle(), color: "var(--muted)" }}>
                  暂无会话。拉起一个沙箱会话开始排障。
                </td>
              </tr>
            )}
            {sessions.map((s) => (
              <tr key={s.session_id} style={{ borderTop: "1px solid var(--border)" }}>
                <td style={tdStyle()}>
                  <Link href={`/sessions/${s.session_id}`}>{s.title}</Link>
                  <div style={{ color: "var(--muted)", fontSize: 12, fontFamily: "var(--mono)" }}>
                    {s.session_id}
                  </div>
                </td>
                <td style={tdStyle()}>
                  <span
                    style={{
                      color: STATUS_COLORS[s.status],
                      fontFamily: "var(--mono)",
                      fontWeight: 600,
                    }}
                  >
                    {s.status}
                  </span>
                </td>
                <td style={tdStyle()}><code>{s.trigger_type}</code></td>
                <td style={tdStyle()}><code style={{ color: "var(--muted)" }}>{s.agent_image}</code></td>
                <td style={tdStyle()}>{new Date(s.created_at).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>}
    </main>
  );
}

function thStyle(): React.CSSProperties {
  return { padding: "12px 14px", fontSize: 12, textTransform: "uppercase" };
}
function tdStyle(): React.CSSProperties {
  return { padding: "12px 14px", fontSize: 14 };
}
function btnStyle(primary = false): React.CSSProperties {
  return {
    background: primary ? "var(--accent)" : "var(--panel-2)",
    color: primary ? "#0b1020" : "var(--text)",
    border: "1px solid var(--border)",
    borderRadius: 6,
    padding: "10px 16px",
    cursor: "pointer",
    fontWeight: 600,
  };
}

function AssetPanel({
  title,
  actionLabel,
  onAction,
  busy,
  children,
}: {
  title: string;
  actionLabel: string;
  onAction: () => void;
  busy: boolean;
  children: React.ReactNode;
}) {
  return (
    <div
      style={{
        background: "var(--panel)",
        border: "1px solid var(--border)",
        borderRadius: 8,
        padding: 16,
      }}
    >
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12 }}>
        <h2 style={{ margin: 0, fontSize: 16 }}>{title}</h2>
        <button onClick={onAction} disabled={busy} style={btnStyle()}>
          {actionLabel}
        </button>
      </div>
      <div style={{ display: "grid", gap: 10, marginTop: 14 }}>{children}</div>
    </div>
  );
}

function assetRowStyle(): React.CSSProperties {
  return {
    display: "flex",
    gap: 10,
    alignItems: "flex-start",
    background: "var(--panel-2)",
    border: "1px solid var(--border)",
    borderRadius: 6,
    padding: 10,
    fontSize: 13,
  };
}

function mutedStyle(): React.CSSProperties {
  return { color: "var(--muted)", margin: 0, fontSize: 13, lineHeight: 1.5 };
}
