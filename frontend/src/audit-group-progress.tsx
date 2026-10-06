import type { Run } from "./api";
export function AuditGroupProgress({ groups }: { groups?: Run["result"]["audit_groups"] }) {
  if (!groups?.length) return null;
  return <details className="coverage">
    <summary>分组审计 · {groups.filter(g => g.status === "completed").length}/{groups.length} 组完成</summary>
    <p>按改动中的源码线索优先分配检查预算；每组仍可检索整个固定提交。优先级和目录分组不代表漏洞判断或调用关系。</p>
    {groups.map(g => <details key={g.id}>
      <summary>{g.id} · {({ running: "审计中", completed: "已完成", failed: "失败", unprocessed: "未处理" })[g.status] || "未知状态"} · {g.files.length} 个文件 · {g.priority_weight === undefined ? "优先级未记录" : g.priority_weight > 1 ? "优先检查" : "标准检查"}</summary>
      <ul>{g.files.map(file => <li key={file}>{file}</li>)}</ul>
    </details>)}
  </details>;
}
