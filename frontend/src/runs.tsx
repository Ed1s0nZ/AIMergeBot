import { useState } from "react";
import { ArrowUpRight, RefreshCw, SlidersHorizontal } from "lucide-react";
import { api, write, APIError, type Project, type Run } from "./api";
import { ErrorBox, Empty, statuses } from "./components";
import { useResource, Heading, RunTable } from "./page-utils";
export function Runs() {
  const [riskType, setRiskType] = useState(""),
    [riskInput, setRiskInput] = useState("");
  const [project, setProject] = useState(""),
    [status, setStatus] = useState(""),
    [level, setLevel] = useState(""),
    [review, setReview] = useState(""),
    [page, setPage] = useState(1),
    [pid, setPid] = useState(""),
    [iid, setIid] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const query = new URLSearchParams({
    page: String(page),
    project_id: project,
    status,
    level,
    review_status: review,
    type: riskType,
  });
  const resource = useResource<{ items: Run[]; total: number; size: number }>(
    "/runs?" + query,
  );
  const projects = useResource<{ items: Project[] }>("/projects");
  const executableProjects =
    projects.data?.items.filter(
      (p) =>
        p.enabled &&
        (p.access_role === "admin" || p.access_role === "operator"),
    ) || [];
  return (
    <>
      <Heading
        title="审计任务"
        sub="每次运行都有独立快照、执行状态和结果。"
        action={
          <button onClick={resource.load}>
            <RefreshCw size={16} />
            刷新
          </button>
        }
      />
      {executableProjects.length > 0 ? (
        <section className="panel submit-panel">
          <div>
            <h2>发起代码审计</h2>
            <p>
              选择项目与 MR 编号。重复提交自动去重；fork MR
              还需源项目的查看权限。
            </p>
          </div>
          <form
            className="inline-form"
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              setError("");
              try {
                const r = await api<{ id: number }>(
                  "/runs",
                  write("POST", {
                    project_id: Number(pid),
                    mr_iid: Number(iid),
                  }),
                );
                location.hash = "/runs/" + r.id;
              } catch (e) {
                setError((e as Error).message);
                if (e instanceof APIError && [403, 404].includes(e.status))
                  await projects.load();
              } finally {
                setBusy(false);
              }
            }}
          >
            <select
              required
              aria-label="审计项目"
              value={pid}
              onChange={(e) => setPid(e.target.value)}
            >
              <option value="">选择项目</option>
              {executableProjects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <input
              aria-label="MR 编号"
              type="number"
              min="1"
              placeholder="MR 编号"
              required
              value={iid}
              onChange={(e) => setIid(e.target.value)}
            />
            <button className="primary" disabled={busy}>
              {busy ? "提交中…" : "开始审计"}
              <ArrowUpRight size={16} />
            </button>
          </form>
        </section>
      ) : (
        !projects.loading && (
          <p className="muted">
            没有可执行审计的启用项目。查看与复核权限仍可浏览下方记录；需要发起审计时请联系管理员。
          </p>
        )
      )}
      <ErrorBox error={error || resource.error || projects.error} />
      <section className="panel runs-list">
        <div className="list-caption">
          <SlidersHorizontal size={17} />
          <h2>运行记录</h2>
          <span>
            {resource.loading
              ? "加载中"
              : `${resource.data?.total ?? 0} 个任务`}
          </span>
        </div>
        <div className="filters">
          <select
            aria-label="项目筛选"
            value={project}
            onChange={(e) => {
              setProject(e.target.value);
              setPage(1);
            }}
          >
            <option value="">全部项目</option>
            {projects.data?.items.map((p) => (
              <option value={p.id} key={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <select
            aria-label="状态筛选"
            value={status}
            onChange={(e) => {
              setStatus(e.target.value);
              setPage(1);
            }}
          >
            <option value="">全部状态</option>
            {[
              "pending",
              "running",
              "succeeded",
              "skipped",
              "failed",
              "incomplete",
              "cancelled",
            ].map((s) => (
              <option key={s} value={s}>
                {statuses[s] || s}
              </option>
            ))}
          </select>
          <select
            aria-label="风险筛选"
            value={level}
            onChange={(e) => {
              setLevel(e.target.value);
              setPage(1);
            }}
          >
            <option value="">全部风险</option>
            <option value="high">高危</option>
            <option value="medium">中危</option>
            <option value="low">低危</option>
          </select>
          <select
            aria-label="复核筛选"
            value={review}
            onChange={(e) => {
              setReview(e.target.value);
              setPage(1);
            }}
          >
            <option value="">全部复核状态</option>
            <option value="pending">待复核</option>
            <option value="accepted">已接受</option>
            <option value="false_positive">误报</option>
            <option value="fixed">已修复</option>
          </select>
          <form
            className="inline-form"
            style={{ padding: 0 }}
            onSubmit={(e) => {
              e.preventDefault();
              setRiskType(riskInput.trim());
              setPage(1);
            }}
          >
            <input
              aria-label="风险类型筛选"
              placeholder="风险类型，如 SQL injection"
              value={riskInput}
              onChange={(e) => setRiskInput(e.target.value)}
            />
            <button>筛选类型</button>
          </form>
        </div>
        <div className="runs-table">
          {resource.loading ? (
            <Empty>加载中…</Empty>
          ) : (
            <RunTable runs={resource.data?.items || []} />
          )}
        </div>
      </section>
      <div className="pagination">
        <button disabled={page <= 1} onClick={() => setPage(page - 1)}>
          上一页
        </button>
        <span>第 {page} 页</span>
        <button
          disabled={
            !resource.data || page * resource.data.size >= resource.data.total
          }
          onClick={() => setPage(page + 1)}
        >
          下一页
        </button>
      </div>
    </>
  );
}
