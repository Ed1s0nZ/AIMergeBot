import { useEffect, useState } from "react";
import { api, write } from "./api";
type Channel = { id: number; revision: number; name: string; provider: string };
type Ticket = {
  id: number;
  integration_id: number;
  provider: string;
  state: string;
  url?: string;
};
const states: Record<string, string> = {
  pending: "等待创建",
  sending: "正在创建",
  created: "已创建",
  failed: "创建失败",
  unknown: "结果待核对",
};
function safeLink(value?: string) {
  try {
    const u = new URL(value || "");
    return u.protocol === "https:" &&
      u.host === "linear.app" &&
      !u.username &&
      !u.password &&
      !u.search &&
      !u.hash &&
      u.pathname.includes("/issue/")
      ? value
      : undefined;
  } catch {
    return undefined;
  }
}
export function FindingTicketsPanel({
  runId,
  findingId,
  headSHA,
  canCreate,
}: {
  runId: number;
  findingId: string;
  headSHA: string;
  canCreate: boolean;
}) {
  const [open, setOpen] = useState(false),
    [tickets, setTickets] = useState<Ticket[]>([]),
    [channels, setChannels] = useState<Channel[]>([]),
    [selected, setSelected] = useState("");
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [loaded, setLoaded] = useState(false),
    [revision, setRevision] = useState(0);
  const base = `/runs/${runId}/findings/${encodeURIComponent(findingId)}`;
  useEffect(() => {
    if (!open) return;
    let active = true;
    setBusy(true);
    setLoaded(false);
    setError("");
    setTickets([]);
    setChannels([]);
    setSelected("");
    Promise.all([
      api<{ items: Ticket[] }>(`${base}/tickets`),
      canCreate
        ? api<{ items: Channel[] }>(`${base}/ticket-channels`)
        : Promise.resolve({ items: [] as Channel[] }),
    ])
      .then(([t, c]) => {
        if (active) {
          setTickets(t.items);
          setChannels(c.items);
          setLoaded(true);
        }
      })
      .catch((e) => {
        if (active) setError((e as Error).message);
      })
      .finally(() => {
        if (active) setBusy(false);
      });
    return () => {
      active = false;
    };
  }, [open, base, canCreate, revision]);
  const channel = channels.find((c) => String(c.id) === selected);
  return (
    <details onToggle={(e) => setOpen(e.currentTarget.open)}>
      <summary>关联工单</summary>
      {busy && <p role="status">正在处理…</p>}
      {error && <p role="alert">{error}</p>}
      <button
        type="button"
        disabled={busy}
        onClick={() => setRevision((v) => v + 1)}
      >
        刷新工单状态
      </button>
      {loaded && (
        <>
          {tickets.length ? (
            <ul>
              {tickets.map((t) => (
                <li key={t.id}>
                  {t.provider} · {states[t.state] || "状态待核对"}
                  {safeLink(t.url) && (
                    <>
                      {" "}
                      ·{" "}
                      <a
                        href={safeLink(t.url)}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        查看工单
                      </a>
                    </>
                  )}
                  {t.state === "unknown" && (
                    <p>请核对远端是否已创建；系统不会自动重发。</p>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <p>尚未关联工单。</p>
          )}
          {canCreate &&
            (channels.length ? (
              <form
                onSubmit={async (e) => {
                  e.preventDefault();
                  if (!channel || busy) return;
                  setBusy(true);
                  setError("");
                  try {
                    const out = await api<{ ticket: Ticket }>(
                      `${base}/tickets`,
                      write("POST", {
                        integration_id: channel.id,
                        expected_revision: channel.revision,
                        head_sha: headSHA,
                      }),
                    );
                    setTickets((old) => [
                      ...old.filter(
                        (t) => t.integration_id !== out.ticket.integration_id,
                      ),
                      out.ticket,
                    ]);
                  } catch (e) {
                    setError((e as Error).message);
                    setChannels([]);
                    setTickets([]);
                    setLoaded(false);
                    setSelected("");
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                <label>
                  工单渠道
                  <select
                    value={selected}
                    disabled={busy}
                    onChange={(e) => setSelected(e.target.value)}
                  >
                    <option value="">请选择渠道</option>
                    {channels.map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.name} · {c.provider}
                      </option>
                    ))}
                  </select>
                </label>
                <p>
                  请求绑定此发现和提交 {headSHA.slice(0, 12)}
                  ，后台处理后请刷新状态。
                </p>
                <button
                  disabled={
                    busy ||
                    !channel ||
                    tickets.some((t) => t.integration_id === channel.id)
                  }
                >
                  创建工单
                </button>
              </form>
            ) : (
              <p>没有可用工单渠道，请联系管理员配置。</p>
            ))}
        </>
      )}
    </details>
  );
}
