import type { Settings } from "./api";

export type RecoveryReason =
  | "unsafe_revision"
  | "invalid_activation"
  | "pending"
  | "unavailable"
  | "settings_changed"
  | "";

export function settingsRecoveryPolicy(
  settings: Settings | null,
  dirty: boolean,
  locked: boolean,
  busy: boolean,
) {
  let reason: RecoveryReason = "";
  let pending = false;
  let reloadAllowed = false;
  if (settings) {
    if (
      !Number.isSafeInteger(settings.config_revision) ||
      settings.config_revision <= 0
    )
      reason = "unsafe_revision";
    else if (
      Object.prototype.hasOwnProperty.call(settings, "repository_activation")
    ) {
      const a = settings.repository_activation;
      if (!a || typeof a !== "object" || typeof a.can_reload !== "boolean")
        reason = "invalid_activation";
      else if (a.state === "active" && a.code === "") {
        /* valid ordinary settings */
      } else if (
        a.state === "pending" &&
        a.code === "credential_activation_pending"
      ) {
        reason = "pending";
        pending = true;
        reloadAllowed = a.can_reload;
      } else if (
        a.state === "unavailable" &&
        !a.can_reload &&
        (a.code === "credential_activation_unavailable" ||
          a.code === "settings_changed")
      ) {
        reason =
          a.code === "settings_changed" ? "settings_changed" : "unavailable";
      } else reason = "invalid_activation";
    }
  }
  const ready = !!settings && !busy && !locked;
  return {
    reason,
    pending,
    showRecovery: !!settings && !!reason,
    canEdit: ready && reason === "",
    canSave: ready && reason === "" && dirty,
    canReload: ready && pending && reloadAllowed && !dirty,
  };
}
