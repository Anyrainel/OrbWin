import { useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { ConfirmDialog } from "../shell/ConfirmDialog";

// The draft stays local until an explicit save confirmation. Backend version,
// writer and transaction checks remain authoritative.
export type VolumeFileEdit = Readonly<{
  volume: string;
  path: string;
  text: string;
  version: string;
}>;

export function VolumeFileEditor({
  volume,
  path,
  text,
  version,
  available,
  refreshing,
  onRefresh,
  onFocusFallback,
  onClose,
}: VolumeFileEdit & {
  available: boolean;
  refreshing: boolean;
  onRefresh: () => void;
  onFocusFallback: () => void;
  onClose: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const editor = useRef<HTMLTextAreaElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const abort = useRef<AbortController | null>(null);
  const [draft, setDraft] = useState(text);
  const [pending, setPending] = useState(false);
  const [review, setReview] = useState<"save" | "discard" | "restore" | null>(
    null,
  );
  const [message, setMessage] = useState("");
  const [saved, setSaved] = useState<{
    sha256: string;
    backup: string;
    receipt: string;
  } | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    const trigger = document.activeElement as HTMLElement | null;
    const node = dialog.current;
    mounted.current = true;
    if (node && !node.open) node.showModal();
    return () => {
      mounted.current = false;
      abort.current?.abort();
      if (node?.open) node.close();
      if (trigger?.isConnected) trigger.focus();
      else onFocusFallback();
    };
  }, []);
  function refreshConnection() {
    onRefresh();
    // Reconnection removes its button; keep focus inside the retained editor.
    if (editor.current && !editor.current.disabled) editor.current.focus();
    else heading.current?.focus();
  }
  function close() {
    if (pending) {
      abort.current?.abort();
      setMessage(
        "Cancellation requested. An accepted save may have committed; inspect the file and its retained versions before retrying.",
      );
      return;
    }
    if (!saved && draft !== text) {
      setReview("discard");
      return;
    }
    onClose();
  }
  async function save(restore = false) {
    if (pending) return;
    if (!available) {
      setReview(null);
      setMessage(
        "Draft retained. Reconnect and refresh before saving or restoring.",
      );
      return;
    }
    setReview(null);
    setPending(true);
    setMessage("");
    const controller = new AbortController();
    abort.current = controller;
    let output = "",
      diagnostic = "";
    try {
      const outcome = await backend.run(
        crypto.randomUUID(),
        {
          kind: "saveVolumeFile",
          volume,
          path,
          expected: restore ? saved!.sha256 : version,
          text: restore ? text : draft,
        },
        (chunk, stream) => {
          if (stream === "stderr")
            diagnostic = (diagnostic + chunk).slice(-32768);
          else output = (output + chunk).slice(-32768);
        },
        controller.signal,
      );
      if (!mounted.current) return;
      let receipt: {
        committed?: boolean;
        conflict?: boolean;
        sha256?: string;
        backup?: string;
        receipt?: string;
      } | null = null;
      try {
        receipt = JSON.parse(output);
      } catch {
        /* Error before dispatch can legitimately have no receipt. */
      }
      if (!outcome.success)
        throw new Error(
          `${diagnostic || outcome.message}${receipt?.committed ? ` The transaction reports a commit; inspect ${receipt.backup} and ${receipt.receipt}.` : ""}`,
        );
      if (
        !receipt?.committed ||
        receipt.conflict ||
        !receipt.sha256 ||
        !receipt.backup ||
        !receipt.receipt
      )
        throw new Error(
          "Save did not return a complete commit receipt. Inspect the file before retrying.",
        );
      setSaved({
        sha256: receipt.sha256,
        backup: receipt.backup,
        receipt: receipt.receipt,
      });
      setMessage(
        `${restore ? "Previous text restored" : "Text saved"}. Displaced version retained at ${receipt.backup}. Transaction receipt: ${receipt.receipt}.`,
      );
      if (restore) setDraft(text);
    } catch (error) {
      if (mounted.current) setMessage(String(error));
    } finally {
      if (mounted.current) setPending(false);
      abort.current = null;
    }
  }
  return (
    <dialog
      ref={dialog}
      className="operation-dialog"
      aria-labelledby="file-editor-title"
      onCancel={(event) => {
        event.preventDefault();
        close();
      }}
    >
      <h2 id="file-editor-title" ref={heading} tabIndex={-1}>
        Edit volume text file
      </h2>
      <p>
        {volume}: {path}
      </p>
      <p>
        Stop every container writing this volume before saving. The current file
        version is checked, and the displaced file is retained as a backup.
        Extended attributes, hard-linked files, symlinks and unsupported
        filesystems refuse. Other Docker clients must remain quiesced; no
        engine-wide lock is claimed.
      </p>
      {!available && (
        <p role="status">
          The engine or selected volume is unavailable. Your draft is retained.
          Reconnect and refresh before starting another save or restore. An
          already accepted save may still have committed.
        </p>
      )}
      <label className="form-field">
        UTF-8 file text
        <textarea
          ref={editor}
          className="file-editor"
          autoFocus
          value={draft}
          disabled={pending || !!saved}
          onChange={(event) => setDraft(event.target.value)}
          aria-label="Edited volume file text"
        />
      </label>
      <p role="status" aria-live="polite">
        {pending ? "Saving with a retained previous version…" : message}
      </p>
      <div className="action-row">
        {!available && (
          <button
            className="button"
            disabled={refreshing}
            onClick={refreshConnection}
          >
            {refreshing ? "Refreshing connection…" : "Refresh connection"}
          </button>
        )}
        {!saved && (
          <button
            className="button primary"
            disabled={
              pending ||
              !available ||
              new TextEncoder().encode(draft).length > 256 * 1024 ||
              draft === text
            }
            onClick={() => setReview("save")}
          >
            Review save…
          </button>
        )}
        {saved && saved.sha256 !== version && (
          <button
            className="button"
            disabled={pending || !available}
            onClick={() => setReview("restore")}
          >
            Restore previous text…
          </button>
        )}
        <button className="button" onClick={close}>
          {pending ? "Cancel save" : "Close"}
        </button>
      </div>
      {review && (
        <ConfirmDialog
          title={
            review === "discard"
              ? "Discard unsaved draft?"
              : review === "restore"
                ? "Restore previous text?"
                : "Save this volume file?"
          }
          confirmLabel={
            review === "discard"
              ? "Discard draft"
              : review === "restore"
                ? "Restore previous text"
                : "Save with backup"
          }
          destructive
          confirmDisabled={review !== "discard" && (pending || !available)}
          onCancel={() => setReview(null)}
          onConfirm={() => {
            if (review === "discard") onClose();
            else void save(review === "restore");
          }}
        >
          <p>
            {review === "discard"
              ? "The draft has not been saved. Source files are not changed by discarding it."
              : `Writes ${path} in ${volume}. An atomic exchange retains the displaced version and a transaction receipt. Cancellation after acceptance may still leave a committed save; inspect the retained versions before retrying.`}
          </p>
        </ConfirmDialog>
      )}
    </dialog>
  );
}
