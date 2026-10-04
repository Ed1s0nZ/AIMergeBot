import { useState } from "react";

import { api, write, type User } from "./api";
import { ErrorBox } from "./components";
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
        {user.username}
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
      <form
        className="panel inline-form"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
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
        <input
          aria-label="新账号"
          required
          placeholder="账号"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <input
          aria-label="初始密码"
          required
          type="password"
          minLength={12}
          placeholder="初始密码，至少 12 字节"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <select
          aria-label="角色"
          value={role}
          onChange={(e) => setRole(e.target.value)}
        >
          <option value="member">成员</option>
          <option value="admin">管理员</option>
        </select>
        <button className="primary" disabled={busy}>
          创建账号
        </button>
      </form>
      <section className="panel table-scroll">
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
      </section>
    </>
  );
}
