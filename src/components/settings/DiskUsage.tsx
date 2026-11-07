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
  const [message, setMessage] = useState("");
  const pending = useRef<AbortController | null>(null);
  async function refresh() {
    if (disabled) return;
    pending.current?.abort();
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setError("");
    setMessage("");
    setDisk(null);
    setDiskError("");
    const diskRead = backend
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
      // The native metadata read is read-only and cannot be interrupted. Keep
      // its generation owned until it settles so unmount/cancel invalidates it.
      await diskRead;
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
        if (abort.signal.aborted)
          setMessage(
            "Measurement cancelled. Results completed before cancellation may remain below; later replies were discarded.",
          );
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
      setDiskError("");
      setMessage("");
      return;
    }
    void refresh();
    return () => {
      pending.current?.abort();
      pending.current = null;
    };
  }, [disabled, revision]);
  return (
    <section>
      <h2>Engine disk usage</h2>
      <p>
        Measured by Docker. Shared image layers and reclaimable build cache are
        reported by the engine; this is separate from the allocated size of its
        Windows virtual disk.
      </p>
      <details>
        <summary>Free space and virtual disk maintenance</summary>
        <p>
          Removing unused resources frees space inside Linux. Windows may keep
          the virtual disk allocation. File length, allocated Windows space and
          Docker usage can differ; OrbWin does not measure allocated file space
          or offer automatic compaction or moving engine storage in this build.
        </p>
        <p>
          Export important development data before offline disk maintenance.
          Confirm the exact OrbWin disk and all workloads that would be
          interrupted. Microsoft documents disk maintenance prerequisites; those
          procedures are not an OrbWin backup or restore workflow. A global WSL
          shutdown also interrupts other engines and distributions.
        </p>
        <p>Copy a Microsoft guidance address into your browser:</p>
        <label>
          WSL disk guidance
          <input
            readOnly
            value="https://learn.microsoft.com/en-us/windows/wsl/disk-space"
            onFocus={(event) => event.currentTarget.select()}
          />
        </label>
        <label>
          Virtual disk compaction prerequisites
          <input
            readOnly
            value="https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/compact-vdisk"
            onFocus={(event) => event.currentTarget.select()}
          />
        </label>
      </details>
      <button
        className="button"
        disabled={disabled || busy}
        onClick={() => void refresh()}
      >
        {busy ? "Measuring…" : "Measure disk usage"}
      </button>
      {busy && (
        <button
          className="button"
          onClick={() => {
            pending.current?.abort();
            setMessage(
              "Measurement cancelled. Waiting for pending reads to settle; earlier engine rows may remain below.",
            );
          }}
        >
          Cancel measurement
        </button>
      )}
      {message && <p role="status">{message}</p>}
      {error && <p role="alert">{error}</p>}
      {disk && (
        <>
          {disk.hostCapacity && (
            <div className="host-capacity">
              <h3>Host data volume capacity</h3>
              <meter
                min={0}
                max={disk.hostCapacity.totalBytes}
                value={
                  disk.hostCapacity.totalBytes -
                  disk.hostCapacity.availableBytes
                }
                aria-label="Host data volume space unavailable to the current Windows user"
              />
              <p>
                {formatSize(disk.hostCapacity.availableBytes)} available to your
                Windows user out of {formatSize(disk.hostCapacity.totalBytes)}{" "}
                capacity. Quotas may limit these values. Host volume free space:{" "}
                {formatSize(disk.hostCapacity.freeBytes)}.
              </p>
              <p>
                This is the Windows volume containing OrbWin data, not free
                space inside Linux. Other files on the same volume also use this
                space.
              </p>
            </div>
          )}
          {disk.hostCapacityError && (
            <p role="status">{disk.hostCapacityError}</p>
          )}
          <dl className="properties">
            <dt>Virtual disk file length</dt>
            <dd>
              {formatSize(disk.fileLengthBytes)}. Sparse/compressed allocation
              can differ; removing resources does not automatically compact the
              Windows file.
            </dd>
            <dt>Data location</dt>
            <dd>
              <code>{disk.path}</code>
            </dd>
          </dl>
          <p>
            Use volume export after quiescing workloads for portable development
            data. Copying a running VHDX is not an application-consistent
            backup. Docker reclaimable bytes and Windows free space are separate
            measurements.
          </p>
        </>
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
