import { useState } from "react";
import {
  APIError,
  api,
  write,
  type FindingAssociations,
  type FindingAssociation,
  type Run,
} from "./api";
import { ErrorBox, date } from "./components";
import { useResource } from "./page-utils";
import "./finding-associations.css";

const labels = {
  pending: "待确认",
  confirmed: "已确认关联",
  rejected: "已拒绝关联",
};
const limitations: Record<string, string> = {
  "Findings without a valid unique source fingerprint were omitted; inspect original anchors manually.":
    "缺少有效唯一源指纹的问题已省略，包括无法区分的重复锚点，请人工查看原始证据。",
  "Git rename describes this task's BASE→HEAD; it does not establish prior HEAD ancestry or semantic risk equivalence. Human review of both versions is required. Decisions never inherit prior risk reviews.":
    "重命名依据来自本次 BASE → HEAD，不能证明旧、新版本的祖先关系或风险语义相同。请对照两端报告确认，旧风险复核不会继承。",
  "Current task is not terminal; suggestions are unavailable.":
    "当前任务未结束，暂不提供关联建议。",
  "No supported server-recorded regular-file rename metadata; arbitrary moves are not inferred.":
    "没有可用的普通文件重命名记录，无法推断任意文件移动。",
  "Only the first 50 association suggestions are shown.":
    "仅展示前 50 条建议，尚有未展示的候选。",
  "Only the latest 20 same-scope historical tasks were inspected.":
    "仅检查最近 20 个同范围历史任务，更早的记录需人工查看。",
  "Historical results exceeding 1 MiB were omitted; inspect original reports manually.":
    "已省略超过 1 MiB 的历史结果，请打开原报告查看。",
  "Current result exceeds the 1 MiB association input limit; inspect the original report manually.":
    "当前结果超过 1 MiB 的关联输入上限，请人工对照原报告。",
};
function AssociationList({ run, canReview }: { run: Run; canReview: boolean }) {
  const resource = useResource<FindingAssociations>(
    `/runs/${run.id}/associations`,
  );
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const decide = async (
    item: FindingAssociation,
    decision: "confirmed" | "rejected" | "pending",
  ) => {
    const reason = reasons[item.id] || "";
    if (decision !== "pending" && !reason.trim()) {
      setError("请先填写对照两端证据后的决定理由。");
      return;
    }
    if ([...reason].length > 1000) {
      setError("理由最多 1000 个字符。");
      return;
    }
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await api(
        `/runs/${run.id}/associations/${item.id}`,
        write("PUT", {
          decision,
          reason,
          expected_revision: item.state.revision,
        }),
      );
      setReasons((previous) => ({ ...previous, [item.id]: "" }));
      setMessage(
        decision === "pending"
          ? "已撤回当前关联决定，历史记录保留。"
          : "关联决定已记录，本次风险仍需单独复核。",
      );
      await resource.load();
    } catch (e) {
      setError(
        e instanceof APIError && e.status === 409
          ? "记录已变化或存在关联冲突。已刷新，请核对当前决定后重新操作；原理由保留。"
          : (e as Error).message,
      );
      await resource.load();
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="association-content">
      <div className="actions">
        <button
          type="button"
          disabled={busy || resource.loading}
          onClick={() => {
            setError("");
            setMessage("");
            void resource.load();
          }}
        >
          刷新关联建议
        </button>
      </div>
      <ErrorBox error={error || resource.error} />
      {message && (
        <p role="status" className="association-message">
          {message}
        </p>
      )}
      {resource.loading && (
        <p role="status" className="muted">
          正在读取固定版本历史…
        </p>
      )}
      {resource.data && (
        <>
          {resource.data.truncated && (
            <p className="coverage">
              历史检查或展示达到上限，以下建议不代表全量历史。
            </p>
          )}
          <ul className="muted">
            {resource.data.limitations.map((limit, i) => (
              <li key={i}>{limitations[limit] || limit}</li>
            ))}
          </ul>
          {!resource.data.items.length && (
            <p className="muted">
              当前没有可展示的移动关联建议。请结合上述范围与限制判断。
            </p>
          )}
          {!canReview && (
            <p className="muted">当前为只读模式，确认关联需要项目复核权限。</p>
          )}
          {resource.data.items.map((item) => (
            <article className="association-item" key={item.id}>
              <div className="section-heading">
                <h3>
                  {run.result.findings.find((f) => f.id === item.finding_id)
                    ?.title || item.finding_id}
                </h3>
                <span className="association-state">
                  {labels[item.state.decision]}
                </span>
              </div>
              {item.ambiguous && (
                <p className="coverage">
                  存在多个历史候选，系统未自动选择。每个当前问题最多确认一个旧问题。
                </p>
              )}
              <div className="association-versions">
                <div>
                  <span className="muted">旧版本</span>
                  <a href={`#/runs/${item.prior_run_id}`}>
                    任务 #{item.prior_run_id} · {item.prior_finding_id}
                  </a>
                  <code>{item.old_path}</code>
                  <code title={item.prior_head_sha}>{item.prior_head_sha}</code>
                </div>
                <div>
                  <span className="muted">当前版本</span>
                  <a href={`#/runs/${run.id}`}>
                    任务 #{run.id} · {item.finding_id}
                  </a>
                  <code>{item.new_path}</code>
                  <code title={item.head_sha}>{item.head_sha}</code>
                </div>
              </div>
              <p className="muted">
                重命名依据：{item.rename_base_sha.slice(0, 12)} →{" "}
                {item.head_sha.slice(0, 12)} · {item.metadata.kind}
              </p>
              <details>
                <summary>查看匹配事实与决定记录</summary>
                <p>风险类型：{item.risk_type}</p>
                <p>触发条件：{item.trigger}</p>
                <pre>{item.evidence}</pre>
                {item.history.length ? (
                  <ol>
                    {item.history.map((entry) => (
                      <li key={entry.revision}>
                        {labels[entry.decision]} · {date(entry.created_at)} ·
                        操作者 #{entry.actor}
                        <p>{entry.reason || "未填写理由"}</p>
                      </li>
                    ))}
                  </ol>
                ) : (
                  <p className="muted">尚无人工关联决定。</p>
                )}
                {item.history_truncated && (
                  <p className="muted">仅展示最近 20 条决定记录。</p>
                )}
              </details>
              {canReview && (
                <div className="association-decision">
                  <label>
                    决定理由
                    <textarea
                      aria-label={`${item.finding_id} 与任务 ${item.prior_run_id} 的关联理由`}
                      rows={3}
                      maxLength={2000}
                      disabled={busy || resource.loading}
                      value={reasons[item.id] || ""}
                      onChange={(event) =>
                        setReasons((previous) => ({
                          ...previous,
                          [item.id]: event.target.value,
                        }))
                      }
                      placeholder="说明两端证据是否对应同一历史问题，最多 1000 个字符"
                    />
                  </label>
                  <div className="actions">
                    <button
                      type="button"
                      className="primary"
                      disabled={
                        busy ||
                        resource.loading ||
                        item.state.decision === "confirmed"
                      }
                      onClick={() => void decide(item, "confirmed")}
                    >
                      确认关联
                    </button>
                    <button
                      type="button"
                      disabled={
                        busy ||
                        resource.loading ||
                        item.state.decision === "rejected"
                      }
                      onClick={() => void decide(item, "rejected")}
                    >
                      拒绝关联
                    </button>
                    {item.state.decision !== "pending" && (
                      <button
                        type="button"
                        disabled={busy || resource.loading}
                        onClick={() => void decide(item, "pending")}
                      >
                        撤回决定
                      </button>
                    )}
                  </div>
                </div>
              )}
            </article>
          ))}
        </>
      )}
    </div>
  );
}
export function FindingAssociationsPanel({
  run,
  canReview,
}: {
  run: Run;
  canReview: boolean;
}) {
  const [opened, setOpened] = useState(false);
  const terminal = !["pending", "running"].includes(run.status);
  return (
    <section className="panel finding-associations" aria-label="历史移动关联">
      <h2>历史移动关联</h2>
      <p className="muted">
        根据固定版本重命名记录与精确证据给出建议。人工确认只记录历史关系，本次风险和旧版复核决定保持独立。
      </p>
      {!terminal && <p className="muted">任务结束后可查看关联建议。</p>}
      <button
        type="button"
        disabled={!terminal}
        aria-expanded={opened}
        aria-controls="finding-associations-content"
        onClick={() => setOpened((previous) => !previous)}
      >
        {opened ? "收起关联建议" : "查看移动关联建议"}
      </button>
      {opened && (
        <div id="finding-associations-content">
          <AssociationList run={run} canReview={canReview} />
        </div>
      )}
    </section>
  );
}
