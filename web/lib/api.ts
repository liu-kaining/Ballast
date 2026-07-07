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
  data: Record<string, string | number | boolean | null | undefined>;
}

export class APIError extends Error {
  status: number;

  constructor(operation: string, status: number) {
    super(`${operation}: ${status}`);
    this.status = status;
  }
}

const requestDefaults: RequestInit = {
  credentials: "include",
};

export async function login(token: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/auth/login`, {
    ...requestDefaults,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token }),
  });
  if (!res.ok) throw new APIError("login", res.status);
}

export async function logout(): Promise<void> {
  const res = await fetch(`${API_BASE}/api/auth/logout`, {
    ...requestDefaults,
    method: "POST",
  });
  if (!res.ok) throw new APIError("logout", res.status);
}

export async function listSessions(
  status?: SessionStatus
): Promise<Session[]> {
  const q = status ? `?status=${status}` : "";
  const res = await fetch(`${API_BASE}/api/sessions${q}`, {
    ...requestDefaults,
    cache: "no-store",
  });
  if (!res.ok) throw new APIError("listSessions", res.status);
  const body = await res.json();
  return body.sessions ?? [];
}

export async function createSession(
  title: string,
  agentImage = "ballast-runner-base:dev"
): Promise<Session> {
  const res = await fetch(`${API_BASE}/api/sessions`, {
    ...requestDefaults,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title, agent_image: agentImage }),
  });
  if (!res.ok) throw new APIError("createSession", res.status);
  return res.json();
}

export async function getSession(id: string): Promise<Session> {
  const res = await fetch(`${API_BASE}/api/sessions/${id}`, {
    ...requestDefaults,
    cache: "no-store",
  });
  if (!res.ok) throw new APIError("getSession", res.status);
  return res.json();
}

export async function approveSession(id: string, approver = "sre-oncall") {
  const res = await fetch(`${API_BASE}/api/sessions/${id}/approve`, {
    ...requestDefaults,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ approver }),
  });
  if (!res.ok) throw new APIError("approveSession", res.status);
  return res.json();
}

export async function destroySession(id: string) {
  const res = await fetch(`${API_BASE}/api/sessions/${id}/destroy`, {
    ...requestDefaults,
    method: "POST",
  });
  if (!res.ok) throw new APIError("destroySession", res.status);
  return res.json();
}

export function sessionWSURL(id: string): string {
  const base = API_BASE.replace(/^http/, "ws");
  return `${base}/api/sessions/${id}/ws`;
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
