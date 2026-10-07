export type CheckAssessment = {
  publication_state?: string;
  remote_id?: number;
  run_id: number;
  head_sha: string;
  state: string;
  high_risk_findings: number;
  unknown_severity: number;
  published: boolean;
  blocking: boolean;
};
export function RunCheckAdvice({ value }: { value: CheckAssessment }) {
  const labels: Record<string, string> = {
    pending: "等待审计",
    running: "审计进行中",
    failed: "审计失败",
    cancelled: "审计取消",
    skipped: "按策略跳过",
    incomplete: "审计覆盖不完整",
    unknown: "状态或证据不足",
    high_risk: "包含高风险发现",
    completed: "审计已完成",
  };
  return (
    <section className="panel">
      <h2>运行检查摘要</h2>
      <p>
        {labels[value.state] || "状态待确认"} · 高风险发现{" "}
        {value.high_risk_findings}
        {value.unknown_severity ? ` · 未知等级 ${value.unknown_severity}` : ""}
      </p>
      <p>
        运行 #{value.run_id} · HEAD {value.head_sha.slice(0, 12)}
        。此摘要对应本次固定提交；审计完成不代表代码安全，也不代表当前 PR
        最新提交。检查发布状态见下方。
      </p>
      <p>
        {value.published
          ? `平台已确认检查 #${value.remote_id} · ${value.blocking ? "阻断模式（实际合并规则以平台设置为准）" : "建议模式"}`
          : (
              {
                disabled: "尚未启用平台检查发布，未启用阻断。",
                pending: "检查发布已排队。",
                sending: "正在发布，尚未确认远端结果。",
                unknown: "远端结果未知，不能视为未发送或成功。",
                failed: "检查发布失败。",
                cancelled: "发布配置已变更，未发送的检查已取消。",
                stale: "检查快照已过期，未发布为当前结果。",
              } as Record<string, string>
            )[value.publication_state || "disabled"] || "发布状态待确认。"}
      </p>
    </section>
  );
}
export function FormatHints({
  items,
}: {
  items?: { file: string; changed_pairs: number }[];
}) {
  if (!items?.length) return null;
  return (
    <section className="panel">
      <h2>空白变更提示</h2>
      <p>
        以下文件的新增与删除行按顺序去除首尾空白后相同，仅供人工查看。缩进、字符串或模板中的空白仍可能改变行为；原始变更已保留审计，不能据此判定语义等价、已修复或安全。
      </p>
      <ul>
        {items.map((hint, i) => (
          <li key={i}>
            {hint.file} · {hint.changed_pairs} 对变更行
          </li>
        ))}
      </ul>
      <p>仅显示匹配候选，最多 200 个文件；没有提示不代表没有格式变更。</p>
    </section>
  );
}
