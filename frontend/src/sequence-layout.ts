import type { SequenceStep } from "./api";
// Weighted wrapping keeps CJK labels readable in exported standalone SVGs.
export function wrapSequenceText(text: string, width: number): string[] {
  const lines: string[] = [];
  let line = "",
    used = 0;
  for (const char of Array.from(text)) {
    const size = char.charCodeAt(0) > 255 ? 12 : 6.8;
    if (used + size > width && line) {
      lines.push(line);
      line = "";
      used = 0;
    }
    line += char;
    used += size;
  }
  if (line) lines.push(line);
  return lines;
}
export function sequenceLayout(
  participants: { id: string; label: string }[],
  steps: SequenceStep[],
) {
  const width = Math.max(620, participants.length * 220 + 100);
  const x = (id: string) =>
    80 + participants.findIndex((p) => p.id === id) * 220;
  const headerHeight = Math.max(
    60,
    ...participants.map((p) => wrapSequenceText(p.label, 140).length * 16 + 24),
  );
  let top = headerHeight + 46;
  const rows = steps.map((step) => {
    const from = x(step.from),
      to = x(step.to);
    const middle = from === to ? from + 35 : (from + to) / 2;
    const available =
      step.kind === "note"
        ? Math.min(280, width - 180)
        : Math.max(150, Math.abs(to - from) - 30);
    const label =
      (step.certainty === "inferred" ? "推测 · " : "") +
      (step.kind === "return" ? "返回 · " : "") +
      step.label;
    const lines = wrapSequenceText(label, available);
    const height = Math.max(100, lines.length * 18 + 55);
    const row = { step, from, to, middle, top, height, lines };
    top += height;
    return row;
  });
  return { width, height: top + 42, headerHeight, x, rows };
}
