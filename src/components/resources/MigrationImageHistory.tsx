import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import {
  parseImageExportHistory,
  type ImageExportHistory,
} from "../../model/migration-image-history";
import { containDialogTab } from "../shell/dialog-focus";

export function MigrationImageHistory() {
  const [history, setHistory] = useState<ImageExportHistory | null>(null);
  const [visible, setVisible] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const dialog = useRef<HTMLDialogElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const dismiss = useRef<HTMLButtonElement>(null);
  const pending = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      pending.current?.abort();
      pending.current = null;
    },
    [],
  );
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
  function cancel() {
    pending.current?.abort();
    pending.current = null;
    setBusy(false);
  }
  function close() {
    cancel();
    setVisible(false);
  }
  async function read() {
    if (pending.current) return;
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setError("");
    setHistory(null);
    let output = "",
      diagnostic = "",
      overflow = false;
    try {
      const result = await backend.run(
        crypto.randomUUID(),
        { kind: "migrationImageHistory" },
        (chunk, stream) => {
          if (stream === "stderr") {
            diagnostic = (diagnostic + chunk).slice(-8192);
            return;
          }
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
        throw new Error("Image history exceeds its response budget.");
      if (abort.signal.aborted || pending.current !== abort) return;
      if (!result.success) throw new Error(diagnostic || result.message);
      setHistory(parseImageExportHistory(output));
    } catch (error) {
      if (pending.current === abort) setError(String(error));
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  return (
    <section aria-label="Source image export history">
      <button
        className="button"
        ref={trigger}
        onClick={() => {
          setVisible(true);
          void read();
        }}
      >
        Read image transfer history…
      </button>
      {visible && (
        <dialog
          ref={dialog}
          className="operation-dialog source-export-dialog"
          aria-labelledby="image-export-history-title"
          onCancel={(event) => {
            event.preventDefault();
            close();
          }}
          onKeyDown={containDialogTab}
        >
          <h2 id="image-export-history-title">Image transfer history</h2>
          <div className="action-row">
            <button
              className="button"
              disabled={busy}
              onClick={() => void read()}
            >
              {busy ? "Reading image receipts…" : "Refresh receipts"}
            </button>
            {busy && (
              <button
                className="button"
                onClick={() => {
                  cancel();
                  setError(
                    "History read cancelled. No image or tag was changed.",
                  );
                }}
              >
                Cancel read
              </button>
            )}
          </div>
          <p role="status">
            {error ||
              (history === null
                ? "Reads saved source-image and import receipts, including when engines are offline."
                : history.records.length
                  ? `${history.records.length} saved source-image export receipts.`
                  : "No saved source-image exports. Earlier archives without receipts do not establish source provenance.")}
          </p>
          {history && (
            <>
              <p>
                These are historical acknowledgements. They do not verify that
                an archive, image or tag still exists. Linked imports match the
                target engine, exact archive checksum and config IDs. An
                unfinished import phase does not mean the image transfer
                completed. Refresh after export or import.
              </p>
              {history.unavailableImportReceipts > 0 && (
                <p role="status">
                  {history.unavailableImportReceipts} unreadable, changing or
                  torn import receipts could not establish links. A job may
                  still be writing.
                </p>
              )}
              {history.records.length > 0 && (
                <div
                  className="source-export-history"
                  tabIndex={0}
                  role="region"
                  aria-label="Scrollable image transfer receipts"
                >
                  <table>
                    <caption>
                      Source image exports and linked import receipts
                    </caption>
                    <thead>
                      <tr>
                        <th>Source / image</th>
                        <th>Archive</th>
                        <th>Export acknowledgement</th>
                        <th>Linked import receipts</th>
                      </tr>
                    </thead>
                    <tbody>
                      {history.records.map((row) => (
                        <tr key={row.transaction}>
                          <td>
                            {row.uncertain ? (
                              "Unverified receipt"
                            ) : (
                              <>
                                {row.sourceKind}: {row.reference}
                                <br />
                                <small>
                                  {row.image}
                                  <br />
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
                            ) : row.exportAcknowledged ? (
                              <>
                                Export acknowledged
                                <br />
                                <small>Archive SHA-256: {row.sha256}</small>
                              </>
                            ) : (
                              "No export acknowledgement. Archive publication is uncertain; inspect the selected destination before retrying."
                            )}
                          </td>
                          <td>
                            {row.imports?.length ? (
                              <ul>
                                {row.imports.map((linked) => (
                                  <li key={linked.transaction}>
                                    {linked.phase}: {linked.verifiedConfigs}/
                                    {row.configIds?.length} configs verified
                                    <br />
                                    <code>{linked.transaction}</code>
                                  </li>
                                ))}
                              </ul>
                            ) : row.exportAcknowledged ? (
                              "No matching import receipt observed."
                            ) : (
                              "Verified archive provenance unavailable."
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
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
