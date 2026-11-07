import { useState } from "react";
import type { LocalServiceState, localServices } from "../../backend";
import { ConfirmDialog } from "../shell/ConfirmDialog";

export function WindowsTrustControls({
  state,
  busy,
  run,
}: {
  state: LocalServiceState;
  busy: boolean;
  run: (request: Parameters<typeof localServices>[0]) => Promise<void>;
}) {
  const [review, setReview] = useState<{
    operation: "trust-install" | "trust-remove";
    revision: number;
    rootSha256: string;
  } | null>(null);
  const root = state.authority?.rootSha256 ?? state.settings.rootSha256;
  if (!root) return null;
  const status =
    state.windowsTrust?.rootSha256 === root ? state.windowsTrust : undefined;
  return (
    <section aria-labelledby="windows-trust-title">
      <h3 id="windows-trust-title">Windows certificate trust</h3>
      <p>
        {!status
          ? "Windows certificate-store presence has not been checked."
          : !status.observed
            ? "Windows certificate-store presence could not be confirmed."
            : status.present
              ? `At the last check, the public root was present in your Windows Root store (${status.managed ? status.state : "preexisting; not managed by OrbWin"}).`
              : `At the last check, the public root was absent from your Windows Root store (${status.state}).`}
      </p>
      <p>
        This covers the current user's physical Root store. Browser policies,
        custom certificate stores, WSL and container clients need separate
        verification.
      </p>
      <div className="action-row">
        <button
          className="button"
          disabled={busy}
          onClick={() =>
            void run({
              operation: "trust-status",
              revision: state.revision,
              rootSha256: root,
            })
          }
        >
          Read Windows trust status
        </button>
        <button
          className="button"
          disabled={
            busy ||
            Boolean(state.rotation) ||
            state.phase === "rotation-pending" ||
            state.settings.rootSha256 !== root ||
            (status?.observed &&
              status.present &&
              status.state !== "install-intent")
          }
          onClick={() =>
            setReview({
              operation: "trust-install",
              revision: state.revision,
              rootSha256: root,
            })
          }
        >
          {status?.state === "install-intent"
            ? "Finish Windows trust setup…"
            : "Install for current Windows user…"}
        </button>
        <button
          className="button"
          disabled={busy || !status?.managed}
          onClick={() =>
            setReview({
              operation: "trust-remove",
              revision: state.revision,
              rootSha256: root,
            })
          }
        >
          Remove this Windows root…
        </button>
      </div>
      {review && (
        <ConfirmDialog
          title={
            review.operation === "trust-install"
              ? "Trust the OrbWin local authority for this Windows user?"
              : "Remove the OrbWin root from this Windows user's store?"
          }
          confirmLabel={
            review.operation === "trust-install"
              ? "Install reviewed root"
              : "Remove reviewed root"
          }
          confirmDisabled={busy}
          destructive={review.operation === "trust-remove"}
          onCancel={() => setReview(null)}
          onConfirm={() => {
            setReview(null);
            void run(review);
          }}
        >
          <p>
            Reviewed SHA-256 fingerprint: <code>{review.rootSha256}</code>
          </p>
          {review.operation === "trust-install" ? (
            <>
              <p>
                Windows applications that use this user's root certificates may
                trust local service certificates from this authority. The
                protected signer is restricted to OrbWin's local DNS namespaces.
                Applications with their own trust policies may behave
                differently.
              </p>
              <p>
                OrbWin will add only this public root to the current user's
                physical Root store. Windows may request consent or deny the
                change. OrbWin will not change machine-wide, WSL, container or
                browser-specific stores.
              </p>
            </>
          ) : (
            <>
              <p>
                Clients that rely on this Windows root may reject local HTTPS
                after removal. Existing signing state and running workloads are
                retained.
              </p>
              <p>
                OrbWin removes only the exact root covered by its ownership
                receipt. A preexisting or externally re-added root is preserved.
              </p>
            </>
          )}
        </ConfirmDialog>
      )}
    </section>
  );
}
