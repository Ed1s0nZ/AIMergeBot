import { useState } from "react";
import { api } from "./api";
import { ErrorBox } from "./components";

export function SARIFDownload({ runId }: { runId: number }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return <div>
    <button disabled={busy} onClick={async () => {
      if (busy) return;
      setBusy(true); setError("");
      try {
        const result = await api<unknown>(`/runs/${runId}/sarif`);
        const url = URL.createObjectURL(new Blob([JSON.stringify(result, null, 2)], { type: "application/sarif+json" }));
        const link = document.createElement("a");
        link.href = url; link.download = `aimangebot-run-${runId}.sarif`;
        document.body.appendChild(link);
        link.click(); link.remove();
        // Allow browsers to resolve the download before releasing its Blob.
        window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      } catch (e) { setError((e as Error).message); }
      finally { setBusy(false); }
    }}>{busy ? "正在导出…" : "导出 SARIF"}</button>
    <ErrorBox error={error} />
  </div>;
}
