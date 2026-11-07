// Observation DATA only. No invoke(), client reader, lifecycle dispatch or native grant.
export const inspectionActions = ["prepare-engine", "resume-setup", "start-engine", "upgrade-engine", "review-private-clients"] as const;
export type InspectionAction = typeof inspectionActions[number];
export const availabilityEntrypoints = ["setup-panel", "sidebar", "command-palette", "recovery"] as const;
export type AvailabilityEntrypoint = typeof availabilityEntrypoints[number];
type Phase = "fresh" | "interrupted" | "committed" | "unsupported-version" | "conflict" | "busy" | "unknown";
type Runtime = "absent" | "uncommitted" | "committed" | "unknown";
type Image = "same" | "different" | "unknown";
export type Inspection = Readonly<{
  schema: 2; evidence: "bounded-observation"; scope: "product" | "historical-debug-proof"; phase: Phase; reason: string;
  runtime: Runtime; image: Image; connection: "not-checked"; clients: "not-checked";
  actions: readonly Readonly<{action: InspectionAction; state: "unsupported"; reason: "route-not-enabled"}>[];
}>;
const unknownReasons = ["route-not-enabled", "missing-component", "control-cut-unsupported", "tuple-cut-unsupported", "malformed-record", "partial-record-unsupported", "observation-changed", "observation-unknown", "client-inspection-unavailable", "proof-scope"];
function one(value: unknown, values: readonly string[]): value is string { return typeof value === "string" && values.includes(value); }
function record(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value); }

/** Exact canonical Go JSON + LF, including field/action order. Failure stays unknown. */
export function decodeInspection(body: Uint8Array, successfulExit: boolean): Inspection | null {
  if (!successfulExit || body.byteLength === 0 || body.byteLength > 16384) return null;
  try {
    const text = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(body);
    const r: unknown = JSON.parse(text);
    if (!record(r) || r.schema !== 2 || r.evidence !== "bounded-observation" || !one(r.scope, ["product", "historical-debug-proof"])
      || !one(r.runtime, ["absent", "uncommitted", "committed", "unknown"]) || !one(r.image, ["same", "different", "unknown"])
      || r.connection !== "not-checked" || r.clients !== "not-checked" || !Array.isArray(r.actions) || r.actions.length !== 5) return null;
    if ((r.scope === "historical-debug-proof" && (r.phase !== "unknown" || r.reason !== "proof-scope"))
      || (r.reason === "proof-scope" && r.scope !== "historical-debug-proof")) return null;
    let valid = false;
    switch (r.phase) {
      case "fresh": valid = r.runtime === "absent" && r.image === "unknown" && r.reason === "initial-absence-observed"; break;
      case "interrupted": valid = r.runtime === "uncommitted" && r.image === "same" && one(r.reason, ["interrupted-same-image", "initialized-uncommitted", "prepared-not-renamed", "renamed-not-committed", "orphan-stage", "missing-control-after-denial"]); break;
      case "committed": valid = r.runtime === "committed" && r.image === "same" && r.reason === "committed-receipt"; break;
      case "unsupported-version": valid = one(r.runtime, ["uncommitted", "committed"]) && r.image === "different" && one(r.reason, ["different-image", "different-source"]); break;
      case "conflict": valid = r.reason === "namespace-conflict"; break;
      case "busy": valid = r.reason === "owner-busy"; break;
      case "unknown": valid = r.runtime === "unknown" && r.image === "unknown" && one(r.reason, unknownReasons); break;
    }
    if (!valid) return null;
    const actions = r.actions.map((a: unknown, i: number) => {
      if (!record(a) || a.action !== inspectionActions[i] || a.state !== "unsupported" || a.reason !== "route-not-enabled") throw Error("invalid action");
      return Object.freeze({ action: inspectionActions[i], state: "unsupported" as const, reason: "route-not-enabled" as const });
    });
    const canonical = { schema: 2, evidence: "bounded-observation", scope: r.scope, phase: r.phase, reason: r.reason, runtime: r.runtime, image: r.image, connection: r.connection, clients: r.clients, actions };
    // Original parsed values retain extra keys; compare them too so no extra nested
    // field can disappear while canonicalizing an otherwise accepted action.
    if (JSON.stringify(r) !== JSON.stringify(canonical) || text !== JSON.stringify(canonical) + "\n") return null;
    return Object.freeze({ ...canonical, actions: Object.freeze(actions) }) as Inspection;
  } catch { return null; }
}

const phaseText: Record<Phase, string> = {
  fresh: "No engine setup record was observed.",
  interrupted: "An earlier engine setup record was observed.",
  committed: "An engine setup record was observed. The connection has not been checked.",
  "unsupported-version": "A setup record for a different engine version was observed.",
  conflict: "Conflicting engine records were observed.", busy: "An engine setup observation found possible use.", unknown: "Engine setup observation was unavailable.",
};
export function setupAvailability(inspection: Inspection | null, entrypoint: AvailabilityEntrypoint) {
  // Every entry point uses this same closed model. No event, phase, caller hint,
  // text match or action suggestion enables an unfinished role.
  const summary = inspection?.scope === "historical-debug-proof" ? "A qualification fixture is selected. Product actions are unavailable."
    : inspection ? phaseText[inspection.phase] : phaseText.unknown;
  return Object.freeze({ entrypoint, summary, connection: "not-checked" as const, clients: "not-checked" as const,
    actions: Object.freeze(inspectionActions.map(action => Object.freeze({ action, enabled: false as const, reason: "route-not-enabled" as const }))),
    guidance: "View diagnostics or refresh after a supported inspection route is available.",
  });
}
