import { type Run } from "./api";
import { date } from "./components";

export function RetryExplanation({ run }: { run: Run }) {
  const info = run.retry_info;
  if (!info) return null;
  const source =
    (
      { model: "模型服务", gitlab: "GitLab", worker: "任务执行器" } as Record<
        string,
        string
      >
    )[info.source] || "上游服务";
  const reason =
    (
      {
        rate_limit: "请求受到限流",
        upstream_server: "服务暂时发生错误",
        temporary_network: "连接暂时失败",
        worker_interrupted: "任务执行中断，已保留检查点",
        transient_unknown: "暂时执行失败",
      } as Record<string, string>
    )[info.kind] || "执行中断";
  const stopped = (
    {
      model_budget_exhausted: "自动重试链已达到 token 停止阈值。",
      model_usage_unknown: "之前模型请求的用量未知，为避免重复消耗，已停止自动重试。",
      exhausted: "已达到两次自动重试上限。",
      wait_exceeds_limit: "上游要求的等待时间超过 24 小时，已停止自动重试。",
      policy_changed: "审计策略版本已变化，已停止自动重试。",
    } as Record<string, string>
  )[info.state];
  return (
    <aside className="coverage" aria-label="重试原因">
      <strong>{run.retry_parent_id ? "本次重试来源" : "执行中断原因"}</strong>
      <p>
        {source}：{reason}
        {info.http_status ? `（HTTP ${info.http_status}）` : ""}。
      </p>
      {info.header_state === "valid" &&
        info.retry_after_until &&
        !(
          run.status === "pending" &&
          info.eligible_at &&
          date(info.eligible_at) === date(info.retry_after_until)
        ) && <p>上游建议最早请求时间：{date(info.retry_after_until)}。</p>}
      {info.header_state === "invalid" && (
        <p>
          上游等待时间格式无效
          {info.state === "scheduled" ? "，采用默认退避时间" : ""}。
        </p>
      )}
      {stopped && <p>{stopped}</p>}
      {info.state === "scheduled" &&
        run.status === "pending" &&
        info.eligible_at && (
          <p>
            最早执行时间：{date(info.eligible_at)}
            {info.header_state === "valid" ? "（已遵守上游等待要求）" : ""}
            。实际执行还需满足队列与配额条件。
          </p>
        )}
    </aside>
  );
}
