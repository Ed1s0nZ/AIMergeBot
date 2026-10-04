import type { Settings } from "./api";
export function GitAuditSettingsFields({
  value,
  onChange,
}: {
  value: Settings["git_audit"];
  onChange: (value: Settings["git_audit"]) => void;
}) {
  return (
    <fieldset>
      <legend>Git 深度调查</legend>
      <label className="check">
        <input
          type="checkbox"
          checked={value.enabled}
          onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
        />
        使用固定提交的本地 Git 仓库
      </label>
      <p className="muted">
        支持任意文本代码，不执行仓库代码。关闭后使用 GitLab
        API，历史与版本对比工具不可用。子模块和 LFS 实体文件不会自动下载。
      </p>
      <div className="field-row">
        <label>
          历史深度
          <input
            type="number"
            min={1}
            max={10000}
            required
            value={value.history_depth}
            onChange={(e) =>
              onChange({ ...value, history_depth: Number(e.target.value) })
            }
          />
          <small>浅历史的边界会显示在工具返回中。</small>
        </label>
        <label>
          仓库预算（MiB）
          <input
            type="number"
            min={16}
            max={4096}
            required
            value={value.max_pack_mib}
            onChange={(e) =>
              onChange({ ...value, max_pack_mib: Number(e.target.value) })
            }
          />
        </label>
      </div>
      <label>
        每次审计工具调用上限
        <input
          type="number"
          min={10}
          max={200}
          required
          value={value.max_tool_calls}
          onChange={(e) =>
            onChange({ ...value, max_tool_calls: Number(e.target.value) })
          }
        />
      </label>
    </fieldset>
  );
}
