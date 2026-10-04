import { ScanLine } from "lucide-react";
import type { ReactNode } from "react";
export const statuses: Record<string, string> = {
  pending: "待处理",
  running: "审计中",
  succeeded: "已完成",
  skipped: "按策略跳过",
  failed: "失败",
  incomplete: "覆盖不完整",
  cancelled: "已取消",
  high: "高危",
  medium: "中危",
  low: "低危",
  accepted: "已接受",
  false_positive: "误报",
  fixed: "已修复",
};
export function Badge({ value }: { value: string }) {
  return <span className={"badge " + value}>{statuses[value] || value}</span>;
}
export function ErrorBox({ error }: { error: string }) {
  return error ? (
    <div className="error" role="alert">
      {error}
    </div>
  ) : null;
}
export function Empty({ children }: { children: ReactNode }) {
  return (
    <div className="empty">
      <span className="empty-icon">
        <ScanLine size={25} />
      </span>
      <div>{children}</div>
    </div>
  );
}
export function date(s: string) {
  return new Date(s).toLocaleString("zh-CN", {
    timeZone: "Asia/Shanghai",
    hour12: false,
  });
}
export function safeURL(s: string) {
  try {
    const u = new URL(s);
    return ["http:", "https:"].includes(u.protocol) ? u.href : undefined;
  } catch {
    return undefined;
  }
}
