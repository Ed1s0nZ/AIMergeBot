import type { PRInvestigationContext } from "./api";
import { ObservationLinks } from "./observation-links";

export function PRInvestigationPanel({ context }: { context?: PRInvestigationContext }) {
  if (!context) return <p className="muted">未记录结构化 PR 影响。</p>;
  return (
    <details>
      <summary>PR 改动影响与反证</summary>
      <p>{context.change_summary}</p>
      <p className="muted">以下为带来源的静态陈述，源码引用不代表调用关系或运行结果已证明。</p>
      <dl>
        <dt>BASE · 改动前</dt><dd>{context.before}</dd>
        <dt>HEAD · 改动后</dt><dd>{context.after}</dd>
      </dl>
      {([
        ["受影响入口或契约", context.entry_points],
        ["已检查的保护条件", context.guards],
      ] as const).map(([label, facts]) => (
        <section key={label}>
          <h4>{label}</h4>
          {facts?.length ? <ul>{facts.map((fact, i) => <li key={i}>
            <p>{fact.statement}</p><ObservationLinks ids={fact.observation_ids} />
          </li>)}</ul> : <p className="muted">尚未记录来源事实。</p>}
        </section>
      ))}
      <h4>未核实关系</h4>
      {context.unresolved_edges?.length ? <ul>{context.unresolved_edges.map((edge, i) => <li key={i}>{edge}</li>)}</ul> : <p className="muted">未记录待核实关系，不代表完整覆盖。</p>}
    </details>
  );
}
