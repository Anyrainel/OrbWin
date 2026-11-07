export type ImageExportReceipt = {
  transaction: string;
  createdAt: string;
  sourceKind: string;
  sourceDaemon: string;
  image: string;
  reference: string;
  destination: string;
  uncertain: boolean;
  exportAcknowledged: boolean;
  sha256?: string;
  configIds?: string[];
  imports?: { transaction: string; phase: string; verifiedConfigs: number }[];
};
export type ImageExportHistory = {
  target?: string;
  records: ImageExportReceipt[];
  unavailableImportReceipts: number;
};
const token = (v: unknown): v is string =>
  typeof v === "string" && /^[a-f0-9]{32}$/.test(v);
const imageID = (v: unknown): v is string =>
  typeof v === "string" && /^sha256:[a-f0-9]{64}$/.test(v);
const phases = new Set([
  "intent",
  "loading",
  "verified",
  "previous-intent",
  "previous-retained",
  "tagging",
  "tag-intent",
  "tagged",
  "complete",
]);
export function parseImageExportHistory(
  text: string,
  expectedTarget?: string,
): ImageExportHistory {
  if (text.length > 2 * 1024 * 1024)
    throw new Error("Image history exceeds its response budget.");
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
      "Image history does not match the reviewed target. Refresh the source review.",
    );
  if (
    !value ||
    !Array.isArray(value.records) ||
    value.records.length > 256 ||
    !Number.isInteger(value.unavailableImportReceipts) ||
    value.unavailableImportReceipts < 0 ||
    value.unavailableImportReceipts > 256
  )
    throw new Error("Invalid image export history.");
  const seen = new Set<string>();
  for (const row of value.records as ImageExportReceipt[]) {
    if (
      !row ||
      !token(row.transaction) ||
      seen.has(row.transaction) ||
      typeof row.uncertain !== "boolean" ||
      typeof row.exportAcknowledged !== "boolean" ||
      ![
        row.createdAt,
        row.sourceKind,
        row.sourceDaemon,
        row.image,
        row.reference,
        row.destination,
      ].every(
        (v) =>
          typeof v === "string" &&
          v.length <= 4096 &&
          !/[\x00-\x1f\x7f]/.test(v),
      )
    )
      throw new Error("Invalid image export receipt.");
    seen.add(row.transaction);
    if (row.uncertain) {
      if (
        row.exportAcknowledged ||
        row.createdAt ||
        row.sourceKind ||
        row.sourceDaemon ||
        row.image ||
        row.reference ||
        row.destination ||
        row.sha256 !== undefined ||
        row.configIds !== undefined ||
        row.imports !== undefined
      )
        throw new Error(
          "Unverified image receipt contains unverified metadata.",
        );
      continue;
    }
    if (
      !Number.isFinite(Date.parse(row.createdAt)) ||
      !["docker-desktop", "rancher-desktop"].includes(row.sourceKind) ||
      !row.sourceDaemon ||
      !imageID(row.image) ||
      !row.reference ||
      !row.destination
    )
      throw new Error("Incomplete image export receipt.");
    if (!row.exportAcknowledged) {
      if (
        row.sha256 !== undefined ||
        row.configIds !== undefined ||
        row.imports !== undefined
      )
        throw new Error(
          "Unacknowledged export cannot establish import linkage.",
        );
      continue;
    }
    if (
      typeof row.sha256 !== "string" ||
      !/^[a-f0-9]{64}$/.test(row.sha256) ||
      !Array.isArray(row.configIds) ||
      row.configIds.length < 1 ||
      row.configIds.length > 128 ||
      !row.configIds.every(imageID) ||
      new Set(row.configIds).size !== row.configIds.length
    )
      throw new Error("Invalid exported archive provenance.");
    if (row.imports !== undefined) {
      if (!Array.isArray(row.imports) || row.imports.length > 256)
        throw new Error("Invalid historical import linkage.");
      const imports = new Set<string>();
      for (const linked of row.imports) {
        if (
          !linked ||
          !token(linked.transaction) ||
          imports.has(linked.transaction) ||
          !phases.has(linked.phase) ||
          !Number.isInteger(linked.verifiedConfigs) ||
          linked.verifiedConfigs < 0 ||
          linked.verifiedConfigs > row.configIds.length ||
          (linked.phase === "complete" &&
            linked.verifiedConfigs !== row.configIds.length)
        )
          throw new Error("Invalid historical import acknowledgement.");
        imports.add(linked.transaction);
      }
    }
  }
  return value;
}
