import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { useOperation } from "../Operations";

type Clone = {
  transaction: string;
  sourceVolume: string;
  volume: string;
  phase: string;
  active: boolean;
  truncated: boolean;
  snapshotPresent: boolean;
  cleanupAvailable: boolean;
  snapshotPath?: string;
  message?: string;
  bytes: number;
};
export function VolumeClones() {
  const open = useOperation();
  const [records, setRecords] = useState<Clone[]>([]),
    [visible, setVisible] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const pending = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      pending.current?.abort();
      pending.current = null;
    },
    [],
  );
  async function refresh() {
    pending.current?.abort();
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setVisible(true);
    setError("");
    let output = "",
      diagnostic = "";
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        { kind: "volumeClones" },
        (chunk, stream) => {
          if (stream === "stderr")
            diagnostic = (diagnostic + chunk).slice(-8192);
          else {
            output += chunk;
            if (output.length > 4 << 20) {
              abort.abort();
              if (pending.current === abort)
                setError(
                  "Clone recovery response exceeded its bounded size; records were retained.",
                );
            }
          }
        },
        abort.signal,
      );
      if (abort.signal.aborted || pending.current !== abort) return;
      if (!result.success) throw new Error(diagnostic || result.message);
      const value = JSON.parse(output) as { records: Clone[] };
      if (
        !Array.isArray(value.records) ||
        value.records.some(
          (record) =>
            !record ||
            typeof record.transaction !== "string" ||
            typeof record.sourceVolume !== "string" ||
            typeof record.volume !== "string" ||
            typeof record.phase !== "string" ||
            typeof record.active !== "boolean" ||
            typeof record.truncated !== "boolean" ||
            typeof record.snapshotPresent !== "boolean" ||
            typeof record.cleanupAvailable !== "boolean",
        )
      )
        throw new Error("Invalid clone recovery response.");
      setRecords(value.records);
    } catch (cause) {
      if (!abort.signal.aborted && pending.current === abort)
        setError(String(cause));
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  return (
    <section
      className="volume-transfer-recovery"
      aria-label="Clone snapshot recovery"
      aria-busy={busy}
    >
      <button className="button" disabled={busy} onClick={() => void refresh()}>
        {visible ? "Refresh clone snapshots" : "Review clone snapshots"}
      </button>
      {busy && (
        <span role="status">
          {" "}
          Reading clone records…{" "}
          <button
            className="button compact"
            onClick={() => {
              pending.current?.abort();
              setBusy(false);
              setError(
                "Recovery read cancelled. Staged data and volumes were not changed.",
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
            Saved snapshots may contain application secrets. They are fully
            validated before a fresh restore. If a clone stopped during copying,
            review its destination and restore receipts before creating another
            copy. Staging cleanup never removes a source or destination volume.
            Refresh after recovery actions.
          </p>
          {records.length === 0 ? (
            <p role="status">No retained clone staging.</p>
          ) : (
            <ul>
              {records.map((record) => (
                <li key={record.transaction}>
                  <strong>
                    {record.sourceVolume} → {record.volume}
                  </strong>{" "}
                  —{" "}
                  {record.active
                    ? "Clone client active"
                    : record.phase === "complete"
                      ? "Clone verified; staging retained"
                      : record.phase === "restoring"
                        ? "Destination completion unconfirmed"
                        : "Snapshot capture interrupted"}
                  {record.message && <p>{record.message}</p>}
                  {record.snapshotPresent && record.snapshotPath && (
                    <button
                      className="button"
                      disabled={!record.cleanupAvailable}
                      onClick={() =>
                        open({
                          title: `Restore saved snapshot of ${record.sourceVolume}`,
                          request: {
                            kind: "restoreVolume",
                            source: record.snapshotPath!,
                            name: "",
                          },
                          description: `Validates the entire saved package before creating a fresh isolated volume. The original source and ${record.volume} are not changed. Do not assume the earlier clone failed completely; review its restore receipt and destination first. Allow a full guest copy plus metadata.`,
                          fields: [{ key: "name", label: "New volume name" }],
                        })
                      }
                    >
                      Restore saved snapshot…
                    </button>
                  )}
                  <button
                    className="button"
                    disabled={!record.cleanupAvailable}
                    onClick={() =>
                      open({
                        title: `Discard local staging for ${record.volume}?`,
                        request: {
                          kind: "discardCloneStaging",
                          transaction: record.transaction,
                        },
                        destructive: true,
                        description:
                          "Permanently removes only the recorded inactive job's known local snapshot/partial files. Source and destination volumes are not changed. A pending destination may still need separate restore recovery. Unknown files, links and changed ownership refuse cleanup.",
                      })
                    }
                  >
                    Discard local staging…
                  </button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </section>
  );
}
