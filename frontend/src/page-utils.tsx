import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowUpRight, GitPullRequest } from "lucide-react";
import { api, APIError, type RunListItem } from "./api";
import { Badge, Empty, date } from "./components";
export function useResource<T>(path: string) {
  const [data, setData] = useState<T | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true);
  const sequence = useRef(0);
  const controller = useRef<AbortController | null>(null);
  const load = useCallback(async () => {
    const current = ++sequence.current;
    controller.current?.abort();
    const request = new AbortController();
    controller.current = request;
    setLoading(true);
    try {
      const result = await api<T>(path, { signal: request.signal });
      if (current === sequence.current) {
        setData(result);
        setError("");
      }
    } catch (e) {
      if (current === sequence.current && !request.signal.aborted) {
        if (e instanceof APIError && [401, 403, 404].includes(e.status))
          setData(null);
        setError((e as Error).message);
      }
    } finally {
      if (current === sequence.current) setLoading(false);
    }
  }, [path]);
  useEffect(() => {
    setData(null);
    setError("");
    void load();
    return () => {
      ++sequence.current;
      controller.current?.abort();
    };
  }, [load]);
  return { data, error, loading, load };
}
export function Heading({
  title,
  sub,
  action,
}: {
  title: string;
  sub: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="page-heading">
      <div>
        <span className="eyebrow">工作空间</span>
        <h1>{title}</h1>
        <p>{sub}</p>
      </div>
      {action}
    </div>
  );
}
export function RunTable({ runs }: { runs: RunListItem[] }) {
  return runs.length ? (
    <div className="table-scroll">
      <table>
        <thead>
          <tr>
            <th>审计任务</th>
            <th>项目 / 提交</th>
            <th>状态</th>
            <th>发现</th>
            <th>创建时间</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {runs.map((r) => (
            <tr key={r.id}>
              <td>
                <a className="task-title" href={"#/runs/" + r.id}>
                  <GitPullRequest size={17} />
                  {r.title || `MR !${r.mr_iid}`}
                </a>
                <small>
                  RUN #{r.id} · MR !{r.mr_iid}
                </small>
              </td>
              <td>
                项目 {r.project_id}
                <small className="mono">
                  {r.head_sha.slice(0, 10) || "历史记录"}
                </small>
              </td>
              <td>
                <Badge value={r.status} />
              </td>
              <td>{r.finding_count}</td>
              <td className="muted">{date(r.created_at)}</td>
              <td>
                <a href={"#/runs/" + r.id} aria-label={"查看任务 " + r.id}>
                  <ArrowUpRight size={17} />
                </a>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  ) : (
    <Empty>还没有审计任务。配置项目后，发起第一次审计。</Empty>
  );
}
