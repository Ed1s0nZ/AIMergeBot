import type { FindingVerification } from "./api";
import { ObservationLinks } from "./observation-links";

export function VerificationEvidence({ verification }: { verification?: FindingVerification }) {
  if (!verification) return <p className="muted">尚无独立复核记录。</p>;
  const labels: Record<string, string> = { supported: "独立复核支持", rejected: "独立复核未支持", inconclusive: "复核信息不足", unavailable: "独立复核未完成", disabled: "独立复核已关闭" };
  return <section className="coverage" aria-label="独立复核">
    <h4>{labels[verification.status] || "独立复核状态待确认"}</h4>
    <p>{verification.reason}</p>
    <p className="muted">复核模型：{verification.model||"历史记录未提供"}</p>
    <ObservationLinks ids={verification.observation_ids} />
    {verification.limitations?.length > 0 && <ul>{verification.limitations.map((item, index) => <li key={index}>{item}</li>)}</ul>}
    <small className="muted">使用独立上下文和任务固定的复核模型进行静态复核，不代表运行复现。人工复核状态单独保存。</small>
  </section>;
}
