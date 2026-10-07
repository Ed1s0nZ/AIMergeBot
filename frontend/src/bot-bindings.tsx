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

export function BotBindings() {
  const [items, setItems] = useState<Binding[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [generation, setGeneration] = useState(0);
  useEffect(() => {
    let current = true;
    setLoading(true);
    setItems([]);
    setError("");
    api<{ items: Binding[] }>("/bot-bindings")
      .then(out => { if (current) setItems(out.items); })
      .catch(e => { if (current) setError((e as Error).message); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [generation]);
  async function revoke(id: number) {
    setBusy(true);
    setNotice("");
    setError("");
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
  return <section>
    <div className="page-head"><div><h1>我的机器人绑定</h1><p className="muted">查看你绑定的机器人身份。身份绑定不会增加项目权限。</p></div>
      <button disabled={loading || busy} onClick={() => { setNotice(""); setGeneration(x => x + 1); }}>刷新</button>
    </div>
    <ErrorBox error={error} />
    {notice && <p role="status">{notice}</p>}
    {loading ? <p role="status">正在加载绑定…</p> : !error && items.length === 0 ? <p>尚未绑定机器人身份。绑定入口正在完善。</p> : null}
    {!loading && items.map(item => <article className="panel" key={`${item.integration_id}:${item.workspace_id}`}>
      <h2>{item.name}</h2>
      <p>{item.enabled ? "渠道已启用" : "渠道已停用，仍可撤销绑定"}</p>
      <p>工作区：<code>{item.workspace_id}</code></p>
      <p>机器人用户：<code>{item.external_user_id}</code></p>
      <button disabled={busy} onClick={() => void revoke(item.integration_id)}>{busy ? "正在处理…" : "撤销绑定及未用凭据"}</button>
    </article>)}
  </section>;
}
