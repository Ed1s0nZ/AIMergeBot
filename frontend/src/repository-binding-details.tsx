import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import type { Project } from "./api";
import { ErrorBox, Empty } from "./components";
import type { RepositoryBinding } from "./repository-types";
import "./repository-management.css";

export function RepositoryBindingDetails({
  project,
  onClose,
}: {
  project: Project;
  onClose: () => void;
}) {
  const [binding, setBinding] = useState<RepositoryBinding | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const panel = useRef<HTMLElement>(null);
  useEffect(() => {
    panel.current?.focus();
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setBinding(null);
    setError("");
    setLoading(true);
    api<RepositoryBinding>(`/projects/${project.id}/repository-binding`, {
      signal: controller.signal,
    })
      .then((value) => {
        if (!controller.signal.aborted) setBinding(value);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError((e as Error).message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [project.id, attempt]);
  return (
    <section
      className="panel repository-management"
      id="repository-binding-panel"
      tabIndex={-1}
      ref={panel}
      aria-label={`${project.name} 的仓库绑定`}
    >
      <h2>{project.name} 的仓库绑定</h2>
      <p>原生仓库审计接入尚未开放，此处仅展示已保存的身份。</p>
      <ErrorBox error={error} />
      {loading ? (
        <Empty>加载仓库绑定…</Empty>
      ) : (
        binding &&
        (binding.revision === 0 ? (
          <p>当前使用默认 GitLab 配置，尚无显式仓库绑定。</p>
        ) : (
          <dl>
            <dt>平台</dt>
            <dd>{binding.provider === "github" ? "GitHub" : "GitLab"}</dd>
            <dt>仓库</dt>
            <dd>{binding.full_name}</dd>
            <dt>API 地址</dt>
            <dd>{binding.api_origin}</dd>
            <dt>远端仓库 ID</dt>
            <dd>{binding.remote_id}</dd>
            <dt>绑定版本</dt>
            <dd>{binding.revision}</dd>
            <dt>凭据配置 ID</dt>
            <dd>{binding.integration_id}</dd>
          </dl>
        ))
      )}
      <button disabled={loading} onClick={() => setAttempt((v) => v + 1)}>
        重新加载绑定
      </button>
      <button onClick={onClose}>关闭绑定面板</button>
    </section>
  );
}
