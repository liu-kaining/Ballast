"use client";

interface Props {
  status: string;
  pendingCommand?: string;
  pendingDecision?: string;
  onApprove: () => void;
  onDestroy: () => void;
  busy?: boolean;
}

// ApproveBar 右侧审批栏：展示被挂起的命令与策略决策，提供放行/销毁按钮。
export default function ApproveBar({
  status,
  pendingCommand,
  pendingDecision,
  onApprove,
  onDestroy,
  busy,
}: Props) {
  const suspended = status === "SUSPENDED";
  const canDestroy = status === "RUNNING" || status === "SUSPENDED";
  return (
    <div
      style={{
        height: "100%",
        padding: 16,
        display: "flex",
        flexDirection: "column",
        gap: 16,
        overflow: "auto",
      }}
    >
      <h3 style={{ marginTop: 0, fontSize: 14, color: "var(--muted)" }}>
        人工断点审批
      </h3>

      <Section label="会话状态">
        <StatusBadge status={status} />
      </Section>

      <Section label="待审批命令">
        <pre
          style={{
            margin: 0,
            padding: 12,
            background: "var(--panel-2)",
            border: "1px solid var(--border)",
            borderRadius: 6,
            fontFamily: "var(--mono)",
            fontSize: 13,
            color: pendingDecision === "SUSPEND" ? "var(--warn)" : "var(--text)",
            whiteSpace: "pre-wrap",
            minHeight: 48,
          }}
        >
          {pendingCommand || "（暂无被拦截的命令）"}
        </pre>
      </Section>

      {pendingDecision && (
        <Section label="策略决策">
          <code
            style={{
              color:
                pendingDecision === "DENY"
                  ? "var(--danger)"
                  : pendingDecision === "SUSPEND"
                  ? "var(--warn)"
                  : "var(--ok)",
              fontWeight: 700,
            }}
          >
            {pendingDecision}
          </code>
        </Section>
      )}

      <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: "auto" }}>
        <button
          onClick={onApprove}
          disabled={!suspended || busy}
          style={btn(suspended && !busy, true)}
        >
          {busy ? "处理中..." : "Approve 放行"}
        </button>
        <button onClick={onDestroy} disabled={busy || !canDestroy} style={btn(!busy && canDestroy, false)}>
          销毁沙箱
        </button>
      </div>

      <p style={{ color: "var(--muted)", fontSize: 12, lineHeight: 1.6 }}>
        Ballast 红线：每一步生产写操作必须经人类点击 Approve 放行。AI 永远是副驾驶。
      </p>
    </div>
  );
}

function Section({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>{label}</div>
      {children}
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const color =
    status === "RUNNING"
      ? "var(--ok)"
      : status === "SUSPENDED"
      ? "var(--warn)"
      : status === "FAILED"
      ? "var(--danger)"
      : "var(--accent)";
  return (
    <code style={{ color, fontWeight: 700, fontFamily: "var(--mono)" }}>{status}</code>
  );
}

function btn(enabled: boolean, primary: boolean): React.CSSProperties {
  return {
    padding: "12px 16px",
    borderRadius: 6,
    cursor: enabled ? "pointer" : "not-allowed",
    fontWeight: 700,
    border: "1px solid var(--border)",
    background: primary ? "var(--ok)" : "var(--panel-2)",
    color: primary ? "#0b1020" : "var(--text)",
    opacity: enabled ? 1 : 0.5,
  };
}
