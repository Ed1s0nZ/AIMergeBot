import { GitMetadataEvidence } from "./git-metadata-evidence";
import { useId, useRef, useState } from "react";
import { Download, GitBranch, LocateFixed } from "lucide-react";
import type { SequenceDiagram } from "./api";
import { sequenceLayout, compactSequenceText } from "./sequence-layout";

export function FindingSequence({
  diagram,
  findingId,
}: {
  diagram?: SequenceDiagram;
  findingId: string;
}) {
  const [selected, setSelected] = useState<number | null>(null),
    [exportError, setExportError] = useState("");
  const svg = useRef<SVGSVGElement>(null),
    id = useId().replace(/[^a-zA-Z0-9]/g, "");
  if (!diagram)
    return (
      <div className="sequence-empty">
        <GitBranch size={16} />
        <span>此历史结果未生成问题时序图，重新审计可生成。</span>
      </div>
    );
  if (diagram.status === "unavailable")
    return (
      <div className="sequence-empty">
        <GitBranch size={16} />
        <span>{diagram.reason || "时序图暂不可用，审计发现已保留。"}</span>
      </div>
    );
  const participants = diagram.participants,
    steps = diagram.steps;
  if (
    !participants ||
    !steps ||
    participants.length < 2 ||
    participants.length > 8 ||
    steps.length < 1 ||
    steps.length > 16 ||
    steps.some(
      (s) =>
        !participants.some((p) => p.id === s.from) ||
        !participants.some((p) => p.id === s.to),
    )
  )
    return (
      <div className="sequence-empty">时序图数据不完整，审计发现已保留。</div>
    );
  const layout = sequenceLayout(participants, steps),
    active =
      selected === null
        ? steps.findIndex((s) => s.risk)
        : Math.min(selected, steps.length - 1),
    step = steps[active];
  const selectStep = (index: number) => {
    setSelected(index);
    const canvas = svg.current?.parentElement;
    const target = svg.current?.querySelector(
      `[data-sequence-step="${index}"]`,
    );
    if (canvas && target) {
      canvas.scrollTo({
        left: Math.max(
          0,
          (layout.rows[index].middle *
            svg.current!.getBoundingClientRect().width) /
            layout.width -
            canvas.clientWidth / 2,
        ),
        top:
          canvas.scrollTop +
          target.getBoundingClientRect().top -
          canvas.getBoundingClientRect().top -
          20,
        behavior: "smooth",
      });
    }
  };
  const download = () => {
    if (!svg.current) return;
    try {
      const data = new XMLSerializer().serializeToString(svg.current),
        url = URL.createObjectURL(
          new Blob([data], { type: "image/svg+xml;charset=utf-8" }),
        );
      const a = document.createElement("a");
      a.href = url;
      a.download = `finding-${findingId.replace(/[^a-zA-Z0-9_-]/g, "").slice(0, 80)}-sequence.svg`;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      setExportError("");
    } catch {
      setExportError("导出失败，请稍后重试。");
    }
  };
  return (
    <section className="finding-sequence" aria-label="问题时序图">
      <div className="sequence-heading">
        <div>
          <h4>
            <GitBranch size={17} />
            问题时序图
          </h4>
          <p>点击步骤查看完整说明与固定提交的代码证据</p>
        </div>
        <div className="sequence-actions">
          {steps.some((s) => s.risk) && (
            <button
              type="button"
              onClick={() => selectStep(steps.findIndex((s) => s.risk))}
            >
              <LocateFixed size={15} />
              定位风险
            </button>
          )}
          <button type="button" onClick={download}>
            <Download size={15} />
            下载 SVG
          </button>
        </div>
      </div>
      {diagram.status === "partial" && (
        <p className="sequence-limit">
          链路包含推测或缺失信息，请结合代码证据复核。
        </p>
      )}
      <div className="sequence-legend">
        <span>
          <i />
          代码证据
        </span>
        <span className="risk">
          <i />
          风险步骤
        </span>
        <span className="inferred">
          <i />
          推测关系
        </span>
        <small>静态推导 · 非实际运行轨迹</small>
      </div>
      <div
        className="sequence-scroll"
        tabIndex={0}
        aria-label="时序图画布，可横向滚动"
      >
        <svg
          ref={svg}
          xmlns="http://www.w3.org/2000/svg"
          width={layout.width}
          height={layout.height}
          viewBox={`0 0 ${layout.width} ${layout.height}`}
          role="img"
          aria-labelledby={`${id}-title ${id}-desc`}
          style={{ fontFamily: "system-ui, sans-serif", background: "#ffffff" }}
        >
          <title id={`${id}-title`}>问题链路时序图</title>
          <desc id={`${id}-desc`}>
            根据代码推导，非实际运行轨迹。
            {steps
              .map(
                (s, i) =>
                  `步骤${i + 1}：${s.label}${s.risk ? "，风险位置" : ""}${s.certainty === "inferred" ? "，推测关系" : ""}`,
              )
              .join("；")}
          </desc>
          <defs>
            {["normal", "risk"].map((type) => (
              <marker
                key={type}
                id={`${id}-${type}`}
                viewBox="0 0 10 10"
                refX={9}
                refY={5}
                markerWidth={7}
                markerHeight={7}
                orient="auto-start-reverse"
              >
                <path
                  d="M 0 0 L 10 5 L 0 10 z"
                  fill={type === "risk" ? "#bc3942" : "#53698b"}
                />
              </marker>
            ))}
          </defs>
          <rect
            x={0}
            y={0}
            width={layout.width}
            height={layout.height}
            fill="#ffffff"
          />
          {participants.map((p) => {
            const x = layout.x(p.id),
              lines = compactSequenceText(p.label, 180);
            return (
              <g key={p.id}>
                <title>{p.label}</title>
                <line
                  x1={x}
                  x2={x}
                  y1={layout.headerHeight + 18}
                  y2={layout.height - 22}
                  stroke="#dce3ef"
                  strokeDasharray="5 5"
                />
                <rect
                  x={x - 102}
                  y={18}
                  width={204}
                  height={layout.headerHeight}
                  rx={10}
                  fill="#f5f7ff"
                  stroke="#dce3ef"
                />
                <text
                  x={x}
                  y={18 + (layout.headerHeight - lines.length * 16) / 2 + 12}
                  textAnchor="middle"
                  fill="#253651"
                  fontSize={13}
                  fontWeight={600}
                >
                  {lines.map((line, n) => (
                    <tspan key={n} x={x} dy={n ? 16 : 0}>
                      {line}
                    </tspan>
                  ))}
                </text>
              </g>
            );
          })}
          {layout.rows.map((row, i) => {
            const s = row.step,
              color = s.risk ? "#bc3942" : "#53698b",
              y = row.top + row.height - 26;
            return (
              <g
                key={i}
                role="button"
                tabIndex={0}
                aria-label={`步骤 ${i + 1}：${s.label}${s.risk ? "，风险位置" : ""}，查看代码证据`}
                onClick={() => setSelected(i)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    setSelected(i);
                  }
                }}
                className="sequence-step"
                data-sequence-step={i}
              >
                <title>{s.label}</title>
                <rect
                  x={10}
                  y={row.top + 4}
                  width={layout.width - 20}
                  height={row.height - 8}
                  rx={10}
                  fill={
                    s.risk ? "#fff1f2" : active === i ? "#f3f6fc" : "#ffffff"
                  }
                  fillOpacity={s.risk || active === i ? 1 : 0}
                  stroke={active === i ? color : "none"}
                  strokeOpacity={0.35}
                />
                <circle
                  cx={30}
                  cy={y}
                  r={12}
                  fill={s.risk ? "#bc3942" : "#e9eef6"}
                />
                <text
                  x={30}
                  y={y + 4}
                  textAnchor="middle"
                  fontSize={11}
                  fill={s.risk ? "#ffffff" : "#53698b"}
                >
                  {i + 1}
                </text>
                {s.kind === "note" ? (
                  <>
                    <rect
                      x={Math.max(
                        60,
                        Math.min(row.middle - 180, layout.width - 390),
                      )}
                      y={row.top + 10}
                      width={360}
                      height={row.height - 20}
                      rx={8}
                      fill={s.risk ? "#ffe4e6" : "#eef3fb"}
                      stroke={color}
                      strokeDasharray={
                        s.certainty === "inferred" ? "6 4" : undefined
                      }
                    />
                    <text
                      x={Math.max(
                        240,
                        Math.min(row.middle, layout.width - 210),
                      )}
                      y={row.top + 30}
                      textAnchor="middle"
                      fontSize={12}
                      fill={color}
                    >
                      {row.lines.map((line, n) => (
                        <tspan
                          key={n}
                          x={Math.max(
                            240,
                            Math.min(row.middle, layout.width - 210),
                          )}
                          dy={n ? 18 : 0}
                        >
                          {line}
                        </tspan>
                      ))}
                    </text>
                  </>
                ) : (
                  <>
                    <text
                      x={row.middle}
                      y={row.top + 24}
                      textAnchor="middle"
                      fontSize={12}
                      fill={color}
                    >
                      {row.lines.map((line, n) => (
                        <tspan key={n} x={row.middle} dy={n ? 18 : 0}>
                          {line}
                        </tspan>
                      ))}
                    </text>
                    {row.from === row.to ? (
                      <path
                        d={`M ${row.from} ${y - 12} h 70 v 20 h -70`}
                        fill="none"
                        stroke={color}
                        strokeWidth={s.risk ? 2.4 : 1.6}
                        strokeDasharray={
                          s.certainty === "inferred" ? "6 4" : undefined
                        }
                        markerEnd={`url(#${id}-${s.risk ? "risk" : "normal"})`}
                      />
                    ) : (
                      <line
                        x1={row.from}
                        x2={row.to}
                        y1={y}
                        y2={y}
                        stroke={color}
                        strokeWidth={s.risk ? 2.4 : 1.6}
                        strokeDasharray={
                          s.certainty === "inferred" || s.kind === "return"
                            ? "6 4"
                            : undefined
                        }
                        markerEnd={`url(#${id}-${s.risk ? "risk" : "normal"})`}
                      />
                    )}
                  </>
                )}
              </g>
            );
          })}
        </svg>
      </div>
      <div className="sequence-step-list" aria-label="选择链路步骤">
        {steps.map((s, i) => (
          <button
            key={i}
            type="button"
            aria-pressed={active === i}
            className={s.risk ? "risk" : ""}
            onClick={() => selectStep(i)}
          >
            {i + 1}
            {s.risk ? " · 风险" : ""}
            {s.certainty === "inferred" ? " · 推测" : ""}
          </button>
        ))}
      </div>
      {step && (
        <div className="sequence-evidence">
          <strong>
            步骤 {active + 1} · {step.label}
          </strong>
          <p className="muted">
            {step.certainty === "inferred"
              ? "关系为推测；附带引用也不能证明这条调用链成立。"
              : "引用已与固定提交的代码或元数据核对；不代表调用关系或可利用性已验证。"}
          </p>
          {step.evidence?.length ? (
            step.evidence.map((ref, i) => (
              <div key={i}>
                <code>
                  {ref.repository_id ? `仓库 #${ref.repository_id} · ` : ""}
                  {ref.side.toUpperCase()} · {ref.file}
                  {ref.anchor_type === "git_metadata"
                    ? " · Git 元数据"
                    : `:${ref.line}`}{" "}
                  · {ref.sha?.slice(0, 12)}
                </code>
                {ref.side === "base" && (
                  <p className="sequence-limit">
                    变更前证据：对应已删除的发现代码或旧版本上下文。
                  </p>
                )}
                {ref.anchor_type === "git_metadata" && ref.metadata ? (
                  <GitMetadataEvidence metadata={ref.metadata} />
                ) : (
                  <pre>{ref.snippet}</pre>
                )}
              </div>
            ))
          ) : (
            <p className="muted">此步骤尚无直接代码引用。</p>
          )}
        </div>
      )}
      {!!diagram.limitations?.length && (
        <div className="sequence-limit">
          <strong>链路限制</strong>
          <ul>
            {diagram.limitations.map((item, i) => (
              <li key={i}>{item}</li>
            ))}
          </ul>
        </div>
      )}
      <details className="sequence-source">
        <summary>查看 Mermaid 源码</summary>
        <pre>{diagram.mermaid || "暂无源码"}</pre>
      </details>
      <p className="muted sequence-disclaimer">
        这张图用于解释审计发现，不是实际运行轨迹。最终风险仍需人工复核。
      </p>
      {exportError && <p role="alert">{exportError}</p>}
    </section>
  );
}
