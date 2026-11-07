import type { migrationProjectReview } from "./migration-projects";
import {
  parseImageExportHistory,
  type ImageExportHistory,
} from "./migration-image-history.ts";
import {
  parseSourceVolumeHistoryEnvelope,
  type SourceVolumeReceipt,
} from "./migration-history.ts";

export type MigrationAccountingProject = ReturnType<
  typeof migrationProjectReview
>;
export type VolumeHistoryEnvelope = {
  target: string;
  records: SourceVolumeReceipt[];
};
export async function readMigrationAccountingHistories(
  target: string,
  read: (
    kind: "migrationImageHistory" | "migrationVolumeHistory",
  ) => Promise<string>,
  signal: AbortSignal,
) {
  const active = () => {
    if (signal.aborted)
      throw new Error("Project receipt read cancelled. No data was changed.");
  };
  active();
  const images = parseImageExportHistory(
    await read("migrationImageHistory"),
    target,
  );
  active();
  const volumes = parseSourceVolumeHistoryEnvelope(
    await read("migrationVolumeHistory"),
    target,
  );
  active();
  return { images, volumes };
}
const text = (value: unknown): value is string =>
  typeof value === "string" &&
  value.length > 0 &&
  value.length <= 256 &&
  !/[\x00-\x1f\x7f]/.test(value);
const volumeName = (value: unknown): value is string =>
  typeof value === "string" && /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$/.test(value);

// This view derives relationships from owning histories and an explicit observed
// project. It stores no job state, chooses no latest receipt and authorizes no
// mutation. A matching asset receipt need not have originated from this project.
export function migrationAccounting(
  project: MigrationAccountingProject,
  images: ImageExportHistory,
  volumes: VolumeHistoryEnvelope,
  mapping: Record<string, string>,
) {
  if (
    !text(project.source.daemon) ||
    !text(project.target) ||
    !text(project.inputs.project) ||
    project.source.daemon === project.target ||
    !["docker-desktop", "rancher-desktop"].includes(project.source.kind) ||
    images.target !== project.target ||
    volumes.target !== project.target
  )
    throw new Error(
      "Transfer histories do not match the explicitly reviewed source and target. Refresh the source review.",
    );
  if (
    images.records.length > 256 ||
    volumes.records.length > 256 ||
    project.images.length > 4096 ||
    project.members.length > 4096
  )
    throw new Error("Project accounting exceeds its observation budget.");
  const sameSource = (row: {
    sourceKind: string;
    sourceDaemon: string;
    uncertain: boolean;
  }) =>
    !row.uncertain &&
    row.sourceKind === project.source.kind &&
    row.sourceDaemon === project.source.daemon;
  const order = <T extends { transaction: string }>(rows: T[]) =>
    [...rows].sort((a, b) => a.transaction.localeCompare(b.transaction));
  const imageIDs = new Set<string>();
  const imageRows = project.images.map((image) => {
    if (!/^sha256:[a-f0-9]{64}$/.test(image.id) || imageIDs.has(image.id))
      throw new Error(
        "Observed project image identities are invalid or repeated.",
      );
    imageIDs.add(image.id);
    const exports = order(
      images.records.filter((row) => sameSource(row) && row.image === image.id),
    );
    return { id: image.id, services: [...image.services], exports };
  });
  const names = [
    ...new Set(
      project.members
        .filter((member) => !member.oneOff)
        .flatMap((member) => member.volumes),
    ),
  ].sort();
  if (names.some((name) => !volumeName(name)))
    throw new Error("Observed source volume identity is unsupported.");
  const volumeRows = names.map((name) => {
    const selected = Object.hasOwn(mapping, name) ? mapping[name] : "";
    if (typeof selected !== "string" || (selected && !volumeName(selected)))
      throw new Error("Selected destination volume identity is invalid.");
    const exports = order(
      volumes.records.filter((row) => sameSource(row) && row.volume === name),
    );
    return {
      name,
      selected,
      exports: exports.map((row) => ({
        receipt: row,
        matchingRestores: selected
          ? (row.restores ?? []).filter(
              (restore) => restore.volume === selected,
            )
          : [],
      })),
    };
  });
  return {
    sourceKind: project.source.kind,
    sourceDaemon: project.source.daemon,
    target: project.target,
    project: project.inputs.project,
    images: imageRows,
    volumes: volumeRows,
    unknownImages: project.unknownImages,
    uncertainImageReceipts: images.records.filter((row) => row.uncertain)
      .length,
    unavailableImportReceipts: images.unavailableImportReceipts,
    uncertainVolumeReceipts: volumes.records.filter((row) => row.uncertain)
      .length,
  };
}
