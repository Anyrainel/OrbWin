import { useCallback, useEffect, useRef, useState } from "react";
import * as backend from "../../backend";
import { ConfirmDialog } from "../shell/ConfirmDialog";
import { LocalServiceChoice, sameServiceSelector } from "./LocalServiceChoice";
import { LocalServiceNetworks } from "./LocalServiceNetworks";
import { WindowsTrustControls } from "./WindowsTrustControls";
import { WslTrustControls } from "./WslTrustControls";
import { WslTrustReceipts } from "./WslTrustReceipts";
import { AuthorityRotation } from "./AuthorityRotation";
import { LanAccess } from "./LanAccess";
import {
  acceptLocalServiceSnapshot,
  newLocalServiceSnapshotOrder,
} from "../../model/local-service-snapshots";

export function LocalServices() {
  const [state, setState] = useState<backend.LocalServiceState | null>(null);
  const [error, setError] = useState("");
  const [readError, setReadError] = useState("");
  const [busy, setBusy] = useState(false);
  const [review, setReview] = useState<{
    revision: number;
    rootSha256?: string;
  } | null>(null);
  const [http, setHttp] = useState("80");
  const [https, setHttps] = useState("443");
  const [notice, setNotice] = useState("");
  const live = useRef(true);
  const savedPorts = useRef("");
  const reading = useRef(false);
  const applying = useRef(false);
  const snapshotOrder = useRef(newLocalServiceSnapshotOrder());
  const receive = useCallback(
    (response: { state?: backend.LocalServiceState; error?: string }) => {
      if (!live.current) return;
      if (response.state) {
        if (!acceptLocalServiceSnapshot(snapshotOrder.current, response.state))
          return;
        setState(response.state);
        const pair = `${response.state.settings.ports.http}/${response.state.settings.ports.https}`;
        if (savedPorts.current !== pair) {
          setHttp(String(response.state.settings.ports.http));
          setHttps(String(response.state.settings.ports.https));
          savedPorts.current = pair;
        }
      }
    },
    [],
  );
  const refresh = useCallback(async () => {
    if (reading.current || applying.current) return;
    reading.current = true;
    try {
      const response = await backend.localServices({ operation: "snapshot" });
      receive(response);
      if (live.current) setReadError(response.error ?? "");
    } catch (cause) {
      if (live.current) setReadError(String(cause));
    } finally {
      reading.current = false;
    }
  }, [receive]);
  useEffect(() => {
    live.current = true;
    void refresh();
    const timer = window.setInterval(() => void refresh(), 5000);
    return () => {
      live.current = false;
      window.clearInterval(timer);
    };
  }, [refresh]);
  async function run(request: Parameters<typeof backend.localServices>[0]) {
    if (applying.current) return;
    applying.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await backend.localServices(request);
      receive(response);
      if (live.current) setError(response.error ?? "");
    } catch (cause) {
      if (live.current) setError(String(cause));
    } finally {
      applying.current = false;
      if (live.current) setBusy(false);
    }
  }
  const dirtyPorts =
    state !== null &&
    (Number(http) !== state.settings.ports.http ||
      Number(https) !== state.settings.ports.https);
  const rotationPending =
    Boolean(state?.rotation) || state?.phase === "rotation-pending";
  function ports() {
    const pair = { http: Number(http), https: Number(https) };
    if (
      ![pair.http, pair.https].every(
        (port) => Number.isInteger(port) && port > 0 && port <= 65535,
      ) ||
      pair.http === pair.https ||
      (pair.http === 80 && pair.https !== 443)
    ) {
      setError(
        "Choose distinct HTTP and HTTPS ports from 1 to 65535. Use an alternate pair together.",
      );
      return null;
    }
    return pair;
  }
  async function save(enabled: boolean) {
    if (!state) return;
    const pair = ports();
    if (!pair) return;
    await run({
      operation: "configure",
      revision: state.revision,
      settings: { ...state.settings, enabled, ports: pair },
    });
  }
  async function copy(value: string, message: string) {
    try {
      await navigator.clipboard.writeText(value);
      setNotice(message);
    } catch (cause) {
      setError(`Could not copy: ${String(cause)}`);
    }
  }
  return (
    <section aria-labelledby="local-services-title">
      <h2 id="local-services-title">Local names and HTTPS</h2>
      <p>
        Use predictable service names without assigning a host port to each
        workload. The local service process continues when this window closes.
      </p>
      {!state && !readError && (
        <p role="status">Reading local service setup…</p>
      )}
      {readError && (
        <p className="notice" role="alert">
          {readError}{" "}
          <button
            className="link-button"
            disabled={busy}
            onClick={() => void refresh()}
          >
            Read again
          </button>
        </p>
      )}
      {error && (
        <p className="notice" role="alert">
          {error}
        </p>
      )}
      {state && (
        <>
          <dl className="properties">
            <dt>Status</dt>
            <dd>
              {state.phase}
              {state.error ? ` — ${state.error}` : ""}
            </dd>
            <dt>Windows and WSL names</dt>
            <dd>
              <code>service.orbwin.localhost</code>
            </dd>
          </dl>
          <div className="action-row">
            <label>
              HTTP port{" "}
              <input
                aria-label="Local service HTTP port"
                type="number"
                min="1"
                max="65535"
                value={http}
                disabled={
                  busy ||
                  rotationPending ||
                  state.settings.enabled ||
                  Boolean(state.settings.rootSha256)
                }
                onChange={(event) => setHttp(event.target.value)}
              />
            </label>
            <label>
              HTTPS port{" "}
              <input
                aria-label="Local service HTTPS port"
                type="number"
                min="1"
                max="65535"
                value={https}
                disabled={busy || state.settings.enabled || rotationPending}
                onChange={(event) => setHttps(event.target.value)}
              />
            </label>
            <button
              className="button"
              disabled={busy || state.settings.enabled || rotationPending}
              onClick={() => void save(false)}
            >
              Save ports
            </button>
          </div>
          <p className="settings-source">
            If another engine uses 80/443, choose an available integration pair
            such as 18080/18443. Changing the HTTP port after certificate
            creation requires certificate rotation.
          </p>
          {dirtyPorts && (
            <p role="status">
              Save the port choices before creating the authority or enabling
              local access.
            </p>
          )}
          <div className="action-row">
            <button
              className="button"
              disabled={busy}
              onClick={() => void run({ operation: "preview" })}
            >
              Discover services
            </button>
            {!state.settings.rootSha256 && (
              <button
                className="button"
                disabled={
                  busy ||
                  state.settings.enabled ||
                  dirtyPorts ||
                  rotationPending
                }
                onClick={() =>
                  setReview({
                    revision: state.revision,
                    rootSha256: state.authority?.rootSha256,
                  })
                }
              >
                {state.authority
                  ? "Use saved certificate authority…"
                  : "Create local certificate authority…"}
              </button>
            )}
            <button
              className={`button ${state.settings.enabled ? "" : "primary"}`}
              disabled={
                busy ||
                rotationPending ||
                (!state.settings.enabled &&
                  (!state.settings.rootSha256 || dirtyPorts))
              }
              onClick={() => void save(!state.settings.enabled)}
            >
              {state.settings.enabled
                ? "Disable local access"
                : "Enable local access"}
            </button>
            {state.phase === "unavailable" && (
              <button
                className="button"
                disabled={busy}
                onClick={() => void run({ operation: "retry" })}
              >
                Retry local access
              </button>
            )}
            {busy && <span role="status">Applying reviewed local setup…</span>}
          </div>
          {state.authority && (
            <>
              <dl className="properties">
                <dt>Certificate fingerprint</dt>
                <dd>
                  <code>{state.authority.rootSha256}</code>
                </dd>
                <dt>Root expires</dt>
                <dd>
                  {new Date(state.authority.rootNotAfter).toLocaleDateString()}
                </dd>
                <dt>Client trust</dt>
                <dd>
                  Creating the authority does not install it in Windows, WSL, or
                  container trust stores. Review Windows trust below; WSL and
                  container clients need separate setup.
                </dd>
              </dl>
              <button
                className="button"
                onClick={() =>
                  void copy(
                    atob(state.authority!.publicCertificatePem),
                    "Public certificate copied. It contains no private signing key.",
                  )
                }
              >
                Copy public certificate
              </button>
            </>
          )}
          <WindowsTrustControls state={state} busy={busy} run={run} />
          <WslTrustControls state={state} busy={busy} run={run} />
          <WslTrustReceipts state={state} busy={busy} run={run} />
          <AuthorityRotation
            key={`${state.settings.rootSha256 ?? "unbound"}/${state.settings.ports.http}/${state.settings.ports.https}`}
            state={state}
            busy={busy}
            run={run}
            copy={copy}
          />
          <LocalServiceNetworks
            selected={state.settings.networks ?? []}
            busy={busy || rotationPending}
            save={async (networks) => {
              await run({
                operation: "configure",
                revision: state.revision,
                settings: { ...state.settings, networks },
              });
            }}
          />
          <LanAccess
            key={state.instance}
            state={state}
            connected={!readError}
            busy={busy}
            receive={receive}
            refresh={refresh}
          />
          <h3>Discovered services</h3>
          {!state.candidates?.length && (
            <p className="notice">
              {state.inventoryReadAt
                ? "No supported running services were found in the last successful discovery."
                : "Use Discover services to read supported running workloads."}{" "}
              A workload with several listeners needs an explicit web port
              choice.
            </p>
          )}
          {state.candidates?.map((candidate) => {
            const observed =
              state.services?.filter(
                (service) => service.containerId === candidate.containerId,
              ) ?? [];
            return (
              <div className="settings-source" key={candidate.containerId}>
                <strong>{candidate.containerName}</strong>
                {(observed.length
                  ? observed
                  : [
                      {
                        names: candidate.names,
                        port: candidate.selectedPort,
                        state: candidate.state,
                        reason: candidate.reason,
                      },
                    ]
                ).map((service, index) => (
                  <div key={`${service.port ?? 0}-${index}`}>
                    <p>
                      {service.state}
                      {service.reason ? `: ${service.reason}` : ""}
                    </p>
                    <p>
                      {service.names?.join(", ") || "Choose a local name."}
                      {service.port ? ` → container port ${service.port}` : ""}
                    </p>
                    {service.state === "active" &&
                      service.names?.map((name) => {
                        const url = `https://${name}${state.settings.ports.https === 443 ? "" : `:${state.settings.ports.https}`}/`;
                        return (
                          <button
                            key={name}
                            className="link-button"
                            onClick={() =>
                              void copy(url, "Service HTTPS link copied.")
                            }
                          >
                            Copy {name} HTTPS link
                          </button>
                        );
                      })}
                  </div>
                ))}
                <LocalServiceChoice
                  selector={candidate.selector}
                  preference={state.settings.preferences?.find((choice) =>
                    sameServiceSelector(choice.selector, candidate.selector),
                  )}
                  busy={busy || rotationPending}
                  save={async (choice) => {
                    const preferences = (
                      state.settings.preferences ?? []
                    ).filter(
                      (existing) =>
                        !sameServiceSelector(
                          existing.selector,
                          candidate.selector,
                        ),
                    );
                    if (choice) preferences.push(choice);
                    await run({
                      operation: "configure",
                      revision: state.revision,
                      settings: { ...state.settings, preferences },
                    });
                  }}
                />
              </div>
            );
          })}
        </>
      )}
      {notice && <p role="status">{notice}</p>}
      {review && state && (
        <ConfirmDialog
          title={
            review.rootSha256
              ? "Use the saved certificate authority?"
              : "Create the local certificate authority?"
          }
          confirmLabel={
            review.rootSha256 ? "Use saved authority" : "Create authority"
          }
          confirmDisabled={busy}
          onCancel={() => setReview(null)}
          onConfirm={() => {
            setReview(null);
            void run(
              review.rootSha256
                ? {
                    operation: "adopt-authority",
                    revision: review.revision,
                    rootSha256: review.rootSha256,
                  }
                : {
                    operation: "create-authority",
                    revision: review.revision,
                  },
            );
          }}
        >
          {review.rootSha256 ? (
            <p>
              Resume setup with the saved authority whose SHA-256 fingerprint is{" "}
              <code>{review.rootSha256}</code>. OrbWin will verify its protected
              signing state and saved HTTP port before selecting it.
            </p>
          ) : (
            <p>
              OrbWin will store a Windows-protected signing key for{" "}
              <code>*.orbwin.localhost</code> and <code>*.orbwin.test</code>.
              The certificate profile restricts server certificates to these DNS
              namespaces and excludes IP addresses. Trust enforcement depends on
              the selected client.
            </p>
          )}
          <p>
            This selects the authority for local access. It does not add trusted
            certificates to any client or start workloads.
          </p>
        </ConfirmDialog>
      )}
    </section>
  );
}
