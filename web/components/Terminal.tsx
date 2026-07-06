"use client";

import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

interface Props {
  // 已渲染的终端行（每项一行）。新增行会追加写入。
  lines: string[];
}

// Terminal 用 xterm.js 渲染只读 TTY 流。
// v0.1 仅只读；v0.2 接入真实 opencode 后开放双向写。
export default function Terminal({ lines }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const writtenRef = useRef(0);

  useEffect(() => {
    if (!containerRef.current || termRef.current) return;
    const term = new XTerm({
      convertEol: true,
      fontSize: 13,
      fontFamily: "var(--mono)",
      theme: {
        background: "#0b1020",
        foreground: "#e6ecff",
        cursor: "transparent",
      },
      cursorBlink: false,
      disableStdin: true,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(containerRef.current);
    fit.fit();
    termRef.current = term;
    fitRef.current = fit;

    const onResize = () => fit.fit();
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("resize", onResize);
      term.dispose();
      termRef.current = null;
    };
  }, []);

  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    for (let i = writtenRef.current; i < lines.length; i++) {
      term.writeln(lines[i] ?? "");
    }
    writtenRef.current = lines.length;
  }, [lines]);

  return (
    <div
      ref={containerRef}
      style={{ height: "100%", padding: 8, background: "#0b1020", overflow: "hidden" }}
    />
  );
}
