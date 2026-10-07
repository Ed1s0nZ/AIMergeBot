import { useState } from "react";
import { api, APIError, write } from "./api";
import { ErrorBox, Empty, date } from "./components";
import { useResource } from "./page-utils";
import {
  NotificationRetryControl,
  type RetryAvailability,
} from "./notification-retry-control";

type Delivery = RetryAvailability & {
  id: number;
  integration_id: number;
  project_id: number;
  run_id: number;
  attempt: number;
  error_code: string;
  created_at: string;
};
const labels: Record<string, string> = {
  pending: "待发送",
  sending: "发送中",
  delivered: "渠道确认送达",
  accepted: "渠道已接受",
  failed: "失败",
  unknown: "结果不确定",
  retry: "等待重试",
  cancelled: "已取消",
};

export function NotificationRecords() {
  const [page, setPage] = useState(1);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [ack, setAck] = useState<Record<number, boolean>>({});
  const resource = useResource<{
    items: Delivery[];
    total: number;
    size: number;
  }>(`/deliveries?page=${page}&size=20`);

  async function retry(d: Delivery) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api(
        `/deliveries/${d.id}/retry`,
        write("POST", {
          expected_attempt: d.attempt,
          expected_status: d.status,
          acknowledge_duplicate: !!ack[d.id],
        }),
      );
      setAck({});
      await resource.load();
    } catch (e) {
      if (e instanceof APIError && e.status === 409) {
        setAck({});
        await resource.load();
        setError("投递状态或渠道配置已变化，已重新查询记录；请核对后再操作。");
      } else setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel">
      <div className="section-heading">
        <h2>投递记录</h2>
        <button disabled={busy} onClick={resource.load}>
          刷新记录
        </button>
      </div>
      <ErrorBox error={[error, resource.error].filter(Boolean).join(" ")} />
      {resource.loading ? (
        <Empty>加载投递记录…</Empty>
      ) : !resource.error && resource.data ? (
        <>
          {!resource.data.items.length && <Empty>没有投递记录。</Empty>}
          {resource.data.items.map((d) => (
            <article className="workspace-finding" key={d.id}>
              <strong>
                #{d.id} · {labels[d.status] || d.status}
              </strong>
              <p>
                渠道 #{d.integration_id} · 项目 {d.project_id} · 尝试{" "}
                {d.attempt}/5 · {date(d.created_at)}
              </p>
              {d.run_id > 0 && <a href={`#/runs/${d.run_id}`}>查看审计</a>}
              {d.error_code && <p>原因：{d.error_code}</p>}
              <NotificationRetryControl
                delivery={d}
                busy={busy}
                acknowledged={!!ack[d.id]}
                onAcknowledge={(value) =>
                  setAck((previous) => ({ ...previous, [d.id]: value }))
                }
                onRetry={() => retry(d)}
              />
            </article>
          ))}
        </>
      ) : null}
      <div className="inline-form">
        <button
          disabled={busy || resource.loading || page <= 1}
          onClick={() => setPage(page - 1)}
        >
          上一页
        </button>
        <span>
          第 {page} 页
          {!resource.error && !resource.loading && resource.data
            ? ` · 共 ${resource.data.total} 条`
            : ""}
        </span>
        <button
          disabled={
            busy ||
            resource.loading ||
            !!resource.error ||
            !resource.data ||
            page * resource.data.size >= resource.data.total
          }
          onClick={() => setPage(page + 1)}
        >
          下一页
        </button>
      </div>
    </section>
  );
}
