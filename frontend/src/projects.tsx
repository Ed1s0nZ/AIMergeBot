import { useState } from "react";
import { GitPullRequest, Plus } from "lucide-react";
import { api, write, type Project } from "./api";
import { Badge, ErrorBox, Empty } from "./components";
import { useResource, Heading } from "./page-utils";
export function Projects({ admin }: { admin: boolean }) {
  const resource = useResource<{ items: Project[] }>("/projects"),
    [id, setId] = useState(""),
    [name, setName] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <>
      <Heading title="项目" sub="管理团队的 GitLab 审计范围。" />
      <ErrorBox error={error || resource.error} />
      {admin && (
        <form
          className="panel inline-form"
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
          <input
            aria-label="GitLab 项目 ID"
            type="number"
            min="1"
            required
            placeholder="GitLab 项目 ID"
            value={id}
            onChange={(e) => setId(e.target.value)}
          />
          <input
            aria-label="项目名称"
            required
            maxLength={200}
            placeholder="group/project"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <button className="primary" disabled={busy}>
            <Plus size={16} />
            添加项目
          </button>
        </form>
      )}
      <div className="project-grid">
        {resource.data?.items.map((p) => (
          <article className="panel project-card" key={p.id}>
            <div className="project-icon">
              <GitPullRequest />
            </div>
            <h2>{p.name}</h2>
            <p>GitLab #{p.id}</p>
            <Badge value={p.enabled ? "已启用" : "已停用"} />
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
                {p.enabled ? "停用审计" : "启用审计"}
              </button>
            )}
          </article>
        ))}
      </div>
      {resource.loading ? (
        <Empty>加载项目…</Empty>
      ) : (
        !resource.data?.items.length && (
          <Empty>
            {admin
              ? "添加一个 GitLab 项目开始使用。"
              : "请联系管理员配置项目。"}
          </Empty>
        )
      )}
    </>
  );
}
