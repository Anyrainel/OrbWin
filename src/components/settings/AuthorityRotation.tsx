import { useState } from "react";
import type { LocalServiceState, localServices } from "../../backend";
import { ConfirmDialog } from "../shell/ConfirmDialog";

type RotationRequest =
  | {
      operation: "rotate-authority";
      revision: number;
      rootSha256: string;
      ports: { http: number; https: number };
    }
  | {
      operation: "resume-authority-rotation" | "cancel-authority-rotation";
      revision: number;
      rootSha256: string;
      newRootSha256: string;
    };

export function AuthorityRotation({
  state,
  busy,
  run,
  copy,
}: {
  state: LocalServiceState;
  busy: boolean;
  run: (request: Parameters<typeof localServices>[0]) => Promise<void>;
  copy: (value: string, message: string) => Promise<void>;
}) {
  const [http, setHttp] = useState(String(state.settings.ports.http));
  const [https, setHttps] = useState(String(state.settings.ports.https));
  const [error, setError] = useState("");
  const [review, setReview] = useState<RotationRequest | null>(null);
  const root = state.settings.rootSha256;
  if (!root) return null;
  const pending = state.rotation;
  const canCancel =
    pending &&
    state.authority?.rootSha256 === pending.oldRootSha256 &&
    state.revision === pending.oldRevision;
  const needsReview = state.phase === "rotation-pending";
  const trusted =
    state.windowsTrust?.rootSha256 === root &&
    state.windowsTrust.observed &&
    state.windowsTrust.present;
  function prepare() {
    const ports = { http: Number(http), https: Number(https) };
    if (
      ![ports.http, ports.https].every(
        (port) => Number.isInteger(port) && port >= 1 && port <= 65535,
      ) ||
      ports.http === ports.https ||
      (ports.http === 80 && ports.https !== 443)
    ) {
      setError(
        "Choose distinct valid HTTP and HTTPS ports. Use an alternate pair together.",
      );
      return;
    }
    setError("");
    setReview({
      operation: "rotate-authority",
      revision: state.revision,
      rootSha256: root!,
      ports,
    });
  }
  return (
    <section aria-labelledby="authority-rotation-title">
      <h3 id="authority-rotation-title">
        Rotate the local certificate authority
      </h3>
      {pending ? (
        <>
          <p role="status">
            A retained rotation needs an explicit decision. Local ingress stays
            off until it is resolved.
          </p>
          <dl className="properties">
            <dt>Old fingerprint</dt>
            <dd>
              <code>{pending.oldRootSha256}</code>
            </dd>
            <dt>Prepared fingerprint</dt>
            <dd>
              <code>{pending.newRootSha256}</code>
            </dd>
            <dt>Reviewed ports</dt>
            <dd>
              {pending.ports.http} / {pending.ports.https}
            </dd>
          </dl>
          <div className="action-row">
            <button
              className="button"
              disabled={busy}
              onClick={() =>
                setReview({
                  operation: "resume-authority-rotation",
                  revision: state.revision,
                  rootSha256: pending.oldRootSha256,
                  newRootSha256: pending.newRootSha256,
                })
              }
            >
              Review and resume…
            </button>
            <button
              className="button"
              disabled={busy || !canCancel}
              onClick={() =>
                setReview({
                  operation: "cancel-authority-rotation",
                  revision: state.revision,
                  rootSha256: pending.oldRootSha256,
                  newRootSha256: pending.newRootSha256,
                })
              }
            >
              Cancel preparation…
            </button>
            <button
              className="button"
              disabled={busy}
              onClick={() =>
                void copy(
                  atob(pending.oldCertificatePem),
                  "Old public certificate copied for reviewed client cleanup.",
                )
              }
            >
              Copy old public certificate
            </button>
          </div>
          <p>
            Cancellation works only before either active authority or settings
            has switched. A started switch must be resumed.
          </p>
        </>
      ) : needsReview ? (
        <p className="notice" role="alert">
          The retained rotation record needs repair before it can be resumed.
          Preserve its files and review the reported error.
        </p>
      ) : (
        <>
          <p>
            Create a new authority when the signer needs renewal or you need a
            different HTTP port. Disable local access and remove the old root
            from your Windows user store first. Remove old trust from other
            stores and selected WSL and container clients through their reviewed
            setup paths.
          </p>
          <div className="action-row">
            <label>
              New HTTP port{" "}
              <input
                aria-label="Rotated authority HTTP port"
                type="number"
                min="1"
                max="65535"
                value={http}
                disabled={busy || state.settings.enabled}
                onChange={(event) => setHttp(event.target.value)}
              />
            </label>
            <label>
              New HTTPS port{" "}
              <input
                aria-label="Rotated authority HTTPS port"
                type="number"
                min="1"
                max="65535"
                value={https}
                disabled={busy || state.settings.enabled}
                onChange={(event) => setHttps(event.target.value)}
              />
            </label>
            <button
              className="button"
              disabled={busy || state.settings.enabled || trusted}
              onClick={prepare}
            >
              Review authority rotation…
            </button>
          </div>
          {trusted && (
            <p role="status">
              Remove the old Windows root above before rotating.
            </p>
          )}
          {error && <p role="alert">{error}</p>}
        </>
      )}
      {review && (
        <ConfirmDialog
          title={
            review.operation === "rotate-authority"
              ? "Rotate the local certificate authority?"
              : review.operation === "resume-authority-rotation"
                ? "Resume this exact authority rotation?"
                : "Discard this prepared authority?"
          }
          confirmLabel={
            review.operation === "rotate-authority"
              ? "Create and select new authority"
              : review.operation === "resume-authority-rotation"
                ? "Resume rotation"
                : "Cancel preparation"
          }
          destructive={review.operation !== "resume-authority-rotation"}
          confirmDisabled={busy}
          onCancel={() => setReview(null)}
          onConfirm={() => {
            setReview(null);
            void run(review);
          }}
        >
          <p>
            Reviewed old fingerprint: <code>{review.rootSha256}</code>
          </p>
          {review.operation === "rotate-authority" ? (
            <p>
              The new authority will use HTTP {review.ports.http} and HTTPS{" "}
              {review.ports.https}. Workload preferences and client-network
              choices are retained. Local access stays disabled and the new
              public root requires a separate trust review.
            </p>
          ) : (
            <p>
              Prepared new fingerprint: <code>{review.newRootSha256}</code>. No
              replacement authority will be generated during recovery.
            </p>
          )}
          <p>
            Rotation does not remove or install certificates in any client.
            Clients that still trust an old root require explicit cleanup.
            Workloads and their data stay in place.
          </p>
          {review.operation === "cancel-authority-rotation" && (
            <p>
              Only a preparation that has not switched active files/settings can
              be discarded. Otherwise the operation refuses and retains recovery
              files.
            </p>
          )}
        </ConfirmDialog>
      )}
    </section>
  );
}
