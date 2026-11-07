export type SourceVolumeReceipt = {
  transaction: string;
  createdAt: string;
  sourceKind: string;
  sourceDaemon: string;
  volume: string;
  destination: string;
  phase: string;
  exportAcknowledged: boolean;
  uncertain: boolean;
  manifestSha256?: string;
  payloadSha256?: string;
  restores?: { transaction: string; volume: string; phase: string }[];
};
const phases = new Set([
  "intent",
  "importing",
  "image",
  "creating",
  "container",
  "exported",
  "helper-removed",
  "image-removed",
  "cleaned",
]);
export function parseSourceVolumeHistory(
  text: string,
  expectedTarget?: string,
): SourceVolumeReceipt[] {
  if (text.length > 2 * 1024 * 1024)
    throw new Error("Source history exceeds its response budget.");
  const value = JSON.parse(text);
  if (
    !value ||
    (expectedTarget !== undefined &&
      (!expectedTarget || value.target !== expectedTarget)) ||
    (value.target !== undefined &&
      (typeof value.target !== "string" ||
        !value.target ||
        value.target.length > 256 ||
        /[\x00-\x1f\x7f]/.test(value.target)))
  )
    throw new Error(
      "Volume history does not match the reviewed target. Refresh the source review.",
    );
  if (!value || !Array.isArray(value.records) || value.records.length > 256)
    throw new Error("Invalid source export history.");
  const seen = new Set<string>();
  return value.records.map((row: SourceVolumeReceipt) => {
    if (
      !row ||
      typeof row.transaction !== "string" ||
      !/^[a-f0-9]{32}$/.test(row.transaction) ||
      seen.has(row.transaction) ||
      typeof row.uncertain !== "boolean" ||
      typeof row.exportAcknowledged !== "boolean" ||
      ![
        row.createdAt,
        row.sourceKind,
        row.sourceDaemon,
        row.volume,
        row.destination,
        row.phase,
      ].every(
        (v) =>
          typeof v === "string" &&
          v.length <= 4096 &&
          !/[\x00-\x1f\x7f]/.test(v),
      )
    )
      throw new Error("Invalid source export receipt.");
    if (
      !row.uncertain &&
      (!phases.has(row.phase) ||
        !["docker-desktop", "rancher-desktop"].includes(row.sourceKind) ||
        !Number.isFinite(Date.parse(row.createdAt)) ||
        !row.sourceDaemon ||
        !row.volume ||
        !row.destination)
    )
      throw new Error("Incomplete source export receipt.");
    if (
      row.uncertain &&
      (row.exportAcknowledged ||
        row.phase ||
        row.destination ||
        row.sourceDaemon ||
        row.sourceKind ||
        row.volume ||
        row.createdAt)
    )
      throw new Error("Uncertain receipt contains unverified metadata.");
    const hasProvenance =
      row.manifestSha256 !== undefined || row.payloadSha256 !== undefined;
    if (
      hasProvenance &&
      (row.uncertain ||
        !row.exportAcknowledged ||
        !/^[a-f0-9]{64}$/.test(row.manifestSha256 ?? "") ||
        !/^[a-f0-9]{64}$/.test(row.payloadSha256 ?? ""))
    )
      throw new Error("Invalid source manifest provenance.");
    if (row.restores !== undefined) {
      const restores = new Set<string>();
      if (
        !hasProvenance ||
        !Array.isArray(row.restores) ||
        row.restores.length > 256
      )
        throw new Error(
          "Restore linkage requires bounded manifest provenance.",
        );
      for (const restore of row.restores) {
        if (
          !restore ||
          !/^[a-f0-9]{32}$/.test(restore.transaction) ||
          restores.has(restore.transaction) ||
          typeof restore.volume !== "string" ||
          !/^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$/.test(restore.volume) ||
          ![
            "intent",
            "created",
            "importing",
            "verifying",
            "complete",
            "discarded",
          ].includes(restore.phase)
        )
          throw new Error("Invalid historical restore linkage.");
        restores.add(restore.transaction);
      }
    }
    seen.add(row.transaction);
    return { ...row };
  });
}

export function parseSourceVolumeHistoryEnvelope(
  text: string,
  expectedTarget: string,
) {
  const records = parseSourceVolumeHistory(text, expectedTarget);
  // The strict parser above verified the response's own target, not a guess
  // from whichever engine happens to be current when a renderer reopens.
  return { target: expectedTarget, records };
}
