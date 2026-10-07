import { useState } from "react";
import { type ModelUsage, type Project } from "./api";
import { Empty, ErrorBox } from "./components";
import { Heading, useResource } from "./page-utils";

type UsageRow = { run_id:number; project_id:number; head_sha:string; model:string; status:string; created_at:string; duration_seconds:number|null; usage:ModelUsage|null; unknown_reason?:string };
type QualityRow = { model:string; policy:string; snapshot:string; findings:number; pending:number; accepted:number; fixed:number; false_positive:number; other:number };
type Result = { items:(UsageRow|QualityRow)[]; total?:number; size?:number; window:{from:string;to:string}; truncated?:boolean; limitations?:string[] };
export function WorkspaceMetrics({mode}:{mode:"usage"|"quality"}) {
 const [project,setProject]=useState("");
 const [days,setDays]=useState("30");
 const [page,setPage]=useState(1);
 const [to,setTo]=useState(()=>new Date().toISOString());
 const query=new URLSearchParams({project_id:project,page:String(page),size:"20",to,from:new Date(Date.parse(to)-Number(days)*86400000).toISOString()});
 const resource=useResource<Result>(`/workspace/${mode}?${query}`);
 const projects=useResource<{items:Project[]}>("/projects");
 const visible=!resource.loading&&!resource.error?resource.data:null;
 const refresh=()=>{setPage(1);setTo(new Date().toISOString());};
 return <>
  <Heading title={mode==="usage"?"用量与成本":"质量反馈"} sub={mode==="usage"?"按任务创建时间统计授权范围；费用是运行时价格快照下的估算，并非账单。":"按模型和策略快照查看人工复核记录；不同提交样本不能直接比较模型能力。"} action={<button onClick={refresh}>刷新</button>}/>
  <section className="panel"><div className="inline-form">
   <label>项目<select value={project} onChange={e=>{setProject(e.target.value);setPage(1);}}><option value="">全部授权项目</option>{projects.data?.items.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
   <label>时间范围<select value={days} onChange={e=>{setDays(e.target.value);setPage(1);}}><option value="7">最近 7 天</option><option value="30">最近 30 天</option><option value="90">最近 90 天</option><option value="365">最近 365 天</option></select></label>
  </div>{visible&&<p>创建时间：{visible.window.from} 至 {visible.window.to}（不含结束时刻）</p>}</section>
  <ErrorBox error={resource.error||projects.error}/>
  {resource.loading?<Empty>正在加载…</Empty>:visible?.items.length?<section className="panel">
   {mode==="usage"?<><p>本页逐次运行数据，重试分别计入；未报告用量及价格显示未知，失败任务可能仍产生费用。</p>{(visible.items as UsageRow[]).map(r=><article className="workspace-finding" key={r.run_id}>
    <a href={`#/runs/${r.run_id}`}>运行 #{r.run_id}</a><p>项目 {r.project_id} · {r.model} · {r.status} · HEAD {r.head_sha.slice(0,12)}</p>
    <p>耗时 {r.duration_seconds===null?"未知":`${r.duration_seconds.toFixed(1)} 秒`} · 创建于 {r.created_at}</p>
    {r.usage?<><p>模型调用 {r.usage.calls} · 未知用量调用 {r.usage.unknown_calls} · 已知输入 / 输出 token {r.usage.prompt_tokens} / {r.usage.completion_tokens}</p><p>记录{r.usage.complete?"完整":"不完整"} · 已知消耗估算 {r.usage.estimated_cost===null?"未知":`${r.usage.estimated_cost.toPrecision(6)} ${r.usage.currency||""}`}{r.usage.estimate_unavailable_reason?` · ${r.usage.estimate_unavailable_reason}`:""} · token 上限 {r.usage.max_tokens||"未设置"}</p></>:<p>用量未知：{r.unknown_reason||"历史记录缺失"}</p>}
   </article>)}</>:<>
    {visible.limitations?.map(v=><p key={v}>{v}</p>)}{visible.truncated&&<p role="status">分组超过 500 个，结果已截断，请缩小时间或项目范围。</p>}
    {(visible.items as QualityRow[]).map((r,i)=><article className="workspace-finding" key={i}><strong>{r.model} · 策略 {r.policy||"未知"} · 快照 {r.snapshot.slice(0,12)}</strong><p>发现 {r.findings} · 待复核 {r.pending} · 已接受 {r.accepted} · 标记修复 {r.fixed} · 标记误报 {r.false_positive} · 其他 {r.other}</p></article>)}
   </>}
  </section>:!resource.error?<Empty>当前范围暂无记录。</Empty>:null}
  {mode==="usage"&&<div className="inline-form" aria-label="用量分页"><button disabled={page<=1||resource.loading} onClick={()=>setPage(page-1)}>上一页</button><span aria-live="polite">第 {page} 页{visible?` · 共 ${visible.total} 次运行`:""}</span><button disabled={!visible||page*(visible.size||20)>=(visible.total||0)} onClick={()=>setPage(page+1)}>下一页</button></div>}
 </>;
}
