import type { BackendStatus, Diagnostic } from "../../backend";

// Classifies the existing read-only `backend_status` result into first-run
// steps. Matching is limited to messages emitted by src-tauri/src/engine.rs and
// the Docker CLI connection error; any other text stays "unknown"/"error"
// rather than being guessed into a more specific state.
export type StepState = "done" | "blocked" | "action" | "pending" | "unknown";
export type BundleState = "ready" | "missing" | "unknown";
export type EngineState =
  | "running"
  | "offline"
  | "foreign"
  | "error"
  | "preview"
  | "unknown";

export function bundleState(status: BackendStatus | null): BundleState {
  if (!status) return "unknown";
  if (status.available) return "ready";
  const detail = status.diagnostic ?? "";
  if (
    /Owned backend evidence is unavailable|Owned backend identity is missing|outside the owned artifact directory/.test(
      detail,
    )
  )
    return "missing";
  // These failures happen after the owned identity and bundle manifest were
  // accepted by engine::target().
  if (engineState(status) === "offline" || engineState(status) === "foreign")
    return "ready";
  return "unknown";
}

export function engineState(status: BackendStatus | null): EngineState {
  if (!status) return "unknown";
  if (status.available) return "running";
  const detail = status.diagnostic;
  if (detail === undefined) return "preview";
  if (/failed to connect|error during connect/i.test(detail)) return "offline";
  if (/does not match the verified owned Linux engine/.test(detail))
    return "foreign";
  if (
    /Owned backend evidence is unavailable|Owned backend identity is missing|outside the owned artifact directory/.test(
      detail,
    )
  )
    return "unknown";
  return "error";
}

// Start is offered only where a start can plausibly help. A foreign daemon on
// the owned endpoint must never be "fixed" by starting another engine (BL-001).
export function canStartEngine(status: BackendStatus | null) {
  const engine = engineState(status);
  return (
    bundleState(status) !== "missing" &&
    (engine === "offline" || engine === "error")
  );
}

export function wslState(
  diagnostics: Diagnostic[] | null,
): Exclude<StepState, "action"> {
  if (!diagnostics) return "unknown";
  const version = diagnostics.find((item) => item.name === "WSL version");
  if (!version) return "unknown";
  return version.success ? "done" : "blocked";
}
