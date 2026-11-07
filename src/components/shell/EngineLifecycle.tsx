import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import * as backend from "../../backend";
import { ConfirmDialog } from "./ConfirmDialog";
import { SetupDecisionDialog } from "./SetupDecisionDialog";
import { useToast } from "./Toasts";

// One shared lifecycle client for the sidebar switch, first-run screen,
// settings and command palette. Explicit setup, running-state and application
// replacement requests are forwarded to rdd; it remains the lifecycle owner. The native adapter
// rejects a second concurrent mutation, so this is not a second orchestrator.
// Cancelling kills the CLI client, not the daemon: rdd may still converge on
// the requested state, so every outcome is followed by a fresh status read.
export type LifecycleAction =
  | "setup"
  | "start"
  | "reconnect"
  | "stop"
  | "restart"
  | "prepareUpdate";
export type LifecycleResult = {
  action: LifecycleAction;
  success: boolean;
  cancelled: boolean;
  message: string;
};
type Lifecycle = {
  action: LifecycleAction | null;
  stage: string;
  startedAt: number;
  output: string;
  result: LifecycleResult | null;
  start: () => void;
  setup: () => void;
  requestStop: () => void;
  requestRestart: () => void;
  requestReconnect: () => void;
  requestPrepareUpdate: () => void;
  cancel: () => void;
};
const Context = createContext<Lifecycle | null>(null);
export function useEngineLifecycle() {
  const value = useContext(Context);
  if (!value) throw new Error("EngineLifecycleProvider is missing");
  return value;
}

const labels: Record<LifecycleAction, string> = {
  setup: "Set up",
  start: "Start",
  reconnect: "Reconnect",
  stop: "Stop",
  restart: "Restart",
  prepareUpdate: "Prepare update",
};

export function EngineLifecycleProvider({
  children,
  onSettled,
}: {
  children: ReactNode;
  onSettled: () => Promise<void>;
}) {
  const notify = useToast();
  const [action, setAction] = useState<LifecycleAction | null>(null);
  const [stage, setStage] = useState("");
  const [startedAt, setStartedAt] = useState(0);
  const [output, setOutput] = useState("");
  const [result, setResult] = useState<LifecycleResult | null>(null);
  const [confirm, setConfirm] = useState<
    "setup" | "stop" | "restart" | "reconnect" | "prepareUpdate" | null
  >(null);
  const [decision, setDecision] = useState<
    (backend.SetupDecision & { operation: string }) | null
  >(null);
  const [decisionBusy, setDecisionBusy] = useState(false);
  const [decisionError, setDecisionError] = useState("");
  const operation = useRef<string | null>(null);
  const decisionSending = useRef(false);
  const active = useRef(false);
  const controller = useRef<AbortController | null>(null);
  const settled = useRef(onSettled);
  settled.current = onSettled;

  async function execute(next: LifecycleAction) {
    if (active.current) return;
    active.current = true;
    const abort = new AbortController();
    controller.current = abort;
    setAction(next);
    setStartedAt(Date.now());
    setOutput("");
    setResult(null);
    const steps =
      next === "restart"
        ? [
            { running: false, stage: "Stopping engine (step 1 of 2)…" },
            { running: true, stage: "Starting engine (step 2 of 2)…" },
          ]
        : [
            {
              running: next === "start" || next === "reconnect",
              stage:
                next === "setup"
                  ? "Setting up OrbWin…"
                  : next === "start"
                    ? "Starting engine…"
                    : next === "reconnect"
                      ? "Reconnecting backend…"
                      : next === "prepareUpdate"
                        ? "Preparing for application update…"
                        : "Stopping engine…",
            },
          ];
    let outcome: LifecycleResult = {
      action: next,
      success: false,
      cancelled: false,
      message: "",
    };
    try {
      for (const step of steps) {
        if (abort.signal.aborted) break;
        setStage(step.stage);
        const id = crypto.randomUUID();
        operation.current = id;
        const reply = await backend.run(
          id,
          next === "setup"
            ? { kind: "setup" }
            : next === "prepareUpdate"
              ? { kind: "prepareUpdate" }
              : { kind: "engine", running: step.running },
          (chunk) => setOutput((previous) => (previous + chunk).slice(-65536)),
          abort.signal,
          (pending) => {
            if (operation.current !== id || abort.signal.aborted) return;
            decisionSending.current = false;
            setDecisionError("");
            setDecisionBusy(false);
            setDecision({ ...pending, operation: id });
            setStage("Waiting for your setup choice…");
          },
        );
        outcome = {
          action: next,
          success: reply.success,
          cancelled: abort.signal.aborted || reply.interrupted,
          message: reply.message,
        };
        if (!reply.success) break;
      }
      if (next === "restart" && abort.signal.aborted && outcome.success)
        outcome = {
          ...outcome,
          success: false,
          cancelled: true,
          message:
            "Restart cancelled after the stop step. The engine is stopped; start it when ready.",
        };
    } catch (error) {
      outcome = {
        action: next,
        success: false,
        cancelled: abort.signal.aborted,
        message: String(error),
      };
    } finally {
      operation.current = null;
      decisionSending.current = false;
      setDecision(null);
      setDecisionBusy(false);
      setDecisionError("");
      setStage("Refreshing engine status…");
      try {
        await settled.current();
      } catch {
        /* The status read reports its own failure. */
      }
      if (outcome.success)
        outcome.message =
          next === "setup"
            ? "OrbWin setup completed and the owned engine identity was verified."
            : next === "prepareUpdate"
              ? "OrbWin is ready for application replacement. Close OrbWin, then run the installer or uninstaller. Engine data is retained."
              : next === "reconnect"
                ? "Backend reconnected and the owned engine endpoint was verified."
                : next === "stop"
                  ? "Engine stopped. Persistent disks are retained."
                  : next === "start"
                    ? "Engine started and the owned endpoint was verified."
                    : "Engine restarted (stopped, then started) and the owned endpoint was verified.";
      setResult(outcome);
      notify(
        outcome.success ? "success" : outcome.cancelled ? "info" : "error",
        outcome.success
          ? outcome.message
          : `${labels[next]} engine ${outcome.cancelled ? "cancelled" : "failed"}: ${outcome.message}`,
      );
      active.current = false;
      controller.current = null;
      setAction(null);
      setStage("");
    }
  }

  const value: Lifecycle = {
    action,
    stage,
    startedAt,
    output,
    result,
    start: () => void execute("start"),
    setup: () => {
      if (!active.current) setConfirm("setup");
    },
    requestStop: () => {
      if (!active.current) setConfirm("stop");
    },
    requestRestart: () => {
      if (!active.current) setConfirm("restart");
    },
    requestReconnect: () => {
      if (!active.current) setConfirm("reconnect");
    },
    requestPrepareUpdate: () => {
      if (!active.current) setConfirm("prepareUpdate");
    },
    cancel: () => (decision ? leaveDecision() : controller.current?.abort()),
  };
  async function continueDecision() {
    if (
      !decision ||
      decisionSending.current ||
      operation.current !== decision.operation
    )
      return;
    const pending = decision;
    decisionSending.current = true;
    setDecisionBusy(true);
    setDecisionError("");
    try {
      await backend.continueSetup(pending.operation, pending.nonce);
      if (operation.current === pending.operation) {
        setDecision(null);
        setStage("Continuing the original setup…");
      }
    } catch {
      if (operation.current === pending.operation)
        setDecisionError(
          "This setup decision could not be delivered. Leave setup and check its status before retrying.",
        );
    } finally {
      if (operation.current === pending.operation) {
        decisionSending.current = false;
        setDecisionBusy(false);
      }
    }
  }
  function leaveDecision() {
    if (!decision || operation.current !== decision.operation) return;
    setDecision(null);
    setStage("Leaving setup…");
    controller.current?.abort();
  }
  return (
    <Context.Provider value={value}>
      {children}
      {decision && (
        <SetupDecisionDialog
          busy={decisionBusy}
          error={decisionError}
          onContinue={() => void continueDecision()}
          onLeave={leaveDecision}
        />
      )}
      {confirm && (
        <ConfirmDialog
          title={
            confirm === "setup"
              ? "Prepare OrbWin engine?"
              : confirm === "reconnect"
                ? "Reconnect OrbWin backend?"
                : confirm === "prepareUpdate"
                  ? "Prepare for application update?"
                  : confirm === "stop"
                    ? "Stop OrbWin engine?"
                    : "Restart OrbWin engine?"
          }
          confirmLabel={
            confirm === "setup"
              ? "Continue"
              : confirm === "reconnect"
                ? "Reconnect backend"
                : confirm === "prepareUpdate"
                  ? "Prepare update"
                  : confirm === "stop"
                    ? "Stop engine"
                    : "Restart engine"
          }
          destructive
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            const chosen = confirm;
            setConfirm(null);
            void execute(chosen);
          }}
        >
          <p>
            {confirm === "setup"
              ? "Creates or resumes the private engine. If a bundled backend update is needed, it stops the owned backend before applying it, interrupting Docker and Kubernetes workloads. Setup verifies the same engine identity before accepting existing data. Guest components may be downloaded. Cancelling stops the client; accepted backend work may continue."
              : confirm === "reconnect"
                ? "Requests the engine's running state again. If the management service has stopped, this backend can restart its guest while reconnecting, interrupting Docker and Kubernetes workloads. Containers resume according to their restart policy. Cancelling stops the request client; accepted backend work may continue."
                : confirm === "prepareUpdate"
                  ? "Stops the OrbWin engine and its background service so application files can be replaced. Docker and Kubernetes workloads are interrupted. Close OrbWin after preparation completes, then run the installer or uninstaller."
                  : confirm === "stop"
                    ? "Stops the OrbWin guest and interrupts its containers and Kubernetes workloads."
                    : "Stops the OrbWin guest, then starts it again. Running containers and Kubernetes workloads are interrupted; containers restart only according to their restart policy."}{" "}
            Persistent disks are retained. Docker Desktop, Rancher Desktop and
            your own WSL distributions are not stopped.
          </p>
          {confirm === "restart" && (
            <p>
              The backend has no single restart command; OrbWin runs a stop
              followed by a start. Cancelling between the steps leaves the
              engine stopped.
            </p>
          )}
        </ConfirmDialog>
      )}
    </Context.Provider>
  );
}

// Elapsed time for an in-flight lifecycle request; purely presentational.
export function useElapsed(startedAt: number, running: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!running) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [running, startedAt]);
  return running ? Math.max(0, Math.floor((now - startedAt) / 1000)) : 0;
}
