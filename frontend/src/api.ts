export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
    public code: string = "",
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
    const messages: Record<string, string> = {
      worker_unavailable: "审计服务正在恢复，请稍后重试。",
      project_permission_required: "当前项目权限不足，请联系管理员。",
      not_found: "资源不存在或没有访问权限。",
      audit_quota_exceeded:
        "审计队列容量已满，请等待任务完成或联系管理员调整配额。",
      invalid_audit_quotas:
        "审计配额必须为范围内的正整数，项目和用户并发不能超过各自任务容量。",
    };
    throw new APIError(
      res.status,
      messages[body.code] || body.error || "请求失败",
      body.code || "",
    );
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
export type ProjectRole = "viewer" | "reviewer" | "operator" | "admin";
export type Project = {
  id: number;
  name: string;
  enabled: boolean;
  access_role?: ProjectRole;
};
export type ProjectPermissions = {
  role: ProjectRole;
  can_submit: boolean;
  can_review: boolean;
  can_cancel: boolean;
};
export type GitEntry = { mode: string; type: string; object_id: string };
export type GitChangeMetadata = {
  kind: string;
  old_path: string;
  new_path: string;
  base: GitEntry | null;
  head: GitEntry | null;
};
export type SequenceReference = {
  anchor_type?: "line" | "git_metadata";
  metadata?: GitChangeMetadata;
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
export type FindingVerification = { status: string; reason: string; limitations: string[]; observation_ids: string[]; base_sha: string; head_sha: string };
export type Finding = {
 verification?: FindingVerification;
  anchor_type?: "line" | "git_metadata";
  metadata?: GitChangeMetadata;
  investigation_id?: string;
  observation_ids?: string[];
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
  retry_info?: {
    kind: string;
    source: string;
    http_status?: number;
    header_state?: string;
    retry_after_until?: string;
    state: string;
    delay_seconds?: number;
    eligible_at?: string;
  };
  retry_child_id?: number;
  retry_parent_id?: number;
  retry_attempt?: number;
  retry_at?: string;
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
    metadata_changes?: GitChangeMetadata[];
    findings: Finding[];
    summary: string;
    coverage_notes: string[];
    excluded_files?: string[];
    investigations?: {
      id: string;
      claim: string;
      status: string;
      evidence: string[];
      counterevidence: string[];
      observation_ids?: string[];
      counter_observation_ids?: string[];
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
  config_revision: number;
  audit_quotas: {
    outstanding_global: number;
    outstanding_project: number;
    outstanding_user: number;
    running_project: number;
    running_user: number;
    daily_global: number;
    daily_project: number;
    daily_user: number;
  };
  project_config_sync?: { pending: boolean; generation: number; error: string };
  generate_sequence_diagrams: boolean;
 verify_findings: boolean;
  git_audit: {
    enabled: boolean;
    history_depth: number;
    max_pack_mib: number;
    max_tool_calls: number;
  };
  public_url: string;
  trusted_proxies?: string[];
  restart_required?: boolean;
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

export type RunListItem = Omit<Run, "result" | "trace" | "audit_policy" | "reviews"> & { finding_count: number };

export type CommentSync = { retry_exhausted: boolean; enabled: boolean; state: string; desired_generation: number; sent_generation: number; reason?: string; discussion_id?: string; note_id?: number; updated_at: string };
