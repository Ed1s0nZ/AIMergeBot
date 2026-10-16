export type RepositoryManagementStatus = { available: boolean; reason: string };

export function repositoryManagementAvailable(value: unknown): boolean {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const status = value as Record<string, unknown>;
  return status.available === true && status.reason === "";
}
