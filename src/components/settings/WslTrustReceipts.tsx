import { useState } from "react";
import type {
  LocalServiceState,
  WslTrustReceipt,
  localServices,
} from "../../backend";
import { ConfirmDialog } from "../shell/ConfirmDialog";

const receiptLabels: Record<string, string> = {
  "install-intent": "Setup needs confirmation",
  installed: "Installation recorded",
  "remove-intent": "Removal needs confirmation",
  removed: "Removal recorded",
  "registration-retiring": "Registration retirement needs confirmation",
  "registration-retired": "Missing registration retired",
};

export function WslTrustReceipts({
  state,
  busy,
  run,
}: {
  state: LocalServiceState;
  busy: boolean;
  run: (request: Parameters<typeof localServices>[0]) => Promise<void>;
}) {
  const [review, setReview] = useState<{
    receipt: WslTrustReceipt;
    revision: number;
  } | null>(null);
  const receipts = state.wslTrustReceipts ?? [];
  const safe = (receipt: WslTrustReceipt) =>
    [receipt.ownerSequence, receipt.guestSequence, state.revision].every(
      (value) => Number.isSafeInteger(value) && value >= 0,
    );
  return (
    <section aria-labelledby="wsl-trust-receipts-title" aria-busy={busy}>
      <h3 id="wsl-trust-receipts-title">Retained WSL trust receipts</h3>
      <p>
        Read OrbWin's public trust history and Windows registration status
        without starting a distribution. Retiring a missing registration
        preserves its receipt and can unblock authority rotation; it does not
        remove or verify certificates in stored disks or exports.
      </p>
      <button
        className="button"
        disabled={busy}
        onClick={() => void run({ operation: "wsl-trust-receipts" })}
      >
        Read retained WSL trust receipts
      </button>
      {state.wslTrustReceiptsError && (
        <p className="notice" role="alert">
          {state.wslTrustReceiptsError}
        </p>
      )}
      {state.wslTrustReceiptsRead &&
        !state.wslTrustReceiptsError &&
        receipts.length === 0 && (
          <p>No retained WSL trust receipts were found.</p>
        )}
      {receipts.map((receipt) => {
        const name = state.wslDistributions?.find(
          (entry) => entry.id === receipt.distributionId,
        )?.name;
        const pending = [
          "install-intent",
          "installed",
          "remove-intent",
          "registration-retiring",
        ].includes(receipt.state);
        return (
          <div
            key={`${receipt.distributionId}/${receipt.rootSha256}`}
            className="settings-trust-receipt"
          >
            <p>
              <strong>{name ?? "Recorded WSL registration"}</strong>:{" "}
              <code>{receipt.distributionId}</code>
            </p>
            <p>
              Root SHA-256: <code>{receipt.rootSha256}</code>
            </p>
            <p>
              Receipt: {receiptLabels[receipt.state] ?? "Needs review"}.{" "}
              {receipt.registrationState === "absent"
                ? "This exact registration was absent at the last registry read."
                : receipt.registrationState === "present"
                  ? "Windows still has this exact registration."
                  : "Registration presence could not be confirmed."}
            </p>
            {receipt.registrationError && (
              <p className="notice" role="status">
                {receipt.registrationError}
              </p>
            )}
            {receipt.state === "registration-retired" && (
              <p>
                Registration retirement was recorded
                {receipt.retiredAt
                  ? ` on ${new Date(receipt.retiredAt).toLocaleString()}`
                  : ""}
                . This does not prove certificate removal. A returned or
                unreadable registration blocks rotation and requires manual
                review; OrbWin does not adopt restored trust.
              </p>
            )}
            {pending && !safe(receipt) && (
              <p className="notice" role="alert">
                This receipt cannot be represented exactly by this desktop
                build. Retirement is unavailable; preserve its records for
                review.
              </p>
            )}
            {pending && (
              <button
                className="button"
                disabled={
                  busy ||
                  receipt.registrationState !== "absent" ||
                  !safe(receipt)
                }
                onClick={() =>
                  setReview({
                    receipt: { ...receipt },
                    revision: state.revision,
                  })
                }
              >
                {receipt.state === "registration-retiring"
                  ? "Finish registration retirement…"
                  : "Retire missing registration…"}
              </button>
            )}
          </div>
        );
      })}
      {review && (
        <ConfirmDialog
          title="Retire this missing WSL registration's trust receipt?"
          confirmLabel="Retire reviewed registration"
          destructive
          confirmDisabled={busy}
          onCancel={() => setReview(null)}
          onConfirm={() => {
            setReview(null);
            void run({
              operation: "retire-wsl-trust-receipt",
              revision: review.revision,
              rootSha256: review.receipt.rootSha256,
              wslReview: {
                distributionId: review.receipt.distributionId,
                machineId: review.receipt.machineId,
                guestSequence: review.receipt.guestSequence,
                ownerSequence: review.receipt.ownerSequence,
              },
            });
          }}
        >
          <p>
            Registration: <code>{review.receipt.distributionId}</code>
          </p>
          <p>
            Reviewed machine: <code>{review.receipt.machineId}</code>
          </p>
          <p>
            Reviewed root SHA-256: <code>{review.receipt.rootSha256}</code>
          </p>
          <p>
            OrbWin will recheck this exact Windows registration and retain a
            retirement record. It will not start WSL, delete a distribution or
            disk, or change any certificate store.
          </p>
          <p>
            Stored disks, exports and restored copies may still trust this root.
            Review their certificates before using them. Retirement releases the
            authority-rotation guard for this missing registration; it is not a
            certificate-removal result.
          </p>
        </ConfirmDialog>
      )}
    </section>
  );
}
