import { useEffect, useRef, useState } from "react";
import { api, write, APIError, type Project, type User } from "./api";
import { Empty, ErrorBox } from "./components";

type Routing = {
  revision: number;
  default_owner: number;
  aliases: Record<string, number[]>;
};
type Member = { user_id: number; disabled: boolean };
type AliasRow = { key: number; alias: string; owners: number[] };

export function OwnerRoutingEditor({
  project,
  onClose,
}: {
  project: Project;
  onClose: () => void;
}) {
  const panel = useRef<HTMLElement>(null);
  const mounted = useRef(true);
  const nextKey = useRef(0);
  const [attempt, setAttempt] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [conflict, setConflict] = useState(false);
  const [revision, setRevision] = useState<number | null>(null);
  const [owner, setOwner] = useState(0);
  const [rows, setRows] = useState<AliasRow[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  useEffect(() => {
    mounted.current = true;
    panel.current?.focus();
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    setMessage("");
    setConflict(false);
    setRevision(null);
    setOwner(0);
    setRows([]);
    setUsers([]);
    const opts = { signal: controller.signal };
    Promise.all([
      api<Routing>(`/projects/${project.id}/owner-routing`, opts),
      api<{ items: User[] }>("/users", opts),
      api<{ items: Member[] }>(`/projects/${project.id}/members`, opts),
    ])
      .then(([routing, accounts, members]) => {
        if (controller.signal.aborted) return;
        const allowed = new Set(
          members.items.filter((m) => !m.disabled).map((m) => m.user_id),
        );
        setUsers(
          accounts.items.filter(
            (u) => !u.disabled && (u.role === "admin" || allowed.has(u.id)),
          ),
        );
        setRevision(routing.revision);
        setOwner(routing.default_owner);
        setRows(
          Object.entries(routing.aliases).map(([alias, owners]) => ({
            key: nextKey.current++,
            alias,
            owners,
          })),
        );
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError((e as Error).message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [project.id, attempt]);
  const eligible = new Set(users.map((u) => u.id));
  const unavailable =
    (owner !== 0 && !eligible.has(owner)) ||
    rows.some((r) => r.owners.some((id) => !eligible.has(id)));
  const save = async () => {
    if (revision === null || busy || conflict) return;
    setError("");
    setMessage("");
    const aliases: Record<string, number[]> = Object.create(null);
    for (const row of rows) {
      if (
        !row.alias ||
        row.alias.length > 128 ||
        /\s/.test(row.alias) ||
        Object.hasOwn(aliases, row.alias) ||
        !row.owners.length ||
        row.owners.length > 20
      ) {
        setError(
          "每条映射须填写唯一且不含空白的仓库身份，并选择 1–20 位责任人。",
        );
        return;
      }
      aliases[row.alias] = row.owners;
    }
    if (unavailable) {
      setError("部分责任人已停用或失去项目权限，请移除或更换后保存。");
      return;
    }
    setBusy(true);
    try {
      const result = await api<Routing>(
        `/projects/${project.id}/owner-routing`,
        write("PUT", {
          expected_revision: revision,
          default_owner: owner,
          aliases,
        }),
      );
      if (!mounted.current) return;
      setRevision(result.revision);
      setOwner(result.default_owner);
      setRows(
        Object.entries(result.aliases).map(([alias, owners]) => ({
          key: nextKey.current++,
          alias,
          owners,
        })),
      );
      setMessage("配置已保存；已有发现的责任人保持不变。");
    } catch (e) {
      if (!mounted.current) return;
      if (e instanceof APIError && e.status === 409) {
        setConflict(true);
        setError("配置已被修改。草稿已保留，请核对后丢弃草稿并重新加载。");
      } else {
        if (e instanceof APIError && [401, 403, 404].includes(e.status)) {
          setRevision(null);
          setOwner(0);
          setRows([]);
          setUsers([]);
        }
        setError((e as Error).message);
      }
    } finally {
      if (mounted.current) setBusy(false);
    }
  };
  return (
    <section
      className="panel"
      id="owner-routing-panel"
      tabIndex={-1}
      ref={panel}
      aria-label={`${project.name} 的责任人配置`}
    >
      <h2>{project.name} 的责任人配置</h2>
      <p>
        明确指定平台账号与仓库身份的对应关系。配置不会授予项目权限，也不会修改已有发现的责任人。
      </p>
      <ErrorBox error={error} />
      {loading ? (
        <Empty>正在加载…</Empty>
      ) : revision !== null ? (
        <>
          <p>配置版本 {revision}</p>
          {unavailable && (
            <p role="alert">部分已保存责任人不再可用，请移除或更换。</p>
          )}
          <fieldset disabled={busy || conflict}>
            <legend>项目默认责任人</legend>
            <label>
              责任人
              <select
                value={owner}
                onChange={(e) => setOwner(Number(e.target.value))}
              >
                <option value={0}>不指定</option>
                {owner !== 0 && !eligible.has(owner) && (
                  <option value={owner} disabled>
                    已停用或无权限的账号 #{owner}
                  </option>
                )}
                {users.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.username}
                  </option>
                ))}
              </select>
            </label>
          </fieldset>
          <fieldset disabled={busy || conflict}>
            <legend>仓库身份映射</legend>
            <p>
              仓库身份可填写用户名、团队（如
              @org/team）或邮箱；不会根据同名账号自动关联。
            </p>
            {!rows.length && <p>尚未配置身份映射。</p>}
            {rows.map((row, index) => (
              <fieldset key={row.key}>
                <legend>映射 {index + 1}</legend>
                <label>
                  仓库身份
                  <input
                    value={row.alias}
                    maxLength={128}
                    onChange={(e) =>
                      setRows((current) =>
                        current.map((r) =>
                          r.key === row.key
                            ? { ...r, alias: e.target.value }
                            : r,
                        ),
                      )
                    }
                  />
                </label>
                <label>
                  责任人（可多选）
                  <select
                    multiple
                    size={Math.min(5, Math.max(2, users.length))}
                    value={row.owners.map(String)}
                    onChange={(e) => {
                      const owners = Array.from(
                        e.target.selectedOptions,
                        (option) => Number(option.value),
                      );
                      setRows((current) =>
                        current.map((r) =>
                          r.key === row.key ? { ...r, owners } : r,
                        ),
                      );
                    }}
                  >
                    {row.owners
                      .filter((id) => !eligible.has(id))
                      .map((id) => (
                        <option key={id} value={id} disabled>
                          已停用或无权限的账号 #{id}
                        </option>
                      ))}
                    {users.map((u) => (
                      <option key={u.id} value={u.id}>
                        {u.username}
                      </option>
                    ))}
                  </select>
                </label>
                <button
                  onClick={() =>
                    setRows((current) =>
                      current.filter((r) => r.key !== row.key),
                    )
                  }
                >
                  移除此映射
                </button>
              </fieldset>
            ))}
            <button
              disabled={rows.length >= 100}
              onClick={() =>
                setRows((current) => [
                  ...current,
                  { key: nextKey.current++, alias: "", owners: [] },
                ])
              }
            >
              添加身份映射
            </button>
          </fieldset>
          <button disabled={busy || conflict || unavailable} onClick={save}>
            {busy ? "保存中…" : "保存责任人配置"}
          </button>
        </>
      ) : (
        !error && <Empty>暂无配置。</Empty>
      )}
      <div className="inline-form">
        <button
          disabled={busy || loading}
          onClick={() => setAttempt((v) => v + 1)}
        >
          {revision !== null ? "丢弃草稿并重新加载" : "重试"}
        </button>
        <button disabled={busy} onClick={onClose}>
          关闭
        </button>
      </div>
      <p role="status">{message}</p>
    </section>
  );
}
