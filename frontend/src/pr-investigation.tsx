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
        <dt>BASE · 改动前</dt><dd>{context.before}<ObservationLinks ids={context.before_observation_ids} /></dd>
        <dt>HEAD · 改动后</dt><dd>{context.after}<ObservationLinks ids={context.after_observation_ids} /></dd>
      </dl>
      {([
        ["受影响入口或契约", context.entry_points],
        ["已检查的保护条件", context.guards],
        ["风险结果", context.impact],
        ["已检查的反例", context.counterexamples],
      ] as const).map(([label, facts]) => (
        <section key={label}>
          <h4>{label}</h4>
          {facts?.length ? <ul>{facts.map((fact, i) => <li key={i}>
            <p>{fact.statement}</p><ObservationLinks ids={fact.observation_ids} />
          </li>)}</ul> : <p className="muted">尚未记录来源事实。</p>}
        </section>
      ))}
      <h4>风险链中的关系</h4>
      {context.relationships?.length ? <ol>{context.relationships.map((edge, i) => <li key={i}>
        <p>{edge.from} → {edge.to} · {edge.certainty === "cited" ? "静态引用" : "推测关系"}</p>
        <p>{edge.relation}</p><ObservationLinks ids={edge.observation_ids} />
      </li>)}</ol> : <p className="muted">未记录逐边关系，不代表路径已核实。</p>}
      <h4>未核实关系</h4>
      {context.unresolved_edges?.length ? <ul>{context.unresolved_edges.map((edge, i) => <li key={i}>{edge}</li>)}</ul> : <p className="muted">未记录待核实关系，不代表完整覆盖。</p>}
    </details>
  );
}
