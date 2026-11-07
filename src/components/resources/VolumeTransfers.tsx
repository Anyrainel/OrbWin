import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { useOperation } from "../Operations";
import { VolumeClones } from "./VolumeClones";

type Transfer = {
  transaction: string;
  volume: string;
  phase: string;
  active: boolean;
  exists: boolean;
  owned: boolean;
  truncated?: boolean;
  message?: string;
};

export function VolumeTransfers() {
  const open = useOperation();
  const [visible, setVisible] = useState(false);
  const [records, setRecords] = useState<Transfer[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  async function refresh() {
    pending.current?.abort();
    const abort = new AbortController();
    pending.current = abort;
    setVisible(true);
    setBusy(true);
    setError("");
    let output = "",
      diagnostic = "";
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        { kind: "volumeTransfers" },
        (chunk, stream) => {
          if (stream === "stderr")
            diagnostic = (diagnostic + chunk).slice(-8192);
          else output += chunk;
        },
        abort.signal,
      );
      if (abort.signal.aborted) return;
      if (!result.success) throw new Error(diagnostic || result.message);
      const value = JSON.parse(output) as Transfer[];
      if (
        !Array.isArray(value) ||
        value.some(
          (record) =>
            typeof record.volume !== "string" ||
            typeof record.transaction !== "string" ||
            typeof record.phase !== "string",
        )
      )
        throw new Error("Invalid volume recovery response.");
      setRecords(value);
    } catch (reason) {
      if (!abort.signal.aborted) setError(String(reason));
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  const incomplete = records.filter(
    (record) => !["complete", "discarded"].includes(record.phase),
  );
  return (
    <>
      <section
        className="volume-transfer-recovery"
        aria-label="Volume restore recovery"
      >
        <button
          className="button"
          disabled={busy}
          onClick={() => void refresh()}
        >
          {visible ? "Refresh restore recovery" : "Review restore recovery"}
        </button>
        {busy && (
          <span role="status">
            {" "}
            Reading recorded transfers…{" "}
            <button
              className="button compact"
              onClick={() => {
                pending.current?.abort();
                setBusy(false);
                setError(
                  "Recovery read cancelled. Volume data was not changed.",
                );
              }}
            >
              Cancel
            </button>
          </span>
        )}
        {error && <p role="alert">{error}</p>}
        {visible && !busy && !error && (
          <>
            <p>
              Interrupted restores retain their source package and pending
              destination. Retry uses a fresh name. Discard is available only
              for an inactive recorded destination with matching ownership and
              no container attachments. Refresh after a recovery action.
            </p>
            {!incomplete.length ? (
              <p role="status">No incomplete restore receipts.</p>
            ) : (
              <ul>
                {incomplete.map((record) => (
                  <li key={record.transaction}>
                    <strong>{record.volume}</strong> —{" "}
                    {record.active
                      ? "Transfer active"
                      : record.phase === "intent"
                        ? "Creation not confirmed"
                        : "Restore incomplete"}
                    {record.message && <p>{record.message}</p>}
                    {record.truncated && (
                      <p>
                        Receipt was interrupted while writing. Inspect the
                        retained data before any removal.
                      </p>
                    )}
                    <button
                      className="button"
                      disabled={
                        record.active ||
                        !record.exists ||
                        !record.owned ||
                        record.truncated
                      }
                      onClick={() =>
                        open({
                          title: `Discard incomplete restore ${record.volume}?`,
                          request: {
                            kind: "discardVolumeTransfer",
                            transaction: record.transaction,
                          },
                          destructive: true,
                          description: `Permanently removes only the pending destination ${record.volume} after rechecking its recorded ownership, inactive transfer lease and absence of any container attachments. The source package and source volume are retained. Keep other clients from replacing or attaching this destination while recovering.`,
                        })
                      }
                    >
                      Discard pending destination…
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </section>
      <VolumeClones />
    </>
  );
}
