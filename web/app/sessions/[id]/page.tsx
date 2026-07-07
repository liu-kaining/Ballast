"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import dynamic from "next/dynamic";
import ReasonTree from "@/components/ReasonTree";
import ApproveBar from "@/components/ApproveBar";
import {
  approveSession,
  destroySession,
  getSession,
  sessionWSURL,
  errorMessage,
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
  const [busy, setBusy] = useState(false);
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

  // 订阅 WebSocket 事件流
  useEffect(() => {
    if (!id) return;
    const ws = new WebSocket(sessionWSURL(id));
    wsRef.current = ws;
    ws.onmessage = (msg) => {
      try {
        const env: EventEnvelope = JSON.parse(msg.data);
        setEvents((prev) => [...prev, env]);
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
        }
      } catch {
        /* ignore malformed */
      }
    };
    ws.onerror = () => setError("WebSocket 连接异常");
    return () => ws.close();
  }, [id, refreshSession]);

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
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
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
        <Pane title="Web-TTY (只读流)">
          <Terminal lines={terminalLines} />
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
        </Pane>
      </section>
    </main>
  );
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

function toTerminalLines(events: EventEnvelope[]): string[] {
  const lines: string[] = [];
  for (const ev of events) {
    switch (ev.type) {
      case "server.connected":
        lines.push("\x1b[36m[connected] mock-opencode v0.1\x1b[0m");
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
