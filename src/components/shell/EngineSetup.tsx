import { useEffect, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import {
  Box,
  CheckCircle2,
  Circle,
  CircleAlert,
  CircleDashed,
  LoaderCircle,
} from "lucide-react";
import * as backend from "../../backend";
import {
  bundleState,
  canStartEngine,
  engineState,
  wslState,
  type StepState,
} from "./engine-state";
import { useElapsed, useEngineLifecycle } from "./EngineLifecycle";

function StepIcon({ state }: { state: StepState | "working" }) {
  if (state === "working")
    return <LoaderCircle size={18} className="spin" aria-hidden="true" />;
  if (state === "done") return <CheckCircle2 size={18} aria-hidden="true" />;
  if (state === "blocked") return <CircleAlert size={18} aria-hidden="true" />;
  if (state === "unknown") return <CircleDashed size={18} aria-hidden="true" />;
  return <Circle size={18} aria-hidden="true" />;
}
const stateText: Record<StepState | "working", string> = {
  done: "Complete",
  blocked: "Needs attention",
  action: "Ready to start",
  pending: "Waiting for the previous step",
  unknown: "Not checked yet",
  working: "In progress",
};

// First-run / unavailable-engine screen. Every step is derived from existing
// read-only checks (WSL version read and backend_status); only the explicit
// Start button issues a lifecycle request.
export function EngineSetup({
  status,
  checking,
  onRefresh,
  onDiagnostics,
}: {
  status: backend.BackendStatus | null;
  checking: boolean;
  onRefresh: () => void;
  onDiagnostics: () => void;
}) {
  const lifecycle = useEngineLifecycle();
  const [wsl, setWsl] = useState<backend.Diagnostic[] | null>(null);
  const [wslError, setWslError] = useState("");
  const [wslBusy, setWslBusy] = useState(false);
  const elapsed = useElapsed(lifecycle.startedAt, lifecycle.action !== null);
  async function readWsl() {
    if (!isTauri()) {
      setWslError(
        "WSL is checked by the Windows desktop application. This browser preview cannot read the host.",
      );
      return;
    }
    setWslBusy(true);
    setWslError("");
    try {
      setWsl(await backend.diagnostics());
    } catch (error) {
      setWslError(String(error));
    } finally {
      setWslBusy(false);
    }
  }
  useEffect(() => {
    void readWsl();
  }, []);
  const bundle = bundleState(status);
  const engine = engineState(status);
  const starting =
    lifecycle.action === "start" || lifecycle.action === "restart";
  const wslStep: StepState | "working" = wslBusy
    ? "working"
    : wslError
      ? "unknown"
      : wslState(wsl);
  const wslVersion = wsl?.find((item) => item.name === "WSL version");
  const bundleStep: StepState | "working" = checking
    ? "working"
    : bundle === "ready"
      ? "done"
      : bundle === "missing"
        ? "blocked"
        : "unknown";
  const engineStep: StepState | "working" =
    starting || (checking && !lifecycle.action)
      ? "working"
      : engine === "running"
        ? "done"
        : engine === "offline"
          ? "action"
          : engine === "foreign" || engine === "preview"
            ? "blocked"
            : engine === "error"
              ? "blocked"
              : "pending";
  return (
    <section className="engine-setup" aria-labelledby="setup-title">
      <div className="setup-card">
        <header className="setup-header">
          <Box size={30} aria-hidden="true" />
          <div>
            <h1 id="setup-title">
              {engine === "offline"
                ? "OrbWin engine is stopped"
                : "Set up OrbWin"}
            </h1>
            <p>
              OrbWin runs Docker and an optional Kubernetes cluster in its own
              WSL 2 guest. It never switches your global Docker or Kubernetes
              defaults, and it does not stop other engines.
            </p>
          </div>
        </header>
        <ol className="setup-steps">
          <li className={`setup-step ${wslStep}`}>
            <StepIcon state={wslStep} />
            <div>
              <h2>Windows Subsystem for Linux</h2>
              <small>{stateText[wslStep]}</small>
              <p>
                {wslStep === "done"
                  ? (wslVersion?.output.split(/\r?\n/).find(Boolean) ??
                    "WSL answered the version read.")
                  : wslStep === "blocked"
                    ? "WSL did not answer `wsl --version`. Install or update WSL from Microsoft (for example `wsl --install` in an administrator terminal), restart Windows if prompted, then check again. OrbWin does not install WSL for you."
                    : wslError || "Reading the installed WSL version…"}
              </p>
              {!wslBusy && wslStep !== "done" && (
                <button className="button" onClick={() => void readWsl()}>
                  Check again
                </button>
              )}
            </div>
          </li>
          <li className={`setup-step ${bundleStep}`}>
            <StepIcon state={bundleStep} />
            <div>
              <h2>OrbWin backend</h2>
              <small>{stateText[bundleStep]}</small>
              <p>
                {bundle === "ready"
                  ? "The owned rdd backend bundle and its recorded engine identity were verified."
                  : bundle === "missing"
                    ? "No verified backend bundle is recorded for this development build (BL-003). Build it with `npm run backend:bundle`, then record the owned engine identity with `npm run backend:guest-proof`. An installer that does this for you is not available yet."
                    : status?.diagnostic && engine !== "preview"
                      ? "The backend could not be classified from the last status read. Open diagnostics for the full message."
                      : "Checked together with the engine connection."}
              </p>
            </div>
          </li>
          <li className={`setup-step ${engineStep}`}>
            <StepIcon state={engineStep} />
            <div>
              <h2>Docker engine</h2>
              <small>
                {lifecycle.action
                  ? `${lifecycle.stage} ${elapsed}s`
                  : stateText[engineStep]}
              </small>
              <p>
                {engine === "running"
                  ? `Connected${status?.version ? ` to Docker ${status.version}` : ""}.`
                  : engine === "offline"
                    ? "The backend is installed but its engine is not answering. Starting can take a few minutes on a cold start."
                    : engine === "foreign"
                      ? "Another daemon answers the OrbWin endpoint. OrbWin will not use or start an engine over it (BL-001). Stop the other process or inspect diagnostics."
                      : engine === "preview"
                        ? status?.message
                        : engine === "error"
                          ? `The engine check failed with an unrecognised error. Starting may not resolve it. ${status?.message ?? ""}`
                          : "Available after the backend is verified."}
              </p>
              {lifecycle.result &&
                !lifecycle.action &&
                !lifecycle.result.success && (
                  <p className="setup-failure" role="status">
                    {lifecycle.result.message}
                  </p>
                )}
              <div className="action-row">
                {lifecycle.action ? (
                  <button className="button" onClick={lifecycle.cancel}>
                    Cancel
                  </button>
                ) : (
                  <button
                    className="button primary"
                    disabled={!canStartEngine(status) || checking}
                    title={
                      canStartEngine(status)
                        ? undefined
                        : "Start is available once the backend is verified and its engine is offline."
                    }
                    onClick={lifecycle.start}
                  >
                    Start engine
                  </button>
                )}
                <button
                  className="button"
                  disabled={checking}
                  onClick={onRefresh}
                >
                  {checking ? "Checking…" : "Check again"}
                </button>
                <button className="button" onClick={onDiagnostics}>
                  View diagnostics
                </button>
              </div>
              {lifecycle.action && (
                <p className="setup-note">
                  Cancel stops waiting and closes the client; the backend may
                  still finish starting. Status is re-read afterwards.
                </p>
              )}
              {lifecycle.output && (
                <details className="setup-output">
                  <summary>Backend output</summary>
                  <pre className="command-output">{lifecycle.output}</pre>
                </details>
              )}
            </div>
          </li>
        </ol>
        <p className="setup-note">
          Creating the OrbWin guest for the first time (download and
          provisioning) is not yet automated; this development build starts the
          guest recorded by the backend proof. See phase-1 coverage, onboarding.
        </p>
      </div>
    </section>
  );
}
