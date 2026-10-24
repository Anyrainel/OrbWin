import { useEffect, useState } from "react";
import * as backend from "../../backend";
import { retryBusyRead } from "../../model/read-retry";
import { useOperation } from "../Operations";
import {
  applyState,
  kubernetesState,
  type AppSettings,
} from "./app-settings-model";
import {
  parseKubernetesVersions,
  versionChangeUnavailable,
  type KubernetesVersion,
} from "./kubernetes-versions";

export function KubernetesVersions({
  settings,
  verified,
}: {
  settings: AppSettings | null;
  verified: boolean;
}) {
  const open = useOperation();
  const [catalog, setCatalog] = useState<KubernetesVersion[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const abort = new AbortController();
    setPending(true);
    setError("");
    let raw = "";
    retryBusyRead(() => {
      raw = "";
      return backend.run(
        crypto.randomUUID(),
        { kind: "readClusterVersions" },
        (chunk) => {
          raw = (raw + chunk).slice(-1048576);
        },
        abort.signal,
      );
    }, abort.signal)
      .then((outcome) => {
        if (abort.signal.aborted) return;
        if (!outcome.success)
          throw new Error(`${outcome.message} ${raw.trim().slice(-1000)}`);
        setCatalog(parseKubernetesVersions(raw));
      })
      .catch((reason) => {
        if (!abort.signal.aborted) {
          setCatalog([]);
          setError(String(reason));
        }
      })
      .finally(() => {
        if (!abort.signal.aborted) setPending(false);
      });
    return () => abort.abort();
  }, [revision]);
  const chosen = catalog.find((entry) => entry.version === selected);
  const unavailable =
    !verified || !settings || settings.kubernetesEnabled === undefined
      ? "Read the current backend configuration before selecting a version."
      : settings.kubernetesVersion &&
          (kubernetesState(settings) !== "ready" ||
            applyState(settings).kind !== "settled")
        ? "Enable the current cluster and wait for it to be ready and settled before changing its version."
        : !chosen
          ? "Select a version from the backend catalog."
          : versionChangeUnavailable(
              settings.kubernetesVersion,
              chosen.version,
            );
  return (
    <section aria-label="Kubernetes version selection">
      <label htmlFor="kubernetes-version">Available version</label>{" "}
      <select
        id="kubernetes-version"
        value={selected}
        disabled={pending || !catalog.length}
        onChange={(event) => setSelected(event.target.value)}
      >
        <option value="">Select a version…</option>
        {catalog.map((entry) => (
          <option
            key={entry.version}
            value={entry.version}
            disabled={
              !!versionChangeUnavailable(
                settings?.kubernetesVersion,
                entry.version,
              )
            }
          >
            {entry.release}
            {entry.channels.length ? ` (${entry.channels.join(", ")})` : ""}
          </option>
        ))}
      </select>{" "}
      <button
        className="button"
        disabled={pending}
        onClick={() => setRevision((value) => value + 1)}
      >
        {pending ? "Reading versions…" : "Reload versions"}
      </button>{" "}
      <button
        className="button"
        disabled={pending || !!error || !!unavailable}
        title={unavailable}
        onClick={() => {
          if (!chosen || unavailable || !settings) return;
          open({
            title: `Apply Kubernetes ${chosen.release}?`,
            request: {
              kind: "configureCluster",
              enabled: settings.kubernetesEnabled!,
              version: chosen.version,
            },
            description:
              "Changes the retained OrbWin cluster version through the backend. Applying restarts the owned guest and interrupts Docker and Kubernetes workloads. Persistent data is retained. Downloads can take several minutes. Cancelling stops this client; an accepted change can continue applying. Refresh and inspect before retrying.",
          });
        }}
      >
        Apply version…
      </button>
      {error && <p role="alert">Versions unavailable: {error}</p>}
      {unavailable && <p className="settings-hint">{unavailable}</p>}
      <p className="settings-hint">
        Versions come from the pinned backend catalog. Existing clusters accept
        forward patch or one-minor upgrades; downgrades and skipped minors are
        unavailable.
      </p>
    </section>
  );
}
