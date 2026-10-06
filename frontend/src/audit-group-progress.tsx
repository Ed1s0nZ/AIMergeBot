import type { Run } from "./api";
const stopReasons: Record<string, string> = {
  model_response_unknown_field: "模型结果包含不支持的字段",
  model_response_json_syntax: "模型结果不是有效 JSON",
  model_response_json_type: "模型结果字段类型错误",
  model_response_json_decode: "模型结果无法解析",
  model_response_trailing_data: "模型结果包含多余内容",
  model_response_required_fields: "模型结果缺少必填内容",
  model_response_finding_limit: "模型结果超过问题数量上限",
  model_response_output_budget: "模型结果超过大小上限",
  model_response_invalid_structure: "模型结果结构不合规",
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
