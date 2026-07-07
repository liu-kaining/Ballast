"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import {
  listSessions,
  createSession,
  login,
  logout,
  APIError,
  errorMessage,
  type Session,
  type SessionStatus,
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

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const list = await listSessions();
      setSessions(list);
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
      await createSession(title);
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
