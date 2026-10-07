import { useState } from "react";
import { api } from "./api";
import { ErrorBox, statuses } from "./components";
type Entry = { id: string; title: string; severity: string; file: string; line: number };
type Identity = { id: number; base_sha: string; head_sha: string; status: string };
type Comparison = { current: Identity; prior: Identity; scope_changed: boolean; truncated: boolean; limitations: string[]; items: { state: string; current?: Entry; prior?: Entry; changed: string[] }[] };
const labels: Record<string,string> = {newly_observed:"本次观察",reobserved:"再次观察",not_reobserved:"未再次观察",unmatched:"无法唯一关联"};
export function RunComparisonPanel({ runId }: { runId: number }) {
 const [prior, setPrior] = useState(""); const [data, setData] = useState<Comparison|null>(null); const [error,setError] = useState(""); const [busy,setBusy] = useState(false);
 return <details className="panel"><summary>对比审计结果</summary>
 <p>选择同一项目、来源和 MR 的另一运行。未再次观察不等于已修复。</p>
 <form className="inline-form" onSubmit={async e => {e.preventDefault();if(busy)return;setBusy(true);setError("");setData(null);try{setData(await api<Comparison>(`/runs/${runId}/comparison?prior_id=${encodeURIComponent(prior)}`));}catch(e){setError((e as Error).message)}finally{setBusy(false)}}}>
 <label>对照运行编号<input type="number" min="1" required value={prior} disabled={busy} onChange={e=>{setPrior(e.target.value);setData(null);setError("")}} /></label><button disabled={busy}>{busy?"对比中…":"对比"}</button></form>
 <ErrorBox error={error}/>{data&&<><p>#{data.prior.id}（{data.prior.head_sha.slice(0,12)}） → #{data.current.id}（{data.current.head_sha.slice(0,12)}）</p>
 {data.limitations.map((v,i)=><p className="coverage" key={i}>{v}</p>)}
 {data.items.length===0&&<p>{data.truncated?"未加载完整结果，请查看原报告。":"没有可展示的发现差异。"}</p>}
 {data.items.map((item,i)=>{const finding=item.current||item.prior!;const id=item.current?data.current.id:data.prior.id;return <article className="workspace-finding" key={i}><strong>{labels[item.state]||item.state}</strong> · <a href={`#/runs/${id}?finding=${encodeURIComponent(finding.id)}`}>{finding.title||finding.id}</a><p>{finding.file}:{finding.line} · {statuses[finding.severity]||finding.severity}{item.changed.length?` · 变化：${item.changed.join("、")}`:""}</p></article>})}</>}
 </details>;
}
