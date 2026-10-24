import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { formatSize } from "./storage-model";
type Usage = {
  Type: string;
  TotalCount: string;
  Active: string;
  Size: string;
  Reclaimable: string;
};
export function DiskUsage({
  disabled,
  revision,
}: {
  disabled: boolean;
  revision: unknown;
}) {
  const [rows, setRows] = useState<Usage[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [disk, setDisk] = useState<Awaited<
    ReturnType<typeof backend.diskInfo>
  > | null>(null);
  const [diskError, setDiskError] = useState("");
  const pending = useRef<AbortController | null>(null);
  async function refresh() {
    if (disabled) return;
    pending.current?.abort();
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setError("");
    setDisk(null);
    setDiskError("");
    void backend
      .diskInfo()
      .then((value) => {
        if (!abort.signal.aborted) setDisk(value);
      })
      .catch((reason) => {
        if (!abort.signal.aborted) setDiskError(String(reason));
      });
    let text = "";
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        { kind: "diskUsage" },
        (chunk, stream) => {
          if (stream !== "stderr") text += chunk;
        },
        abort.signal,
      );
      if (abort.signal.aborted) return;
      if (!result.success) throw new Error(`${result.message}\n${text}`);
      const rows = text
        .split(/\r?\n/)
        .filter(Boolean)
        .map((line) => JSON.parse(line) as Usage);
      if (rows.some((row) => typeof row.Type !== "string"))
        throw new Error("Disk usage response is invalid.");
      setRows(rows);
    } catch (reason) {
      if (!abort.signal.aborted) {
        setRows([]);
        setError(String(reason));
      }
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  useEffect(() => {
    if (disabled) {
      pending.current?.abort();
      pending.current = null;
      setBusy(false);
      setRows([]);
      setDisk(null);
      return;
    }
    void refresh();
    return () => pending.current?.abort();
  }, [disabled, revision]);
  return (
    <section>
      <h2>Engine disk usage</h2>
      <p>
        Measured by Docker. Shared image layers and reclaimable build cache are
        reported by the engine; this is separate from the allocated size of its
        Windows virtual disk.
      </p>
      <button
        className="button"
        disabled={disabled || busy}
        onClick={() => void refresh()}
      >
        {busy ? "Measuring…" : "Measure disk usage"}
      </button>
      {error && <p role="alert">{error}</p>}
      {disk && (
        <dl className="properties">
          <dt>Virtual disk file length</dt>
          <dd>
            {formatSize(disk.fileLengthBytes)}. Sparse/compressed allocation can
            differ; removing resources does not automatically compact the
            Windows file.
          </dd>
          <dt>Data location</dt>
          <dd>
            <code>{disk.path}</code>
          </dd>
        </dl>
      )}
      {diskError && <p role="status">{diskError}</p>}
      {rows.length > 0 && (
        <table className="usage-table">
          <thead>
            <tr>
              <th>Resource</th>
              <th>Total</th>
              <th>Active</th>
              <th>Size</th>
              <th>Reclaimable</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.Type}>
                <th scope="row">{row.Type}</th>
                <td>{row.TotalCount}</td>
                <td>{row.Active}</td>
                <td>{row.Size}</td>
                <td>{row.Reclaimable}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
