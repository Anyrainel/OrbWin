// Pure helpers for Kubernetes list data. Everything shown is derived from fields
// the cluster returned; nothing is inferred or invented. Keep this file free of
// TypeScript-only runtime constructs so scripts/cluster-model.test.mjs can load it.
export type ClusterKind =
  | "pods"
  | "services"
  | "deployments"
  | "persistentVolumeClaims";
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type Json = Record<string, any>;
export type K8sObject = {
  metadata: {
    name: string;
    namespace: string;
    uid: string;
    creationTimestamp?: string;
    deletionTimestamp?: string;
    labels?: Record<string, string>;
  };
  spec?: Json;
  status?: Json;
};
export type Tone = "ok" | "warn" | "bad" | "idle";
export type Health = { label: string; tone: Tone };
export type PortInfo = {
  label: string;
  port: number;
  protocol: string;
  target?: string;
  nodePort?: number;
};

export function parseList(output: string): K8sObject[] {
  let data: { items?: K8sObject[] };
  try {
    data = JSON.parse(output);
  } catch {
    throw new Error("Cluster returned a response that is not valid JSON.");
  }
  if (
    !data ||
    !Array.isArray(data.items) ||
    data.items.some(
      (item) =>
        !item?.metadata?.uid || !item.metadata.name || !item.metadata.namespace,
    )
  )
    throw new Error("Cluster returned an invalid resource list.");
  return data.items;
}

export type ReadFailure = "disabled" | "preview" | "failed";
export function classifyReadError(message: string): ReadFailure {
  if (/no mirrored kubeconfig/i.test(message)) return "disabled";
  if (/require the Windows desktop application/i.test(message))
    return "preview";
  return "failed";
}

export function ageOf(timestamp: string | undefined, now = Date.now()): string {
  if (!timestamp) return "—";
  const then = Date.parse(timestamp);
  if (Number.isNaN(then)) return "—";
  const seconds = Math.max(0, Math.floor((now - then) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h`;
  const days = Math.floor(hours / 24);
  if (days < 730) return `${days}d`;
  return `${Math.floor(days / 365)}y`;
}

const BAD_REASONS =
  /^(CrashLoopBackOff|ImagePullBackOff|ErrImagePull|InvalidImageName|CreateContainerConfigError|CreateContainerError|RunContainerError|Error|OOMKilled|Evicted|Failed|Unknown|NodeLost|ContainerStatusUnknown|DeadlineExceeded)$/;

export function podCounts(pod: K8sObject): {
  ready: number;
  total: number;
  restarts: number;
} {
  const statuses: Json[] = pod.status?.containerStatuses ?? [];
  const total =
    (pod.spec?.containers as unknown[] | undefined)?.length ?? statuses.length;
  return {
    ready: statuses.filter((s) => s.ready).length,
    total,
    restarts: statuses.reduce(
      (sum, s) => sum + (Number(s.restartCount) || 0),
      0,
    ),
  };
}
export function podHealth(pod: K8sObject): Health {
  if (pod.metadata.deletionTimestamp)
    return { label: "Terminating", tone: "warn" };
  const phase: string = pod.status?.phase ?? "Unknown";
  const init: Json[] = pod.status?.initContainerStatuses ?? [];
  for (const status of init) {
    const terminated = status.state?.terminated;
    if (terminated && terminated.exitCode !== 0)
      return {
        label: `Init:${terminated.reason ?? "Error"}`,
        tone: "bad",
      };
    const waiting = status.state?.waiting;
    if (waiting?.reason && waiting.reason !== "PodInitializing")
      return {
        label: `Init:${waiting.reason}`,
        tone: BAD_REASONS.test(waiting.reason) ? "bad" : "warn",
      };
  }
  const containers: Json[] = pod.status?.containerStatuses ?? [];
  for (const status of containers) {
    const waiting = status.state?.waiting?.reason;
    if (waiting)
      return {
        label: waiting,
        tone: BAD_REASONS.test(waiting) ? "bad" : "warn",
      };
    const terminated = status.state?.terminated;
    if (terminated && phase !== "Succeeded" && terminated.exitCode !== 0)
      return { label: terminated.reason ?? "Error", tone: "bad" };
  }
  const reason: string | undefined = pod.status?.reason;
  if (reason)
    return { label: reason, tone: BAD_REASONS.test(reason) ? "bad" : "warn" };
  if (phase === "Succeeded") return { label: "Completed", tone: "idle" };
  if (phase === "Failed") return { label: "Failed", tone: "bad" };
  if (phase === "Pending") return { label: "Pending", tone: "warn" };
  if (phase === "Running") {
    const { ready, total } = podCounts(pod);
    return ready === total && total > 0
      ? { label: "Running", tone: "ok" }
      : { label: "Running", tone: "warn" };
  }
  return { label: phase, tone: BAD_REASONS.test(phase) ? "bad" : "idle" };
}
export function deploymentCounts(d: K8sObject): {
  desired: number;
  ready: number;
  updated: number;
  available: number;
} {
  return {
    desired: Number(d.spec?.replicas ?? 1),
    ready: Number(d.status?.readyReplicas ?? 0),
    updated: Number(d.status?.updatedReplicas ?? 0),
    available: Number(d.status?.availableReplicas ?? 0),
  };
}
export function deploymentHealth(d: K8sObject): Health {
  const { desired, ready } = deploymentCounts(d);
  const conditions: Json[] = d.status?.conditions ?? [];
  if (
    conditions.some(
      (c) =>
        c.type === "Progressing" && c.reason === "ProgressDeadlineExceeded",
    )
  )
    return { label: "Not progressing", tone: "bad" };
  if (desired === 0) return { label: "Scaled to 0", tone: "idle" };
  if (ready >= desired) return { label: "Available", tone: "ok" };
  return { label: ready === 0 ? "Unavailable" : "Updating", tone: "warn" };
}
export function claimHealth(c: K8sObject): Health {
  const phase: string = c.status?.phase ?? "Unknown";
  if (phase === "Bound") return { label: "Bound", tone: "ok" };
  if (phase === "Pending") return { label: "Pending", tone: "warn" };
  if (phase === "Lost") return { label: "Lost", tone: "bad" };
  return { label: phase, tone: "idle" };
}
export function serviceType(s: K8sObject): string {
  return s.spec?.type ?? "ClusterIP";
}
export function health(kind: ClusterKind, object: K8sObject): Health {
  switch (kind) {
    case "pods":
      return podHealth(object);
    case "deployments":
      return deploymentHealth(object);
    case "persistentVolumeClaims":
      return claimHealth(object);
    default:
      return { label: serviceType(object), tone: "ok" };
  }
}

export function servicePorts(s: K8sObject): PortInfo[] {
  return ((s.spec?.ports ?? []) as Json[]).map((p) => ({
    label: p.name ?? "",
    port: Number(p.port),
    protocol: p.protocol ?? "TCP",
    target: p.targetPort === undefined ? undefined : String(p.targetPort),
    nodePort: p.nodePort === undefined ? undefined : Number(p.nodePort),
  }));
}
export function podPorts(pod: K8sObject): (PortInfo & { container: string })[] {
  const out: (PortInfo & { container: string })[] = [];
  for (const container of (pod.spec?.containers ?? []) as Json[])
    for (const p of (container.ports ?? []) as Json[])
      out.push({
        container: container.name,
        label: p.name ?? "",
        port: Number(p.containerPort),
        protocol: p.protocol ?? "TCP",
      });
  return out;
}
export function forwardablePorts(
  kind: ClusterKind,
  object: K8sObject,
): PortInfo[] {
  const ports =
    kind === "pods"
      ? podPorts(object)
      : kind === "services"
        ? servicePorts(object)
        : [];
  return ports.filter(
    (p) =>
      p.protocol === "TCP" &&
      Number.isInteger(p.port) &&
      p.port > 0 &&
      p.port < 65536,
  );
}
export function portText(p: PortInfo): string {
  return `${p.port}/${p.protocol}`;
}
export function images(object: K8sObject): string[] {
  const template: Json | undefined = object.spec?.template?.spec ?? object.spec;
  return ((template?.containers ?? []) as Json[]).map((c) => String(c.image));
}

// Row text. Secondary line summarizes returned state only.
export function summary(kind: ClusterKind, object: K8sObject): string {
  switch (kind) {
    case "pods": {
      const { ready, total, restarts } = podCounts(object);
      return `${podHealth(object).label} · ${ready}/${total} ready${restarts ? ` · ${restarts} restarts` : ""}`;
    }
    case "services": {
      const ports = servicePorts(object).map(portText).join(", ");
      return `${serviceType(object)} · ${object.spec?.clusterIP ?? "—"}${ports ? ` · ${ports}` : ""}`;
    }
    case "deployments": {
      const { desired, ready } = deploymentCounts(object);
      return `${ready}/${desired} ready`;
    }
    default: {
      const size =
        object.status?.capacity?.storage ??
        object.spec?.resources?.requests?.storage;
      return `${claimHealth(object).label}${size ? ` · ${size}` : ""}`;
    }
  }
}
export function searchText(kind: ClusterKind, object: K8sObject): string {
  return [
    object.metadata.name,
    object.metadata.namespace,
    summary(kind, object),
    object.spec?.clusterIP,
    object.spec?.storageClassName,
    object.status?.podIP,
    object.spec?.nodeName,
    ...images(object),
    ...Object.entries(object.metadata.labels ?? {}).map(
      ([k, v]) => `${k}=${v}`,
    ),
  ]
    .filter(Boolean)
    .join("\n")
    .toLowerCase();
}

export function namespacesOf(objects: K8sObject[]): string[] {
  return [...new Set(objects.map((o) => o.metadata.namespace))].sort();
}
export function groupByNamespace(
  objects: K8sObject[],
): [string, K8sObject[]][] {
  const groups = new Map<string, K8sObject[]>();
  for (const object of objects) {
    const list = groups.get(object.metadata.namespace) ?? [];
    list.push(object);
    groups.set(object.metadata.namespace, list);
  }
  return [...groups]
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([ns, list]) => [
      ns,
      list.sort((a, b) =>
        a.metadata.name < b.metadata.name
          ? -1
          : a.metadata.name > b.metadata.name
            ? 1
            : 0,
      ),
    ]);
}

// kubectl port-forward prints "Forwarding from 127.0.0.1:PORT -> REMOTE".
export function forwardUrls(
  output: string,
): { local: number; remote: number }[] {
  const found = new Map<number, number | null>();
  for (const match of output.matchAll(
    /^Forwarding from 127\.0\.0\.1:(\d{1,5}) -> (\d{1,5})\r?$/gm,
  )) {
    const local = Number(match[1]),
      remote = Number(match[2]);
    if (local < 1 || local > 65535 || remote < 1 || remote > 65535) continue;
    if (found.has(local) && found.get(local) !== remote) found.set(local, null);
    else if (!found.has(local)) found.set(local, remote);
  }
  return [...found].flatMap(([local, remote]) =>
    remote === null ? [] : [{ local, remote }],
  );
}
