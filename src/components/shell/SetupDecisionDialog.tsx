import { ConfirmDialog } from "./ConfirmDialog";
import { SetupLogAction } from "./SetupLogAction";

export function SetupDecisionDialog({
  busy,
  error,
  onContinue,
  onLeave,
}: {
  busy: boolean;
  error: string;
  onContinue: () => void;
  onLeave: () => void;
}) {
  return (
    <ConfirmDialog
      title="Continue the original setup?"
      confirmLabel={busy ? "Sending decision…" : "Continue setup"}
      confirmDisabled={busy || !!error}
      focusCancel
      cancelLabel="Leave setup"
      onConfirm={onContinue}
      onCancel={onLeave}
    >
      <p>
        The previous engine configuration is missing. Continue the original
        setup, or leave it paused and inspect diagnostics. Existing files and
        logs will be kept.
      </p>
      <p>
        This continues the same setup operation. It does not reset existing
        settings.
      </p>
      {error && <p role="alert">{error}</p>}
      <SetupLogAction />
    </ConfirmDialog>
  );
}
