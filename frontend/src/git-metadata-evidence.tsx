import { useState } from "react";
import { type GitChangeMetadata, type GitEntry } from "./api";
import "./git-metadata-evidence.css";

function mode(entry?: GitEntry | null) {
  if (!entry) return "不存在";
  const names: Record<string, string> = {
    "100644": "普通文件",
    "100755": "可执行文件",
    "120000": "符号链接",
    "160000": "子模块引用",
  };
  return `${names[entry.mode] || entry.type} · ${entry.mode}`;
}
export function GitMetadataEvidence({
  metadata,
}: {
  metadata: GitChangeMetadata;
}) {
  return (
    <div className="git-metadata-evidence">
      <table aria-label="Git 元数据前后对比">
        <thead>
          <tr>
            <th>属性</th>
            <th>BASE</th>
            <th>HEAD</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <th scope="row">路径</th>
            <td>{metadata.base ? metadata.old_path : "—"}</td>
            <td>{metadata.head ? metadata.new_path : "—"}</td>
          </tr>
          <tr>
            <th scope="row">文件模式</th>
            <td>{mode(metadata.base)}</td>
            <td>{mode(metadata.head)}</td>
          </tr>
          <tr>
            <th scope="row">对象 ID</th>
            <td>
              <code>{metadata.base?.object_id || "—"}</code>
            </td>
            <td>
              <code>{metadata.head?.object_id || "—"}</code>
            </td>
          </tr>
        </tbody>
      </table>
      <p className="muted">元数据来自固定提交，不代表风险已运行复现。</p>
      {(metadata.base?.type === "commit" ||
        metadata.head?.type === "commit") && (
        <p className="muted">
          此处仅检查子模块引用，不下载或审计子模块仓库内容。
        </p>
      )}
    </div>
  );
}
export function GitMetadataChanges({
  changes,
}: {
  changes?: GitChangeMetadata[];
}) {
  const [page, setPage] = useState(0);
  if (!changes?.length) return null;
  const start = Math.min(
    page * 10,
    Math.max(0, Math.floor((changes.length - 1) / 10) * 10),
  );
  return (
    <details className="git-metadata-changes">
      <summary>Git 元数据变更 · {changes.length} 项</summary>
      {changes.slice(start, start + 10).map((item, i) => (
        <GitMetadataEvidence
          key={`${start + i}:${item.new_path}`}
          metadata={item}
        />
      ))}
      {changes.length > 10 && (
        <div className="git-metadata-pagination">
          <button
            type="button"
            disabled={start === 0}
            onClick={() => setPage(Math.max(0, page - 1))}
          >
            上一页
          </button>
          <span>
            {start + 1}–{Math.min(start + 10, changes.length)} /{" "}
            {changes.length}
          </span>
          <button
            type="button"
            disabled={start + 10 >= changes.length}
            onClick={() => setPage(page + 1)}
          >
            下一页
          </button>
        </div>
      )}
    </details>
  );
}
