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
      context_repository_unavailable:
        "关联仓库授权已变更或不可用，请联系管理员后重新提交。",
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
export type ContextRepository = { project_id: number; sha: string };
export type SequenceReference = {
  repository_id?: number;
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
export type FindingVerification = {
  model?: string;
  status: string;
  reason: string;
  limitations: string[];
  observation_ids: string[];
  base_sha: string;
  head_sha: string;
};
export type PRInvestigationContext = {
  change_summary: string;
  before: string;
  after: string;
  entry_points: { statement: string; observation_ids: string[] }[];
  guards: { statement: string; observation_ids: string[] }[];
  unresolved_edges: string[];
};
export type Finding = {
  pr_context?: PRInvestigationContext;
  fingerprint?: string;
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
  audit_policy?: { context_repositories?: ContextRepository[] };
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
    audit_groups?: {
      id: string;
      files: string[];
      status: "running" | "completed" | "failed" | "unprocessed";
    }[];
    metadata_changes?: GitChangeMetadata[];
    findings: Finding[];
    summary: string;
    coverage_notes: string[];
    excluded_files?: string[];
    investigations?: {
      pr_context?: PRInvestigationContext;
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
    total_tokens?: number;
    prompt_tokens?: number;
    completion_tokens?: number;
    usage_reported?: boolean;
    error?: string;
  }[];
};
export type Review = {
	 revision: number;
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
  verification_model: string;
  model_budget: {
    verification_pricing_configured: boolean;
    verification_input_price_per_million: number;
    verification_output_price_per_million: number;
    max_tokens: number;
    input_price_per_million: number;
    output_price_per_million: number;
    currency: string;
  };
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

export type RunListItem = Omit<
  Run,
  "result" | "trace" | "audit_policy" | "reviews"
> & { finding_count: number };

export type CommentSync = {
  retry_exhausted: boolean;
  enabled: boolean;
  state: string;
  desired_generation: number;
  sent_generation: number;
  reason?: string;
  discussion_id?: string;
  note_id?: number;
  updated_at: string;
};

export type FindingOccurrence = {
  run_id: number;
  finding_id: string;
  head_sha: string;
  run_status: string;
};
export type FindingHistory = {
  finding_id: string;
  fingerprint: string;
  first_run_id: number;
  last_run_id: number;
  occurrences: FindingOccurrence[];
  reviews: {
    run_id: number;
    finding_id: string;
    status: string;
    reason: string;
    actor: number;
    created_at: string;
    imported: boolean;
  }[];
  occurrences_truncated: boolean;
  reviews_truncated: boolean;
};
export type AssociationDecision = {
  revision: number;
  decision: "pending" | "confirmed" | "rejected";
  reason: string;
  actor: number;
  created_at: string;
};
export type FindingAssociation = {
  id: string;
  finding_id: string;
  prior_run_id: number;
  prior_finding_id: string;
  old_path: string;
  new_path: string;
  prior_head_sha: string;
  head_sha: string;
  rename_base_sha: string;
  metadata: GitChangeMetadata;
  evidence: string;
  trigger: string;
  risk_type: string;
  ambiguous: boolean;
  state: AssociationDecision;
  history: AssociationDecision[];
  history_truncated: boolean;
};
export type FindingAssociations = {
  items: FindingAssociation[];
  truncated: boolean;
  limitations: string[];
};
export type FindingLifecycle = {
  history_truncated: boolean;
  current: FindingHistory[];
  not_reobserved: FindingOccurrence[];
  not_reobserved_truncated: boolean;
};

export type ModelUsage = {
  calls: number;
  unknown_calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  complete: boolean;
  estimated_cost: number | null;
  estimate_unavailable_reason?: string;
  currency?: string;
  max_tokens: number;
  stages: {
    stage: string;
    calls: number;
    unknown_calls: number;
    prompt_tokens: number;
    completion_tokens: number;
  }[];
};

export type RetryChainUsage = {
  root_id: number;
  attempts: { run_id: number; status: string; usage: ModelUsage }[];
  usage: ModelUsage;
};
