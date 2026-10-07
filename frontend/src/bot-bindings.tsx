import { useEffect, useState } from "react";
import { api, write } from "./api";
import { ErrorBox } from "./components";

type Binding = {
  integration_id: number;
  name: string;
  enabled: boolean;
  workspace_id: string;
  external_user_id: string;
  created_at: string;
};
type Channel = { id: number; revision: number; name: string; provider: string };
type Challenge = { token: string; expires_at: string };

export function BotBindings() {
  const [items, setItems] = useState<Binding[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [generation, setGeneration] = useState(0);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [selected, setSelected] = useState("");
  const [challenge, setChallenge] = useState<Challenge | null>(null);
  useEffect(() => {
    if (!challenge) return;
    const timer = window.setTimeout(() => {
      setChallenge(null);
      setNotice("绑定凭据已到期，请重新生成。");
    }, Math.max(0, Date.parse(challenge.expires_at) - Date.now()));
    return () => window.clearTimeout(timer);
  }, [challenge]);
  useEffect(() => {
    let current = true;
    setLoading(true);
    setItems([]);
    setChannels([]);
    setSelected("");
    setChallenge(null);
    setError("");
    Promise.all([
      api<{ items: Binding[] }>("/bot-bindings"),
      api<{ items: Channel[] }>("/bot-binding-channels"),
    ])
      .then(([bindings, available]) => { if (current) { setItems(bindings.items); setChannels(available.items); } })
      .catch(e => { if (current) setError((e as Error).message); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [generation]);
  async function revoke(id: number) {
    setBusy(true);
    setNotice("");
    setError("");
    setChallenge(null);
    try {
      const out = await api<{ revoked: boolean }>(`/bot-bindings/${id}`, write("DELETE"));
      setNotice(out.revoked ? "绑定及未用凭据已撤销。" : "当前已没有绑定或未用凭据。");
      setGeneration(x => x + 1);
    } catch (e) {
      setError((e as Error).message);
      setItems([]);
    } finally {
      setBusy(false);
    }
  }
  async function issue() {
    const channel = channels.find(x => String(x.id) === selected);
    if (!channel) return;
    setBusy(true); setError(""); setNotice(""); setChallenge(null);
    try {
      const out = await api<Challenge>(`/bot-bindings/${channel.id}/challenge`, write("POST", { expected_revision: channel.revision }));
      if (!/^[0-9a-f]{48}$/.test(out.token) || !Number.isFinite(Date.parse(out.expires_at)) || Date.parse(out.expires_at) <= Date.now()) throw new Error("绑定凭据不可用，请刷新后重试。");
      setChallenge(out);
    } catch (e) {
      setError((e as Error).message); setChannels([]); setSelected("");
    } finally { setBusy(false); }
  }
  return <section>
    <div className="page-head"><div><h1>我的机器人绑定</h1><p className="muted">查看你绑定的机器人身份。身份绑定不会增加项目权限。</p></div>
      <button disabled={loading || busy} onClick={() => { setNotice(""); setGeneration(x => x + 1); }}>刷新</button>
    </div>
    <ErrorBox error={error} />
    {notice && <p role="status">{notice}</p>}
    {loading ? <p role="status">正在加载绑定…</p> : !error && items.length === 0 ? <p>尚未绑定机器人身份。</p> : null}
    {!loading && !error && <article className="panel">
      <h2>绑定 Slack 身份</h2>
      {channels.length === 0 ? <p>暂无可用绑定渠道，请联系管理员配置渠道或项目访问权限。</p> : <>
        <label>选择渠道 <select value={selected} disabled={busy} onChange={e => { setSelected(e.target.value); setChallenge(null); setNotice(""); }}>
          <option value="">请选择渠道</option>
          {channels.map(x => <option key={x.id} value={x.id}>{x.name}</option>)}
        </select></label>
        <button disabled={busy || !selected || items.some(x => String(x.integration_id) === selected)} onClick={() => void issue()}>{busy ? "正在处理…" : "生成一次性绑定凭据"}</button>
        {challenge && <div>
          <p>在该工作区使用管理员配置的 Slack 命令，参数为下方内容。此凭据用于绑定身份，请勿分享。</p>
          <pre style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>bind {challenge.token}</pre>
          <p>到期时间：{new Date(challenge.expires_at).toLocaleString()}</p>
          <button disabled={busy} onClick={() => void revoke(Number(selected))}>撤销此凭据</button>
          <p>Slack 提示绑定成功后，点击刷新确认状态。重新生成会使旧凭据失效。</p>
        </div>}
      </>}
    </article>}
    {!loading && items.map(item => <article className="panel" key={`${item.integration_id}:${item.workspace_id}`}>
      <h2>{item.name}</h2>
      <p>{item.enabled ? "渠道已启用" : "渠道已停用，仍可撤销绑定"}</p>
      <p>工作区：<code>{item.workspace_id}</code></p>
      <p>机器人用户：<code>{item.external_user_id}</code></p>
      <button disabled={busy} onClick={() => void revoke(item.integration_id)}>{busy ? "正在处理…" : "撤销绑定及未用凭据"}</button>
    </article>)}
  </section>;
}
