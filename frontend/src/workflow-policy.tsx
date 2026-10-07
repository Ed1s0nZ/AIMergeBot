import { useEffect, useState } from "react";
import { api, write, type Project } from "./api";
import { Empty, ErrorBox } from "./components";
import { Heading, useResource } from "./page-utils";

type Policy = {
  checks?: {
    enabled: boolean;
    mode: string;
    minimum_severity: string;
    block_on_failure: boolean;
    block_on_incomplete: boolean;
    publisher: number;
  };
  format_noise_hints: boolean;
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
const defaultChecks: NonNullable<Policy["checks"]> = {
  enabled: false,
  mode: "advisory",
  minimum_severity: "high",
  block_on_failure: true,
  block_on_incomplete: true,
  publisher: 0,
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
          checks: draft.checks,
          format_noise_hints: draft.format_noise_hints,
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
  const checks = draft?.checks ?? defaultChecks;
  const changeChecks = (patch: Partial<NonNullable<Policy["checks"]>>) => {
    if (draft) setDraft({ ...draft, checks: { ...checks, ...patch } });
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
            关注项调整审计优先级，不限制其他有证据支持的风险。
          </p>
          <label>
            <input
              type="checkbox"
              disabled={busy}
              checked={draft.format_noise_hints || false}
              onChange={(e) =>
                setDraft({ ...draft, format_noise_hints: e.target.checked })
              }
            />
            显示空白变更提示（辅助人工审查，保留完整变更审计）
          </label>
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
          <fieldset disabled={busy}>
            <legend>提交检查发布规则</legend>
            <p>
              当前仅保存规则，自动发布尚未接入；保存后不会立即向代码平台发送检查。
            </p>
            <label>
              <input
                type="checkbox"
                checked={checks.enabled}
                onChange={(e) => changeChecks({ enabled: e.target.checked })}
              />
              授权后续新任务发布检查
            </label>
            <label>
              模式
              <select
                value={checks.mode}
                disabled={!checks.enabled}
                onChange={(e) => changeChecks({ mode: e.target.value })}
              >
                <option value="advisory">提示</option>
                <option value="blocking">阻断</option>
              </select>
            </label>
            <label>
              最低风险等级
              <select
                value={checks.minimum_severity}
                disabled={!checks.enabled}
                onChange={(e) =>
                  changeChecks({ minimum_severity: e.target.value })
                }
              >
                <option value="critical">严重</option>
                <option value="high">高</option>
                <option value="medium">中</option>
                <option value="low">低</option>
                <option value="info">信息</option>
              </select>
            </label>
            <label>
              <input
                type="checkbox"
                checked={checks.block_on_failure}
                disabled={!checks.enabled || checks.mode !== "blocking"}
                onChange={(e) =>
                  changeChecks({ block_on_failure: e.target.checked })
                }
              />
              审计失败时阻断
            </label>
            <label>
              <input
                type="checkbox"
                checked={checks.block_on_incomplete}
                disabled={!checks.enabled || checks.mode !== "blocking"}
                onChange={(e) =>
                  changeChecks({ block_on_incomplete: e.target.checked })
                }
              />
              覆盖不完整时阻断
            </label>
            <p>
              默认关闭。实际合并限制由代码平台规则决定；失败或覆盖不完整不表示审计安全。修改或禁用规则会撤销旧配置的待发布任务，历史运行保留原策略。
            </p>
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
