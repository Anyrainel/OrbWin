export type LANSelection = {
  containerId: string;
  startedAt: string;
  hostPort: number;
  containerPort: number;
  protocol: "tcp" | "udp";
  interfaceGuid: string;
  interfaceLuid: string;
  localAddress: string;
  profile: "private" | "domain";
  clientNetworks: string[];
};

export type LANPublicationChoice = Pick<
  LANSelection,
  "containerId" | "startedAt" | "hostPort" | "containerPort" | "protocol"
> & {
  containerName: string;
  available: boolean;
  availabilityReason?: string;
};
export type LANInterfaceChoice = {
  interfaceGuid: string;
  interfaceLuid: string;
  alias: string;
  privateAddresses: string[];
  profile: "private" | "domain" | "public" | "unknown";
  available: boolean;
  availabilityReason?: string;
};
export type LANOptions = {
  version: 1;
  readAt: string;
  expiresAt: string;
  complete: boolean;
  sourceAvailable: boolean;
  sourceUnavailableReason?: string;
  helperAvailable: boolean;
  helperUnavailableReason?: string;
  publications: LANPublicationChoice[];
  interfaces: LANInterfaceChoice[];
};
export type LANReview = {
  reviewId: string;
  reviewSha256: string;
  expiresAt: string;
  selection: LANSelection;
  publicationGeneration: string;
  purpose:
    | "activate"
    | "remove"
    | "settle-remove"
    | "cleanup-remove"
    | "cleanup-settle";
  available: boolean;
  reasonCode?: string;
};
export type LANAccessStatus = {
  operationId: string;
  reviewId: string;
  leaseSha256?: string;
  selection: LANSelection;
  sequence: number;
  updatedAt: string;
  phase:
    | "awaiting-admin"
    | "awaiting-confirmation"
    | "staging"
    | "starting"
    | "ready"
    | "closing"
    | "closed"
    | "cancelled"
    | "cleanup-pending"
    | "removed"
    | "failed";
  ingress: "closed" | "ready" | "unknown";
  firewall:
    | "not-dispatched"
    | "created"
    | "removed"
    | "create-unknown"
    | "remove-unknown";
  canCancel: boolean;
  canInspect: boolean;
  canReviewRemoval: boolean;
  canSettleRemoval?: boolean;
  canReviewCleanup?: boolean;
  reviewSha256?: string;
  runtimeInstance?: string;
  reasonCode?: string;
  error?: string;
};

// Retained before dispatch. This is only a denial fence, never permission to
// replay a request or infer native ownership from a renderer record.
export type LANPendingChange = {
  runtimeInstance: string;
  reviewId: string;
  reviewSha256: string;
  purpose: LANReview["purpose"];
  operationId?: string;
  leaseSha256?: string;
};
export type LANReconciliation = {
  reviewId: string;
  reviewSha256: string;
  outcome: "revoked" | "operation" | "unknown" | "retired" | "review-revoked";
  operationId?: string;
  runtimeInstance?: string;
  leaseSha256?: string;
  nativeSequence?: number;
  managedSequence?: number;
  observedAt?: string;
};

export function lanParsePending(raw: string | null): LANPendingChange[] {
  if (raw === null) return [];
  if (raw.length > 65536)
    throw new Error("LAN recovery records exceed the storage limit.");
  const records: unknown = JSON.parse(raw);
  if (!Array.isArray(records) || records.length > 16)
    throw new Error("Invalid LAN recovery records.");
  const seen = new Set<string>();
  for (const value of records) {
    if (!value || typeof value !== "object" || Array.isArray(value))
      throw new Error("Invalid LAN recovery record.");
    const p = value as Record<string, unknown>;
    if (
      Object.keys(p).some(
        (key) =>
          ![
            "runtimeInstance",
            "reviewId",
            "reviewSha256",
            "purpose",
            "operationId",
            "leaseSha256",
          ].includes(key),
      ) ||
      typeof p.runtimeInstance !== "string" ||
      !p.runtimeInstance ||
      p.runtimeInstance.length > 256 ||
      typeof p.reviewId !== "string" ||
      !/^[a-f0-9]{32}$/.test(p.reviewId) ||
      typeof p.reviewSha256 !== "string" ||
      !/^[a-f0-9]{64}$/.test(p.reviewSha256) ||
      ![
        "activate",
        "remove",
        "settle-remove",
        "cleanup-remove",
        "cleanup-settle",
      ].includes(p.purpose as string) ||
      (p.leaseSha256 !== undefined &&
        (typeof p.leaseSha256 !== "string" ||
          !/^[a-f0-9]{64}$/.test(p.leaseSha256))) ||
      (p.operationId !== undefined &&
        (typeof p.operationId !== "string" ||
          !/^[a-f0-9]{32}$/.test(p.operationId)))
    )
      throw new Error("Invalid LAN recovery record.");
    const key = `${p.runtimeInstance}/${p.reviewId}`;
    if (seen.has(key)) throw new Error("Duplicate LAN recovery record.");
    seen.add(key);
  }
  return records as LANPendingChange[];
}

export const lanPublicationKey = (p: LANPublicationChoice) =>
  `${p.containerId}/${p.startedAt}/${p.hostPort}/${p.containerPort}/${p.protocol}`;

export function lanOptionsCurrent(options: LANOptions, now: number): boolean {
  const read = Date.parse(options.readAt);
  const expiry = Date.parse(options.expiresAt);
  return (
    options.version === 1 &&
    options.complete &&
    options.sourceAvailable &&
    options.helperAvailable &&
    Number.isFinite(read) &&
    read <= now &&
    expiry > now &&
    expiry <= read + 5000
  );
}

export function lanRetainChoices(
  before: LANOptions | null,
  after: LANOptions,
  choice: { publication: string; network: string; address: string },
) {
  const publication = after.publications.some(
    (p) => p.available && lanPublicationKey(p) === choice.publication,
  )
    ? choice.publication
    : "";
  const previous = before?.interfaces.find(
    (n) => `${n.interfaceGuid}/${n.interfaceLuid}` === choice.network,
  );
  const current = after.interfaces.find(
    (n) =>
      n.available && `${n.interfaceGuid}/${n.interfaceLuid}` === choice.network,
  );
  const network =
    previous && current && previous.profile === current.profile
      ? choice.network
      : "";
  return {
    publication,
    network,
    address:
      network && current?.privateAddresses.includes(choice.address)
        ? choice.address
        : "",
  };
}

export function lanReconcilePending(
  pending: LANPendingChange,
  runtimeInstance: string,
  result: LANReconciliation,
  now = Date.now(),
): LANPendingChange | null {
  if (
    typeof runtimeInstance !== "string" ||
    !runtimeInstance ||
    runtimeInstance.length > 256
  )
    return pending;
  if (
    result.reviewId !== pending.reviewId ||
    result.reviewSha256 !== pending.reviewSha256
  )
    return pending;
  if (result.outcome === "review-revoked") {
    const age = now - Date.parse(result.observedAt ?? "");
    // This is positive revocation of an authenticated durable issuance before
    // consumption. Absence, actor-only revocation, and consumed/lease records
    // cannot settle a pending request from another SDK generation.
    return !pending.operationId &&
      !pending.leaseSha256 &&
      result.operationId === undefined &&
      result.leaseSha256 === undefined &&
      result.nativeSequence === undefined &&
      result.runtimeInstance === pending.runtimeInstance &&
      /^[a-f0-9]{32}$/.test(result.runtimeInstance ?? "") &&
      result.managedSequence === 2 &&
      Number.isFinite(age) &&
      age >= 0 &&
      age < 5000
      ? null
      : pending;
  }
  if (result.outcome === "retired") {
    const age = now - Date.parse(result.observedAt ?? "");
    return result.runtimeInstance === pending.runtimeInstance &&
      /^[a-f0-9]{32}$/.test(result.operationId ?? "") &&
      (!pending.operationId || pending.operationId === result.operationId) &&
      /^[a-f0-9]{64}$/.test(result.leaseSha256 ?? "") &&
      (!pending.leaseSha256 || pending.leaseSha256 === result.leaseSha256) &&
      result.nativeSequence === 5 &&
      Number.isFinite(age) &&
      age >= 0 &&
      age < 5000
      ? null
      : pending;
  }
  if (pending.runtimeInstance !== runtimeInstance) return pending;
  // A consumed review cannot later become unconsumed. A contradictory reply
  // must not erase a known operation which still requires observation.
  if (result.outcome === "revoked")
    return pending.operationId || pending.leaseSha256 ? pending : null;
  if (
    result.outcome === "operation" &&
    /^[a-f0-9]{32}$/.test(result.operationId ?? "") &&
    (!pending.operationId || pending.operationId === result.operationId)
  )
    return { ...pending, operationId: result.operationId };
  return pending;
}

export function lanObservePending(
  pending: LANPendingChange,
  runtimeInstance: string,
  operations: LANAccessStatus[],
  snapshotUpdatedAt: string,
  connected: boolean,
  now: number,
): LANPendingChange | null {
  if (pending.runtimeInstance !== runtimeInstance || !connected) return pending;
  const operation = operations.find((op) =>
    pending.operationId
      ? op.operationId === pending.operationId
      : op.reviewId === pending.reviewId,
  );
  if (
    !operation ||
    operation.reviewId !== pending.reviewId ||
    !/^[a-f0-9]{32}$/.test(operation.operationId) ||
    !Number.isSafeInteger(operation.sequence) ||
    operation.sequence < 1
  )
    return pending;
  if (pending.leaseSha256 && operation.leaseSha256 !== pending.leaseSha256)
    return pending;
  const mapped = {
    ...pending,
    operationId: operation.operationId,
    ...(/^[a-f0-9]{64}$/.test(operation.leaseSha256 ?? "")
      ? { leaseSha256: operation.leaseSha256 }
      : {}),
  };
  if (
    pending.purpose === "activate" &&
    lanIngressFresh(operation, snapshotUpdatedAt, connected, now)
  )
    return null;
  if (
    ["closed", "removed", "cancelled", "failed"].includes(operation.phase) &&
    operation.ingress === "closed" &&
    ["not-dispatched", "created", "removed"].includes(operation.firewall)
  )
    return null;
  return mapped;
}

function ipv4(raw: string): number | null {
  const bytes = raw.split(".");
  if (
    bytes.length !== 4 ||
    bytes.some(
      (part) => !/^(0|[1-9][0-9]{0,2})$/.test(part) || Number(part) > 255,
    )
  )
    return null;
  return bytes.reduce((value, part) => value * 256 + Number(part), 0);
}

// Convenience validation only. The owner independently canonicalizes and
// re-observes every selection; this cannot confer native/firewall authority.
export function lanClientNetworks(raw: string): string[] {
  const networks = raw
    .trim()
    .split(/[\s,]+/)
    .filter(Boolean);
  if (networks.length < 1 || networks.length > 8)
    throw new Error("Enter one to eight private IPv4 client ranges.");
  if (new Set(networks).size !== networks.length)
    throw new Error("Enter each client range once.");
  for (const network of networks) {
    const parts = network.split("/");
    const address = ipv4(parts[0]);
    const bits = Number(parts[1]);
    if (
      parts.length !== 2 ||
      address === null ||
      !/^(0|[1-9][0-9]?)$/.test(parts[1]) ||
      bits > 32
    )
      throw new Error("Use IPv4 CIDR ranges, for example 192.168.1.0/24.");
    const privateRange =
      (address >>> 24 === 10 && bits >= 8) ||
      (address >>> 20 === 0xac1 && bits >= 12) ||
      (address >>> 16 === 0xc0a8 && bits >= 16);
    const mask = bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0;
    if (!privateRange || (address & mask) >>> 0 !== address)
      throw new Error(
        "Use a private network boundary, such as 192.168.1.0/24.",
      );
  }
  return networks.sort();
}

export function lanIngressFresh(
  status: LANAccessStatus,
  snapshotUpdatedAt: string,
  connected: boolean,
  now: number,
): boolean {
  return (
    connected &&
    status.phase === "ready" &&
    status.ingress === "ready" &&
    status.firewall === "created" &&
    /^[a-f0-9]{32}$/.test(status.operationId) &&
    /^[a-f0-9]{64}$/.test(status.leaseSha256 ?? "") &&
    Number.isSafeInteger(status.sequence) &&
    status.sequence > 0 &&
    [status.updatedAt, snapshotUpdatedAt].every((stamp) => {
      const age = now - Date.parse(stamp);
      return Number.isFinite(age) && age >= 0 && age < 5000;
    })
  );
}

export function lanReviewCurrent(review: LANReview, now: number): boolean {
  return (
    [
      "activate",
      "remove",
      "settle-remove",
      "cleanup-remove",
      "cleanup-settle",
    ].includes(review.purpose) &&
    review.available &&
    /^[a-f0-9]{32}$/.test(review.reviewId) &&
    /^[a-f0-9]{64}$/.test(review.reviewSha256) &&
    Number.isFinite(Date.parse(review.expiresAt)) &&
    Date.parse(review.expiresAt) > now &&
    Date.parse(review.expiresAt) <= now + 60_000
  );
}

export function lanReviewOperation(purpose: LANReview["purpose"]) {
  switch (purpose) {
    case "activate":
      return "lan-activate";
    case "remove":
      return "lan-remove";
    case "settle-remove":
      return "lan-settle-remove";
    case "cleanup-remove":
    case "cleanup-settle":
      return "lan-cleanup";
    default:
      throw new Error("Unknown LAN review purpose.");
  }
}

export function lanReason(code?: string): string {
  const reasons: Record<string, string> = {
    "publication-v2-unavailable":
      "The backend cannot yet verify LAN port publications.",
    "helper-unregistered": "LAN setup is unavailable in this build.",
    "interface-changed":
      "The selected network connection changed. Read the choices again.",
    "public-profile-refused": "Choose a Private or Domain network connection.",
    "ownership-unconfirmed":
      "Rule ownership is unconfirmed. Inspect the retained operation.",
    "review-expired": "This review expired. Request a fresh review.",
  };
  return (
    (code && reasons[code]) ||
    "LAN access is unavailable. Read the current status again."
  );
}
