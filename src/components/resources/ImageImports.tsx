import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import {
  imageImportChoices,
  imageImportReady,
  parseImageImportReview,
  type ImageImportReview,
} from "../../model/image-imports";
import { useOperation } from "../Operations";
import { containDialogTab } from "../shell/dialog-focus";

type History = {
  transaction: string;
  createdAt: string;
  phase: string;
  active: boolean;
  truncated: boolean;
  imageCount: number;
  appliedTags: number;
  requestedTags: number;
  skippedTags: number;
  pendingReference: string;
};
export function ImageImports({ connected }: { connected: boolean }) {
  const open = useOperation();
  const [visible, setVisible] = useState(false),
    [source, setSource] = useState("");
  const [review, setReview] = useState<ImageImportReview | null>(null),
    [choices, setChoices] = useState<backend.ImageImportChoice[]>([]);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [history, setHistory] = useState<History[] | null>(null),
    [historyBusy, setHistoryBusy] = useState(false),
    [historyError, setHistoryError] = useState("");
  const [transaction, setTransaction] = useState("");
  const dialog = useRef<HTMLDialogElement>(null),
    pending = useRef<AbortController | null>(null),
    historyPending = useRef<AbortController | null>(null);
  const pickerEpoch = useRef(0);
  const importTrigger = useRef<HTMLButtonElement>(null);
  useEffect(
    () => () => {
      pending.current?.abort();
      historyPending.current?.abort();
      pickerEpoch.current++;
    },
    [],
  );
  useEffect(() => {
    if (visible && dialog.current && !dialog.current.open) {
      const trigger = document.activeElement as HTMLElement | null;
      dialog.current.showModal();
      return () => trigger?.focus?.();
    }
  }, [visible]);
  function close() {
    pending.current?.abort();
    pending.current = null;
    pickerEpoch.current++;
    setBusy(false);
    setReview(null);
    setChoices([]);
    setVisible(false);
  }
  function changeSource(path: string) {
    setSource(path);
    setReview(null);
    setChoices([]);
    setError("");
  }
  async function read(request: backend.Operation, abort: AbortController) {
    let output = "",
      diagnostic = "";
    const result = await backend.run(
      crypto.randomUUID(),
      request,
      (text, stream) => {
        if (stream === "stderr") diagnostic = (diagnostic + text).slice(-8192);
        else {
          output += text;
          if (output.length > 2 * 1024 * 1024) abort.abort();
        }
      },
      abort.signal,
    );
    if (abort.signal.aborted)
      throw new Error(
        "Read cancelled; no image or tag was changed by this read.",
      );
    if (!result.success) throw new Error(diagnostic || result.message);
    return output;
  }
  async function inspect() {
    pending.current?.abort();
    const abort = new AbortController();
    pending.current = abort;
    setBusy(true);
    setError("");
    setReview(null);
    setChoices([]);
    try {
      const value = parseImageImportReview(
        await read({ kind: "reviewImageImport", source }, abort),
      );
      if (pending.current !== abort || abort.signal.aborted) return;
      setReview(value);
      setChoices(imageImportChoices(value));
    } catch (error) {
      if (pending.current === abort) setError(String(error));
    } finally {
      if (pending.current === abort) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  async function readHistory() {
    historyPending.current?.abort();
    const abort = new AbortController();
    historyPending.current = abort;
    setHistoryBusy(true);
    setHistoryError("");
    try {
      const value = JSON.parse(
        await read({ kind: "imageImportHistory" }, abort),
      ) as { records: History[] };
      if (
        !Array.isArray(value.records) ||
        value.records.length > 512 ||
        value.records.some(
          (record) =>
            !record ||
            typeof record.transaction !== "string" ||
            !/^[a-f0-9]{32}$/.test(record.transaction) ||
            typeof record.createdAt !== "string" ||
            typeof record.phase !== "string" ||
            typeof record.active !== "boolean" ||
            typeof record.truncated !== "boolean" ||
            ![
              record.imageCount,
              record.appliedTags,
              record.requestedTags,
              record.skippedTags,
            ].every((count) => Number.isSafeInteger(count) && count >= 0),
        )
      )
        throw new Error("Invalid image import history.");
      if (historyPending.current === abort && !abort.signal.aborted)
        setHistory(value.records);
    } catch (error) {
      if (historyPending.current === abort) setHistoryError(String(error));
    } finally {
      if (historyPending.current === abort) {
        historyPending.current = null;
        setHistoryBusy(false);
      }
    }
  }
  function detail(token: string) {
    open({
      title: "Read image import receipt",
      confirmLabel: "Read receipt",
      request: { kind: "imageImportRecord", transaction: token },
      description:
        "Reads historical intent, verified image IDs, previous-content references and acknowledged/uncertain tag writes. No image or tag is changed. Later clients may have changed the references; inspect current image inventory before acting. This never resumes or rolls back a job.",
    });
  }
  function continueImport() {
    if (!review || busy || !imageImportReady(choices)) return;
    const replacements = choices.filter(
      (choice) => choice.apply && choice.expectedId,
    ).length;
    const request: backend.Operation = {
      kind: "importImages",
      source: review.source,
      sha256: review.sha256,
      tags: structuredClone(choices),
    };
    close();
    open({
      title: "Import reviewed image archive?",
      confirmLabel: "Import reviewed images",
      returnFocus: importTrigger.current ?? undefined,
      request,
      destructive: replacements > 0,
      description: `Archive SHA-256 ${review.sha256}. Loads and verifies ${review.images.length} image configs under isolated recovery references, then applies ${choices.filter((choice) => choice.apply).length} reviewed original tags (${replacements} replacements); ${choices.filter((choice) => !choice.apply).length} original tags are skipped. Previous replacement content receives recovery references before any original tag is written. Stop other clients from writing selected tags during this transaction: Docker has no tag compare-and-swap. The original archive is retained. Cancellation can leave loaded images and partial tag changes; inspect import history before retrying. OCI index digests, attestations/signatures and emulated execution are not preserved or implied. Recovery references remain in image inventory and can be removed explicitly by tag.`,
    });
  }
  return (
    <section
      className="image-import-controls"
      aria-label="Image archive import"
    >
      <div className="action-row">
        <button
          ref={importTrigger}
          className="button"
          disabled={!connected}
          onClick={() => {
            setError("");
            setReview(null);
            setChoices([]);
            setVisible(true);
          }}
        >
          Import archive…
        </button>
        <button
          className="button"
          disabled={historyBusy}
          onClick={() => void readHistory()}
        >
          {historyBusy ? "Reading import history…" : "Import history"}
        </button>
        {historyBusy && (
          <button
            className="button compact"
            onClick={() => historyPending.current?.abort()}
          >
            Cancel history read
          </button>
        )}
      </div>
      {historyError && <p role="alert">{historyError}</p>}
      {history !== null && !historyBusy && !historyError && (
        <div className="image-import-history">
          <p>
            Historical receipts; reads never replay writes or remove images.
            Refresh after an operation.
          </p>
          {history.length === 0 ? (
            <p role="status">No recorded image imports.</p>
          ) : (
            <ul>
              {history.map((record) => (
                <li key={record.transaction}>
                  <strong>
                    {record.active
                      ? "Client active or inactivity unconfirmed"
                      : record.truncated
                        ? "Interrupted receipt"
                        : record.phase === "complete"
                          ? "Import completed at recorded time"
                          : "Import completion unconfirmed"}
                  </strong>
                  <div>
                    <time dateTime={record.createdAt}>
                      {new Date(record.createdAt).toLocaleString()}
                    </time>{" "}
                    ·{" "}
                    <code title={record.transaction}>
                      {record.transaction.slice(0, 8)}
                    </code>
                  </div>
                  <span>
                    {" "}
                    {record.imageCount}{" "}
                    {record.imageCount === 1 ? "config" : "configs"} ·{" "}
                    {record.appliedTags}/{record.requestedTags} tag
                    acknowledgements · {record.skippedTags} skipped
                  </span>
                  <button
                    className="button compact"
                    onClick={() => detail(record.transaction)}
                  >
                    Read receipt…
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
      {(history !== null || historyError) && (
        <label>
          Exact recorded transaction{" "}
          <input
            aria-label="Image import transaction"
            value={transaction}
            onChange={(event) => setTransaction(event.target.value)}
            maxLength={32}
          />
          <button
            className="button compact"
            disabled={!/^[a-f0-9]{32}$/.test(transaction)}
            onClick={() => detail(transaction)}
          >
            Read receipt
          </button>
        </label>
      )}
      {visible && (
        <dialog
          ref={dialog}
          className="operation-dialog image-import-dialog"
          onKeyDown={containDialogTab}
          aria-labelledby="image-import-title"
          onCancel={(event) => {
            event.preventDefault();
            close();
          }}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void inspect();
            }}
          >
            <h2 id="image-import-title">Review image archive</h2>
            <p>
              Plain Docker .tar only. Inspection verifies configs/layers without
              extracting files on Windows or loading images. Image contents can
              contain application secrets. Import keeps isolated recovery
              references; it does not run imported programs.
            </p>
            <label>
              Archive on a local Windows drive
              <input
                autoFocus
                value={source}
                disabled={busy}
                onChange={(event) => changeSource(event.target.value)}
                maxLength={4096}
              />
            </label>
            <div className="action-row">
              <button
                type="button"
                className="button"
                disabled={busy}
                onClick={async () => {
                  const epoch = ++pickerEpoch.current;
                  try {
                    const path = await backend.choosePath(false);
                    if (path && epoch === pickerEpoch.current)
                      changeSource(path);
                  } catch (error) {
                    if (epoch === pickerEpoch.current) setError(String(error));
                  }
                }}
              >
                Browse…
              </button>
              <button
                className="button"
                disabled={busy || !source || !connected}
              >
                {busy ? "Inspecting…" : "Inspect archive"}
              </button>
              {busy && (
                <button
                  type="button"
                  className="button"
                  onClick={() => pending.current?.abort()}
                >
                  Cancel read
                </button>
              )}
              <button type="button" className="button" onClick={close}>
                Close
              </button>
            </div>
          </form>
          {error && <p role="alert">{error}</p>}
          {review && !busy && (
            <div className="image-import-review">
              <p>
                <strong>Verified archive</strong> ·{" "}
                {review.bytes.toLocaleString()} bytes · SHA-256{" "}
                <code>{review.sha256}</code>
              </p>
              <ul aria-label="Archived image configurations">
                {review.images.map((image) => (
                  <li key={image.id}>
                    <code>{image.id}</code> · {image.os}/{image.architecture}
                    {image.variant ? `/${image.variant}` : ""} ·{" "}
                    {image.layerCount}{" "}
                    {image.layerCount === 1 ? "layer" : "layers"}
                  </li>
                ))}
              </ul>
              <p>
                Choose which original tags to apply. Existing tags default to
                Skip. Several configs for one tag require an explicit
                platform/config choice. Untagged or skipped images remain
                available under new isolated recovery references. Coordinate
                external tag writers; this review does not reserve references or
                disk space.
              </p>
              {choices.length === 0 && (
                <p>
                  No original tags. All verified content will be imported under
                  isolated recovery references.
                </p>
              )}
              <div className="image-import-tags">
                {choices.map((choice, i) => (
                  <fieldset key={choice.reference}>
                    <legend>{choice.reference}</legend>
                    <p>
                      {choice.expectedId ? (
                        <>
                          Currently points to <code>{choice.expectedId}</code>
                        </>
                      ) : (
                        "Currently absent; rechecked before writing."
                      )}
                    </p>
                    <label>
                      Original tag action
                      <select
                        aria-label={`Import action for ${choice.reference}`}
                        value={choice.apply ? choice.configId : "skip"}
                        onChange={(event) =>
                          setChoices((current) =>
                            current.map((item, j) =>
                              j === i
                                ? {
                                    ...item,
                                    apply: event.target.value !== "skip",
                                    configId:
                                      event.target.value === "skip"
                                        ? ""
                                        : event.target.value,
                                    replace: false,
                                  }
                                : item,
                            ),
                          )
                        }
                      >
                        <option value="skip">Skip original tag</option>
                        {review.tags[i].configIds.map((config) => {
                          const image = review.images.find(
                            (image) => image.id === config,
                          )!;
                          return (
                            <option key={config} value={config}>
                              {choice.expectedId ? "Replace with" : "Apply"}{" "}
                              {image.os}/{image.architecture} · {config}
                            </option>
                          );
                        })}
                      </select>
                    </label>
                    {choice.apply && choice.expectedId && (
                      <label>
                        <input
                          type="checkbox"
                          checked={choice.replace}
                          onChange={(event) =>
                            setChoices((current) =>
                              current.map((item, j) =>
                                j === i
                                  ? { ...item, replace: event.target.checked }
                                  : item,
                              ),
                            )
                          }
                        />{" "}
                        I approve replacing this exact observed tag after
                        retaining its previous content.
                      </label>
                    )}
                  </fieldset>
                ))}
              </div>
              <button
                type="button"
                className="button"
                disabled={!connected || !imageImportReady(choices)}
                onClick={continueImport}
              >
                Continue with reviewed import…
              </button>
            </div>
          )}
        </dialog>
      )}
    </section>
  );
}
