"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import dynamic from "next/dynamic";
import ReasonTree from "@/components/ReasonTree";
import ApproveBar from "@/components/ApproveBar";
import {
  approveSession,
  destroySession,
  execManualCommand,
  getSession,
  listAuditLogs,
  listSessionEvents,
  sessionWSURL,
  errorMessage,
  type AuditLog,
  type EventEnvelope,
  type Session,
} from "@/lib/api";

const Terminal = dynamic(() => import("@/components/Terminal"), {
  ssr: false,
  loading: () => <div style={{ padding: 16, color: "var(--muted)" }}>加载终端...</div>,
});

export default function SessionWorkspacePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const [id, setId] = useState<string>("");
  const [session, setSession] = useState<Session | null>(null);
  const [events, setEvents] = useState<EventEnvelope[]>([]);
  const [pendingCommand, setPendingCommand] = useState<string | undefined>();
  const [pendingDecision, setPendingDecision] = useState<string | undefined>();
  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([]);
  const [busy, setBusy] = useState(false);
  const [manualCommand, setManualCommand] = useState("kubectl get pods -n ballast-demo");
  const [manualBusy, setManualBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  // 解析 params（Next 15 Promise）
  useEffect(() => {
    params.then((p) => setId(p.id));
  }, [params]);

  // 拉取会话元信息
  const refreshSession = useCallback(async () => {
    if (!id) return;
    try {
      const s = await getSession(id);
      setSession(s);
    } catch (e: unknown) {
      setError(errorMessage(e));
    }
  }, [id]);

  useEffect(() => {
    refreshSession();
  }, [refreshSession]);

  const refreshAudit = useCallback(async () => {
    if (!id) return;
    try {
      const logs = await listAuditLogs(id);
      setAuditLogs(logs);
    } catch (e: unknown) {
      setError(errorMessage(e));
    }
  }, [id]);

  useEffect(() => {
    refreshAudit();
  }, [refreshAudit]);

  const refreshEvents = useCallback(async () => {
    if (!id) return;
    try {
      const persisted = await listSessionEvents(id);
      setEvents((prev) => mergeEvents(persisted, prev));
    } catch (e: unknown) {
      setError(errorMessage(e));
    }
  }, [id]);

  useEffect(() => {
    refreshEvents();
  }, [refreshEvents]);

  // 订阅 WebSocket 事件流
  useEffect(() => {
    if (!id) return;
    const ws = new WebSocket(sessionWSURL(id));
    wsRef.current = ws;
    ws.onmessage = (msg) => {
      try {
        const env: EventEnvelope = JSON.parse(msg.data);
        setEvents((prev) => mergeEvents(prev, [env]));
        if (env.type === "policy.decision" && env.data) {
          setPendingCommand(
            typeof env.data.command === "string" ? env.data.command : undefined
          );
          setPendingDecision(
            typeof env.data.decision === "string" ? env.data.decision : undefined
          );
        }
        if (env.type === "policy.resumed") {
          setPendingDecision(undefined);
          setPendingCommand(undefined);
        }
        if (env.type === "session.completed" || env.type === "session.destroyed") {
          setPendingDecision(undefined);
          setPendingCommand(undefined);
        }
        if (
          env.type === "policy.decision" ||
          env.type === "policy.resumed" ||
          env.type === "session.completed" ||
          env.type === "session.destroyed"
        ) {
          void refreshSession();
          void refreshAudit();
        }
      } catch {
        /* ignore malformed */
      }
    };
    ws.onerror = () => setError("WebSocket 连接异常");
    return () => ws.close();
  }, [id, refreshSession, refreshAudit]);

  // 状态变化时刷新会话元信息
  useEffect(() => {
    refreshSession();
    const t = setInterval(refreshSession, 3000);
    return () => clearInterval(t);
  }, [refreshSession]);

  const terminalLines = useMemo(() => toTerminalLines(events), [events]);

  async function handleApprove() {
    if (!id) return;
    setBusy(true);
    try {
      await approveSession(id);
      await refreshAudit();
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleDestroy() {
    if (!id) return;
    setBusy(true);
    try {
      await destroySession(id);
      await refreshSession();
      await refreshAudit();
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleManualExec() {
    if (!id || !manualCommand.trim()) return;
    setManualBusy(true);
    setError(null);
    try {
      const result = await execManualCommand(id, manualCommand.trim());
      setEvents((prev) =>
        mergeEvents(prev, [
          {
            type: "manual.command",
            data: {
              command: result.command,
              stdout: result.stdout,
              stderr: result.stderr,
              error: result.error,
              policy_decision: result.policy_decision,
              approver: result.approver,
            },
          },
        ])
      );
      await refreshAudit();
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setManualBusy(false);
    }
  }

  if (!id) return <main style={{ padding: 24 }}>加载中...</main>;

  return (
    <main
      style={{
        height: "100vh",
        display: "flex",
        flexDirection: "column",
        padding: 16,
        gap: 12,
      }}
    >
      <header
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          gap: 16,
        }}
      >
        <div>
          <h1 style={{ fontSize: 18, margin: 0 }}>{session?.title ?? id}</h1>
          <code style={{ color: "var(--muted)", fontSize: 12 }}>{id}</code>
        </div>
        {error && (
          <code style={{ color: "var(--danger)", fontSize: 12 }}>{error}</code>
        )}
      </header>

      <section
        style={{
          flex: 1,
          display: "grid",
          gridTemplateColumns: "320px 1fr 320px",
          gap: 12,
          minHeight: 0,
        }}
      >
        <Pane title="Reason Tree">
          <ReasonTree events={events} />
        </Pane>
        <Pane title="Web-TTY + Takeover">
          <div style={{ height: "100%", display: "flex", flexDirection: "column" }}>
            <div style={{ flex: 1, minHeight: 0 }}>
              <Terminal lines={terminalLines} />
            </div>
            <ManualTakeover
              command={manualCommand}
              onCommandChange={setManualCommand}
              onExec={handleManualExec}
              busy={manualBusy}
              disabled={!session || session.status === "SUCCESS" || session.status === "FAILED"}
            />
          </div>
        </Pane>
        <Pane title="Approve">
          <ApproveBar
            status={session?.status ?? "RUNNING"}
            pendingCommand={pendingCommand}
            pendingDecision={pendingDecision}
            onApprove={handleApprove}
            onDestroy={handleDestroy}
            busy={busy}
          />
          <AuditTrail logs={auditLogs} />
        </Pane>
      </section>
    </main>
  );
}

function ManualTakeover({
  command,
  onCommandChange,
  onExec,
  busy,
  disabled,
}: {
  command: string;
  onCommandChange: (value: string) => void;
  onExec: () => void;
  busy: boolean;
  disabled: boolean;
}) {
  return (
    <div
      style={{
        borderTop: "1px solid var(--border)",
        padding: 10,
        display: "grid",
        gap: 8,
        background: "rgba(15, 23, 42, 0.55)",
      }}
    >
      <div style={{ color: "var(--muted)", fontSize: 12 }}>
        人工接管：在当前沙箱内执行一次命令；DENY 策略仍会阻断，执行结果进入审计。
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        <input
          value={command}
          onChange={(event) => onCommandChange(event.target.value)}
          onKeyDown={(event) => event.key === "Enter" && onExec()}
          disabled={disabled || busy}
          placeholder="kubectl get pods -n ballast-demo"
          style={{
            flex: 1,
            background: "var(--panel-2)",
            border: "1px solid var(--border)",
            borderRadius: 6,
            color: "var(--text)",
            padding: "9px 10px",
            fontFamily: "var(--mono)",
            fontSize: 12,
          }}
        />
        <button
          onClick={onExec}
          disabled={disabled || busy || !command.trim()}
          style={{
            padding: "9px 12px",
            borderRadius: 6,
            border: "1px solid var(--border)",
            background: "var(--panel-2)",
            color: "var(--text)",
            cursor: disabled || busy || !command.trim() ? "not-allowed" : "pointer",
            opacity: disabled || busy || !command.trim() ? 0.5 : 1,
            fontWeight: 700,
          }}
        >
          {busy ? "执行中..." : "接管执行"}
        </button>
      </div>
    </div>
  );
}

function mergeEvents(existing: EventEnvelope[], incoming: EventEnvelope[]): EventEnvelope[] {
  const out: EventEnvelope[] = [];
  const seen = new Set<string>();
  for (const ev of [...existing, ...incoming]) {
    const key = `${ev.type}:${JSON.stringify(ev.data ?? {})}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(ev);
  }
  return out;
}

function Pane({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div
      style={{
        background: "var(--panel)",
        border: "1px solid var(--border)",
        borderRadius: 8,
        minHeight: 0,
        display: "flex",
        flexDirection: "column",
        overflow: "hidden",
      }}
    >
      <div
        style={{
          padding: "8px 14px",
          borderBottom: "1px solid var(--border)",
          fontSize: 12,
          color: "var(--muted)",
          textTransform: "uppercase",
        }}
      >
        {title}
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>{children}</div>
    </div>
  );
}

function AuditTrail({ logs }: { logs: AuditLog[] }) {
  return (
    <div
      style={{
        borderTop: "1px solid var(--border)",
        padding: 12,
        display: "grid",
        gap: 8,
        maxHeight: 260,
        overflow: "auto",
      }}
    >
      <div style={{ color: "var(--muted)", fontSize: 12, textTransform: "uppercase" }}>
        Audit Trail
      </div>
      {logs.length === 0 ? (
        <div style={{ color: "var(--muted)", fontSize: 13 }}>暂无审计记录</div>
      ) : (
        logs.map((log) => (
          <div
            key={log.audit_id}
            style={{
              background: "var(--panel-2)",
              border: "1px solid var(--border)",
              borderRadius: 6,
              padding: 8,
              fontSize: 12,
            }}
          >
            <code style={{ color: decisionColor(log.policy_decision) }}>
              {log.policy_decision || "EVENT"}
            </code>
            <div style={{ marginTop: 4 }}>{log.executed_command || "(no command)"}</div>
            {log.approver && (
              <div style={{ color: "var(--muted)", marginTop: 4 }}>approver={log.approver}</div>
            )}
          </div>
        ))
      )}
    </div>
  );
}

function decisionColor(decision: string): string {
  if (decision === "DENY") return "var(--danger)";
  if (decision === "SUSPEND") return "var(--warn)";
  if (decision === "APPROVE") return "var(--ok)";
  return "var(--muted)";
}

function toTerminalLines(events: EventEnvelope[]): string[] {
  const lines: string[] = [];
  for (const ev of events) {
    switch (ev.type) {
      case "server.connected":
        {
          const p = ev.data || {};
          const server = typeof p.server === "string" ? p.server : "runner";
          const version = typeof p.version === "string" ? ` v${p.version}` : "";
          lines.push(`\x1b[36m[connected] ${server}${version}\x1b[0m`);
        }
        break;
      case "reason.step": {
        const p = ev.data || {};
        lines.push(`\x1b[35m#${p.index} ${p.title}\x1b[0m`);
        if (typeof p.thought === "string") lines.push(`  ${p.thought}`);
        break;
      }
      case "tool.call": {
        const p = ev.data || {};
        lines.push(`\x1b[33m$\x1b[0m ${p.command}`);
        if (typeof p.stdout === "string") lines.push(p.stdout);
        break;
      }
      case "tool.result": {
        const p = ev.data || {};
        if (typeof p.stdout === "string") lines.push(p.stdout);
        if (p.stderr) lines.push(`\x1b[31m${p.stderr}\x1b[0m`);
        break;
      }
      case "policy.decision": {
        const p = ev.data || {};
        const color =
          p.decision === "DENY"
            ? "\x1b[31m"
            : p.decision === "SUSPEND"
            ? "\x1b[33m"
            : "\x1b[32m";
        lines.push(`${color}[policy] ${p.decision}\x1b[0m  command=${p.command}`);
        break;
      }
      case "policy.resumed":
        lines.push(`\x1b[32m[policy] resumed by ${ev.data?.approver}\x1b[0m`);
        break;
      case "manual.command": {
        const p = ev.data || {};
        lines.push(`\x1b[36m[manual]\x1b[0m ${p.command}`);
        if (typeof p.stdout === "string" && p.stdout) lines.push(p.stdout);
        if (typeof p.stderr === "string" && p.stderr) lines.push(`\x1b[31m${p.stderr}\x1b[0m`);
        if (typeof p.error === "string" && p.error) lines.push(`\x1b[31m[manual error] ${p.error}\x1b[0m`);
        break;
      }
      case "message.completed": {
        const p = ev.data || {};
        if (typeof p.text === "string") lines.push(`\x1b[36m[assistant] ${p.text}\x1b[0m`);
        break;
      }
      default:
        break;
    }
  }
  return lines;
}
