import { useEffect, useRef, useState } from "react";
import {
  networkDiagnostics,
  type NetworkDiagnosticsSnapshot,
  type NetworkProbe,
  type NetworkProxyConfiguration,
} from "../backend";

function probeText(probe: NetworkProbe): string {
  if (probe.state === "verified-owned-listener") return "Verified";
  if (probe.state === "not-run") return "Not checked";
  const reason = {
    canceled: "canceled",
    timeout: "timed out",
    "not-found": "name not found",
    unavailable: "unavailable",
    "ownership-mismatch": "listener identity did not match",
  }[probe.errorClass ?? "unavailable"];
  return probe.state === "listen-unavailable"
    ? `Listener unavailable · ${reason}`
    : `Failed · ${reason}`;
}
function proxyText(value: NetworkProxyConfiguration): string {
  return {
    unknown: "Configuration unavailable",
    "direct-configured": "Direct connection configured",
    "automatic-configured": "Automatic proxy configured",
    "manual-configured": "Manual proxy configured",
    "mixed-configured": "Automatic and manual proxy configured",
  }[value.mode];
}
type State =
  | { kind: "idle" | "running" | "discarding" | "canceled" }
  | { kind: "error"; message: string }
  | { kind: "result"; snapshot: NetworkDiagnosticsSnapshot };

export function NetworkDiagnostics() {
  const [state, setState] = useState<State>({ kind: "idle" });
  const mounted = useRef(true);
  const inFlight = useRef(false);
  const discard = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      discard.current = true;
    };
  }, []);
  const busy = state.kind === "running" || state.kind === "discarding";
  async function run() {
    if (inFlight.current) return;
    inFlight.current = true;
    discard.current = false;
    setState({ kind: "running" });
    try {
      const snapshot = await networkDiagnostics();
      if (mounted.current && !discard.current)
        setState({ kind: "result", snapshot });
    } catch (error) {
      if (mounted.current && !discard.current)
        setState({ kind: "error", message: String(error) });
    } finally {
      inFlight.current = false;
      if (mounted.current && discard.current) setState({ kind: "canceled" });
    }
  }
  return (
    <section aria-labelledby="network-checks-title" aria-busy={busy}>
      <h2 id="network-checks-title">Local network checks</h2>
      <p>
        Check Windows loopback, local name resolution and proxy configuration.
        Uses temporary loopback listeners and fixed local names. These results
        stay here and are not added to diagnostic snapshots.
      </p>
      <div className="action-row">
        <button className="button" disabled={busy} onClick={() => void run()}>
          {busy ? "Checking network…" : "Run network checks"}
        </button>
        {state.kind === "running" && (
          <button
            className="button"
            onClick={() => {
              discard.current = true;
              setState({ kind: "discarding" });
            }}
          >
            Cancel
          </button>
        )}
      </div>
      <div role="status" aria-live="polite">
        {state.kind === "idle" && <p>No network check has been run.</p>}
        {state.kind === "running" && (
          <p>
            Checking local listeners and configuration. No network settings are
            changed.
          </p>
        )}
        {state.kind === "discarding" && (
          <p>
            Results canceled. The bounded local check is finishing; its result
            will be discarded.
          </p>
        )}
        {state.kind === "canceled" && (
          <p>Network check canceled. No result was retained.</p>
        )}
        {state.kind === "error" && <p className="notice">{state.message}</p>}
      </div>
      {state.kind === "result" && <NetworkResults snapshot={state.snapshot} />}
      <p className="notice">
        This checks OrbWin’s temporary local sockets, not a published workload
        or browser. Proxy configuration is not its effective behavior. VPN,
        split DNS, browser fallback and container access need separate checks.
      </p>
    </section>
  );
}

function NetworkResults({
  snapshot,
}: {
  snapshot: NetworkDiagnosticsSnapshot;
}) {
  const { loopback, proxy } = snapshot;
  return (
    <>
      <p role="status">
        Network observations ready
        {snapshot.complete ? "." : "; some observations are unavailable."} Read
        at{" "}
        <time dateTime={loopback.readAt}>
          {new Date(loopback.readAt).toLocaleString()}
        </time>
        .
      </p>
      <dl className="properties">
        <dt>IPv4 loopback</dt>
        <dd>{probeText(loopback.ipv4)}</dd>
        <dt>IPv6 loopback</dt>
        <dd>{probeText(loopback.ipv6)}</dd>
      </dl>
      {loopback.names.map((name) => (
        <section
          className="diagnostic-result"
          key={name.name}
          aria-label={`${name.name} checks`}
        >
          <h3>{name.name}</h3>
          <dl className="properties">
            <dt>Resolution</dt>
            <dd>
              {name.resolution === "observed"
                ? "Loopback answers observed"
                : `Unavailable · ${name.resolution.replaceAll("-", " ")}`}
            </dd>
            <dt>Address order</dt>
            <dd>
              {name.families.length
                ? name.families
                    .map((family) => (family === "ipv6" ? "IPv6" : "IPv4"))
                    .join(" → ")
                : "Unavailable"}
            </dd>
            <dt>Named connection</dt>
            <dd>{probeText(name.nativeDial)}</dd>
            <dt>IPv4 fallback with IPv6 refused</dt>
            <dd>{probeText(name.ipv4OnlyDial)}</dd>
          </dl>
        </section>
      ))}
      <h3>Proxy configuration</h3>
      <dl className="properties">
        <dt>Current Windows user</dt>
        <dd>{proxyText(proxy.currentUser)}</dd>
        <dt>WinHTTP registry default</dt>
        <dd>{proxyText(proxy.winHttpDefault)}</dd>
        <dt>Effective proxy</dt>
        <dd>Not evaluated. No proxy server or setup script was contacted.</dd>
      </dl>
      <p>
        Proxy addresses, setup script URLs, credentials and bypass entries are
        omitted.
      </p>
    </>
  );
}
