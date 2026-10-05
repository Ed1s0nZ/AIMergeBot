import { ErrorBox } from "./components";

// Call only when a previous successful resource is still visible. Permission
// failures clear it through useResource and must use the initial error state.
export function ResourceRefreshFailure({ error, loading, onRetry }: {
  error: string;
  loading: boolean;
  onRetry: () => void;
}) {
  if (!error) return null;
  return <section className="coverage" aria-label="刷新状态">
    <strong>未能获取最新任务状态</strong>
    <p>以下显示最近一次成功获取的记录，可能已有变化。请重新获取后再判断当前审计和同步进度。</p>
    <ErrorBox error={error} />
    <button type="button" disabled={loading} onClick={onRetry}>{loading ? "正在重试…" : "重新获取最新任务"}</button>
  </section>;
}
