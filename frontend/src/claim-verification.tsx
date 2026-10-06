import type { ClaimVerification } from "./api";
import { ObservationLinks } from "./observation-links";
import "./claim-verification.css";

const statusLabels: Record<ClaimVerification["status"], string> = {
  consistent: "两次静态判断一致",
  disagreed: "初审与独立复核存在分歧",
  inconclusive: "命题复核证据不足",
  unavailable: "独立命题复核未完成",
  disabled: "独立命题复核已关闭",
};

export function ClaimVerificationPanel({ verification, investigationStatus, runStatus }: {
  verification?: ClaimVerification;
  investigationStatus: string;
  runStatus: string;
}) {
  const running = runStatus === "pending" || runStatus === "running";
  if (!verification) {
    const message = investigationStatus === "investigating"
      ? "初审调查尚未收尾，尚未执行命题复核。"
      : running
        ? "任务仍在进行，尚无独立命题复核记录。"
        : "未保存独立命题复核记录，不能据此推断复核结论；历史记录不会自动补写。";
    return <section className="coverage claim-verification" aria-label="独立命题复核"><p className="muted">{message}</p></section>;
  }
  const verdictLabels: Record<string, string> = { true: "独立复核认为命题成立", false: "独立复核认为命题不成立", unknown: "必要证据不足，命题判断仍未知" };
  const assessed = ["consistent", "disagreed", "inconclusive"].includes(verification.status);
  return <section className="coverage claim-verification" aria-label="独立命题复核">
    <h4>{statusLabels[verification.status] || "独立命题复核状态待确认"}</h4>
    {running && verification.status === "unavailable" && <p className="muted">任务仍在进行，此项尚无完成的独立复核结论。</p>}
    <p><strong>复核命题：</strong>{verification.assessed_claim}</p>
    {assessed && verification.verdict && <p>{verdictLabels[verification.verdict] || "命题判断待确认"}</p>}
    <p>{verification.reason}</p>
    {verification.model && <p className="muted">复核模型：{verification.model}</p>}
    <ObservationLinks ids={verification.observation_ids} />
    {!!verification.limitations?.length && <ul>{verification.limitations.map((item, index) => <li key={index}>{item}</li>)}</ul>}
    {(verification.base_sha || verification.head_sha) && <p className="muted">固定源码：BASE <code>{verification.base_sha || "未记录"}</code> · HEAD <code>{verification.head_sha || "未记录"}</code></p>}
    <small className="muted">原命题与初审判断保留。两次静态判断一致不代表安全证明或运行复现，判断分歧需人工复核。</small>
  </section>;
}
