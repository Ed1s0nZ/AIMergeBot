import {
  eligibleRepositoryProfiles,
  type RepositoryDraft,
  type RepositoryIntegration,
} from "./repository-types";
export function RepositoryFields({
  draft,
  onChange,
  profiles,
  projectID,
  profilesReady = true,
}: {
  draft: RepositoryDraft;
  onChange: (draft: RepositoryDraft) => void;
  profiles: RepositoryIntegration[];
  projectID?: number;
  profilesReady?: boolean;
}) {
  const eligible = eligibleRepositoryProfiles(
    profiles,
    draft.provider,
    projectID,
  );
  return (
    <div className="repository-fields">
      <label>
        仓库平台
        <select
          value={draft.provider}
          onChange={(e) =>
            onChange({
              ...draft,
              provider: e.target.value as RepositoryDraft["provider"],
              integration_id: 0,
              api_origin: "",
            })
          }
        >
          <option value="github">GitHub</option>
          <option value="gitlab">GitLab</option>
        </select>
      </label>
      <label>
        仓库凭据配置
        <select
          required
          value={draft.integration_id || ""}
          onChange={(e) =>
            onChange({ ...draft, integration_id: Number(e.target.value) })
          }
        >
          <option value="">请选择凭据配置</option>
          {draft.integration_id > 0 &&
            !eligible.some((p) => p.id === draft.integration_id) && (
              <option value={draft.integration_id} disabled>
                {profilesReady ? "不可用配置" : "尚未读取的配置"} #
                {draft.integration_id}
              </option>
            )}
          {eligible.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name} · 版本 {p.revision}
            </option>
          ))}
        </select>
      </label>
      {profilesReady && !eligible.length && (
        <p>
          没有可用的凭据配置。请先在<a href="#/integrations">集成与通知</a>
          中保存并启用对应平台的 API
          Token。编辑绑定还需将当前项目加入该配置的范围。
        </p>
      )}
      <label>
        API 根地址
        <input
          type="url"
          required
          maxLength={2048}
          value={draft.api_origin}
          placeholder={
            draft.provider === "github"
              ? "https://api.github.com"
              : "https://gitlab.example.com"
          }
          onChange={(e) => onChange({ ...draft, api_origin: e.target.value })}
        />
      </label>
      <label>
        远端仓库 ID
        <input
          inputMode="numeric"
          required
          pattern="[1-9][0-9]*"
          value={draft.remote_id}
          onChange={(e) => onChange({ ...draft, remote_id: e.target.value })}
        />
      </label>
      <label>
        仓库完整路径
        <input
          required
          maxLength={255}
          value={draft.full_name}
          placeholder="owner/repository"
          onChange={(e) => onChange({ ...draft, full_name: e.target.value })}
        />
      </label>
      <p>
        根地址须与所选凭据配置一致；这里只保存仓库身份，不验证远端连通性。当前平台审计接入尚未完成。
      </p>
    </div>
  );
}
