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

export interface Skill {
  skill_id: string;
  name: string;
  description: string;
  trigger_words: string[];
  markdown_content: string;
  version: number;
  updated_by: string;
  updated_at?: string;
}

export interface TriggerRule {
  rule_id: string;
  name: string;
  is_active: boolean;
  trigger_source: string;
  match_expression: Record<string, unknown>;
  bind_skills: string[];
  agent_image: string;
  policy_group: string;
}

export interface MCPPlugin {
  plugin_id: string;
  name: string;
  command: string;
  args: string[];
  env: Record<string, string>;
  is_active: boolean;
  updated_by: string;
  updated_at?: string;
}

export interface AuditLog {
  audit_id: number;
  session_id: string;
  executed_command: string;
  policy_decision: string;
  approver: string;
  created_at: string;
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
  agentImage = "ballast-runner-base:dev",
  skillIDs: string[] = [],
  mcpPluginIDs: string[] = []
): Promise<Session> {
  const res = await fetch(`${API_BASE}/api/sessions`, {
    ...requestDefaults,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      title,
      agent_image: agentImage,
      skill_ids: skillIDs,
      mcp_plugin_ids: mcpPluginIDs,
    }),
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

export async function listSkills(): Promise<Skill[]> {
  const res = await fetch(`${API_BASE}/api/skills`, {
    ...requestDefaults,
    cache: "no-store",
  });
  if (!res.ok) throw new APIError("listSkills", res.status);
  const body = await res.json();
  return body.skills ?? [];
}

export async function upsertSkill(skill: Skill): Promise<Skill> {
  const res = await fetch(`${API_BASE}/api/skills`, {
    ...requestDefaults,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(skill),
  });
  if (!res.ok) throw new APIError("upsertSkill", res.status);
  return res.json();
}

export async function listTriggerRules(): Promise<TriggerRule[]> {
  const res = await fetch(`${API_BASE}/api/trigger-rules`, {
    ...requestDefaults,
    cache: "no-store",
  });
  if (!res.ok) throw new APIError("listTriggerRules", res.status);
  const body = await res.json();
  return body.trigger_rules ?? [];
}

export async function listMCPPlugins(): Promise<MCPPlugin[]> {
  const res = await fetch(`${API_BASE}/api/mcp-plugins`, {
    ...requestDefaults,
    cache: "no-store",
  });
  if (!res.ok) throw new APIError("listMCPPlugins", res.status);
  const body = await res.json();
  return body.mcp_plugins ?? [];
}

export async function upsertMCPPlugin(plugin: MCPPlugin): Promise<MCPPlugin> {
  const res = await fetch(`${API_BASE}/api/mcp-plugins`, {
    ...requestDefaults,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(plugin),
  });
  if (!res.ok) throw new APIError("upsertMCPPlugin", res.status);
  return res.json();
}

export async function upsertTriggerRule(rule: TriggerRule): Promise<TriggerRule> {
  const res = await fetch(`${API_BASE}/api/trigger-rules`, {
    ...requestDefaults,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(rule),
  });
  if (!res.ok) throw new APIError("upsertTriggerRule", res.status);
  return res.json();
}

export async function listAuditLogs(id: string, limit = 100): Promise<AuditLog[]> {
  const res = await fetch(`${API_BASE}/api/sessions/${id}/audit?limit=${limit}`, {
    ...requestDefaults,
    cache: "no-store",
  });
  if (!res.ok) throw new APIError("listAuditLogs", res.status);
  const body = await res.json();
  return body.audit_logs ?? [];
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
