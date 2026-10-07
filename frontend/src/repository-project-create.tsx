import { useEffect, useRef, useState } from "react";
import { api, APIError, write } from "./api";
import { ErrorBox, Empty } from "./components";
import { useResource } from "./page-utils";
import { RepositoryFields } from "./repository-form";
import {
  clearCreationAttempt,
  readCreationAttempt,
  storeCreationAttempt,
} from "./repository-project-attempt";
import {
  emptyRepositoryDraft,
  eligibleRepositoryProfiles,
  repositoryPayload,
  type RepositoryDraft,
  type RepositoryCreationInput,
  type RepositoryCreationReceipt,
  type RepositoryIntegration,
} from "./repository-types";
import "./repository-management.css";

export function RepositoryProjectCreate({
  userID,
  projectsReady,
  onSaved,
  onClose,
}: {
  userID: number;
  projectsReady: boolean;
  onSaved: () => Promise<void>;
  onClose: () => void;
}) {
  const profiles = useResource<{ items: RepositoryIntegration[] }>(
    "/integrations",
  );
  const [restored] = useState(() => {
    try {
      return { attempt: readCreationAttempt(userID), error: "" };
    } catch {
      return {
        attempt: null,
        error:
          "创建请求恢复记录无法读取。请核对项目列表后放弃该请求，或允许本站使用会话存储。",
      };
    }
  });
  const [attempt, setAttempt] = useState<RepositoryCreationInput | null>(
    restored.attempt,
  );
  const [name, setName] = useState(restored.attempt?.name || "");
  const [draft, setDraft] = useState<RepositoryDraft>(
    restored.attempt
      ? {
          provider: restored.attempt.provider,
          api_origin: restored.attempt.api_origin,
          remote_id: String(restored.attempt.remote_id),
          full_name: restored.attempt.full_name,
          integration_id: restored.attempt.integration_id,
        }
      : { ...emptyRepositoryDraft },
  );
  const [busy, setBusy] = useState(false),
    [locked, setLocked] = useState(!!restored.error),
    [denied, setDenied] = useState(false),
    [error, setError] = useState(restored.error),
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
  const ready = !profiles.loading && !profiles.error && !!profiles.data;
  const profile = ready
    ? eligibleRepositoryProfiles(profiles.data!.items, draft.provider).find(
        (p) => p.id === draft.integration_id,
      )
    : undefined;
  const discard = async () => {
    try {
      clearCreationAttempt(userID);
      setAttempt(null);
      setLocked(false);
      setError("");
      await profiles.load();
    } catch {
      setError("无法清除恢复记录，请检查本站会话存储权限。");
    }
  };
  const submit = async () => {
    if (busy || locked || denied || (!attempt && (!profile || !projectsReady)))
      return;
    setError("");
    setMessage("");
    let input = attempt;
    try {
      if (!input) {
        input = {
          ...repositoryPayload(draft),
          name,
          request_id: crypto.randomUUID(),
          expected_integration_revision: profile!.revision,
        };
        storeCreationAttempt(userID, input);
        setAttempt(input);
      }
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "无法保存重试记录，请检查会话存储。",
      );
      return;
    }
    setBusy(true);
    try {
      const receipt = await api<RepositoryCreationReceipt>(
        "/repository-projects",
        write("POST", input),
      );
      clearCreationAttempt(userID);
      if (!mounted.current) return;
      setAttempt(null);
      setName("");
      setDraft({ ...emptyRepositoryDraft });
      setMessage(
        `已创建项目 #${receipt.project.id}。仓库身份已保存，审计接入待完成。`,
      );
      await Promise.all([onSaved(), profiles.load()]);
    } catch (e) {
      if (!mounted.current) return;
      if (e instanceof APIError && [401, 403].includes(e.status)) {
        setDenied(true);
        setAttempt(null);
        setName("");
        setDraft({ ...emptyRepositoryDraft });
        setError("当前权限不可用，编辑内容已隐藏。请重新确认管理员权限。");
      } else if (e instanceof APIError && e.status === 409) {
        setLocked(true);
        setError(
          "配置或请求版本冲突。原请求已保留，请核对项目列表和凭据配置后再操作。",
        );
      } else if (e instanceof APIError && [400, 404].includes(e.status)) {
        try {
          clearCreationAttempt(userID);
          setAttempt(null);
        } catch {
          setLocked(true);
        }
        setError(e.message);
      } else {
        setError(
          e instanceof APIError && e.code === "project_config_sync_pending"
            ? "项目已创建，配置同步待恢复。请重试原请求完成同步。"
            : "创建结果待确认。请重试原请求，避免重复创建。",
        );
      }
    } finally {
      if (mounted.current) setBusy(false);
    }
  };
  return (
    <section
      className="panel repository-management"
      id="repository-create-panel"
      tabIndex={-1}
      ref={panel}
      aria-label="创建多平台仓库项目"
    >
      <h2>创建多平台仓库项目</h2>
      <p>
        系统分配独立的内部项目
        ID，并将新项目加入所选凭据配置的范围。成员权限需单独配置。
      </p>
      <ErrorBox error={error} />
      {message && <p role="status">{message}</p>}
      {attempt && (
        <p role="status">
          原创建请求已保留。请求 ID：{attempt.request_id}
          。关闭或重载页面后仍可重试。
        </p>
      )}
      {attempt && profiles.loading && <p>正在读取凭据配置；原请求仍可重试。</p>}
      {attempt && profiles.error && <ErrorBox error={profiles.error} />}
      {!denied && (
        <>
          {!attempt && profiles.loading ? (
            <Empty>加载仓库凭据…</Empty>
          ) : !attempt && profiles.error ? (
            <ErrorBox error={profiles.error} />
          ) : (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void submit();
              }}
            >
              <fieldset disabled={busy || locked || !!attempt || !ready}>
                <legend>新仓库身份</legend>
                <label>
                  项目名称
                  <input
                    required
                    maxLength={200}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </label>
                <RepositoryFields
                  draft={draft}
                  onChange={setDraft}
                  profiles={ready ? profiles.data!.items : []}
                  profilesReady={ready}
                />
              </fieldset>
              <button
                className="primary"
                disabled={
                  busy ||
                  locked ||
                  (!attempt && (!ready || !profile || !projectsReady))
                }
              >
                {busy ? "提交中…" : attempt ? "重试原创建请求" : "创建仓库项目"}
              </button>
            </form>
          )}
          {(attempt || locked) && (
            <button disabled={busy} onClick={() => void discard()}>
              已核对项目，放弃该请求并重新编辑
            </button>
          )}
          {!attempt && (
            <button
              disabled={busy || profiles.loading}
              onClick={() => void profiles.load()}
            >
              刷新凭据配置
            </button>
          )}
        </>
      )}
      <button disabled={busy} onClick={onClose}>
        关闭创建面板
      </button>
    </section>
  );
}
