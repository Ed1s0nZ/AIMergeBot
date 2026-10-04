import type { Settings } from "./api";
import "./audit-quota-settings.css";

const groups = [
  {
    title: "等待与执行中的任务",
    description: "达到容量时拒绝新任务；相同版本的已有任务仍可复用。",
    fields: [
      ["outstanding_global", "工作空间总容量", 10000],
      ["outstanding_project", "每个项目容量", 10000],
      ["outstanding_user", "每个用户容量", 10000],
    ],
  },
  {
    title: "同时执行",
    description: "超出并发的任务保留在队列中，其他项目仍可执行。",
    fields: [
      ["running_project", "每个项目并发", 16],
      ["running_user", "每个用户并发", 16],
    ],
  },
  {
    title: "滚动 24 小时审计次数",
    description:
      "按开始执行计数，包含自动重试和失败尝试；达到上限后等待额度恢复。",
    fields: [
      ["daily_global", "工作空间审计次数", 100000],
      ["daily_project", "每个项目审计次数", 100000],
      ["daily_user", "每个用户审计次数", 100000],
    ],
  },
] as const;

export function AuditQuotaSettingsFields({
  value,
  onChange,
}: {
  value: Settings["audit_quotas"];
  onChange: (value: Settings["audit_quotas"]) => void;
}) {
  return (
    <fieldset className="audit-quota-fields">
      <legend>容量与次数</legend>
      <p className="muted">
        保存后实时生效。管理员同样受限；自动任务共用工作空间与项目额度。次数额度不代表
        token 费用。
      </p>
      {groups.map((group) => (
        <div className="audit-quota-group" key={group.title}>
          <h3>{group.title}</h3>
          <p className="muted">{group.description}</p>
          <div className="audit-quota-grid">
            {group.fields.map(([key, label, max]) => (
              <label key={key}>
                {label}
                <input
                  type="number"
                  min={1}
                  max={max}
                  step={1}
                  required
                  value={value[key]}
                  onChange={(e) =>
                    onChange({ ...value, [key]: Number(e.target.value) })
                  }
                />
              </label>
            ))}
          </div>
        </div>
      ))}
      <small className="muted">
        项目、用户并发不能高于各自任务容量。降低额度会保留已接收任务和审计证据。
      </small>
    </fieldset>
  );
}
