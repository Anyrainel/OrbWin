import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import {
  parseSourceVolumeHistory,
  type SourceVolumeReceipt,
} from "../../model/migration-history";
import { useOperation } from "../Operations";
import { containDialogTab } from "../shell/dialog-focus";

export function MigrationVolumeHistory() {
  const open = useOperation();
  const [records, setRecords] = useState<SourceVolumeReceipt[] | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const pending = useRef<AbortController | null>(null);
  const [visible, setVisible] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const dismiss = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (visible && dialog.current && !dialog.current.open) {
      const element = dialog.current;
      element.showModal();
      dismiss.current?.focus();
      return () => {
        element.close();
        trigger.current?.focus();
      };
    }
  }, [visible]);
  function cancelRead() {
    pending.current?.abort();
    pending.current = null;
    setBusy(false);
  }
  function close() {
    cancelRead();
    setVisible(false);
  }
  useEffect(
    () => () => {
      pending.current?.abort();
      pending.current = null;
    },
    [],
  );
  async function read() {
    if (pending.current) return;
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setError("");
    let output = "";
    let overflow = false;
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        { kind: "migrationVolumeHistory" },
        (chunk) => {
          if (output.length + chunk.length > 2 * 1024 * 1024) {
            overflow = true;
            abort.abort();
            return;
          }
          output += chunk;
        },
        abort.signal,
      );
      if (overflow)
        throw new Error("Source history exceeds its response budget.");
      if (abort.signal.aborted || pending.current !== abort) return;
      if (!result.success) throw new Error(result.message);
      setRecords(parseSourceVolumeHistory(output));
    } catch (error) {
      if (pending.current === abort) {
        setError(String(error));
        setRecords(null);
      }
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  return (
    <section aria-label="Source volume export history">
      <button
        className="button"
        ref={trigger}
        onClick={() => {
          setVisible(true);
          void read();
        }}
      >
        Read source export history…
      </button>
      {visible && (
        <dialog
          ref={dialog}
          className="operation-dialog source-export-dialog"
          aria-labelledby="source-export-history-title"
          onCancel={(event) => {
            event.preventDefault();
            close();
          }}
          onKeyDown={(event) => {
            containDialogTab(event);
          }}
        >
          <h2 id="source-export-history-title">Source volume export history</h2>
          <div className="action-row">
            <button
              className="button"
              disabled={busy}
              onClick={() => void read()}
            >
              {busy ? "Reading source receipts…" : "Refresh receipts"}
            </button>
            {busy && (
              <button
                className="button"
                onClick={() => {
                  cancelRead();
                  setError("History read cancelled. No transfer was changed.");
                }}
              >
                Cancel read
              </button>
            )}
          </div>
          <p role="status">
            {error ||
              (records === null
                ? "Read saved source-volume receipts, including when the engines are offline."
                : records.length
                  ? `${records.length} saved source-volume receipts. Refresh after export or recovery.`
                  : "No saved source-volume exports.")}
          </p>
          {records && records.length > 0 && (
            <>
              <p>
                Receipt phases are historical. An export acknowledgement does
                not verify that its package still exists, and tool cleanup does
                not mean data was imported. Jobs may still be active; recovery
                rechecks ownership and refuses active jobs. Linked restores
                match the recorded source and full manifest; they do not verify
                current destination data or ownership. Legacy, unreadable or
                torn restore receipts cannot establish a link and are omitted
                from this column.
              </p>
              <div
                className="source-export-history"
                tabIndex={0}
                role="region"
                aria-label="Scrollable source export receipts"
              >
                <table>
                  <caption>Source volume export receipts</caption>
                  <thead>
                    <tr>
                      <th>Source / volume</th>
                      <th>Destination</th>
                      <th>Recorded state</th>
                      <th>Linked restore receipts</th>
                      <th>Recovery</th>
                    </tr>
                  </thead>
                  <tbody>
                    {records.map((row) => (
                      <tr key={row.transaction}>
                        <td>
                          {row.uncertain ? (
                            "Unverified receipt"
                          ) : (
                            <>
                              {row.sourceKind}: {row.volume}
                              <br />
                              <small>
                                {row.sourceDaemon}
                                <br />
                                {row.createdAt}
                              </small>
                            </>
                          )}
                          <br />
                          <code>{row.transaction}</code>
                        </td>
                        <td>
                          {row.destination ||
                            "Retain the original receipt for review."}
                        </td>
                        <td>
                          {row.uncertain ? (
                            "Incomplete, changed or unavailable. A job may still be writing."
                          ) : (
                            <>
                              {row.phase}
                              <br />
                              {row.exportAcknowledged
                                ? "Export acknowledged"
                                : "No export acknowledgement"}
                            </>
                          )}
                        </td>
                        <td>
                          {row.restores?.length ? (
                            <ul>
                              {row.restores.map((restore) => (
                                <li key={restore.transaction}>
                                  {restore.volume}: {restore.phase}
                                  <br />
                                  <code>{restore.transaction}</code>
                                </li>
                              ))}
                            </ul>
                          ) : row.manifestSha256 ? (
                            "No matching restore acknowledgement observed."
                          ) : (
                            "Manifest provenance unavailable; no source linkage inferred."
                          )}
                        </td>
                        <td>
                          <button
                            className="button"
                            disabled={row.uncertain || row.phase === "cleaned"}
                            onClick={() => {
                              close();
                              open({
                                title: `Recover source tools for ${row.volume}?`,
                                confirmLabel: "Recover tools",
                                destructive: true,
                                returnFocus: trigger.current ?? undefined,
                                request: {
                                  kind: "recoverDesktopMigrationVolume",
                                  transaction: row.transaction,
                                },
                                description: `Uses receipt ${row.transaction} to verify ${row.sourceKind}, daemon ${row.sourceDaemon}, and remove only this inactive job's temporary helper and scratch image. It never removes the source volume or host package, resumes export or imports data. Active jobs and changed/adopted resources refuse. Inspect the destination separately before retrying an export. Refresh this history after recovery.`,
                              });
                            }}
                          >
                            Recover tools…
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
          <div className="action-row">
            <button className="button" ref={dismiss} onClick={close}>
              Close history
            </button>
          </div>
        </dialog>
      )}
    </section>
  );
}
