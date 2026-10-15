import type { RecoveryReason } from "./settings-recovery-policy";

const explanations: Record<Exclude<RecoveryReason, "">, string> = {
  pending:
    "已保存的配置尚未生效。恢复会读取服务器 config.yaml 中已保存的完整配置和实际密钥；文件中的空密钥会清除对应凭据，不会使用页面草稿。",
  unavailable:
    "当前无法核对配置生效状态，暂不能保存或恢复。请重新读取后检查；持续不可用时请联系管理员。",
  settings_changed: "配置在读取期间发生变化，请重新读取后核对。",
  invalid_activation:
    "当前配置生效状态无法识别，暂不能保存或恢复。请重新读取后核对。",
  unsafe_revision:
    "当前配置版本不能在页面中准确表示，已暂停保存和恢复。请由管理员通过支持精确版本的接口处理。",
};

export function SettingsRecovery({
  reason,
  locked,
  dirty,
  busy,
  pending,
  canReload,
  load,
  reload,
}: {
  reason: RecoveryReason;
  locked: boolean;
  dirty: boolean;
  busy: boolean;
  pending: boolean;
  canReload: boolean;
  load: () => Promise<void>;
  reload: () => Promise<void>;
}) {
  if (!reason && !locked && !dirty) return null;
  return (
    <section
      className="coverage settings-recovery"
      aria-label="配置状态与恢复"
      aria-busy={busy}
    >
      {reason && (
        <p role="status">
          {locked && "上次读取的状态（当前状态待核对）："}
          {explanations[reason]}
        </p>
      )}
      {locked && !busy && (
        <p>写操作已锁定，请先重新读取当前设置再核对操作结果。</p>
      )}
      {dirty && (
        <p>
          页面有未保存的修改。重新读取会放弃当前草稿，请先确认需要保留的内容。
        </p>
      )}
      {pending && !canReload && !dirty && !locked && !busy && (
        <p>当前暂不允许恢复，请重新读取后核对。</p>
      )}
      <div className="actions">
        <button type="button" disabled={busy} onClick={() => void load()}>
          {busy
            ? "正在处理…"
            : dirty
              ? "放弃修改并重新读取"
              : "重新读取当前设置"}
        </button>
        {pending && (
          <button
            type="button"
            disabled={!canReload}
            onClick={() => void reload()}
          >
            从服务器已保存文件恢复
          </button>
        )}
      </div>
    </section>
  );
}
