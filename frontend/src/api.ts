export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const res = await fetch("/api/v1" + path, {
    ...options,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...options.headers },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: "服务暂时不可用" }));
    if (res.status === 401 && path != "/auth/login")
      window.dispatchEvent(new Event("session-expired"));
    throw new APIError(res.status, body.error || "请求失败");
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}
export const write = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});
export type User = {
  id: number;
  username: string;
  role: "admin" | "member";
  disabled: boolean;
};
export type Project = { id: number; name: string; enabled: boolean };
export type SequenceReference = {
  side: "head" | "base";
  file: string;
  line: number;
  snippet: string;
  sha: string;
};
export type SequenceStep = {
  from: string;
  to: string;
  label: string;
  kind: "call" | "return" | "note";
  certainty: "cited" | "inferred";
  risk: boolean;
  evidence: SequenceReference[];
};
export type SequenceDiagram = {
  status: "ready" | "partial" | "unavailable";
  reason?: string;
  participants?: { id: string; label: string }[];
  steps?: SequenceStep[];
  limitations?: string[];
  mermaid?: string;
};
export type Finding = {
  sequence_diagram?: SequenceDiagram;
  side: string;
  type: string;
  id: string;
  file: string;
  line: number;
  severity: string;
  title: string;
  description: string;
  evidence: string;
  trigger: string;
  suggestion: string;
  confidence: string;
};
export type Run = {
  id: number;
  project_id: number;
  source_project_id: number;
  mr_iid: number;
  base_sha: string;
  head_sha: string;
  title: string;
  url: string;
  status: string;
  error: string;
  created_at: string;
  result: {
    findings: Finding[];
    summary: string;
    coverage_notes: string[];
    investigations?: {
      id: string;
      claim: string;
      status: string;
      evidence: string[];
      counterevidence: string[];
      next_steps: string[];
    }[];
  };
  trace: {
    name: string;
    stage?: string;
    output?: string;
    observation_id?: string;
    partial?: boolean;
    arguments: string;
    duration_ms: number;
    prompt_tokens?: number;
    completion_tokens?: number;
    usage_reported?: boolean;
    error?: string;
  }[];
};
export type Review = {
  finding_id: string;
  status: string;
  reason: string;
  actor: number;
  updated_at: string;
};
export type Settings = {
  generate_sequence_diagrams: boolean;
  git_audit: {
    enabled: boolean;
    history_depth: number;
    max_pack_mib: number;
    max_tool_calls: number;
  };
  public_url: string;
  listen: string;
  gitlab: { url: string; token: string };
  openai: { url: string; model: string; api_key: string };
  enable_polling: boolean;
  enable_webhook: boolean;
  enable_mr_comment: boolean;
  scan_existing_mrs: boolean;
  webhook_token: string;
  audit_workers: number;
  audit_timeout_seconds: number;
  whitelist_extensions: string[];
  react: {
    enabled: boolean;
    model: string;
    temperature: number;
    max_retries: number;
    max_steps: number;
  };
  mcp: { enabled: boolean; mode: string; verbose_logging: boolean };
  has_api_key: boolean;
  has_gitlab_token: boolean;
  has_webhook_token: boolean;
};
