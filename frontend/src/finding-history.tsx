import type { FindingLifecycle, Finding } from "./api";
import { date } from "./components";

const decisions: Record<string,string> = {pending:"待复核",accepted:"已接受",false_positive:"误报",fixed:"已修复"};
export function FindingHistoryPanel({lifecycle,findings}:{lifecycle?:FindingLifecycle;findings:Finding[]}) {
 if (!lifecycle || (!lifecycle.current.length && !lifecycle.not_reobserved.length && !lifecycle.history_truncated)) return null;
 return <section className="panel summary">
  <h2>问题历史</h2>
  <p>仅关联同一 MR 中证据、路径、风险类型和触发条件一致的问题。过去的复核决定供参考，新版本仍需重新复核。</p>
  {lifecycle.history_truncated && <p className="coverage">历史响应达到总量上限，仅展示部分记录；请打开原任务查看完整证据和复核。</p>}
  {lifecycle.current.map(history => <details className="coverage" key={history.finding_id}>
   <summary>{findings.find(f=>f.id===history.finding_id)?.title || history.finding_id} · {history.first_run_id === history.last_run_id ? "首次记录" : "再次发现"}</summary>
   <p>首次记录 <a href={`#/runs/${history.first_run_id}`}>任务 #{history.first_run_id}</a> · 最近记录 <a href={`#/runs/${history.last_run_id}`}>任务 #{history.last_run_id}</a></p>
   <ul>{history.occurrences.map(o=><li key={`${o.run_id}:${o.finding_id}`}><a href={`#/runs/${o.run_id}`}>任务 #{o.run_id}</a> · HEAD {o.head_sha.slice(0,12)} · {o.run_status}</li>)}</ul>
   {history.occurrences_truncated && <p>仅展示最近 20 次记录。</p>}
   <h3>人工复核历史</h3>
   {history.reviews.length ? <ul>{history.reviews.map((r,i)=><li key={i}><a href={`#/runs/${r.run_id}`}>任务 #{r.run_id}</a> · {decisions[r.status] || r.status} · {date(r.created_at)} · 操作者 #{r.actor}{r.imported ? " · 历史基线" : ""}<p>{r.reason || "未填写理由"}</p></li>)}</ul> : <p>尚无人工复核决定。</p>}
   {history.reviews_truncated && <p>仅展示最近 20 条复核记录。</p>}
  </details>)}
  {lifecycle.not_reobserved.length>0 && <details className="coverage">
   <summary>本次未再发现的历史问题 · {lifecycle.not_reobserved.length}{lifecycle.not_reobserved_truncated ? "+" : ""} 项</summary>
   <p>未再发现不代表已修复或安全。需结合当前审计状态、覆盖限制及原始代码人工确认，尤其是尚未完成、失败或取消的审计。</p>
   <ul>{lifecycle.not_reobserved.map(o=><li key={`${o.run_id}:${o.finding_id}`}><a href={`#/runs/${o.run_id}`}>查看原发现 · 任务 #{o.run_id}</a> · HEAD {o.head_sha.slice(0,12)} · {o.finding_id}</li>)}</ul>
   {lifecycle.not_reobserved_truncated && <p>仅展示最近 50 个历史问题。</p>}
  </details>}
 </section>;
}
