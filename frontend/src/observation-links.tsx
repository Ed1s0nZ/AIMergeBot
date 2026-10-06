import "./observation-links.css";
export function ObservationLinks({ ids }: { ids?: string[] }) {
  if (!ids?.length) return null;
  return <span className="observation-links">{ids.map(id => <button type="button" key={id} onClick={() => {
    const target = document.getElementById(`trace-${id}`);
    if (target instanceof HTMLDetailsElement) target.open = true;
    target?.scrollIntoView({ behavior: "smooth", block: "center" });
    const summary = target?.querySelector(":scope > summary");
    if (summary instanceof HTMLElement) summary.focus({ preventScroll: true });
  }}>{id}</button>)}</span>;
}
