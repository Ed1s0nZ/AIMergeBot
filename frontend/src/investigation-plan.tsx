import type { InvestigationTask } from "./api";
import { ObservationLinks } from "./observation-links";

const kinds: Record<InvestigationTask["kind"], string> = {
  input_control: "输入与入口", pr_causality: "改动前后与 PR 因果",
  guards: "保护与反证", outcome: "有害结果", contract: "相关契约",
};
const statuses: Record<InvestigationTask["status"], string> = {
  pending: "待检查", checked: "已记录检查", unavailable: "证据不可用",
};
export function InvestigationPlan({ plan }: { plan?: InvestigationTask[] }) {
  return <section aria-label="调查计划">
    <h4>调查计划与未完成项</h4>
    <p className="muted">已记录检查只表示关联了静态来源，不代表判断正确或运行复现。</p>
    {plan?.length ? <ol>{plan.map(task => <li key={task.id}>
      <strong>{kinds[task.kind] || task.kind} · {statuses[task.status] || task.status}</strong>
      <p>{task.question}</p>
      {task.reason && <p>{task.reason}</p>}
      <ObservationLinks ids={task.observation_ids} />
    </li>)}</ol> : <p className="muted">未记录调查计划，不能确认必要检查已经完成。</p>}
  </section>;
}
