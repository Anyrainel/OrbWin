import { useEffect, useRef, type ReactNode } from "react";

// Modal confirmation with focus return. Escape and Cancel never confirm.
export function ConfirmDialog({
  title,
  children,
  confirmLabel,
  destructive,
  confirmDisabled,
  onConfirm,
  onCancel,
}: {
  title: string;
  children: ReactNode;
  confirmLabel: string;
  destructive?: boolean;
  confirmDisabled?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const trigger = document.activeElement as HTMLElement | null;
    if (dialog.current && !dialog.current.open) dialog.current.showModal();
    // Destructive confirmations start on Cancel so Enter cannot confirm them.
    if (destructive) cancel.current?.focus();
    return () => trigger?.focus?.();
  }, [destructive]);
  return (
    <dialog
      ref={dialog}
      className="operation-dialog confirm-dialog"
      aria-labelledby="confirm-title"
      onCancel={(event) => {
        event.preventDefault();
        onCancel();
      }}
    >
      <h2 id="confirm-title">{title}</h2>
      <div className="confirm-body">{children}</div>
      <div className="action-row">
        <button
          className={`button ${destructive ? "danger" : "primary"}`}
          disabled={confirmDisabled}
          onClick={onConfirm}
        >
          {confirmLabel}
        </button>
        <button ref={cancel} className="button" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </dialog>
  );
}
