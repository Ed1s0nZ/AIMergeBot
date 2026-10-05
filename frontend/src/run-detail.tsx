import { FrozenContextRepositories } from "./context-repositories";
import { FindingAssociationsPanel } from "./finding-associations";
import { RetryUsagePanel } from "./retry-usage";
import { FollowupAuditPanel } from "./followup-audit";
import { ModelUsagePanel } from "./model-usage";
import { FindingHistoryPanel } from "./finding-history";
import { VerificationEvidence } from "./finding-verification";
import {
  GitMetadataEvidence,
  GitMetadataChanges,
} from "./git-metadata-evidence";
import { RetryExplanation } from "./retry-explanation";
import { ObservationLinks } from "./observation-links";
import { FindingSequence } from "./finding-sequence";
import { ToolObservation } from "./tool-observation";
import { useEffect, useState } from "react";
import { ArrowUpRight, RefreshCw } from "lucide-react";
import {
  api,
  APIError,
  write,
  type Run,
  type Review,
  type Finding,
  type ProjectPermissions,
  type CommentSync,
  type FindingLifecycle,
  type ModelUsage,
  type RetryChainUsage,
} from "./api";
import { Badge, ErrorBox, Empty, date, safeURL, statuses } from "./components";
import { useResource, Heading } from "./page-utils";
function FindingCard({
  finding,
  review,
  runId,
  onSaved,
  canReview,
}: {
  finding: Finding;
  review?: Review;
  runId: number;
  onSaved: () => void;
  canReview: boolean;
}) {
  const [status, setStatus] = useState(review?.status || "pending"),
    [reason, setReason] = useState(review?.reason || ""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <article className="panel finding">
      <div className="finding-heading">
        <Badge value={finding.severity} />
        <h3>{finding.title}</h3>
        <span className="muted">
          {finding.confidence === "candidate" ? "待验证候选" : "证据支持"}
        </span>
      </div>
      <div className="mono file-location">
        {finding.side === "base" ? "BASE · " : "HEAD · "}
        {finding.file}
        {finding.anchor_type === "git_metadata"
          ? " · Git 元数据"
          : `:${finding.line}`}
        {finding.type && ` · ${finding.type}`}
      </div>
      <p>{finding.description}</p>
      {finding.investigation_id && (
        <p className="muted">关联调查：{finding.investigation_id}</p>
      )}
      <ObservationLinks ids={finding.observation_ids} />
      {finding.anchor_type === "git_metadata" && finding.metadata ? (
        <GitMetadataEvidence metadata={finding.metadata} />
      ) : (
        <pre>{finding.evidence}</pre>
      )}
      <dl>
        <dt>触发条件</dt>
        <dd>{finding.trigger}</dd>
        <dt>修复建议</dt>
        <dd>{finding.suggestion}</dd>
      </dl>
      <VerificationEvidence verification={finding.verification} />
      <FindingSequence
        diagram={finding.sequence_diagram}
        findingId={finding.id}
      />
      {canReview ? (
        <form
          className="review-form"
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            setError("");
            try {
              await api(
                `/runs/${runId}/findings/${finding.id}/review`,
                write("PUT", { status, reason }),
              );
              onSaved();
            } catch (e) {
              setError((e as Error).message);
              if (e instanceof APIError && [403, 404].includes(e.status))
                onSaved();
            } finally {
              setBusy(false);
            }
          }}
        >
          <select
            aria-label="复核状态"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
          >
            {["pending", "accepted", "false_positive", "fixed"].map((s) => (
              <option key={s} value={s}>
                {statuses[s] || s}
              </option>
            ))}
          </select>
          <input
            aria-label="复核原因"
            placeholder="记录复核依据…"
            maxLength={4000}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
          <button disabled={busy}>保存复核</button>
        </form>
      ) : (
        <p className="muted">当前项目为查看权限，复核需管理员授权。</p>
      )}
      {review && (
        <small className="muted">
          上次由用户 #{review.actor} 于 {date(review.updated_at)} 更新
        </small>
      )}
      <ErrorBox error={error} />
    </article>
  );
}
export function RunDetail({ id }: { id: number }) {
  const resource = useResource<{
      run: Run;
      usage?: ModelUsage;
      retry_usage?: RetryChainUsage;
      retry_usage_error?: string;
      reviews: Review[];
      finding_lifecycle?: FindingLifecycle;
      permissions: ProjectPermissions;
      comment_sync?: CommentSync | null;
      queue_wait?: { reason: string; eligible_at?: string } | null;
    }>("/runs/" + id),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const r = resource.data?.run;
  const chainRunning =
    resource.data?.retry_usage?.attempts.some((a) =>
      ["pending", "running"].includes(a.status),
    ) || false;
  useEffect(() => {
    if (
      r &&
      (["pending", "running"].includes(r.status) ||
        chainRunning ||
        (resource.data?.comment_sync?.enabled &&
          !resource.data.comment_sync.retry_exhausted &&
          ["pending", "sending", "unknown"].includes(
            resource.data.comment_sync.state,
          )))
    ) {
      const interval = setInterval(resource.load, 2000);
      return () => clearInterval(interval);
    }
  }, [
    id,
    r?.status,
    chainRunning,
    resource.data?.comment_sync?.enabled,
    resource.data?.comment_sync?.state,
    resource.data?.comment_sync?.retry_exhausted,
  ]);
  const action = async (kind: "cancel" | "retry") => {
    setBusy(true);
    setError("");
    try {
      if (kind === "cancel") {
        await api(`/runs/${id}/cancel`, write("POST"));
        await resource.load();
      } else {
        const next = await api<{ id: number }>(
          "/runs",
          write("POST", {
            project_id: r!.project_id,
            mr_iid: r!.mr_iid,
            force: true,
          }),
        );
        location.hash = "/runs/" + next.id;
      }
    } catch (e) {
      setError((e as Error).message);
      if (e instanceof APIError && [403, 404].includes(e.status))
        await resource.load();
    } finally {
      setBusy(false);
    }
  };
  if (!r)
    return (
      <>
        <ErrorBox error={resource.error} />
        <Empty>
          {resource.loading ? "加载任务…" : "无法加载任务"}
          <button onClick={resource.load}>重试</button>
        </Empty>
      </>
    );
  return (
    <>
      <a className="back" href="#/runs">
        ← 返回审计任务
      </a>
      <Heading
        title={r.title || `MR !${r.mr_iid}`}
        sub={`RUN #${id} · 项目 ${r.project_id} · ${date(r.created_at)}`}
        action={
          <div className="actions">
            {resource.data?.permissions.can_submit && (
              <button disabled={busy} onClick={() => action("retry")}>
                <RefreshCw size={15} />
                重新审计
              </button>
            )}
            {resource.data?.permissions.can_cancel &&
              ["pending", "running"].includes(r.status) && (
                <button disabled={busy} onClick={() => action("cancel")}>
                  取消任务
                </button>
              )}
            {safeURL(r.url) && (
              <a
                className="button"
                target="_blank"
                rel="noopener noreferrer"
                href={safeURL(r.url)}
              >
                打开 MR <ArrowUpRight size={16} />
              </a>
            )}
          </div>
        }
      />
      <ErrorBox error={error || resource.error} />
      <section className="panel snapshot">
        <Badge value={r.status} />
        {r.source_project_id !== r.project_id && (
          <div>
            <small>FORK 源项目</small>
            <code>#{r.source_project_id}</code>
          </div>
        )}
        <div>
          <small>HEAD SHA</small>
          <code>{r.head_sha || "旧数据未记录"}</code>
        </div>
        <div>
          <small>BASE SHA</small>
          <code>{r.base_sha || "旧数据未记录"}</code>
        </div>
      </section>
      <FrozenContextRepositories run={r} />
      {r.status === "pending" &&
        resource.data?.queue_wait &&
        !(
          resource.data.queue_wait.reason === "retry_delay" && r.retry_info
        ) && (
          <div className="coverage" role="status">
            {(
              {
                retry_delay: "任务已保留，等待自动重试时间。",
                project_running: "任务已保留，等待项目并发额度。",
                user_running: "任务已保留，等待用户并发额度。",
                global_daily: "任务已保留，等待工作空间 24 小时次数额度恢复。",
                project_daily: "任务已保留，等待项目 24 小时次数额度恢复。",
                user_daily: "任务已保留，等待用户 24 小时次数额度恢复。",
                worker_available: "任务已保留，等待空闲 Worker。",
              } as Record<string, string>
            )[resource.data.queue_wait.reason] || "任务等待调度。"}
            {resource.data.queue_wait.eligible_at &&
              ` 最早可重新检查时间：${date(resource.data.queue_wait.eligible_at)}。`}
          </div>
        )}
      {r.retry_child_id && (
        <p className="muted">
          已创建自动重试记录 ·{" "}
          <a href={`#/runs/${r.retry_child_id}`}>查看并管理重试任务</a>
        </p>
      )}
      {r.retry_parent_id && (
        <p className="muted">
          自动重试第 {r.retry_attempt} 次 ·{" "}
          <a href={`#/runs/${r.retry_parent_id}`}>查看前次记录</a>
          {r.status === "pending" && r.retry_at && !r.retry_info
            ? ` · 最早执行时间 ${date(r.retry_at)}`
            : ""}
        </p>
      )}
      <RetryExplanation run={r} />
      {resource.data?.comment_sync && (
        <section className="panel summary">
          <h2>GitLab 评论同步</h2>
          <p>
            {!resource.data.comment_sync.enabled
              ? "自动同步已关闭，复核记录保存在工作台。"
              : (
                  {
                    pending: "等待同步最新复核状态。",
                    sending: "正在核对或同步评论。",
                    sent: "评论已同步。",
                    unknown: "发送结果未确认，正在核对原评论，不会重复创建。",
                    conflict: "原评论或发布身份发生变化，自动同步已停止。",
                    stale: "MR 提交已变化，此次审计评论停止同步。",
                    blocked: "同步条件不满足，自动同步已停止。",
                  } as Record<string, string>
                )[resource.data.comment_sync.state] || "同步状态待确认。"}
          </p>
          {resource.data.comment_sync.retry_exhausted && (
            <p>
              自动核对次数已用尽，请管理员检查原评论和同步原因。系统不会重复创建评论。
            </p>
          )}
          {resource.data.comment_sync.note_id && safeURL(r.url) && (
            <p>
              <a
                href={`${safeURL(r.url)}#note_${resource.data.comment_sync.note_id}`}
                target="_blank"
                rel="noreferrer"
              >
                查看原评论
              </a>
            </p>
          )}
          <small>
            已同步版本 {resource.data.comment_sync.sent_generation} / 当前版本{" "}
            {resource.data.comment_sync.desired_generation} ·{" "}
            {date(resource.data.comment_sync.updated_at)}
          </small>
        </section>
      )}

      {r.error && <ErrorBox error={r.error} />}
      <section className="panel summary">
        <h2>审计摘要</h2>
        <p>{r.result.summary || "等待审计结果。"}</p>
        {(r.result.audit_groups?.length || 0) > 0 && (
          <details className="coverage">
            <summary>
              分组审计 ·{" "}
              {
                r.result.audit_groups!.filter((g) => g.status === "completed")
                  .length
              }
              /{r.result.audit_groups!.length} 组完成
            </summary>
            <p>
              按文件分配输入和预算；每组仍可检索整个固定提交的仓库。目录分组不代表调用关系。
            </p>
            {r.result.audit_groups!.map((g) => (
              <details key={g.id}>
                <summary>
                  {g.id} ·{" "}
                  {{
                    running: "审计中",
                    completed: "已完成",
                    failed: "失败",
                    unprocessed: "未处理",
                  }[g.status] || "未知状态"}{" "}
                  · {g.files.length} 个文件
                </summary>
                <ul>
                  {g.files.map((file) => (
                    <li key={file}>{file}</li>
                  ))}
                </ul>
              </details>
            ))}
          </details>
        )}
        <GitMetadataChanges changes={r.result.metadata_changes} />
        {(r.result.excluded_files?.length || 0) > 0 && (
          <details className="coverage">
            <summary>
              按策略排除 {r.result.excluded_files!.length} 个文件
            </summary>
            <ul>
              {r.result.excluded_files!.map((file) => (
                <li key={file}>{file}</li>
              ))}
            </ul>
          </details>
        )}
        {r.result.coverage_notes.length > 0 && (
          <div className="coverage">
            <strong>覆盖与限制</strong>
            <ul>
              {r.result.coverage_notes.map((n, i) => (
                <li key={i}>{n}</li>
              ))}
            </ul>
          </div>
        )}
      </section>
      <FollowupAuditPanel
        key={`followup:${r.id}`}
        run={r}
        canSubmit={resource.data?.permissions.can_submit || false}
      />
      <ModelUsagePanel usage={resource.data?.usage} />
      {resource.data?.retry_usage_error && (
        <p className="coverage">
          重试链用量无法核对，当前尝试的已报告用量仍保留。请联系管理员检查任务关联。
        </p>
      )}
      <RetryUsagePanel chain={resource.data?.retry_usage} />
      <FindingHistoryPanel
        lifecycle={resource.data?.finding_lifecycle}
        findings={r.result.findings}
      />
      <FindingAssociationsPanel
        key={`associations:${r.id}`}
        run={r}
        canReview={resource.data?.permissions.can_review || false}
      />
      <div className="section-heading">
        <h2>
          发现 <span className="muted">{r.result.findings.length}</span>
        </h2>
        <span className="muted">逐项复核，保留依据</span>
      </div>
      {r.result.findings.length ? (
        r.result.findings.map((f) => (
          <FindingCard
            key={`${id}:${f.id}`}
            finding={f}
            runId={id}
            review={resource.data?.reviews.find((x) => x.finding_id === f.id)}
            onSaved={resource.load}
            canReview={resource.data?.permissions.can_review || false}
          />
        ))
      ) : (
        <Empty>
          {r.status === "succeeded"
            ? "当前审计未报告有证据支持的发现。"
            : "当前没有可展示的发现，请查看任务状态与覆盖说明。"}
        </Empty>
      )}
      {r.result.investigations?.length ? (
        <section className="panel trace">
          <h2>调查记录</h2>
          {r.result.investigations.map((item) => (
            <details key={item.id}>
              <summary>
                {item.claim}{" "}
                <span className="muted">
                  {(
                    {
                      investigating: "待确认",
                      supported: "有证据支持",
                      rejected: "已排除",
                    } as Record<string, string>
                  )[item.status] || item.status}
                </span>
              </summary>
              <p className="muted">这里记录代码调查结果，不代表已运行复现。</p>
              <p>证据：{item.evidence?.join("；") || "尚未记录"}</p>
              <ObservationLinks ids={item.observation_ids} />
              <p>反证：{item.counterevidence?.join("；") || "尚未记录"}</p>
              <ObservationLinks ids={item.counter_observation_ids} />
              <p>待查：{item.next_steps?.join("；") || "无"}</p>
            </details>
          ))}
        </section>
      ) : null}
      <section className="panel trace">
        <h2>工具调用</h2>
        {r.trace?.length ? (
          r.trace.map((t, i) => (
            <details
              key={i}
              id={t.observation_id ? `trace-${t.observation_id}` : undefined}
            >
              <summary>
                {t.name}
                {t.stage === "compression" || t.stage === "verification_compression" || t.stage === "diagram_compression" || t.stage === "synthesis_compression"
                  ? ` · ${t.stage === "verification_compression" ? "复核上下文压缩" : t.stage === "diagram_compression" ? "时序图上下文压缩" : t.stage === "synthesis_compression" ? "跨组汇总上下文压缩" : "上下文压缩"}`
                  : t.stage === "diagram"
                  ? " · 时序图生成"
                  : t.stage === "verification"
                    ? " · 独立复核"
                    : t.stage === "synthesis"
                      ? " · 跨组汇总"
                      : ""}{" "}
                <span className="muted">
                  {t.duration_ms}ms {t.error ? "· 失败" : ""}
                </span>
              </summary>
              <pre>{t.arguments}</pre>
              {t.output && (
                <>
                  <p className="muted">
                    返回证据 · {t.observation_id}
                    {t.partial ? " · 有后续分页" : ""}
                  </p>
                  <ToolObservation output={t.output} />
                </>
              )}
              {t.name === "model" && (
                <p className="muted">
                  {t.usage_reported
                    ? `输入 ${t.prompt_tokens || 0} tokens · 输出 ${t.completion_tokens || 0} tokens`
                    : "接口未返回 token usage"}
                </p>
              )}
              {t.error && <ErrorBox error={t.error} />}
            </details>
          ))
        ) : (
          <p className="muted">没有工具调用记录。</p>
        )}
      </section>
    </>
  );
}
