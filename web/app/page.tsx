import Link from "next/link";

export default function HomePage() {
  return (
    <main style={{ maxWidth: 960, margin: "0 auto", padding: "64px 24px" }}>
      <h1 style={{ fontSize: 32, marginBottom: 8 }}>Ballast</h1>
      <p style={{ color: "var(--muted)", marginBottom: 32 }}>
        AI 时代云原生基础设施的安全自愈与演进底座 (Harness)。
      </p>
      <nav style={{ display: "flex", gap: 16, flexWrap: "wrap" }}>
        <Link
          href="/sessions"
          style={{
            padding: "12px 20px",
            background: "var(--panel)",
            border: "1px solid var(--border)",
            borderRadius: 8,
          }}
        >
          进入自主运维工作区 →
        </Link>
      </nav>

      <section style={{ marginTop: 48, color: "var(--muted)", fontSize: 14, lineHeight: 1.8 }}>
        <p>当前范围：控制面 + Docker 沙箱 + Harness-Agent + OPA 审批 + Skill/MCP 资产挂载 + Webhook/Cron 自动触发。</p>
        <p>真实 OpenCode CLI、真实 Vault、Web-TTY 双向接管和 Git PR 自动提交保持可插拔扩展。</p>
      </section>
    </main>
  );
}
