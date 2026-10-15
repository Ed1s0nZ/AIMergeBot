import { useEffect, useRef, useState } from "react";
import { api, APIError, write, type Settings } from "./api";
import { settingsRecoveryPolicy } from "./settings-recovery-policy";

type EditorState = {
  settings: Settings | null;
  baseline: string;
  baselineProxyDraft: string;
  trustedProxyDraft: string;
  busy: boolean;
  locked: boolean;
  error: string;
  saved: string;
};

const initial: EditorState = {
  settings: null,
  baseline: "",
  baselineProxyDraft: "",
  trustedProxyDraft: "",
  busy: false,
  locked: true,
  error: "",
  saved: "",
};
const isDirty = (s: EditorState) =>
  !!s.settings &&
  (JSON.stringify(s.settings) !== s.baseline ||
    s.trustedProxyDraft !== s.baselineProxyDraft);

export function useSystemSettings() {
  const [state, setState] = useState<EditorState>(initial);
  const latest = useRef(state);
  const mounted = useRef(false);
  const operation = useRef<{
    controller: AbortController;
    timer: ReturnType<typeof setTimeout>;
  } | null>(null);
  const update = (patch: Partial<EditorState>) => {
    if (!mounted.current) return;
    latest.current = { ...latest.current, ...patch };
    setState(latest.current);
  };
  const policy = () =>
    settingsRecoveryPolicy(
      latest.current.settings,
      isDirty(latest.current),
      latest.current.locked,
      latest.current.busy,
    );
  const accept = (settings: Settings) => {
    if (
      !settings ||
      typeof settings !== "object" ||
      !settings.gitlab ||
      !settings.openai ||
      !settings.react
    )
      throw new Error("invalid settings response");
    const proxies = (settings.trusted_proxies || []).join(", ");
    update({
      settings,
      baseline: JSON.stringify(settings),
      trustedProxyDraft: proxies,
      baselineProxyDraft: proxies,
      locked: false,
    });
  };
  const begin = () => {
    if (!mounted.current || operation.current) return null;
    const controller = new AbortController();
    const next = {
      controller,
      timer: setTimeout(() => controller.abort(), 20000),
    };
    operation.current = next;
    update({ busy: true });
    return next;
  };
  const owns = (op: NonNullable<typeof operation.current>) =>
    mounted.current &&
    operation.current === op &&
    !op.controller.signal.aborted;
  const finish = (op: NonNullable<typeof operation.current>) => {
    clearTimeout(op.timer);
    if (operation.current === op) {
      operation.current = null;
      update({ busy: false });
    }
  };
  const denied = (err: unknown) => {
    if (!(err instanceof APIError) || ![401, 403].includes(err.status))
      return false;
    update({
      ...initial,
      error: "当前没有设置操作权限，已清除配置和密钥草稿。请重新读取或登录。",
    });
    return true;
  };
  const load = async () => {
    const op = begin();
    if (!op) return;
    update({ locked: true, saved: "" });
    try {
      const settings = await api<Settings>("/settings", {
        signal: op.controller.signal,
      });
      if (owns(op)) {
        accept(settings);
        update({ error: "" });
      }
    } catch (err) {
      if (mounted.current && operation.current === op && !denied(err))
        update({
          locked: true,
          error: isDirty(latest.current)
            ? "无法读取当前设置。草稿已保留，请重新读取后核对。"
            : "无法读取当前设置，请重新读取后核对。",
        });
    } finally {
      finish(op);
    }
  };
  const mutate = async (kind: "save" | "reload") => {
    const current = latest.current;
    const allowed = policy();
    if (
      !current.settings ||
      (kind === "save" ? !allowed.canSave : !allowed.canReload)
    )
      return;
    const op = begin();
    if (!op) return;
    update({ locked: true, error: "", saved: "" });
    let committed = false;
    try {
      const payload = { ...current.settings };
      delete payload.repository_activation;
      await api<{ settings: Settings }>(
        kind === "save"
          ? "/settings"
          : "/settings/repository-activation/reload",
        {
          ...write(
            kind === "save" ? "PUT" : "POST",
            kind === "save"
              ? payload
              : {
                  expected_config_revision: String(
                    current.settings.config_revision,
                  ),
                },
          ),
          signal: op.controller.signal,
        },
      );
      committed = true;
      if (!owns(op)) return;
      const settings = await api<Settings>("/settings", {
        signal: op.controller.signal,
      });
      if (!owns(op)) return;
      accept(settings);
      const active = settingsRecoveryPolicy(
        settings,
        false,
        false,
        false,
      ).canEdit;
      update({
        saved: active
          ? settings.restart_required
            ? "配置已生效，服务配置仍需重启；重启前继续使用原来的监听、并发和可信代理配置。"
            : kind === "reload"
              ? "已从服务器保存的文件恢复配置，新审计将使用当前有效配置。"
              : "设置已保存，新审计将使用当前有效配置。"
          : "操作已完成并读取当前设置，请按配置生效状态处理。",
      });
    } catch (err) {
      if (mounted.current && operation.current === op && !denied(err)) {
        const validationOnly =
          !committed &&
          kind === "save" &&
          err instanceof APIError &&
          err.status === 400 &&
          ["invalid_trusted_proxies", "invalid_audit_quotas"].includes(
            err.code,
          );
        update({
          locked: !validationOnly,
          error: committed
            ? "操作已返回成功，但当前状态读取失败。请重新读取核对，不要重复提交。"
            : validationOnly
              ? err instanceof APIError &&
                err.code === "invalid_trusted_proxies"
                ? "可信代理最多填写32个明确IP或CIDR，不能使用全网范围、域名或未指定地址。"
                : "审计配额不符合范围或并发约束，请核对后保存。"
              : "操作结果待核对，服务器文件可能已保存。页面内容已保留，请先重新读取当前设置，再决定下一步。",
        });
      }
    } finally {
      finish(op);
    }
  };
  useEffect(() => {
    mounted.current = true;
    void load();
    return () => {
      mounted.current = false;
      const op = operation.current;
      if (op) {
        clearTimeout(op.timer);
        op.controller.abort();
        operation.current = null;
      }
    };
  }, []);
  return {
    ...state,
    dirty: isDirty(state),
    ...settingsRecoveryPolicy(
      state.settings,
      isDirty(state),
      state.locked,
      state.busy,
    ),
    load,
    save: () => mutate("save"),
    reload: () => mutate("reload"),
    setSettings: (settings: Settings) => {
      if (policy().canEdit) update({ settings, saved: "" });
    },
    setTrustedProxyDraft: (trustedProxyDraft: string) => {
      if (policy().canEdit) update({ trustedProxyDraft, saved: "" });
    },
  };
}
