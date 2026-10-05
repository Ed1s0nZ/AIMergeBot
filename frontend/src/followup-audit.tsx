import { useEffect, useState } from "react";
import { api, write, type Run } from "./api";
import { ErrorBox } from "./components";
import "./followup-audit.css";
type RunScope = {
  followup_of?: number;
  selected_files: string[];
  base_sha: string;
  head_sha: string;
  files: { path: string; status: string; selectable: boolean }[];
  notes: string[];
  truncated: boolean;
};
export function FollowupAuditPanel({
  run,
  canSubmit,
}: {
  run: Run;
  canSubmit: boolean;
}) {
  const [scope, setScope] = useState<RunScope | null>(null),
    [selected, setSelected] = useState<string[]>([]),
    [loading, setLoading] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [limit, setLimit] = useState(100);
  useEffect(() => {
    setScope(null);
    setSelected([]);
    setError("");
    setLimit(100);
  }, [run.id]);
  const terminal = !["pending", "running"].includes(run.status);
  const load = async () => {
    setLoading(true);
    setError("");
    try {
      setScope(await api<RunScope>(`/runs/${run.id}/scope`));
      setSelected([]);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  };
  const labels: Record<string, string> = {
    included: "纳入本次输入",
    group_failed: "所属分组审计失败",
    group_unprocessed: "所属分组未完成",
    not_in_input: "未纳入输入",
    outside_scope: "此前未选择",
    excluded: "按策略排除",
  };
  return (
    <section className="panel trace" aria-label="选文件补审">
      <h2>选文件补审</h2>
      <p className="muted">
        沿用此任务的固定版本与策略，新建独立审计。可检索全仓库上下文，仅对选择的变更文件报告问题；原结果和人工复核保留。
      </p>
      {!terminal && <p className="muted">等待当前任务结束后可提交补审。</p>}
      {!canSubmit && (
        <p className="muted">提交需要项目操作权限，且项目处于启用状态。</p>
      )}
      <button type="button" disabled={loading || busy} onClick={load}>
        {loading
          ? "正在读取固定版本…"
          : scope
            ? "刷新变更清单"
            : "查看固定版本变更文件"}
      </button>
      <ErrorBox error={error} />
      {scope && (
        <>
          {scope.followup_of && (
            <p>
              当前是原任务{" "}
              <a href={`#/runs/${scope.followup_of}`}>#{scope.followup_of}</a>{" "}
              的选文件补审。
            </p>
          )}
          <p className="muted">
            “纳入输入”不代表已完成深度审计，请结合任务状态、分组状态和覆盖说明判断。
          </p>
          {!scope.files.length && <p>固定版本没有可展示的变更文件。</p>}
          <div className="followup-file-list">
            {scope.files.slice(0, limit).map((file) => (
              <label className="followup-file" key={file.path}>
                <input
                  type="checkbox"
                  disabled={!file.selectable || !canSubmit || !terminal || busy}
                  checked={selected.includes(file.path)}
                  onChange={(e) => {
                    if (e.target.checked && selected.length >= 100) {
                      setError("每次最多选择100个文件，可分批补审。");
                      return;
                    }
                    setError("");
                    setSelected(
                      e.target.checked
                        ? [...selected, file.path]
                        : selected.filter((p) => p !== file.path),
                    );
                  }}
                />
                <span className="mono">
                  {file.path}
                  <small>{labels[file.status] || file.status}</small>
                </span>
              </label>
            ))}
          </div>
          {limit < scope.files.length && (
            <button type="button" onClick={() => setLimit(limit + 100)}>
              再显示100个文件
            </button>
          )}
          {scope.truncated && (
            <p className="muted">
              范围信息达到展示上限（文件或说明），请结合原任务证据判断缺失。
            </p>
          )}
          {scope.notes.length > 0 && (
            <details>
              <summary>固定版本覆盖说明</summary>
              <ul>
                {scope.notes.map((note, i) => (
                  <li key={i}>{note}</li>
                ))}
              </ul>
            </details>
          )}
          <p>已选择 {selected.length} 个文件（每批最多100个）。</p>
          <button
            type="button"
            className="primary"
            disabled={!canSubmit || !terminal || busy || !selected.length}
            onClick={async () => {
              setBusy(true);
              setError("");
              try {
                const next = await api<{ id: number; created: boolean }>(
                  `/runs/${run.id}/followup`,
                  write("POST", { files: selected }),
                );
                location.hash = `/runs/${next.id}`;
              } catch (e) {
                setError((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            {busy ? "正在提交…" : "提交固定版本补审"}
          </button>
        </>
      )}
    </section>
  );
}
