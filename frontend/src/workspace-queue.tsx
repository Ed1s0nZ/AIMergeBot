import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { type Project, type RunListItem } from "./api";
import { ErrorBox, Empty, statuses } from "./components";
import { Heading, RunTable, useResource } from "./page-utils";

type FindingItem = { run_id: number; project_id: number; mr_iid: number; head_sha: string; run_status: string; finding_id: string; severity: string; type: string; title: string; file: string; line: number; review_status: string; disposition: string; owner: number; expires_at: string };
type Page<T> = { items: T[]; total: number; page: number; size: number };

export function WorkspaceQueue({ mode }: { mode: "tasks" | "findings" }) {
 const [project, setProject] = useState("");
 const [kind, setKind] = useState("pending_review");
 const [severity, setSeverity] = useState("");
 const [review, setReview] = useState("");
 const [page, setPage] = useState(1);
 const query = new URLSearchParams({ page: String(page), size: "20", project_id: project });
 if (mode === "tasks") query.set("kind", kind);
 else { query.set("severity", severity); query.set("review_status", review); }
 const resource = useResource<Page<FindingItem | RunListItem>>(`/workspace/${mode}?${query}`);
 const projects = useResource<{ items: Project[] }>("/projects");
 const reset = (setter: (value: string) => void, value: string) => { setter(value); setPage(1); };
 const visible = !resource.loading && !resource.error ? resource.data : null;
 return <>
  <Heading title={mode === "tasks" ? "待办中心" : "发现中心"} sub={mode === "tasks" ? "按事项类型跟进授权项目中的审计。不同分类可能包含同一次运行。" : "跨任务查看发现。历史发现保留原提交身份，未再次观察不等于已修复。"} action={<button onClick={resource.load}><RefreshCw size={16} />刷新</button>} />
  <section className="panel">
   <div className="inline-form">
    <label>项目<select value={project} onChange={e => reset(setProject, e.target.value)}><option value="">全部授权项目</option>{projects.data?.items.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
    {mode === "tasks" ? <label>事项<select value={kind} onChange={e => reset(setKind, e.target.value)}><option value="pending_review">待复核发现</option><option value="failed">失败审计</option><option value="risk_expired">风险接受到期</option><option value="incomplete">覆盖不完整</option></select></label> : <>
     <label>严重程度<select value={severity} onChange={e => reset(setSeverity, e.target.value)}><option value="">全部</option>{["critical", "high", "medium", "low", "info"].map(v => <option key={v}>{v}</option>)}</select></label>
     <label>复核状态<select value={review} onChange={e => reset(setReview, e.target.value)}><option value="">全部</option><option value="pending">待复核</option><option value="accepted">已接受</option><option value="fixed">已修复</option><option value="false_positive">误报</option></select></label>
    </>}
   </div>
  </section>
  <ErrorBox error={resource.error || projects.error} />
  {resource.loading ? <Empty>正在加载…</Empty> : visible?.items.length ? <section className="panel">
   {mode === "tasks" ? <RunTable runs={visible.items as RunListItem[]} /> : <div className="workspace-findings">{(visible.items as FindingItem[]).map(f => <article key={`${f.run_id}:${f.finding_id}`} className="workspace-finding">
    <a href={`#/runs/${f.run_id}?finding=${encodeURIComponent(f.finding_id)}`}>{f.title || f.finding_id}</a>
    <p>项目 {f.project_id} · MR !{f.mr_iid} · 运行 #{f.run_id} · {statuses[f.severity] || f.severity} · {statuses[f.review_status] || f.review_status}</p>
    <p>风险处理：{({open:"待处理",accepted:"暂时接受风险",resolved:"人工确认修复",unknown:"证据不足"} as Record<string,string>)[f.disposition] || f.disposition}{f.owner?` · 责任人 #${f.owner}`:""}{f.expires_at?` · 到期 ${f.expires_at}`:""}</p>
    <p>{f.file}:{f.line} · HEAD {f.head_sha.slice(0, 12)} · {f.run_status}</p>
   </article>)}</div>}
  </section> : !resource.error ? <Empty>当前筛选没有事项。</Empty> : null}
  <div className="inline-form" aria-label="列表分页">
   <button disabled={page <= 1 || resource.loading} onClick={() => setPage(page - 1)}>上一页</button>
   <span aria-live="polite">第 {page} 页{visible ? ` · 共 ${visible.total} 条` : ""}</span>
   <button disabled={!visible || page * visible.size >= visible.total} onClick={() => setPage(page + 1)}>下一页</button>
  </div>
 </>;
}
