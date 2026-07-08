"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  errorMessage,
  listMCPPlugins,
  listSkills,
  listTriggerRules,
  upsertMCPPlugin,
  upsertSkill,
  upsertTriggerRule,
  type MCPPlugin,
  type Skill,
  type TriggerRule,
} from "@/lib/api";

type Tab = "skill" | "trigger" | "mcp";

const SAMPLE_SKILL: Skill = {
  skill_id: "k8s-debug",
  name: "K8s CrashLoop Debug",
  description: "CrashLoopBackOff 安全排障 Skill。",
  trigger_words: ["k8s", "pod", "crashloop"],
  markdown_content:
    "---\nname: k8s-debug\nsummary: Diagnose Kubernetes CrashLoopBackOff safely\n---\n# K8s CrashLoop Debug\n\n1. 先执行只读 kubectl get/describe/logs。\n2. 写操作必须等待 Ballast 审批。\n",
  version: 1,
  updated_by: "operator",
};

const SAMPLE_TRIGGER: TriggerRule = {
  rule_id: "k8s-crashloop",
  name: "K8s CrashLoopBackOff triage",
  is_active: true,
  trigger_source: "prometheus_alertmanager",
  match_expression: { alertname: "K8sPodCrashLooping", severity: "critical" },
  bind_skills: ["k8s-debug"],
  agent_image: "ballast-runner-base:dev",
  policy_group: "k8s_prod_write_intercept",
};

const SAMPLE_MCP: MCPPlugin = {
  plugin_id: "prometheus",
  name: "Prometheus MCP",
  command: "prometheus-mcp",
  args: ["--stdio"],
  env: { PROM_URL: "http://prometheus:9090" },
  is_active: true,
  updated_by: "operator",
};

export default function AssetsPage() {
  const [tab, setTab] = useState<Tab>("skill");
  const [skills, setSkills] = useState<Skill[]>([]);
  const [triggers, setTriggers] = useState<TriggerRule[]>([]);
  const [mcps, setMCPs] = useState<MCPPlugin[]>([]);
  const [skillDraft, setSkillDraft] = useState<Skill>(SAMPLE_SKILL);
  const [triggerJSON, setTriggerJSON] = useState(formatJSON(SAMPLE_TRIGGER));
  const [mcpJSON, setMCPJSON] = useState(formatJSON(SAMPLE_MCP));
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const [skillList, triggerList, mcpList] = await Promise.all([
        listSkills(),
        listTriggerRules(),
        listMCPPlugins(),
      ]);
      setSkills(skillList);
      setTriggers(triggerList);
      setMCPs(mcpList);
      if (skillList.length > 0) setSkillDraft(skillList[0]);
      if (triggerList.length > 0) setTriggerJSON(formatJSON(triggerList[0]));
      if (mcpList.length > 0) setMCPJSON(formatJSON(mcpList[0]));
    } catch (e: unknown) {
      setError(errorMessage(e));
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const activeList = useMemo(() => {
    if (tab === "skill") return skills.map((item) => ({ id: item.skill_id, label: item.name }));
    if (tab === "trigger") return triggers.map((item) => ({ id: item.rule_id, label: item.name }));
    return mcps.map((item) => ({ id: item.plugin_id, label: item.name }));
  }, [mcps, skills, tab, triggers]);

  async function saveSkill() {
    await save(async () => {
      await upsertSkill({
        ...skillDraft,
        trigger_words: skillDraft.trigger_words.map((item) => item.trim()).filter(Boolean),
        updated_by: skillDraft.updated_by || "operator",
      });
    });
  }

  async function saveTrigger() {
    await save(async () => upsertTriggerRule(JSON.parse(triggerJSON) as TriggerRule));
  }

  async function saveMCP() {
    await save(async () => upsertMCPPlugin(JSON.parse(mcpJSON) as MCPPlugin));
  }

  async function save(fn: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    setMessage(null);
    try {
      await fn();
      setMessage("已保存并刷新资产列表。");
      await refresh();
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  function selectAsset(id: string) {
    if (tab === "skill") {
      const item = skills.find((skill) => skill.skill_id === id);
      if (item) setSkillDraft(item);
    } else if (tab === "trigger") {
      const item = triggers.find((rule) => rule.rule_id === id);
      if (item) setTriggerJSON(formatJSON(item));
    } else {
      const item = mcps.find((plugin) => plugin.plugin_id === id);
      if (item) setMCPJSON(formatJSON(item));
    }
  }

  return (
    <main style={{ maxWidth: 1200, margin: "0 auto", padding: "40px 24px" }}>
      <header style={{ display: "flex", justifyContent: "space-between", gap: 16, marginBottom: 24 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 24 }}>资产与扩展中枢</h1>
          <p style={{ color: "var(--muted)", marginBottom: 0 }}>
            编辑 Skill、自动化触发路由和 MCP 插件；保存后会话拉起时即可注入沙箱。
          </p>
        </div>
        <nav style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <Link href="/sessions" style={buttonStyle(false)}>返回会话</Link>
          <button onClick={refresh} disabled={busy} style={buttonStyle(false)}>刷新</button>
        </nav>
      </header>

      <section style={{ display: "flex", gap: 8, marginBottom: 16 }}>
        <TabButton active={tab === "skill"} onClick={() => setTab("skill")}>Skill IDE</TabButton>
        <TabButton active={tab === "trigger"} onClick={() => setTab("trigger")}>触发路由</TabButton>
        <TabButton active={tab === "mcp"} onClick={() => setTab("mcp")}>MCP 插件</TabButton>
      </section>

      {error && <Notice color="var(--danger)">{error}</Notice>}
      {message && <Notice color="var(--ok)">{message}</Notice>}

      <section style={{ display: "grid", gridTemplateColumns: "280px 1fr", gap: 16 }}>
        <aside style={panelStyle()}>
          <h2 style={{ fontSize: 14, marginTop: 0 }}>资产列表</h2>
          {activeList.length === 0 ? (
            <p style={{ color: "var(--muted)", fontSize: 13 }}>暂无资产，可直接编辑右侧模板后保存。</p>
          ) : (
            <div style={{ display: "grid", gap: 8 }}>
              {activeList.map((item) => (
                <button key={item.id} onClick={() => selectAsset(item.id)} style={assetButtonStyle()}>
                  <strong>{item.label}</strong>
                  <code style={{ display: "block", color: "var(--muted)", marginTop: 4 }}>{item.id}</code>
                </button>
              ))}
            </div>
          )}
        </aside>

        <section style={panelStyle()}>
          {tab === "skill" && (
            <SkillEditor draft={skillDraft} onChange={setSkillDraft} onSave={saveSkill} busy={busy} />
          )}
          {tab === "trigger" && (
            <JSONEditor
              title="触发路由 JSON"
              value={triggerJSON}
              onChange={setTriggerJSON}
              onSave={saveTrigger}
              busy={busy}
              hint="match_expression 支持匹配 Alertmanager labels；bind_skills 绑定已有 Skill ID。"
            />
          )}
          {tab === "mcp" && (
            <JSONEditor
              title="MCP 插件 JSON"
              value={mcpJSON}
              onChange={setMCPJSON}
              onSave={saveMCP}
              busy={busy}
              hint="command 必须是单个可执行文件名，args/env 会生成 mcp_config.json 并挂载进沙箱。"
            />
          )}
        </section>
      </section>
    </main>
  );
}

function SkillEditor({
  draft,
  onChange,
  onSave,
  busy,
}: {
  draft: Skill;
  onChange: (value: Skill) => void;
  onSave: () => void;
  busy: boolean;
}) {
  return (
    <div style={{ display: "grid", gap: 12 }}>
      <h2 style={{ fontSize: 16, margin: 0 }}>Skill IDE</h2>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
        <Field label="Skill ID">
          <input value={draft.skill_id} onChange={(e) => onChange({ ...draft, skill_id: e.target.value })} style={inputStyle()} />
        </Field>
        <Field label="名称">
          <input value={draft.name} onChange={(e) => onChange({ ...draft, name: e.target.value })} style={inputStyle()} />
        </Field>
        <Field label="触发词（逗号分隔）">
          <input
            value={draft.trigger_words.join(", ")}
            onChange={(e) => onChange({ ...draft, trigger_words: e.target.value.split(",") })}
            style={inputStyle()}
          />
        </Field>
        <Field label="更新人">
          <input value={draft.updated_by} onChange={(e) => onChange({ ...draft, updated_by: e.target.value })} style={inputStyle()} />
        </Field>
      </div>
      <Field label="描述">
        <input value={draft.description} onChange={(e) => onChange({ ...draft, description: e.target.value })} style={inputStyle()} />
      </Field>
      <Field label="SKILL.md">
        <textarea
          value={draft.markdown_content}
          onChange={(e) => onChange({ ...draft, markdown_content: e.target.value })}
          rows={18}
          style={textareaStyle()}
        />
      </Field>
      <button onClick={onSave} disabled={busy} style={buttonStyle(true)}>
        {busy ? "保存中..." : "保存 Skill"}
      </button>
    </div>
  );
}

function JSONEditor({
  title,
  value,
  onChange,
  onSave,
  busy,
  hint,
}: {
  title: string;
  value: string;
  onChange: (value: string) => void;
  onSave: () => void;
  busy: boolean;
  hint: string;
}) {
  return (
    <div style={{ display: "grid", gap: 12 }}>
      <h2 style={{ fontSize: 16, margin: 0 }}>{title}</h2>
      <p style={{ color: "var(--muted)", fontSize: 13, margin: 0 }}>{hint}</p>
      <textarea value={value} onChange={(e) => onChange(e.target.value)} rows={26} style={textareaStyle()} />
      <button onClick={onSave} disabled={busy} style={buttonStyle(true)}>
        {busy ? "保存中..." : "保存"}
      </button>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label style={{ display: "grid", gap: 6 }}>
      <span style={{ color: "var(--muted)", fontSize: 12 }}>{label}</span>
      {children}
    </label>
  );
}

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button onClick={onClick} style={buttonStyle(active)}>
      {children}
    </button>
  );
}

function Notice({ color, children }: { color: string; children: React.ReactNode }) {
  return (
    <div style={{ color, border: `1px solid ${color}`, borderRadius: 8, padding: 10, marginBottom: 12 }}>
      {children}
    </div>
  );
}

function formatJSON(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

function panelStyle(): React.CSSProperties {
  return {
    background: "var(--panel)",
    border: "1px solid var(--border)",
    borderRadius: 8,
    padding: 16,
  };
}

function inputStyle(): React.CSSProperties {
  return {
    background: "var(--panel-2)",
    border: "1px solid var(--border)",
    borderRadius: 6,
    color: "var(--text)",
    padding: "9px 10px",
  };
}

function textareaStyle(): React.CSSProperties {
  return {
    ...inputStyle(),
    minHeight: 220,
    fontFamily: "var(--mono)",
    fontSize: 12,
    lineHeight: 1.5,
  };
}

function buttonStyle(active: boolean): React.CSSProperties {
  return {
    background: active ? "var(--accent)" : "var(--panel-2)",
    color: active ? "#0b1020" : "var(--text)",
    border: "1px solid var(--border)",
    borderRadius: 6,
    padding: "10px 14px",
    cursor: "pointer",
    fontWeight: 700,
    textDecoration: "none",
  };
}

function assetButtonStyle(): React.CSSProperties {
  return {
    textAlign: "left",
    background: "var(--panel-2)",
    color: "var(--text)",
    border: "1px solid var(--border)",
    borderRadius: 6,
    padding: 10,
    cursor: "pointer",
  };
}
