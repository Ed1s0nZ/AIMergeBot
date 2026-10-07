import type { Project } from "./api";
export type RepositoryBinding = {
  revision: number;
  provider: "github" | "gitlab";
  api_origin: string;
  remote_id: number;
  full_name: string;
  integration_id: number;
};
export type RepositoryIntegration = {
  id: number;
  revision: number;
  name: string;
  kind: string;
  enabled: boolean;
  project_ids: number[];
  has_endpoint: boolean;
  has_secret: boolean;
};
export type RepositoryDraft = Omit<
  RepositoryBinding,
  "revision" | "remote_id"
> & { remote_id: string };
export const emptyRepositoryDraft: RepositoryDraft = {
  provider: "github",
  api_origin: "",
  remote_id: "",
  full_name: "",
  integration_id: 0,
};
export type RepositoryCreationInput = Omit<RepositoryBinding, "revision"> & {
  request_id: string;
  name: string;
  expected_integration_revision: number;
};
export type RepositoryCreationReceipt = {
  project: Project;
  repository: RepositoryBinding;
  integration_revision: number;
  replayed: boolean;
};
export function eligibleRepositoryProfiles(
  items: RepositoryIntegration[],
  provider: string,
  projectID?: number,
) {
  return items.filter(
    (p) =>
      p.kind === provider &&
      p.enabled &&
      p.has_endpoint &&
      p.has_secret &&
      (projectID
        ? p.project_ids.includes(projectID)
        : p.project_ids.length < 100),
  );
}
export function repositoryPayload(draft: RepositoryDraft) {
  const remote_id = Number(draft.remote_id);
  if (
    !/^[1-9][0-9]*$/.test(draft.remote_id) ||
    !Number.isSafeInteger(remote_id)
  )
    throw new Error("请填写有效的仓库 ID。");
  return { ...draft, remote_id };
}
