import type { FindingVerification } from "./api";
import { ObservationLinks } from "./observation-links";

const coverageLabels: Record<string, string> = {
  full: "复核模型声明全文获得静态支持",
  partial: "仅部分断言获得静态支持",
  unknown: "全文支持范围未确定",
};

export function VerificationNotice({ verification }: { verification?: FindingVerification }) {
  if (!verification || !["partial", "unknown"].includes(verification.claim_coverage || "")) return null;
  return <aside className="coverage" aria-label="发现证据范围提示">
    <strong>全文尚未获得独立支持</strong>
    <p>{coverageLabels[verification.claim_coverage!]}。请结合下方独立复核的原因与限制判断，原候选内容保留供人工复核。</p>
  </aside>;
}

export function VerificationEvidence({ verification }: { verification?: FindingVerification }) {
  if (!verification) return <p className="muted">尚无独立复核记录。</p>;
  const labels: Record<string, string> = { supported: "独立复核支持", rejected: "独立复核未支持", inconclusive: "复核信息不足", unavailable: "独立复核未完成", disabled: "独立复核已关闭" };
  return <section className="coverage" aria-label="独立复核">
    <h4>{labels[verification.status] || "独立复核状态待确认"}</h4>
    <p>{coverageLabels[verification.claim_coverage || ""] || "历史记录未声明全文支持范围"}</p>
    <p>{verification.reason}</p>
    <p className="muted">复核模型：{verification.model||"历史记录未提供"}</p>
    <ObservationLinks ids={verification.observation_ids} />
    <h5>分项静态复核</h5>
    {verification.checks?.length ? <ul>{verification.checks.map(check => <li key={check.kind}>
      <strong>{({input_control: "输入可控性", pr_causality: "PR 因果关系", guards: "保护与反证", outcome: "有害结果"})[check.kind]} · {({supported: "静态支持", rejected: "证据反驳", inconclusive: "信息不足"})[check.status]}</strong>
      <p>{check.reason}</p><ObservationLinks ids={check.observation_ids} />
    </li>)}</ul> : <p className="muted">未记录分项复核，不能据此确认各项均已检查。</p>}
    <small className="muted">分项状态仍是复核模型的静态判断，来源引用不等于语义或运行结果已证明。</small>
    {verification.limitations?.length > 0 && <ul>{verification.limitations.map((item, index) => <li key={index}>{item}</li>)}</ul>}
    <small className="muted">使用独立上下文和任务固定的复核模型进行静态复核，不代表运行复现。人工复核状态单独保存。</small>
  </section>;
}
