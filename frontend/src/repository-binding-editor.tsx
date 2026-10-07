import { useEffect, useRef, useState } from "react";
import { api, APIError, write, type Project } from "./api";
import { ErrorBox, Empty } from "./components";
import { RepositoryFields } from "./repository-form";
import {
  eligibleRepositoryProfiles,
  emptyRepositoryDraft,
  repositoryPayload,
  type RepositoryBinding,
  type RepositoryDraft,
  type RepositoryIntegration,
} from "./repository-types";
import "./repository-management.css";
export function RepositoryBindingEditor({
  project,
  onSaved,
  onClose,
}: {
  project: Project;
  onSaved: () => Promise<void>;
  onClose: () => void;
}) {
  const [attempt, setAttempt] = useState(0),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [locked, setLocked] = useState(false);
  const [revision, setRevision] = useState<number | null>(null),
    [draft, setDraft] = useState<RepositoryDraft>({ ...emptyRepositoryDraft }),
    [profiles, setProfiles] = useState<RepositoryIntegration[]>([]),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const panel = useRef<HTMLElement>(null),
    mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    panel.current?.focus();
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    const options = { signal: controller.signal };
    setLoading(true);
    setRevision(null);
    setProfiles([]);
    setDraft({ ...emptyRepositoryDraft });
    setLocked(false);
    setError("");
    Promise.all([
      api<RepositoryBinding>(
        `/projects/${project.id}/repository-binding`,
        options,
      ),
      api<{ items: RepositoryIntegration[] }>("/integrations", options),
    ])
      .then(([binding, accounts]) => {
        if (controller.signal.aborted) return;
        setRevision(binding.revision);
        setProfiles(accounts.items);
        if (binding.revision > 0)
          setDraft({
            provider: binding.provider,
            api_origin: binding.api_origin,
            remote_id: String(binding.remote_id),
            full_name: binding.full_name,
            integration_id: binding.integration_id,
          });
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError((e as Error).message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [project.id, attempt]);
  const available = eligibleRepositoryProfiles(
    profiles,
    draft.provider,
    project.id,
  ).some((p) => p.id === draft.integration_id);
  const save = async () => {
    if (busy || locked || revision === null || !available) return;
    let payload: ReturnType<typeof repositoryPayload>;
    try {
      payload = repositoryPayload(draft);
    } catch (e) {
      setError((e as Error).message);
      return;
    }
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const saved = await api<RepositoryBinding>(
        `/projects/${project.id}/repository-binding`,
        write("PATCH", { ...payload, expected_revision: revision }),
      );
      if (!mounted.current) return;
      setRevision(saved.revision);
      setDraft({
        provider: saved.provider,
        api_origin: saved.api_origin,
        remote_id: String(saved.remote_id),
        full_name: saved.full_name,
        integration_id: saved.integration_id,
      });
      setMessage("仓库绑定已保存。当前审计接入待完成，历史任务保留原身份。");
      await onSaved();
    } catch (e) {
      if (!mounted.current) return;
      if (e instanceof APIError && e.status === 409) {
        setLocked(true);
        setError(
          "绑定版本或凭据配置已变化，草稿已保留。请核对后丢弃草稿并重新加载。",
        );
      } else if (e instanceof APIError && [401, 403, 404].includes(e.status)) {
        setRevision(null);
        setProfiles([]);
        setDraft({ ...emptyRepositoryDraft });
        setError("配置或权限不可用，已清除编辑内容。");
      } else {
        setLocked(true);
        setError(
          e instanceof APIError && e.code === "project_config_sync_pending"
            ? "绑定已保存，配置同步待恢复。请重新加载确认新版本，不能按旧版本再次保存。"
            : "保存结果待确认，请重新加载后核对绑定版本。",
        );
      }
    } finally {
      if (mounted.current) setBusy(false);
    }
  };
  return (
    <section
      className="panel repository-management"
      id="repository-binding-panel"
      tabIndex={-1}
      ref={panel}
      aria-label={`${project.name} 的仓库绑定`}
    >
      <h2>{project.name} 的仓库绑定</h2>
      <p>
        保存绑定后，该项目当前的审计与发布会暂停，直到平台接入完成。已有任务不会自动按新仓库解释。
      </p>
      <ErrorBox error={error} />
      {message && <p role="status">{message}</p>}
      {loading ? (
        <Empty>加载仓库绑定…</Empty>
      ) : (
        revision !== null && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void save();
            }}
          >
            <p>
              绑定版本 {revision}
              {revision === 0 && " · 当前使用默认 GitLab 配置"}
            </p>
            <fieldset disabled={busy || locked}>
              <legend>仓库身份</legend>
              <RepositoryFields
                draft={draft}
                onChange={setDraft}
                profiles={profiles}
                projectID={project.id}
              />
            </fieldset>
            <button
              className="primary"
              disabled={busy || locked || !available || !project.enabled}
            >
              {busy ? "保存中…" : "保存仓库绑定"}
            </button>
            {!project.enabled && <p>请先启用项目再保存绑定。</p>}
          </form>
        )
      )}
      <button
        disabled={busy || loading}
        onClick={() => {
          setMessage("");
          setAttempt((v) => v + 1);
        }}
      >
        丢弃草稿并重新加载
      </button>
      <button disabled={busy} onClick={onClose}>
        关闭绑定面板
      </button>
    </section>
  );
}
