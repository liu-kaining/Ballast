export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

export type SessionStatus = "RUNNING" | "SUSPENDED" | "SUCCESS" | "FAILED";

export interface Session {
  session_id: string;
  title: string;
  trigger_type: string;
  status: SessionStatus;
  agent_image: string;
  created_at: string;
  updated_at: string;
}

export interface EventEnvelope {
  type: string;
  data: any;
}

export async function listSessions(
  status?: SessionStatus
): Promise<Session[]> {
  const q = status ? `?status=${status}` : "";
  const res = await fetch(`${API_BASE}/api/sessions${q}`, { cache: "no-store" });
  if (!res.ok) throw new Error(`listSessions: ${res.status}`);
  const body = await res.json();
  return body.sessions ?? [];
}

export async function createSession(
  title: string,
  agentImage = "ballast-runner-base:dev"
): Promise<Session> {
  const res = await fetch(`${API_BASE}/api/sessions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title, agent_image: agentImage }),
  });
  if (!res.ok) throw new Error(`createSession: ${res.status}`);
  return res.json();
}

export async function getSession(id: string): Promise<Session> {
  const res = await fetch(`${API_BASE}/api/sessions/${id}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error(`getSession: ${res.status}`);
  return res.json();
}

export async function approveSession(id: string, approver = "sre-oncall") {
  const res = await fetch(`${API_BASE}/api/sessions/${id}/approve`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ approver }),
  });
  if (!res.ok) throw new Error(`approveSession: ${res.status}`);
  return res.json();
}

export async function destroySession(id: string) {
  const res = await fetch(`${API_BASE}/api/sessions/${id}/destroy`, {
    method: "POST",
  });
  if (!res.ok) throw new Error(`destroySession: ${res.status}`);
  return res.json();
}

export function sessionWSURL(id: string): string {
  const base = API_BASE.replace(/^http/, "ws");
  return `${base}/api/sessions/${id}/ws`;
}
