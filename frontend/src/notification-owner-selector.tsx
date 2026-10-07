import { useEffect, useState } from "react";
import { api, type Project, type User } from "./api";
import { Empty, ErrorBox } from "./components";
import "./notification-owner-selector.css";

type Member = { user_id: number; role: string; disabled: boolean };
export type OwnerSelectionStatus = { key: string; valid: boolean };
export const notificationKinds = [
  "email",
  "feishu",
  "dingtalk",
  "wecom",
  "slack",
  "teams",
  "webhook",
];
export function ownerSelectionKey(
  projects: number[],
  owners: number[],
  events: string[],
) {
  return JSON.stringify([
    [...projects].sort((a, b) => a - b),
    [...owners].sort((a, b) => a - b),
    [...events].sort(),
  ]);
}
export function NotificationOwnerSelector({
  projectIDs,
  projects,
  projectsLoading,
  ownerIDs,
  events,
  disabled,
  onChange,
  onValidity,
}: {
  projectIDs: number[];
  projects: Project[];
  projectsLoading: boolean;
  ownerIDs: number[];
  events: string[];
  disabled: boolean;
  onChange: (owners: number[]) => void;
  onValidity: (status: OwnerSelectionStatus) => void;
}) {
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<{
    key: string;
    users: User[];
    error: string;
    ready: boolean;
  } | null>(null);
  const scope = JSON.stringify([...projectIDs].sort((a, b) => a - b));
  const requestKey = `${scope}:${attempt}`;
  const projectOK =
    projectIDs.length > 0 &&
    projectIDs.every((id) => projects.some((p) => p.id === id && p.enabled));
  const canLoad = !projectsLoading && projectOK;
  useEffect(() => {
    const controller = new AbortController();
    setState(null);
    if (!canLoad) return () => controller.abort();
    const ids = JSON.parse(scope) as number[];
    const opts = { signal: controller.signal };
    void (async () => {
      try {
        const accounts = await api<{ items: User[] }>("/users", opts);
        const scopes: Set<number>[] = [];
        for (let start = 0; start < ids.length; start += 5) {
          if (controller.signal.aborted) return;
          const results = await Promise.all(
            ids
              .slice(start, start + 5)
              .map((id) =>
                api<{ items: Member[] }>(`/projects/${id}/members`, opts),
              ),
          );
          scopes.push(
            ...results.map(
              (result) =>
                new Set(
                  result.items
                    .filter(
                      (m) =>
                        !m.disabled &&
                        ["viewer", "reviewer", "operator"].includes(m.role),
                    )
                    .map((m) => m.user_id),
                ),
            ),
          );
        }
        if (!controller.signal.aborted)
          setState({
            key: requestKey,
            users: accounts.items.filter(
              (u) =>
                !u.disabled &&
                (u.role === "admin" || scopes.every((s) => s.has(u.id))),
            ),
            error: "",
            ready: true,
          });
      } catch (e) {
        if (!controller.signal.aborted)
          setState({
            key: requestKey,
            users: [],
            error: (e as Error).message,
            ready: false,
          });
      }
    })();
    return () => controller.abort();
  }, [scope, requestKey, canLoad]);
  const current = canLoad && state?.key === requestKey ? state : null;
  const users = current?.ready ? current.users : [];
  const eligible = new Set(users.map((u) => u.id));
  const unavailable = ownerIDs.filter((id) => !eligible.has(id));
  const eventOK =
    events.length > 0 &&
    events.every((kind) => ["finding.reviewed", "risk.expired"].includes(kind));
  const valid =
    ownerIDs.length === 0 ||
    (!!current?.ready &&
      ownerIDs.length <= 100 &&
      new Set(ownerIDs).size === ownerIDs.length &&
      unavailable.length === 0 &&
      eventOK);
  const key = ownerSelectionKey(projectIDs, ownerIDs, events);
  useEffect(() => onValidity({ key, valid }), [key, valid, onValidity]);
  return (
    <fieldset className="notification-owner-selector" disabled={disabled}>
      <legend>责任人通知筛选（可选）</legend>
      <p>
        留空按项目通知。选择后，仅接收这些已分配责任人的发现复核与风险到期提醒；消息仍发到当前渠道保存的地址。
      </p>
      {projectsLoading ? (
        <Empty>加载项目范围…</Empty>
      ) : !projectOK ? (
        <p>请先选择启用的项目。</p>
      ) : !current ? (
        <Empty>加载责任人候选…</Empty>
      ) : (
        <ErrorBox error={current.error} />
      )}
      {current?.ready && (
        <label>
          责任人（可多选，最多 100 位）
          <select
            multiple
            size={Math.min(6, Math.max(2, users.length))}
            value={ownerIDs.map(String)}
            onChange={(e) =>
              onChange(
                Array.from(e.target.selectedOptions, (option) =>
                  Number(option.value),
                ),
              )
            }
          >
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.username} · #{u.id}
              </option>
            ))}
          </select>
        </label>
      )}
      {!!ownerIDs.length && (
        <p role="status">
          已选择 {ownerIDs.length} 位责任人。
          {!eventOK &&
            "请在通知事件中仅保留发现复核或风险接受到期；不会自动修改已有事件选择。"}
        </p>
      )}
      {ownerIDs.length > 100 && (
        <ErrorBox error="责任人范围最多 100 位，请减少选择。" />
      )}
      {!!ownerIDs.length && current?.ready && !!unavailable.length && (
        <div role="alert">
          <p>
            部分已选账号停用或不具备全部所选项目权限，请明确移除或更换后保存。
          </p>
          {unavailable.map((id, index) => (
            <button
              type="button"
              key={`${id}:${index}`}
              onClick={() => onChange(ownerIDs.filter((value) => value !== id))}
            >
              移除不可用账号 #{id}
            </button>
          ))}
        </div>
      )}
      {!!ownerIDs.length && !current?.ready && (
        <p>候选尚未就绪，已选账号不会被静默清空，保存筛选范围暂不可用。</p>
      )}
      <div className="inline-form">
        <button
          type="button"
          disabled={!canLoad || disabled}
          onClick={() => setAttempt((value) => value + 1)}
        >
          刷新责任人候选
        </button>
        <button
          type="button"
          disabled={!ownerIDs.length || disabled}
          onClick={() => onChange([])}
        >
          清空责任人筛选
        </button>
      </div>
    </fieldset>
  );
}
