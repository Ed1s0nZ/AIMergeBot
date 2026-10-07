import { RepositoryProjectCreate } from "./repository-project-create";
import { RepositoryBindingEditor } from "./repository-binding-editor";
import { OwnerRoutingEditor } from "./owner-routing";
import { ContextRepositories } from "./context-repositories";
import { useRef, useState } from "react";
import { Plus, FolderGit2 } from "lucide-react";
import { api, write, type Project } from "./api";
import { ErrorBox, Empty } from "./components";
import { useResource, Heading } from "./page-utils";
import { ProjectMembers, projectRoleNames } from "./project-members";
export function Projects({
  admin,
  userID,
}: {
  admin: boolean;
  userID: number;
}) {
  const [creating, setCreating] = useState(false);
  const [repositoryMessage, setRepositoryMessage] = useState("");
  const [bindingProject, setBindingProject] = useState<Project | null>(null);
  const trigger = useRef<HTMLButtonElement | null>(null);
  const closeRepository = () => {
    setCreating(false);
    setBindingProject(null);
    trigger.current?.focus();
  };
  const [ownerProject, setOwnerProject] = useState<Project | null>(null);
  const [contextProject, setContextProject] = useState<Project | null>(null);
  const [selectedProject, setSelectedProject] = useState<Project | null>(null);
  const resource = useResource<{ items: Project[] }>("/projects"),
    [id, setId] = useState(""),
    [name, setName] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <>
      <Heading
        title="项目"
        sub="管理仓库身份、团队权限与审计范围。"
        action={
          <>
            <button
              disabled={resource.loading}
              onClick={() => void resource.load()}
            >
              刷新项目
            </button>
            {admin && (
              <button
                disabled={resource.loading || !!resource.error}
                ref={creating ? trigger : undefined}
                aria-expanded={creating}
                aria-controls="repository-create-panel"
                onClick={(e) => {
                  trigger.current = e.currentTarget;
                  setBindingProject(null);
                  setCreating(true);
                  setRepositoryMessage("");
                  setOwnerProject(null);
                  setContextProject(null);
                  setSelectedProject(null);
                }}
              >
                创建多平台仓库
              </button>
            )}
          </>
        }
      />
      <ErrorBox error={error || resource.error} />
      {repositoryMessage && <p role="status">{repositoryMessage}</p>}
      {admin && (
        <details className="panel create-panel">
          <summary>
            <Plus size={17} />
            <strong>添加默认 GitLab 项目</strong>
            <span>接入新的 GitLab 仓库</span>
          </summary>
          <form
            className="inline-form create-form"
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              setError("");
              try {
                await api(
                  "/projects",
                  write("POST", { id: Number(id), name, enabled: true }),
                );
                setId("");
                setName("");
                await resource.load();
              } catch (e) {
                setError((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            <label>
              GitLab 项目 ID
              <input
                aria-label="GitLab 项目 ID"
                type="number"
                min="1"
                required
                placeholder="GitLab 项目 ID"
                value={id}
                onChange={(e) => setId(e.target.value)}
              />
            </label>
            <label>
              项目名称
              <input
                aria-label="项目名称"
                required
                maxLength={200}
                placeholder="group/project"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </label>
            <button className="primary" disabled={busy}>
              <Plus size={16} />
              添加项目
            </button>
          </form>
        </details>
      )}
      <div className="project-grid">
        {!resource.loading &&
          !resource.error &&
          resource.data?.items.map((p) => (
            <article className="panel project-card" key={p.id}>
              <div className="project-icon">
                <FolderGit2 />
              </div>
              <h2>{p.name}</h2>
              <span className="repository-caption">
                {p.repository
                  ? `${p.repository.provider === "github" ? "GitHub" : "GitLab"} · ${p.repository.full_name}`
                  : "GitLab（默认配置）"}
              </span>
              <p>
                {p.repository
                  ? `内部项目 #${p.id} · 远端仓库 #${p.repository.remote_id}`
                  : `GitLab #${p.id}`}
              </p>
              {!admin && p.access_role && (
                <p className="muted">
                  项目权限：{projectRoleNames[p.access_role]}
                </p>
              )}
              <span
                className={"project-status " + (p.enabled ? "enabled" : "")}
              >
                <i />
                {!p.enabled
                  ? "项目已停用"
                  : p.repository && !p.repository_execution_available
                    ? "已配置 · 审计接入待完成"
                    : "审计已启用"}
              </span>
              {admin && (
                <button
                  ref={bindingProject?.id === p.id ? trigger : undefined}
                  aria-expanded={bindingProject?.id === p.id}
                  aria-controls="repository-binding-panel"
                  onClick={(e) => {
                    trigger.current = e.currentTarget;
                    setCreating(false);
                    setBindingProject(p);
                    setRepositoryMessage("");
                    setOwnerProject(null);
                    setContextProject(null);
                    setSelectedProject(null);
                  }}
                >
                  仓库绑定
                </button>
              )}
              {admin && (
                <button
                  aria-expanded={selectedProject?.id === p.id}
                  aria-controls="project-members-panel"
                  onClick={() => {
                    setOwnerProject(null);
                    setContextProject(null);
                    setSelectedProject(p);
                  }}
                >
                  成员权限
                </button>
              )}
              {admin && (
                <button
                  aria-expanded={contextProject?.id === p.id}
                  aria-controls="context-repositories-panel"
                  onClick={() => {
                    setOwnerProject(null);
                    setSelectedProject(null);
                    setContextProject(p);
                  }}
                >
                  关联仓库
                </button>
              )}
              {admin && (
                <button
                  aria-expanded={ownerProject?.id === p.id}
                  aria-controls="owner-routing-panel"
                  onClick={() => {
                    setSelectedProject(null);
                    setContextProject(null);
                    setOwnerProject(p);
                  }}
                >
                  责任人配置
                </button>
              )}
              {admin && (
                <button
                  disabled={busy}
                  onClick={async () => {
                    setBusy(true);
                    try {
                      await api(
                        "/projects/" + p.id,
                        write("PATCH", { ...p, enabled: !p.enabled }),
                      );
                      await resource.load();
                    } catch (e) {
                      setError((e as Error).message);
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  {p.enabled ? "停用项目" : "启用项目"}
                </button>
              )}
            </article>
          ))}
      </div>
      {admin && creating && (
        <RepositoryProjectCreate
          key={userID}
          userID={userID}
          projectsReady={
            !resource.loading && !resource.error && !!resource.data
          }
          onSaved={resource.load}
          onClose={closeRepository}
        />
      )}
      {admin &&
        bindingProject &&
        !resource.loading &&
        !resource.error &&
        resource.data?.items.some((p) => p.id === bindingProject.id) && (
          <RepositoryBindingEditor
            key={bindingProject.id}
            project={
              resource.data?.items.find((p) => p.id === bindingProject.id) ||
              bindingProject
            }
            onSaved={async () => {
              setRepositoryMessage(
                "仓库绑定已保存。当前审计接入待完成，历史任务保留原身份。",
              );
              await resource.load();
            }}
            onClose={closeRepository}
          />
        )}
      {admin && contextProject && (
        <ContextRepositories
          key={contextProject.id}
          project={
            resource.data?.items.find(
              (item) => item.id === contextProject.id,
            ) || contextProject
          }
          projects={resource.data?.items || []}
          onClose={() => setContextProject(null)}
        />
      )}
      {admin && ownerProject && (
        <OwnerRoutingEditor
          key={ownerProject.id}
          project={ownerProject}
          onClose={() => setOwnerProject(null)}
        />
      )}
      {admin && selectedProject && (
        <ProjectMembers
          key={selectedProject.id}
          project={selectedProject}
          onClose={() => setSelectedProject(null)}
        />
      )}
      {resource.loading ? (
        <Empty>加载项目…</Empty>
      ) : (
        !resource.data?.items.length && (
          <Empty>
            {admin
              ? "可添加默认 GitLab 项目，或先在集成页配置凭据再创建多平台仓库。"
              : "请联系管理员配置项目。"}
          </Empty>
        )
      )}
    </>
  );
}
