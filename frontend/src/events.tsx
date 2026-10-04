import { useState } from "react";

import { ErrorBox, date } from "./components";
import { useResource, Heading } from "./page-utils";
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
                <td>{e.action}</td>
                <td>
                  <code>{e.target}</code>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
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
