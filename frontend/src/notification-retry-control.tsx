export type RetryAvailability = {
  status: string;
  can_retry?: boolean;
  retry_unavailable_reason?: string;
};

const reasons: Record<string, string> = {
  state_not_retryable: "当前状态不能重试。",
  invalid_attempt: "尝试次数异常，不能重试。",
  attempts_exhausted: "已达到五次尝试上限。",
  configuration_unavailable: "原渠道配置不可用。",
  configuration_changed:
    "渠道配置已变更，旧记录不能重试，也不会自动改发到新配置。",
  integration_disabled: "渠道已停用，当前不能重试。",
};

export function NotificationRetryControl({
  delivery,
  busy,
  acknowledged,
  onAcknowledge,
  onRetry,
}: {
  delivery: RetryAvailability;
  busy: boolean;
  acknowledged: boolean;
  onAcknowledge: (value: boolean) => void;
  onRetry: () => void;
}) {
  if (!["failed", "unknown"].includes(delivery.status)) return null;
  if (delivery.can_retry !== true) {
    const reason = delivery.retry_unavailable_reason || "";
    const message =
      delivery.can_retry === false && Object.hasOwn(reasons, reason)
        ? reasons[reason]
        : "重试可用状态待确认，请刷新记录。";
    return <p>{message}</p>;
  }
  return (
    <>
      {delivery.status === "unknown" && (
        <label>
          <input
            type="checkbox"
            disabled={busy}
            checked={acknowledged}
            onChange={(e) => onAcknowledge(e.target.checked)}
          />
          我已核对接收渠道，重试可能产生重复消息。
        </label>
      )}
      <button
        disabled={busy || (delivery.status === "unknown" && !acknowledged)}
        onClick={onRetry}
      >
        重试投递
      </button>
    </>
  );
}
