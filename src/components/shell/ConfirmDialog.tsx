import { useEffect, useId, useRef, type ReactNode } from "react";

// Modal confirmation with focus return. Escape and Cancel never confirm.
export function ConfirmDialog({
  title,
  children,
  confirmLabel,
  destructive,
  confirmDisabled,
  focusCancel,
  cancelLabel = "Cancel",
  onConfirm,
  onCancel,
}: {
  title: string;
  children: ReactNode;
  confirmLabel: string;
  destructive?: boolean;
  confirmDisabled?: boolean;
  focusCancel?: boolean;
  cancelLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const trigger = document.activeElement as HTMLElement | null;
    const node = dialog.current;
    if (node && !node.open) node.showModal();
    // Destructive confirmations start on Cancel so Enter cannot confirm them.
    if (destructive || focusCancel) cancel.current?.focus();
    return () => {
      if (node?.open) node.close();
      trigger?.focus?.();
    };
  }, [destructive, focusCancel]);
  return (
    <dialog
      ref={dialog}
      className="operation-dialog confirm-dialog"
      aria-labelledby={titleId}
      onKeyDown={(event) => {
        if (event.key !== "Tab" || !dialog.current) return;
        const controls = Array.from(
          dialog.current.querySelectorAll<HTMLElement>(
            "button, [href], input, select, textarea, [tabindex]",
          ),
        ).filter(
          (element) =>
            element.tabIndex >= 0 &&
            !element.matches(":disabled") &&
            element.getClientRects().length > 0 &&
            getComputedStyle(element).visibility !== "hidden",
        );
        const first = controls[0];
        const last = controls[controls.length - 1];
        if (!first || !last) {
          event.preventDefault();
          return;
        }
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }}
      onCancel={(event) => {
        event.preventDefault();
        onCancel();
      }}
    >
      <h2 id={titleId}>{title}</h2>
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
          {cancelLabel}
        </button>
      </div>
    </dialog>
  );
}
