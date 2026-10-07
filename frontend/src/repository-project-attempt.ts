import type { RepositoryCreationInput } from "./repository-types";
const key = (actor: number) => `aim.repository-create.v1.${actor}`;
export function readCreationAttempt(
  actor: number,
): RepositoryCreationInput | null {
  const raw = sessionStorage.getItem(key(actor));
  if (!raw) return null;
  if (raw.length > 8192)
    throw new Error("创建请求恢复记录不可用，请先核对项目列表。");
  const value = JSON.parse(raw) as RepositoryCreationInput;
  if (
    !value ||
    !/^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/i.test(
      value.request_id,
    ) ||
    typeof value.name !== "string" ||
    value.name.length > 200 ||
    !value.name.trim() ||
    !["github", "gitlab"].includes(value.provider) ||
    typeof value.api_origin !== "string" ||
    value.api_origin.length > 2048 ||
    typeof value.full_name !== "string" ||
    value.full_name.length > 255 ||
    ![
      value.remote_id,
      value.integration_id,
      value.expected_integration_revision,
    ].every((v) => Number.isSafeInteger(v) && v > 0)
  )
    throw new Error("创建请求恢复记录不可用，请先核对项目列表。");
  return value;
}
export function storeCreationAttempt(
  actor: number,
  input: RepositoryCreationInput,
) {
  sessionStorage.setItem(key(actor), JSON.stringify(input));
}
export function clearCreationAttempt(actor: number) {
  sessionStorage.removeItem(key(actor));
}
