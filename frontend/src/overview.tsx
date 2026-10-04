import {
  ArrowUpRight,
  Plus,
  ShieldCheck,
  FolderGit2,
  ScanLine,
  Flag,
} from "lucide-react";
import { type Project, type Run } from "./api";
import { ErrorBox, Empty } from "./components";
import { useResource, Heading, RunTable } from "./page-utils";
export function Overview() {
  const { data, error, loading } = useResource<{ items: Run[]; total: number }>(
    "/runs?size=8",
  );
  const projects = useResource<{ items: Project[] }>("/projects");
  return (
    <>
      <Heading
        title="工作台概览"
        sub="了解最近审计，跟进发现与覆盖缺口。"
        action={
          <a className="primary button" href="#/runs">
            <Plus size={16} />
            发起审计
          </a>
        }
      />
      <ErrorBox error={error || projects.error} />
      <div className="metrics">
        <div>
          <span className="metric-label">
            <FolderGit2 size={18} />
            已接入项目
          </span>
          <strong>{projects.data?.items.length ?? "—"}</strong>
          <small>团队共享的 GitLab 项目</small>
        </div>
        <div>
          <span className="metric-label">
            <ScanLine size={18} />
            审计任务
          </span>
          <strong>{data?.total ?? "—"}</strong>
          <small>保存每次执行历史</small>
        </div>
        <div>
          <span className="metric-label">
            <Flag size={18} />
            最近任务中的发现
          </span>
          <strong>
            {data?.items.reduce((n, r) => n + r.result.findings.length, 0) ??
              "—"}
          </strong>
          <small>统计范围：最近 8 次运行</small>
        </div>
      </div>
      <div className="section-heading">
        <h2>最近审计</h2>
        <a href="#/runs">
          查看全部 <ArrowUpRight size={15} />
        </a>
      </div>
      <section className="panel">
        {loading ? (
          <Empty>加载任务…</Empty>
        ) : (
          <RunTable runs={data?.items || []} />
        )}
      </section>
      <div className="guide">
        <ShieldCheck size={30} />
        <div>
          <h3>证据优先，保留边界</h3>
          <p>
            审计固定到 Git
            提交。失败和覆盖不完整会单独标记；发现需结合触发条件进行人工复核。
          </p>
        </div>
      </div>
    </>
  );
}
