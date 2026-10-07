import "./finding-owners.css";
import { useState } from "react";
import { ErrorBox, Empty } from "./components";
import { useResource } from "./page-utils";

type Candidate = {
  user_id: number;
  username: string;
  source: string;
  alias?: string;
  line?: number;
  section?: string;
};
type Rule = {
  line: number;
  pattern: string;
  section?: string;
  owners: string[];
  excluded?: boolean;
};
type Recommendation = {
  run_id: number;
  finding_id: string;
  revision: number;
  file: string;
  document: {
    provider: string;
    repository_id: number;
    sha: string;
    path: string;
    present: boolean;
  };
  matches: { rules: Rule[]; exclusions: Rule[] };
  candidates: Candidate[];
  unmapped: string[];
  filtered: boolean;
  truncated: boolean;
  source_state: string;
  notes: string[];
};
function OwnerRecommendations({
  runId,
  findingId,
  headSHA,
  onChoose,
}: {
  runId: number;
  findingId: string;
  headSHA: string;
  onChoose?: (id: number) => void;
}) {
  const resource = useResource<Recommendation>(
    `/runs/${runId}/findings/${encodeURIComponent(findingId)}/owners`,
  );
  const [chosen, setChosen] = useState(0);
  const identityMatches =
    resource.data?.run_id === runId &&
    resource.data?.finding_id === findingId &&
    resource.data?.document.sha === headSHA;
  const visible =
    !resource.loading && !resource.error && identityMatches
      ? resource.data
      : null;
  return (
    <>
      <ErrorBox
        error={
          resource.error ||
          (!resource.loading && resource.data && !identityMatches
            ? "推荐对应的任务或提交已变化，请刷新后核对。"
            : "")
        }
      />
      {resource.loading ? (
        <Empty>正在读取责任人推荐…</Empty>
      ) : visible ? (
        <>
          <p>
            配置版本 {visible.revision} · 文件 {visible.file}
          </p>
          <p>
            来源仓库 #{visible.document.repository_id} · 固定提交{" "}
            {visible.document.sha}
          </p>
          <p>
            CODEOWNERS：
            {visible.source_state === "available"
              ? visible.document.path
              : visible.source_state === "missing"
                ? "已确认未找到文件"
                : "来源不可用或未完整读取"}
          </p>
          {visible.truncated && (
            <p role="alert">结果已截断，不能视为完整责任人名单。</p>
          )}
          {visible.filtered && (
            <p>部分账号已停用或缺少本次审计完整范围权限，已过滤。</p>
          )}
          {visible.candidates.length ? (
            <ul>
              {visible.candidates.map((candidate, index) => (
                <li key={`${candidate.user_id}:${index}`}>
                  <strong>{candidate.username}</strong>（用户 #
                  {candidate.user_id}） ·{" "}
                  {candidate.source === "project_default"
                    ? "项目默认责任人"
                    : `${candidate.alias} · 规则第 ${candidate.line} 行${candidate.section ? ` · ${candidate.section}` : ""}`}
                  {onChoose && (
                    <button
                      type="button"
                      onClick={() => {
                        onChoose(candidate.user_id);
                        setChosen(candidate.user_id);
                      }}
                    >
                      填入责任人草稿：{candidate.username}
                    </button>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <Empty>没有可推荐的责任人，保持未分配。</Empty>
          )}
          {!!visible.unmapped.length && (
            <p>
              未映射仓库身份：{visible.unmapped.join("、")}
              。请联系管理员配置明确映射。
            </p>
          )}
          {(visible.matches.rules.length > 0 ||
            visible.matches.exclusions.length > 0) && (
            <details>
              <summary>匹配依据</summary>
              {[...visible.matches.rules, ...visible.matches.exclusions].map(
                (rule, index) => (
                  <p key={index}>
                    第 {rule.line} 行 · {rule.section || "默认范围"} ·{" "}
                    <code>{rule.pattern}</code> ·{" "}
                    {rule.excluded
                      ? "排除规则"
                      : rule.owners.length
                        ? rule.owners.join("、")
                        : "明确无责任人"}
                  </p>
                ),
              )}
            </details>
          )}
          {visible.notes.map((note, index) => (
            <p key={index}>{note}</p>
          ))}
          {chosen !== 0 && onChoose && (
            <p role="status">
              已将用户 #{chosen} 填入草稿，请核对后保存风险处理。
            </p>
          )}
          {!onChoose && <p>当前仅可查看推荐，责任分配需项目操作权限。</p>}
        </>
      ) : null}
      <button
        type="button"
        disabled={resource.loading}
        onClick={() => {
          setChosen(0);
          void resource.load();
        }}
      >
        刷新责任人推荐
      </button>
    </>
  );
}
export function FindingOwnersPanel(props: {
  runId: number;
  findingId: string;
  headSHA: string;
  onChoose?: (id: number) => void;
}) {
  const [opened, setOpened] = useState(false);
  return (
    <details
      className="owner-recommendations"
      onToggle={(event) => {
        if (event.currentTarget.open) setOpened(true);
      }}
    >
      <summary>责任人推荐</summary>
      <p>
        根据固定提交规则与明确的平台账号映射推荐；不会自动分配，不授予权限，也不代表代码平台必审人。
      </p>
      {opened && <OwnerRecommendations {...props} />}
    </details>
  );
}
