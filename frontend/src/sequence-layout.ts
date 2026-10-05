import type { SequenceStep } from "./api";
const textWidth = (text: string) =>
  Array.from(text).reduce((n, c) => n + (c.charCodeAt(0) > 255 ? 12 : 6.8), 0);
// Keep words intact; only identifiers longer than a whole line are split.
export function wrapSequenceText(text: string, width: number): string[] {
  const lines: string[] = [];
  let line = "";
  const tokens = text.match(/[\x21-\xff]+|[^\x00-\xff]|\s+/gu) || [];
  for (const token of tokens) {
    if (/^\s+$/u.test(token)) {
      if (line && !line.endsWith(" ")) line += " ";
      continue;
    }
    if (line && textWidth(line + token) > width) {
      lines.push(line.trimEnd());
      line = "";
    }
    if (textWidth(token) <= width) {
      line += token;
      continue;
    }
    for (const char of Array.from(token)) {
      if (line && textWidth(line + char) > width) {
        lines.push(line);
        line = "";
      }
      line += char;
    }
  }
  if (line.trim()) lines.push(line.trimEnd());
  return lines;
}
export function compactSequenceText(text: string, width: number, maxLines = 3) {
  const lines = wrapSequenceText(text, width);
  if (lines.length <= maxLines) return lines;
  const visible = lines.slice(0, maxLines);
  let last = visible[maxLines - 1];
  while (textWidth(last + "…") > width)
    last = Array.from(last).slice(0, -1).join("");
  visible[maxLines - 1] = last.trimEnd() + "…";
  return visible;
}
export function sequenceLayout(
  participants: { id: string; label: string }[],
  steps: SequenceStep[],
  overview = false,
) {
  const gap = 280,
    width = Math.max(820, (participants.length - 1) * gap + 300);
  const x = (id: string) =>
    150 + participants.findIndex((p) => p.id === id) * gap;
  const headerHeight = Math.max(
    64,
    ...participants.map(
      (p) => compactSequenceText(p.label, 180).length * 17 + 30,
    ),
  );
  let top = headerHeight + 42;
  const rows = steps.map((step) => {
    const from = x(step.from),
      to = x(step.to);
    const middle =
      from === to ? Math.min(from + 40, width - 210) : (from + to) / 2;
    const available =
      step.kind === "note" ? 330 : Math.max(220, Math.abs(to - from) - 40);
    const sourceIDs = [
      ...new Set(
        step.evidence
          .map((ref) => ref.repository_id)
          .filter((id): id is number => !!id),
      ),
    ];
    const label =
      (sourceIDs.length
        ? `仓库 ${sourceIDs.map((id) => "#" + id).join(" / ")} · `
        : "") +
      (step.certainty === "inferred" ? "推测 · " : "") +
      (step.kind === "return" ? "返回 · " : "") +
      step.label;
    const lines = compactSequenceText(label, available, overview ? 1 : 3);
    const height = overview ? 58 : Math.max(82, lines.length * 18 + 48);
    const row = { step, from, to, middle, top, height, lines };
    top += height;
    return row;
  });
  return { width, height: top + 24, headerHeight, x, rows };
}
