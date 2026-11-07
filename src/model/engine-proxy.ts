export type EngineProxyConfig = {
  version: 1;
  mode: "none" | "url" | "auto";
  url: string;
  noProxy: string[];
};
export type EngineProxyReview = {
  uid: string;
  resourceVersion: string;
  generation: number;
  config: EngineProxyConfig;
};
export type EngineProxySnapshot = Omit<EngineProxyReview, "config"> & {
  desired: EngineProxyConfig | null;
  running: boolean;
  stage: "unmanaged" | "pending" | "ready" | "unavailable";
};
const object = (v: unknown): Record<string, unknown> =>
  v !== null && typeof v === "object" && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
const ascii = (v: unknown, max: number): v is string =>
  typeof v === "string" &&
  v.length > 0 &&
  v.length <= max &&
  /^[\x21-\x7e]+$/.test(v);
const integer = (v: unknown, max = Number.MAX_SAFE_INTEGER): v is number =>
  typeof v === "number" && Number.isSafeInteger(v) && v > 0 && v <= max;
const port = (value: string) =>
  /^[1-9][0-9]*$/.test(value) && Number(value) <= 65535;
const domain = (value: string) =>
  value.length <= 253 &&
  value
    .split(".")
    .every(
      (part) =>
        part.length > 0 &&
        part.length <= 63 &&
        /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(part),
    );
const ipv4 = (value: string) =>
  /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(
    value,
  ) && value.split(".").every((part) => Number(part) <= 255);
function ipv6(value: string): boolean {
  if (!value.includes(":") || value.includes("%") || value.includes("."))
    return false;
  try {
    return (
      new URL(`http://[${value}]/`).hostname === `[${value}]` &&
      !value.startsWith("::ffff:")
    );
  } catch {
    return false;
  }
}
function hostPort(value: string): boolean {
  if (ipv4(value) || ipv6(value)) return true;
  const bracket = value.match(/^\[([^\]]+)\]:([0-9]+)$/);
  if (bracket) return ipv6(bracket[1]) && port(bracket[2]);
  const parts = value.split(":");
  return (
    (parts.length === 1 || (parts.length === 2 && port(parts[1]))) &&
    domain(parts[0])
  );
}
function bypass(value: string): boolean {
  if (!ascii(value, 512)) return false;
  if (value === "*") return true;
  if (value.includes("/")) {
    const parts = value.split("/");
    if (parts.length !== 2 || !/^(0|[1-9][0-9]*)$/.test(parts[1])) return false;
    const bits = Number(parts[1]);
    if (ipv4(parts[0])) {
      const address = parts[0]
        .split(".")
        .reduce((n, byte) => n * 256 + Number(byte), 0);
      return (
        bits <= 32 &&
        (bits === 0
          ? address === 0
          : (address & (0xffffffff << (32 - bits))) >>> 0 === address)
      );
    }
    if (!ipv6(parts[0]) || bits > 128) return false;
    const halves = parts[0].split("::");
    const left = halves[0] ? halves[0].split(":") : [];
    const right = halves[1] ? halves[1].split(":") : [];
    const groups =
      halves.length === 1
        ? left
        : [
            ...left,
            ...Array(8 - left.length - right.length).fill("0"),
            ...right,
          ];
    const address = groups.reduce(
      (n: bigint, part: string) => (n << 16n) | BigInt(parseInt(part, 16)),
      0n,
    );
    return bits === 128 || address % (1n << BigInt(128 - bits)) === 0n;
  }
  return hostPort(value.replace(/^(\*\.|\.)/, ""));
}
function validateConfig(value: unknown): EngineProxyConfig {
  const v = object(value);
  if (
    Object.keys(v).sort().join() !== "mode,noProxy,url,version" ||
    v.version !== 1 ||
    !["none", "url", "auto"].includes(String(v.mode)) ||
    typeof v.url !== "string" ||
    !Array.isArray(v.noProxy) ||
    v.noProxy.length > 128 ||
    v.noProxy.some((entry) => typeof entry !== "string" || !bypass(entry)) ||
    new Set(v.noProxy).size !== v.noProxy.length
  )
    throw new Error(
      "The proxy configuration has an unsupported or incomplete shape.",
    );
  if (v.mode === "url") {
    const match = v.url.match(
      /^https?:\/\/(\[[^\]]+\]|[^:/?#\\%@]+)(?::([0-9]+))?\/?$/,
    );
    let parsed: URL;
    try {
      parsed = new URL(v.url);
    } catch {
      throw new Error(
        "Enter an HTTP or HTTPS proxy endpoint without credentials or a path.",
      );
    }
    if (
      !ascii(v.url, 2048) ||
      !match ||
      parsed.hostname !== match[1] ||
      (match[2] && !port(match[2])) ||
      parsed.hostname === "0.0.0.0" ||
      parsed.hostname === "[::]" ||
      !hostPort(
        match[1].startsWith("[") ? `${match[1]}:${match[2] ?? "80"}` : match[1],
      )
    )
      throw new Error(
        "Use a canonical HTTP or HTTPS proxy host and optional port, without credentials, query, or path.",
      );
  } else if (v.url !== "")
    throw new Error("Only URL mode accepts a proxy endpoint.");
  if (JSON.stringify(v).length > 16384)
    throw new Error("The proxy configuration exceeds its size limit.");
  return {
    version: 1,
    mode: v.mode as EngineProxyConfig["mode"],
    url: v.url,
    noProxy: [...v.noProxy] as string[],
  };
}
export function engineProxyConfig(
  mode: EngineProxyConfig["mode"],
  url: string,
  noProxy: string,
): EngineProxyConfig {
  return validateConfig({
    version: 1,
    mode,
    url: mode === "url" ? url.trim() : "",
    noProxy: noProxy
      .trim()
      .split(/[\s,]+/)
      .filter(Boolean),
  });
}
function plan(value: unknown, uid: string, generation: number) {
  const p = object(value),
    b = object(p.binding);
  if (
    p.appUID !== uid ||
    !integer(p.observedGeneration) ||
    p.observedGeneration > generation ||
    !integer(p.proxyRevision) ||
    typeof p.configDigest !== "string" ||
    !/^[a-f0-9]{64}$/.test(p.configDigest) ||
    !["prepare", "commit", "disable"].includes(String(p.phase)) ||
    !ascii(b.limaVMUID, 128) ||
    !integer(b.hostPID, 4294967295) ||
    typeof b.hostCreationTime !== "string" ||
    !/^[a-f0-9]{16}$/.test(b.hostCreationTime) ||
    typeof b.transportEpoch !== "string" ||
    !/^[a-f0-9]{32}$/.test(b.transportEpoch)
  )
    return null;
  return JSON.stringify([
    p.appUID,
    p.observedGeneration,
    p.proxyRevision,
    p.configDigest,
    b.limaVMUID,
    b.hostPID,
    b.hostCreationTime,
    b.transportEpoch,
    p.phase,
  ]);
}
export function readEngineProxySnapshot(raw: unknown): EngineProxySnapshot {
  const document = object(raw),
    metadata = object(document.metadata),
    spec = object(document.spec),
    status = object(document.status);
  if (
    document.kind !== "App" ||
    metadata.name !== "app" ||
    (metadata.namespace && metadata.namespace !== "") ||
    !ascii(metadata.uid, 128) ||
    !ascii(metadata.resourceVersion, 128) ||
    !integer(metadata.generation)
  )
    throw new Error(
      "The backend did not return the current owned App identity.",
    );
  const desired =
    spec.networkProxy === undefined || spec.networkProxy === null
      ? null
      : validateConfig(spec.networkProxy);
  const controller = object(status.networkProxy),
    host = object(status.networkProxyHost);
  const condition = Array.isArray(status.conditions)
    ? status.conditions.map(object).find((c) => c.type === "NetworkProxyReady")
    : undefined;
  const current = condition?.observedGeneration === metadata.generation;
  const permission = plan(controller.plan, metadata.uid, metadata.generation);
  const currentController =
    desired !== null &&
    current &&
    condition?.status === "True" &&
    condition.reason === "Ready" &&
    controller.appUID === metadata.uid &&
    controller.observedGeneration === metadata.generation &&
    controller.mode === desired.mode &&
    integer(controller.proxyRevision);
  const untouchedDirect =
    currentController &&
    desired?.mode === "none" &&
    controller.managedEver === false &&
    controller.plan == null &&
    controller.guestAppliedRevision === 0;
  const ready =
    untouchedDirect ||
    (currentController &&
      desired !== null &&
      controller.guestAppliedRevision === controller.proxyRevision &&
      object(controller.plan).proxyRevision === controller.proxyRevision &&
      object(controller.plan).configDigest === controller.configDigest &&
      permission !== null &&
      plan(host.plan, metadata.uid, metadata.generation) === permission &&
      object(controller.plan).phase ===
        (desired.mode === "none" ? "disable" : "commit") &&
      host.phase === (desired.mode === "none" ? "direct" : "active") &&
      host.reason === "Ready");
  return {
    uid: metadata.uid,
    resourceVersion: metadata.resourceVersion,
    generation: metadata.generation,
    desired,
    running: spec.running === true,
    stage: ready
      ? "ready"
      : current && condition?.reason === "Unavailable"
        ? "unavailable"
        : desired === null && !status.networkProxy
          ? "unmanaged"
          : "pending",
  };
}
export function engineProxyReviewCurrent(
  review: EngineProxyReview,
  snapshot: EngineProxySnapshot | null,
): boolean {
  return (
    !!snapshot &&
    snapshot.uid === review.uid &&
    snapshot.resourceVersion === review.resourceVersion &&
    snapshot.generation === review.generation &&
    snapshot.running
  );
}
