import { useEffect, useRef, useState } from "react";
import { Plus, Trash2, X, GitBranch, Save } from "lucide-react";
import {
  api,
  write,
  type ContextRepository,
  type Project,
  type Run,
} from "./api";
import { ErrorBox, Empty } from "./components";
import { useResource } from "./page-utils";
import "./context-repositories.css";

export function ContextRepositories({
  project,
  projects,
  onClose,
}: {
  project: Project;
  projects: Project[];
  onClose: () => void;
}) {
  const panel = useRef<HTMLElement>(null);
  const resource = useResource<{ items: ContextRepository[] }>(
    `/projects/${project.id}/context-repositories`,
  );
  const [draft, setDraft] = useState<ContextRepository[]>([]);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => {
    panel.current?.focus();
  }, []);
  useEffect(() => {
    if (resource.data)
      setDraft(resource.data.items.map((item) => ({ ...item })));
  }, [resource.data]);
  const save = async () => {
    if (
      draft.some(
        (item) =>
          item.project_id <= 0 ||
          !/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/i.test(item.sha),
      ) ||
      new Set(draft.map((item) => item.project_id)).size !== draft.length
    ) {
      setError("请选择不同的关联项目，并填写完整的 40 或 64 位提交 SHA。");
      return;
    }
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const result = await api<{ config_sync_pending: boolean }>(
        `/projects/${project.id}/context-repositories`,
        write("PUT", { items: draft }),
      );
      setMessage(
        result.config_sync_pending
          ? "授权已保存。配置文件同步待恢复，再次保存可重试同步。"
          : "关联仓库已保存并同步到配置文件。",
      );
      await resource.load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const available = projects.filter((item) => item.id !== project.id);
  return (
    <section
      className="panel context-repositories"
      id="context-repositories-panel"
      ref={panel}
      tabIndex={-1}
      aria-label={`${project.name} 关联仓库`}
    >
      <div className="section-heading">
        <div>
          <h2>
            <GitBranch size={19} />
            关联仓库
          </h2>
          <p className="muted">
            {project.name} · 为审计提供固定版本的跨仓库上下文
          </p>
        </div>
        <button
          type="button"
          onClick={onClose}
          disabled={busy}
          aria-label="关闭关联仓库配置"
        >
          <X size={16} />
        </button>
      </div>
      <p className="muted">
        最多关联 4 个已登记项目。填写实际提交
        SHA，审计会冻结此版本；提交者和报告读者需要所有关联仓库的查看权限。
      </p>
      <p className="context-boundary">
        跨仓库报告仅在工作台提供，不自动发布到
        MR。更换或移除授权会停止使用旧授权的未完成任务，并保留已有证据。
      </p>
      <ErrorBox error={error || resource.error} />
      {resource.loading && !resource.data ? (
        <Empty>加载关联仓库…</Empty>
      ) : resource.error && !resource.data ? (
        <button onClick={resource.load}>重试加载</button>
      ) : (
        resource.data && (
          <>
            <div className="context-repository-rows">
              {!draft.length && (
                <p className="muted">
                  尚未关联仓库。当前审计只使用主 PR 仓库。
                </p>
              )}
              {draft.map((item, index) => (
                <div className="context-repository-row" key={index}>
                  <label>
                    关联项目
                    <select
                      aria-label={`关联项目 ${index + 1}`}
                      value={item.project_id || ""}
                      disabled={busy}
                      onChange={(event) =>
                        setDraft((previous) =>
                          previous.map((entry, i) =>
                            i === index
                              ? {
                                  ...entry,
                                  project_id: Number(event.target.value),
                                }
                              : entry,
                          ),
                        )
                      }
                    >
                      <option value="">选择项目</option>
                      {available.map((candidate) => (
                        <option
                          key={candidate.id}
                          value={candidate.id}
                          disabled={
                            !candidate.enabled ||
                            draft.some(
                              (entry, i) =>
                                i !== index &&
                                entry.project_id === candidate.id,
                            )
                          }
                        >
                          {candidate.name} · #{candidate.id}
                          {candidate.enabled ? "" : "（已停用）"}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    固定提交 SHA
                    <input
                      aria-label={`固定提交 SHA ${index + 1}`}
                      value={item.sha}
                      maxLength={64}
                      spellCheck={false}
                      autoComplete="off"
                      placeholder="完整 Git 提交 SHA"
                      disabled={busy}
                      onChange={(event) =>
                        setDraft((previous) =>
                          previous.map((entry, i) =>
                            i === index
                              ? { ...entry, sha: event.target.value.trim() }
                              : entry,
                          ),
                        )
                      }
                    />
                  </label>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() =>
                      setDraft((previous) =>
                        previous.filter((_, i) => i !== index),
                      )
                    }
                    aria-label={`移除关联仓库 ${index + 1}`}
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              ))}
            </div>
            {!project.enabled && (
              <p className="muted">项目已停用，启用后可修改关联授权。</p>
            )}
            <div className="actions context-repository-actions">
              <button
                type="button"
                disabled={
                  busy ||
                  draft.length >= 4 ||
                  !project.enabled ||
                  !available.some((item) => item.enabled)
                }
                onClick={() =>
                  setDraft((previous) => [
                    ...previous,
                    { project_id: 0, sha: "" },
                  ])
                }
              >
                <Plus size={16} />
                添加关联仓库
              </button>
              <button
                type="button"
                className="primary"
                disabled={busy || !project.enabled}
                onClick={save}
              >
                <Save size={16} />
                {busy ? "保存中…" : "保存关联仓库"}
              </button>
            </div>
            {message && (
              <p role="status" className="muted">
                {message}
              </p>
            )}
          </>
        )
      )}
    </section>
  );
}

export function FrozenContextRepositories({ run }: { run: Run }) {
  const items = run.audit_policy?.context_repositories || [];
  if (!items.length) return null;
  return (
    <section
      className="panel context-repositories"
      aria-label="固定跨仓库审计范围"
    >
      <h2>
        <GitBranch size={19} />
        跨仓库上下文
      </h2>
      <p className="muted">
        本任务使用以下固定版本。主 PR
        是问题锚点，关联仓库用于条件、调用背景和反证调查；读取源码不等于确认运行链路。
      </p>
      <div className="context-fixed-list">
        {items.map((item) => (
          <div key={item.project_id}>
            <strong>仓库 #{item.project_id}</strong>
            <code title={item.sha}>{item.sha}</code>
          </div>
        ))}
      </div>
      <p className="context-boundary">
        报告需要上述仓库的权限交集，仅在工作台提供，不自动发布到 MR。
      </p>
    </section>
  );
}
