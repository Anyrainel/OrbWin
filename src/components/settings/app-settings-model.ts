// Pure parsing of the rdd App resource returned by the read-only `readSettings`
// operation (`kubectl get apps/app --output=json` against the owned management
// API). Field names follow rdd/pkg/apis/app/v1alpha1/app_types.go at the pinned
// upstream commit. Absent fields stay undefined: absence means unknown, never a
// default value presented as observed state.

export type AppCondition = {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  observedGeneration?: number;
};
export type AppSettings = {
  running?: boolean;
  kubernetesEnabled?: boolean;
  kubernetesVersion?: string;
  containerEngine?: string;
  cpus?: number;
  memory?: string;
  generation?: number;
  conditions: AppCondition[];
};

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}
function optional<T>(value: unknown, type: string): T | undefined {
  return typeof value === type ? (value as T) : undefined;
}

// The client output may interleave stderr warnings with the JSON document.
// Accept only one balanced top-level object; anything else is an error.
export function extractJson(raw: string): unknown {
  const start = raw.indexOf("{");
  const end = raw.lastIndexOf("}");
  if (start < 0 || end < start)
    throw new Error("The backend returned no configuration document.");
  return JSON.parse(raw.slice(start, end + 1));
}

export function parseAppSettings(raw: string): AppSettings {
  const document = record(extractJson(raw));
  if (document.kind !== "App")
    throw new Error("The backend response is not an App configuration.");
  const spec = record(document.spec);
  const kubernetes = record(spec.kubernetes);
  const machine = record(spec.virtualMachine);
  const status = record(document.status);
  const conditions = Array.isArray(status.conditions)
    ? status.conditions.map(record).flatMap((item) =>
        typeof item.type === "string" && typeof item.status === "string"
          ? [
              {
                type: item.type,
                status: item.status,
                reason: optional<string>(item.reason, "string"),
                message: optional<string>(item.message, "string"),
                observedGeneration: optional<number>(
                  item.observedGeneration,
                  "number",
                ),
              },
            ]
          : [],
      )
    : [];
  return {
    running: optional<boolean>(spec.running, "boolean"),
    kubernetesEnabled: optional<boolean>(kubernetes.enabled, "boolean"),
    kubernetesVersion: optional<string>(kubernetes.version, "string"),
    containerEngine: optional<string>(
      record(spec.containerEngine).name,
      "string",
    ),
    cpus: optional<number>(machine.cpus, "number"),
    memory: optional<string>(machine.memory, "string"),
    generation: optional<number>(
      record(document.metadata).generation,
      "number",
    ),
    conditions,
  };
}

export function condition(settings: AppSettings, type: string) {
  return settings.conditions.find((item) => item.type === type);
}

// rdd stamps every condition with the App generation it observed; a condition
// from an older generation is stale (app_types.go invariant).
export function isCurrent(settings: AppSettings, item?: AppCondition) {
  return (
    item !== undefined &&
    (settings.generation === undefined ||
      item.observedGeneration === undefined ||
      item.observedGeneration >= settings.generation)
  );
}

export type ApplyState =
  | { kind: "settled" }
  | { kind: "applying"; reason?: string; message?: string }
  | { kind: "unknown" };

// Settled=True for the current generation means the reconcile chain has caught
// up with the stored spec. Anything else means a change is still being applied
// (or failed to apply); the reason is forwarded verbatim from the backend.
export function applyState(settings: AppSettings): ApplyState {
  const settled = condition(settings, "Settled");
  if (!settled) return { kind: "unknown" };
  if (settled.status === "True" && isCurrent(settings, settled))
    return { kind: "settled" };
  return {
    kind: "applying",
    reason: settled.reason,
    message: settled.message,
  };
}

export type KubernetesState =
  | "disabled"
  | "ready"
  | "starting"
  | "enabled"
  | "unknown";
export function kubernetesState(settings: AppSettings | null): KubernetesState {
  if (!settings || settings.kubernetesEnabled === undefined) return "unknown";
  if (!settings.kubernetesEnabled) return "disabled";
  const ready = condition(settings, "KubernetesReady");
  if (!ready || !isCurrent(settings, ready)) return "enabled";
  return ready.status === "True" ? "ready" : "starting";
}
