import { ObservationLinks } from "./observation-links";
import { FindingSequence } from "./finding-sequence";
import { ToolObservation } from "./tool-observation";
import { useEffect, useState } from "react";
import { ArrowUpRight, RefreshCw } from "lucide-react";
import { api, write, type Run, type Review, type Finding } from "./api";
import { Badge, ErrorBox, Empty, date, safeURL, statuses } from "./components";
import { useResource, Heading } from "./page-utils";
function FindingCard({
  finding,
  review,
  runId,
  onSaved,
}: {
  finding: Finding;
  review?: Review;
  runId: number;
  onSaved: () => void;
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
        {finding.file}:{finding.line}
        {finding.type && ` · ${finding.type}`}
      </div>
      <p>{finding.description}</p>
 {finding.investigation_id && <p className="muted">关联调查：{finding.investigation_id}</p>}
 <ObservationLinks ids={finding.observation_ids} />
      <pre>{finding.evidence}</pre>
      <dl>
        <dt>触发条件</dt>
        <dd>{finding.trigger}</dd>
        <dt>修复建议</dt>
        <dd>{finding.suggestion}</dd>
      </dl>
      <FindingSequence
        diagram={finding.sequence_diagram}
        findingId={finding.id}
      />
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
  const resource = useResource<{ run: Run; reviews: Review[] }>("/runs/" + id),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const r = resource.data?.run;
  useEffect(() => {
    if (r && ["pending", "running"].includes(r.status)) {
      const interval = setInterval(resource.load, 2000);
      return () => clearInterval(interval);
    }
  }, [id, r?.status]);
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
            <button disabled={busy} onClick={() => action("retry")}>
              <RefreshCw size={15} />
              重新审计
            </button>
            {["pending", "running"].includes(r.status) && (
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
        <div>
          <small>HEAD SHA</small>
          <code>{r.head_sha || "旧数据未记录"}</code>
        </div>
        <div>
          <small>BASE SHA</small>
          <code>{r.base_sha || "旧数据未记录"}</code>
        </div>
      </section>
      {r.retry_child_id && <p className="muted">已创建自动重试记录 · <a href={`#/runs/${r.retry_child_id}`}>查看并管理重试任务</a></p>}
      {r.retry_parent_id && <p className="muted">自动重试第 {r.retry_attempt} 次 · <a href={`#/runs/${r.retry_parent_id}`}>查看前次记录</a>{r.status === "pending" && r.retry_at ? ` · 最早执行时间 ${date(r.retry_at)}` : ""}</p>}
      {r.error && <ErrorBox error={r.error} />}
      <section className="panel summary">
        <h2>审计摘要</h2>
        <p>{r.result.summary || "等待审计结果。"}</p>
        {(r.result.excluded_files?.length || 0) > 0 && (<details className="coverage"><summary>按策略排除 {r.result.excluded_files!.length} 个文件</summary><ul>{r.result.excluded_files!.map(file=><li key={file}>{file}</li>)}</ul></details>)}
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
            <details key={i} id={t.observation_id ? `trace-${t.observation_id}` : undefined}>
              <summary>
                {t.name}
                {t.stage === "diagram" ? " · 时序图生成" : ""}{" "}
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
