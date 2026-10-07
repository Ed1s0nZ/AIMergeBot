import { useEffect, useState } from "react";
import { api, write, type Project } from "./api";
import { Empty, ErrorBox } from "./components";
import { Heading, useResource } from "./page-utils";

type Policy = {
  revision: number;
  focus: string[];
  excluded_extensions: string[];
};
const priorities: Record<string, string> = {
  authorization: "身份与权限",
  injection: "注入",
  secrets: "密钥泄露",
  path_traversal: "路径穿越",
  ssrf: "SSRF",
  deserialization: "反序列化",
  dependencies: "依赖风险",
  business_logic: "业务逻辑",
};
function PolicyEditor({ project }: { project: Project }) {
  const resource = useResource<Policy>(
    `/projects/${project.id}/workflow-policy`,
  );
  const [draft, setDraft] = useState<Policy | null>(null);
  const [extensions, setExtensions] = useState("");
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  useEffect(() => {
    if (
      !resource.loading &&
      !resource.error &&
      resource.data &&
      draft === null
    ) {
      setDraft(resource.data);
      setExtensions(resource.data.excluded_extensions.join("\n"));
    }
  }, [resource.data, resource.loading, resource.error, draft]);
  const reload = () => {
    setDraft(null);
    setError("");
    setMessage("");
    void resource.load();
  };
  const save = async () => {
    if (!draft) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const result = await api<Policy>(
        `/projects/${project.id}/workflow-policy`,
        write("PUT", {
          expected_revision: draft.revision,
          focus: draft.focus,
          excluded_extensions: extensions.split(/\s+/).filter(Boolean),
        }),
      );
      setDraft(result);
      setExtensions(result.excluded_extensions.join("\n"));
      setMessage("已保存；后续新任务使用此版本，历史任务保持原快照。");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="panel">
      <h2>{project.name} 的审计策略</h2>
      <ErrorBox error={error || resource.error} />
      {resource.loading ? (
        <Empty>正在加载…</Empty>
      ) : draft ? (
        <>
          <p>
            版本 {draft.revision} ·
            关注项调整审计优先级，不限制其他有证据支持的风险。当前配置不自动阻断合并。
          </p>
          <fieldset disabled={busy}>
            <legend>重点关注</legend>
            {Object.entries(priorities).map(([key, label]) => (
              <label key={key}>
                <input
                  type="checkbox"
                  checked={draft.focus.includes(key)}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      focus: e.target.checked
                        ? [...draft.focus, key]
                        : draft.focus.filter((v) => v !== key),
                    })
                  }
                />
                {label}
              </label>
            ))}
          </fieldset>
          <label>
            额外排除的扩展名（每行一个，如 .svg）
            <textarea
              value={extensions}
              disabled={busy}
              onChange={(e) => setExtensions(e.target.value)}
            />
          </label>
          <p>
            与系统排除规则合并，减少实际审计范围；空列表保留系统默认范围。扩展名须为小写，不支持路径或通配符。
          </p>
          <div className="inline-form">
            <button disabled={busy} onClick={save}>
              {busy ? "保存中…" : "保存策略"}
            </button>
            <button disabled={busy} onClick={reload}>
              丢弃草稿并重新加载
            </button>
          </div>
          <p role="status">{message}</p>
        </>
      ) : !resource.error ? (
        <Empty>暂无策略记录。</Empty>
      ) : (
        <button onClick={reload}>重试</button>
      )}
    </section>
  );
}
export function WorkflowPolicies() {
  const resource = useResource<{ items: Project[] }>("/projects");
  const [selected, setSelected] = useState("");
  const visible =
    !resource.loading && !resource.error ? resource.data?.items : undefined;
  const project = visible?.find((p) => String(p.id) === selected);
  return (
    <>
      <Heading
        title="审计策略"
        sub="为各项目设置关注项及额外排除规则；任务保留提交与策略版本，历史结果不随配置变更。"
      />
      <ErrorBox error={resource.error} />
      {resource.loading ? (
        <Empty>正在加载…</Empty>
      ) : visible?.length ? (
        <section className="panel">
          <label>
            项目
            <select
              value={selected}
              onChange={(e) => setSelected(e.target.value)}
            >
              <option value="">请选择项目</option>
              {visible.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
        </section>
      ) : !resource.error ? (
        <Empty>暂无项目。</Empty>
      ) : (
        <button onClick={resource.load}>重试</button>
      )}
      {project && <PolicyEditor key={project.id} project={project} />}
    </>
  );
}
