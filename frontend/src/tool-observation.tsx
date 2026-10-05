export function ToolObservation({ output }: { output: string }) {
  let data: Record<string, unknown>;
  try {
    data = JSON.parse(output);
    if (!data || typeof data !== "object") throw new Error();
  } catch {
    return <pre>{output}</pre>;
  }
  return (
    <>
      {typeof data.repository_id === "number" && (
        <p className="muted">关联仓库 #{data.repository_id} · 固定版本</p>
      )}
      {Array.isArray(data.repositories) && (
        <ul>
          {data.repositories
            .filter(
              (item) =>
                item &&
                typeof item === "object" &&
                typeof item.project_id === "number" &&
                typeof item.sha === "string",
            )
            .map((item) => (
              <li key={item.project_id}>
                仓库 #{item.project_id} ·{" "}
                <code title={item.sha}>{item.sha.slice(0, 12)}</code> ·{" "}
                {item.available ? "可读取" : "不可用"}
              </li>
            ))}
        </ul>
      )}
      {typeof data.base_sha === "string" &&
        typeof data.head_sha === "string" && (
          <p className="muted">
            BASE {data.base_sha.slice(0, 12)} · HEAD{" "}
            {data.head_sha.slice(0, 12)}
          </p>
        )}
      {typeof data.text === "string" && (
        <pre>{data.text || "没有匹配的文本结果"}</pre>
      )}
      {Array.isArray(data.files) && (
        <pre>
          {data.files.filter((item) => typeof item === "string").join("\n") ||
            "没有文件"}
        </pre>
      )}
      {typeof data.next_cursor === "number" && (
        <p className="muted">后续游标：{data.next_cursor}</p>
      )}
    </>
  );
}
