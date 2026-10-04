import { useState } from "react";

import { ErrorBox, Empty, date } from "./components";
import { useResource, Heading } from "./page-utils";
const actions: Record<string, string> = {
  "settings.saved": "更新系统设置",
  "project.saved": "保存项目",
  "user.created": "创建成员",
  "user.updated": "更新成员",
  "run.created": "提交审计",
  "run.cancelled": "取消审计",
  "finding.reviewed": "复核发现",
};
function EventTarget({ target }: { target: string }) {
  try {
    const value = JSON.parse(target) as {
      run_id?: number;
      finding_id?: string;
      reason?: string;
    };
    if (value && typeof value === "object" && value.run_id) {
      return (
        <div className="event-target">
          <a href={"#/runs/" + value.run_id}>审计任务 #{value.run_id}</a>
          <details>
            <summary>查看记录</summary>
            <pre>{JSON.stringify(value, null, 2)}</pre>
          </details>
        </div>
      );
    }
  } catch {
    /* Scalar event targets retain their original display. */
  }
  return <code className="event-target-scalar">{target}</code>;
}
export function Events() {
  const [page, setPage] = useState(1);
  const resource = useResource<{
    items: {
      id: number;
      actor: number;
      action: string;
      target: string;
      created_at: string;
    }[];
  }>("/events?page=" + page);
  return (
    <>
      <Heading title="操作日志" sub="追踪项目、账号、任务与复核记录的变更。" />
      <ErrorBox error={resource.error} />
      <section className="panel table-scroll">
        {resource.loading ? (
          <Empty>加载操作记录…</Empty>
        ) : !resource.data?.items.length ? (
          <Empty>暂无操作记录。团队的配置与复核操作将记录在这里。</Empty>
        ) : (
          <>
            <table>
              <thead>
                <tr>
                  <th>时间</th>
                  <th>用户</th>
                  <th>操作</th>
                  <th>对象</th>
                </tr>
              </thead>
              <tbody>
                {resource.data?.items.map((e) => (
                  <tr key={e.id}>
                    <td>{date(e.created_at)}</td>
                    <td>#{e.actor}</td>
                    <td>
                      <span className="event-action">
                        {actions[e.action] || e.action}
                      </span>
                    </td>
                    <td>
                      <EventTarget target={e.target} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </section>
      <div className="pagination">
        <button disabled={page === 1} onClick={() => setPage(page - 1)}>
          上一页
        </button>
        <span>{page}</span>
        <button
          disabled={!resource.data || resource.data.items.length < 20}
          onClick={() => setPage(page + 1)}
        >
          下一页
        </button>
      </div>
    </>
  );
}
