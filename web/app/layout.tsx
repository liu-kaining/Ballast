import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Ballast — AI SRE Safety Harness",
  description: "安全台架：告警接入 -> 隔离沙箱 -> 意图驱动排障 -> 变更策略拦截 -> 人工断点审批",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
