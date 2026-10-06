import type { Run } from "./api";
const stopReasons: Record<string, string> = {
  agent_step_budget: "决策轮数预算耗尽", repository_unavailable: "固定源码读取服务不可用",
  context_compression: "上下文压缩不可用", model_token_budget: "模型 token 预算耗尽",
  model_usage_unknown: "模型用量信息缺失", deadline: "检查时间耗尽",
  canceled: "检查已取消", agent_failure: "审计流程失败，具体原因未分类",
};
export function AuditGroupProgress({ groups }: { groups?: Run["result"]["audit_groups"] }) {
  if (!groups?.length) return null;
  return <details className="coverage">
    <summary>分组审计 · {groups.filter(g => g.status === "completed").length}/{groups.length} 组完成</summary>
    <p>按改动中的源码线索优先分配检查预算；每组仍可检索整个固定提交。优先级和目录分组不代表漏洞判断或调用关系。</p>
    {groups.map(g => <details key={g.id}>
      <summary>{g.id} · {({ running: "审计中", completed: "已完成", failed: "失败", unprocessed: "未处理" })[g.status] || "未知状态"} · {g.files.length} 个文件 · {g.priority_weight === undefined ? "优先级未记录" : g.priority_weight > 1 ? "优先检查" : "标准检查"}</summary>
      {g.status === "failed" && <p>停止原因：{g.stop_reason ? stopReasons[g.stop_reason] || "未知停止类型" : "历史记录未提供"}。本组未完成，不能视为已排除风险。</p>}
      <ul>{g.files.map(file => <li key={file}>{file}</li>)}</ul>
    </details>)}
  </details>;
}
