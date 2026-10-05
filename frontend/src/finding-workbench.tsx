import { useId, useState, type ReactNode } from "react";
import type { Finding, Review } from "./api";
import { statuses } from "./components";
import "./finding-workbench.css";

const verificationLabels: Record<string, string> = {
  supported: "独立复核支持", rejected: "独立复核未支持",
  inconclusive: "复核信息不足", unavailable: "独立复核未完成",
  disabled: "独立复核已关闭", unverified: "无独立复核记录",
};
export function FindingWorkbench({ findings, reviews, renderFinding }: {
  findings: Finding[];
  reviews: Review[];
  renderFinding: (finding: Finding, review?: Review) => ReactNode;
}) {
  const prefix = useId();
  const [query, setQuery] = useState("");
  const [severity, setSeverity] = useState("");
  const [reviewStatus, setReviewStatus] = useState("");
  const [verification, setVerification] = useState("");
  const reviewById = new Map(reviews.map(r => [r.finding_id, r]));
  const severityOptions = [...new Set(findings.map(f => f.severity))].sort();
  const reviewOptions = [...new Set(["pending", ...reviews.map(r => r.status)])].sort();
  const verificationOptions = [...new Set(findings.map(f => f.verification?.status || "unverified"))].sort();
  const search = query.trim().toLocaleLowerCase();
  const matches = (f: Finding) =>
    (!search || [f.title, f.file, f.description, f.type].some(v => v.toLocaleLowerCase().includes(search))) &&
    (!severity || f.severity === severity) &&
    (!reviewStatus || (reviewById.get(f.id)?.status || "pending") === reviewStatus) &&
    (!verification || (f.verification?.status || "unverified") === verification);
  const visible = findings.filter(matches);
  const anchor = (id: string) => `${prefix}-finding-${encodeURIComponent(id)}`;
  const active = Boolean(query || severity || reviewStatus || verification);
  return <section aria-label="发现工作台">
    <div className="panel finding-filters">
      <label>搜索发现<input type="search" value={query} placeholder="标题、路径、描述或风险类型" onChange={e => setQuery(e.target.value)} /></label>
      <label>严重度<select value={severity} onChange={e => setSeverity(e.target.value)}>
        <option value="">全部严重度</option>
        {severityOptions.map(s => <option key={s} value={s}>{statuses[s] || s || "未提供"}</option>)}
      </select></label>
      <label>人工复核<select value={reviewStatus} onChange={e => setReviewStatus(e.target.value)}>
        <option value="">全部人工复核状态</option>
        {reviewOptions.map(s => <option key={s} value={s}>{statuses[s] || s}</option>)}
      </select></label>
      <label>独立静态复核<select value={verification} onChange={e => setVerification(e.target.value)}>
        <option value="">全部独立复核状态</option>
        {verificationOptions.map(s => <option key={s} value={s}>{verificationLabels[s] || s}</option>)}
      </select></label>
      <button type="button" disabled={!active} onClick={() => { setQuery(""); setSeverity(""); setReviewStatus(""); setVerification(""); }}>清空筛选</button>
      <p role="status">显示 {visible.length} / {findings.length} 条发现</p>
      <small className="muted">独立复核为静态代码复核，不代表运行复现。筛选后未提交草稿仍保留。</small>
    </div>
    {visible.length ? <nav className="panel finding-navigation" aria-label="发现快速导航">
      {visible.map(f => <button key={f.id} type="button" onClick={() => {
        const target = document.getElementById(anchor(f.id));
        target?.scrollIntoView({ block: "start" });
        target?.focus({ preventScroll: true });
      }}>{f.title} · {f.file}{f.line > 0 ? `:${f.line}` : ""}</button>)}
    </nav> : <div className="empty">没有匹配的发现，请调整或清空筛选。</div>}
    {findings.map(f => <div key={f.id} id={anchor(f.id)} tabIndex={-1} className="finding-target" hidden={!matches(f)}>
      {renderFinding(f, reviewById.get(f.id))}
    </div>)}
  </section>;
}
