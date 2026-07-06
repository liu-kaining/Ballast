"use client";

import { useMemo } from "react";
import type { EventEnvelope } from "@/lib/api";

interface ReasonStep {
  index: number;
  title: string;
  thought: string;
}

interface Props {
  events: EventEnvelope[];
}

// ReasonTree 从事件流中聚合 reason.step + tool.call，渲染左侧思考步骤树。
export default function ReasonTree({ events }: Props) {
  const steps = useMemo(() => aggregate(events), [events]);

  return (
    <div style={{ height: "100%", overflow: "auto", padding: 16 }}>
      <h3 style={{ marginTop: 0, fontSize: 14, color: "var(--muted)" }}>
        Reason Tree
      </h3>
      {steps.length === 0 && (
        <div style={{ color: "var(--muted)", fontSize: 13 }}>
          等待 AI 思考步骤...
        </div>
      )}
      <ol style={{ listStyle: "none", padding: 0, margin: 0 }}>
        {steps.map((s) => (
          <li key={s.key} style={{ marginBottom: 16, paddingLeft: 16, position: "relative" }}>
            <span
              style={{
                position: "absolute",
                left: 0,
                top: 6,
                width: 8,
                height: 8,
                borderRadius: "50%",
                background: "var(--accent)",
              }}
            />
            <div style={{ fontWeight: 600, fontSize: 14 }}>
              #{s.index} {s.title}
            </div>
            <div style={{ color: "var(--muted)", fontSize: 12, marginTop: 2 }}>
              {s.thought}
            </div>
            {s.tools.length > 0 && (
              <ul style={{ margin: "8px 0 0", padding: "0 0 0 12px", listStyle: "none" }}>
                {s.tools.map((t, i) => (
                  <li key={i} style={{ fontFamily: "var(--mono)", fontSize: 12, color: "var(--text)" }}>
                    <span style={{ color: "var(--warn)" }}>$</span> {t.command}
                    {t.stdout && (
                      <pre style={{ margin: "4px 0 0", color: "var(--muted)", whiteSpace: "pre-wrap" }}>
                        {t.stdout}
                      </pre>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </li>
        ))}
      </ol>
    </div>
  );
}

interface AggStep {
  key: string;
  index: number;
  title: string;
  thought: string;
  tools: { command: string; stdout: string }[];
}

function aggregate(events: EventEnvelope[]): AggStep[] {
  const steps: AggStep[] = [];
  let pendingTools: { command: string; stdout: string }[] = [];
  for (const ev of events) {
    if (ev.type === "reason.step") {
      const p = ev.data || {};
      const step: AggStep = {
        key: `step-${p.index}-${steps.length}`,
        index: p.index ?? steps.length + 1,
        title: p.title ?? "",
        thought: p.thought ?? "",
        tools: pendingTools,
      };
      pendingTools = [];
      steps.push(step);
    } else if (ev.type === "tool.call") {
      const p = ev.data || {};
      // tool.call 出现在 reason.step 之前时挂到下一步；否则挂到最后一步
      if (steps.length === 0) {
        pendingTools.push({ command: p.command ?? "", stdout: p.stdout ?? "" });
      } else {
        steps[steps.length - 1].tools.push({ command: p.command ?? "", stdout: p.stdout ?? "" });
      }
    }
  }
  if (pendingTools.length > 0) {
    steps.push({
      key: `step-tail-${steps.length}`,
      index: steps.length + 1,
      title: "执行",
      thought: "",
      tools: pendingTools,
    });
  }
  return steps;
}
