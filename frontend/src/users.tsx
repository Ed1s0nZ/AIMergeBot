import { Plus, Users } from "lucide-react";
import { useState } from "react";

import { api, write, type User } from "./api";
import { ErrorBox, Empty } from "./components";
import { useResource, Heading } from "./page-utils";
function UserRow({ user, onSaved }: { user: User; onSaved: () => void }) {
  const [role, setRole] = useState(user.role),
    [disabled, setDisabled] = useState(user.disabled),
    [password, setPassword] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <tr>
      <td>
        <div className="member-identity">
          <span className="member-avatar">
            {user.username[0].toUpperCase()}
          </span>
          <span>
            <strong>{user.username}</strong>
            <small>用户 #{user.id}</small>
          </span>
        </div>
        <ErrorBox error={error} />
      </td>
      <td>
        <select
          aria-label={user.username + " 角色"}
          value={role}
          onChange={(e) => setRole(e.target.value as User["role"])}
        >
          <option value="member">成员</option>
          <option value="admin">管理员</option>
        </select>
      </td>
      <td>
        <label className="check">
          <input
            type="checkbox"
            checked={disabled}
            onChange={(e) => setDisabled(e.target.checked)}
          />
          禁用
        </label>
      </td>
      <td>
        <div className="member-actions">
          <input
            type="password"
            autoComplete="new-password"
            aria-label={user.username + " 新密码"}
            placeholder="新密码（留空保留）"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <button
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              setError("");
              try {
                await api(
                  "/users/" + user.id,
                  write("PATCH", { role, disabled, password }),
                );
                setPassword("");
                onSaved();
              } catch (e) {
                setError((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            保存
          </button>
        </div>
      </td>
    </tr>
  );
}

export function UserAdmin() {
  const resource = useResource<{ items: User[] }>("/users"),
    [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [role, setRole] = useState("member"),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <>
      <Heading
        title="团队成员"
        sub="由管理员创建账号；禁用或修改权限后，已有会话会失效。"
      />
      <ErrorBox error={error || resource.error} />
      <details className="panel create-panel">
        <summary>
          <Plus size={17} />
          <strong>添加成员</strong>
          <span>创建团队工作台账号</span>
        </summary>
        <form
          className="inline-form create-form"
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            setError("");
            try {
              await api("/users", write("POST", { username, password, role }));
              setUsername("");
              setPassword("");
              await resource.load();
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <label>
            账号
            <input
              aria-label="新账号"
              required
              placeholder="账号"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
          </label>
          <label>
            初始密码
            <input
              aria-label="初始密码"
              required
              type="password"
              minLength={12}
              placeholder="初始密码，至少 12 字节"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
          <label>
            角色
            <select
              aria-label="角色"
              value={role}
              onChange={(e) => setRole(e.target.value)}
            >
              <option value="member">成员</option>
              <option value="admin">管理员</option>
            </select>
          </label>
          <button className="primary" disabled={busy}>
            创建账号
          </button>
        </form>
      </details>
      <section className="panel members-panel">
        <div className="list-caption">
          <Users size={17} />
          <h2>团队账号</h2>
          <span>{resource.data?.items.length ?? "—"} 位成员</span>
        </div>
        <div className="table-scroll">
          {resource.loading ? (
            <Empty>加载团队成员…</Empty>
          ) : !resource.data?.items.length ? (
            <Empty>暂无团队成员。</Empty>
          ) : (
            <>
              <table>
                <thead>
                  <tr>
                    <th>账号</th>
                    <th>角色</th>
                    <th>状态</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {resource.data?.items.map((u) => (
                    <UserRow key={u.id} user={u} onSaved={resource.load} />
                  ))}
                </tbody>
              </table>
            </>
          )}
        </div>
      </section>
    </>
  );
}
