import { useState } from "react";
import type {
  LocalServiceState,
  WslTrustReview,
  localServices,
} from "../../backend";
import { ConfirmDialog } from "../shell/ConfirmDialog";

export function WslTrustControls({
  state,
  busy,
  run,
}: {
  state: LocalServiceState;
  busy: boolean;
  run: (request: Parameters<typeof localServices>[0]) => Promise<void>;
}) {
  const [selected, setSelected] = useState("");
  const [review, setReview] = useState<{
    operation: "wsl-trust-install" | "wsl-trust-remove";
    revision: number;
    rootSha256: string;
    wslReview: WslTrustReview;
    name: string;
  } | null>(null);
  const root = state.authority?.rootSha256 ?? state.settings.rootSha256;
  if (!root) return null;
  const registrations = state.wslDistributions ?? [];
  const registration = registrations.find((entry) => entry.id === selected);
  const status =
    state.wslTrust?.rootSha256 === root &&
    state.wslTrust.distributionId === selected
      ? state.wslTrust
      : undefined;
  const guest = status?.guest;
  const observed = Boolean(status?.observed && guest?.observed);
  const safeSequences = Boolean(
    status &&
      guest &&
      [status.ownerSequence, guest.sequence, state.revision].every(
        (value) => Number.isSafeInteger(value) && value >= 0,
      ),
  );
  const owned = Boolean(
    status &&
      [
        "install-intent",
        "installed",
        "remove-intent",
        "registration-retiring",
      ].includes(status.ownerState),
  );
  const present = Boolean(guest?.filePresent || guest?.bundlePresent);
  const rotationPending =
    Boolean(state.rotation) || state.phase === "rotation-pending";
  const installDisabled =
    busy ||
    !registration ||
    !observed ||
    !safeSequences ||
    rotationPending ||
    state.settings.rootSha256 !== root ||
    status?.ownerState === "remove-intent" ||
    status?.ownerState === "registration-retiring" ||
    status?.ownerState === "registration-retired" ||
    (!owned && present) ||
    (status?.ownerState === "installed" &&
      guest?.filePresent &&
      guest?.bundlePresent);
  function confirm(operation: "wsl-trust-install" | "wsl-trust-remove") {
    if (!status || !guest || !observed || !safeSequences || !root) return;
    setReview({
      operation,
      revision: state.revision,
      rootSha256: root,
      name: registration?.name ?? selected,
      wslReview: {
        distributionId: selected,
        machineId: guest.machineId,
        guestSequence: guest.sequence,
        ownerSequence: status.ownerSequence,
      },
    });
  }
  return (
    <section aria-labelledby="wsl-trust-title">
      <h3 id="wsl-trust-title">Selected WSL certificate trust</h3>
      <p>
        Automatic trust currently supports Ubuntu 24.04 and its standard system
        certificate bundle. Other distributions and custom application stores
        can use the exported public root with their own setup.
      </p>
      <p>
        Finding distributions reads your Windows registrations. Reading trust
        status may start the selected distribution and runs the narrow helper as
        Linux root.
      </p>
      <div className="action-row">
        <button
          className="button"
          disabled={busy}
          onClick={() => void run({ operation: "wsl-trust-distributions" })}
        >
          Find WSL distributions
        </button>
        <label>
          WSL distribution{" "}
          <select
            value={selected}
            disabled={busy}
            onChange={(event) => setSelected(event.target.value)}
          >
            <option value="">Select a distribution</option>
            {registrations.map((entry) => (
              <option value={entry.id} key={entry.id}>
                {entry.name}
              </option>
            ))}
            {selected && !registration && (
              <option value={selected}>
                Previously selected (not in the current list)
              </option>
            )}
          </select>
        </label>
      </div>
      {state.wslDistributionsRead && registrations.length === 0 && (
        <p>
          No ordinary WSL 2 distributions were found. Container-engine
          distributions are excluded.
        </p>
      )}
      {selected && (
        <>
          <p>
            Registration: <code>{selected}</code>
          </p>
          <p>
            {!status
              ? "Trust has not been checked for this distribution and root."
              : !observed
                ? "Selected-distribution trust could not be confirmed. Read its status before applying another change."
                : `At the last check, the certificate file was ${guest!.filePresent ? "present" : "absent"} and the main certificate bundle ${guest!.bundlePresent ? "contained" : "did not contain"} this root. Windows ownership: ${status.ownerState}.`}
          </p>
          {observed && guest?.updaterWarning && (
            <p className="notice" role="status">
              The distro updater reported a warning or an unconfirmed result.
              Main-bundle presence does not verify its hooks or
              application-specific stores.
            </p>
          )}
          {observed && !safeSequences && (
            <p className="notice" role="alert">
              This receipt cannot be represented exactly by this desktop build.
              Trust changes are unavailable; preserve its records for review.
            </p>
          )}
          <div className="action-row">
            <button
              className="button"
              disabled={busy || !registration}
              onClick={() =>
                void run({
                  operation: "wsl-trust-status",
                  revision: state.revision,
                  rootSha256: root,
                  wslReview: { distributionId: selected },
                })
              }
            >
              Read selected WSL trust
            </button>
            <button
              className="button"
              disabled={installDisabled}
              onClick={() => confirm("wsl-trust-install")}
            >
              {status?.ownerState === "install-intent"
                ? "Finish selected WSL trust setup…"
                : "Install in selected WSL distribution…"}
            </button>
            <button
              className="button"
              disabled={
                busy || !registration || !owned || !observed || !safeSequences
              }
              onClick={() => confirm("wsl-trust-remove")}
            >
              Remove this WSL root…
            </button>
          </div>
        </>
      )}
      {review && (
        <ConfirmDialog
          title={
            review.operation === "wsl-trust-install"
              ? `Trust the local authority in ${review.name}?`
              : `Remove the local authority from ${review.name}?`
          }
          confirmLabel={
            review.operation === "wsl-trust-install"
              ? "Install reviewed root"
              : "Remove reviewed root"
          }
          confirmDisabled={busy}
          destructive={review.operation === "wsl-trust-remove"}
          onCancel={() => setReview(null)}
          onConfirm={() => {
            setReview(null);
            void run({
              operation: review.operation,
              revision: review.revision,
              rootSha256: review.rootSha256,
              wslReview: review.wslReview,
            });
          }}
        >
          <p>
            Distribution: <strong>{review.name}</strong>. Registration:{" "}
            <code>{review.wslReview.distributionId}</code>
          </p>
          <p>
            Reviewed machine: <code>{review.wslReview.machineId}</code>
          </p>
          <p>
            Reviewed SHA-256 fingerprint: <code>{review.rootSha256}</code>
          </p>
          <p>
            This action may start the selected distribution and runs as Linux
            root inside it. Windows administrator access is not required.
          </p>
          {review.operation === "wsl-trust-install" ? (
            <p>
              Applications using this distribution's system roots may trust
              local certificates from this authority. OrbWin adds only the
              reviewed public root and runs the distribution's standard
              certificate updater, including its installed hooks. Java, NSS and
              other custom stores require separate verification.
            </p>
          ) : (
            <p>
              Applications that depend on this root may reject local HTTPS after
              removal. OrbWin removes only its exact owned certificate and runs
              the distribution's standard updater. Preexisting or externally
              re-added trust is preserved.
            </p>
          )}
          <p>
            This action retains projects and workloads. It does not install
            roots in other distributions or container images.
          </p>
        </ConfirmDialog>
      )}
    </section>
  );
}
