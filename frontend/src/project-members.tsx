import { useEffect, useRef, useState } from "react";
import { api, write, type Project, type ProjectRole, type User } from "./api";
import { ErrorBox, Empty } from "./components";
import { useResource } from "./page-utils";
import "./project-members.css";

type Member = {
  user_id: number;
  username: string;
  role: ProjectRole;
  disabled: boolean;
};
export const projectRoleNames: Record<ProjectRole, string> = {
  viewer: "查看",
  reviewer: "复核",
  operator: "执行",
  admin: "管理员",
};
const roles = ["viewer", "reviewer", "operator"] as const;

export function ProjectMembers({
  project,
  onClose,
}: {
  project: Project;
  onClose: () => void;
}) {
  const panel = useRef<HTMLElement>(null);
  useEffect(() => {
    panel.current?.focus();
  }, []);
  const members = useResource<{ items: Member[] }>(
    `/projects/${project.id}/members`,
  );
  const users = useResource<{ items: User[] }>("/users");
  const [user, setUser] = useState("");
  const [role, setRole] = useState<ProjectRole>("viewer");
  const [drafts, setDrafts] = useState<Record<number, ProjectRole>>({});
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const candidates =
    users.data?.items.filter(
      (u) =>
        u.role === "member" &&
        !u.disabled &&
        !members.data?.items.some((m) => m.user_id === u.id),
    ) || [];
  const save = async (id: number, next?: ProjectRole) => {
    setBusy(true);
    setError("");
    try {
      await api(
        `/projects/${project.id}/members/${id}`,
        next ? write("PUT", { role: next }) : write("DELETE"),
      );
      await members.load();
      setDrafts((prev) => {
        const copy = { ...prev };
        delete copy[id];
        return copy;
      });
      setUser("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <section
      id="project-members-panel"
      ref={panel}
      tabIndex={-1}
      className="panel project-members"
      aria-label={`${project.name}成员权限`}
    >
      <div className="section-heading">
        <h2>{project.name} · 成员权限</h2>
        <button type="button" onClick={onClose}>
          关闭
        </button>
      </div>
      <p className="muted">
        查看：读取结果；复核：标记发现；执行：发起、重审及取消任务。移除执行权限会取消该成员在此项目中的未完成任务。管理员始终拥有完整权限。
      </p>
      <ErrorBox error={error || members.error || users.error} />
      <p className="muted">
        fork PR
        的源项目也需授予查看权限。源项目可以停用审计，保留代码调查所需的读取范围。
      </p>
      {members.loading ? (
        <Empty>加载成员权限…</Empty>
      ) : members.data?.items.length ? (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>成员</th>
                <th>项目角色</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {members.data.items.map((m) => (
                <tr key={m.user_id}>
                  <td>
                    {m.username}
                    {m.disabled && (
                      <small className="muted"> · 账号已禁用</small>
                    )}
                  </td>
                  <td>
                    <select
                      aria-label={`${m.username}的项目角色`}
                      disabled={busy}
                      value={drafts[m.user_id] || m.role}
                      onChange={(e) =>
                        setDrafts({
                          ...drafts,
                          [m.user_id]: e.target.value as ProjectRole,
                        })
                      }
                    >
                      {roles.map((r) => (
                        <option value={r} key={r}>
                          {projectRoleNames[r]}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <div className="actions">
                      <button
                        disabled={
                          busy ||
                          !drafts[m.user_id] ||
                          drafts[m.user_id] === m.role
                        }
                        onClick={() => void save(m.user_id, drafts[m.user_id])}
                      >
                        保存权限
                      </button>
                      <button
                        disabled={busy}
                        onClick={() => void save(m.user_id)}
                      >
                        移除权限
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <Empty>尚未授权成员。新成员默认无法访问此项目。</Empty>
      )}
      <form
        className="inline-form"
        onSubmit={(e) => {
          e.preventDefault();
          void save(Number(user), role);
        }}
      >
        <label>
          团队成员
          <select
            aria-label="授权成员"
            required
            disabled={busy || members.loading || users.loading}
            value={user}
            onChange={(e) => setUser(e.target.value)}
          >
            <option value="">选择成员</option>
            {candidates.map((u) => (
              <option value={u.id} key={u.id}>
                {u.username}
              </option>
            ))}
          </select>
        </label>
        <label>
          项目角色
          <select
            aria-label="授权角色"
            disabled={busy}
            value={role}
            onChange={(e) => setRole(e.target.value as ProjectRole)}
          >
            {roles.map((r) => (
              <option value={r} key={r}>
                {projectRoleNames[r]}
              </option>
            ))}
          </select>
        </label>
        <button
          className="primary"
          disabled={busy || !user || !members.data || !users.data}
        >
          {busy ? "保存中…" : "授予权限"}
        </button>
      </form>
      {!users.loading && !candidates.length && (
        <p className="muted">
          没有可新增授权的启用成员。可在团队成员页创建账号。
        </p>
      )}
    </section>
  );
}
