import type { ComposeInputs } from "./compose-inputs";
import { migrationProjectReview } from "./migration-projects.ts";
import {
  reviewMigrationMounts,
  migrationMountDraft,
  type MigrationMount,
} from "./migration-mounts.ts";
import { parseMigrationImages } from "./migration-images.ts";

export type MigrationDraft = {
  version: 1;
  source: {
    kind: "docker-desktop" | "rancher-desktop";
    daemon: string;
    project: string;
    wsl?: { registration: string; machine: string };
  };
  target: string;
  members: string[];
  images: string[];
  unknownImages: number;
  mounts: MigrationMount[];
  mapping?: {
    namespace: string;
    selections: Record<string, string>;
    file: { path: string; sha256: string };
  };
};
const text = (value: unknown, max = 256): value is string =>
  typeof value === "string" &&
  value.length > 0 &&
  value.length <= max &&
  !/[\x00-\x1f\x7f]/.test(value);
const token = (value: unknown): value is string =>
  typeof value === "string" && /^[a-f0-9]{64}$/.test(value);
const name = (value: unknown): value is string =>
  typeof value === "string" && /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$/.test(value);
const windows = (value: unknown): value is string =>
  text(value, 32767) &&
  /^[a-z]:[\\/]/i.test(value) &&
  !value.slice(2).includes(":") &&
  !value.split(/[\\/]/).some((part) => part === "." || part === "..");
const pathKey = (value: string) => value.replaceAll("\\", "/").toLowerCase();
export function validMigrationDraft(
  value: unknown,
  mappingPath = windows,
): value is MigrationDraft {
  try {
    const v = value as MigrationDraft;
    const fields = (value: object, names: string[]) =>
      Object.keys(value).every((key) => names.includes(key));
    if (
      !v ||
      JSON.stringify(v).length > 256 * 1024 ||
      !fields(v, [
        "version",
        "source",
        "target",
        "members",
        "images",
        "unknownImages",
        "mounts",
        "mapping",
      ])
    )
      return false;
    if (
      !v ||
      v.version !== 1 ||
      !v.source ||
      !["docker-desktop", "rancher-desktop"].includes(v.source.kind) ||
      !text(v.source.daemon) ||
      !text(v.source.project) ||
      !text(v.target) ||
      v.source.daemon === v.target
    )
      return false;
    if (
      v.source.kind === "rancher-desktop" &&
      (!v.source.wsl ||
        !/^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/.test(
          v.source.wsl.registration,
        ) ||
        !/^[a-f0-9]{32}$/.test(v.source.wsl.machine))
    )
      return false;
    if (
      !fields(v.source, ["kind", "daemon", "project", "wsl"]) ||
      (v.source.wsl && !fields(v.source.wsl, ["registration", "machine"])) ||
      (v.source.kind === "docker-desktop" && v.source.wsl !== undefined)
    )
      return false;
    if (
      !Number.isInteger(v.unknownImages) ||
      v.unknownImages < 0 ||
      v.unknownImages > 256 ||
      !Array.isArray(v.members) ||
      !v.members.length ||
      v.members.length > 256 ||
      !v.members.every(token) ||
      new Set(v.members).size !== v.members.length ||
      !Array.isArray(v.images) ||
      (!v.images.length && !v.unknownImages) ||
      v.images.length > 256 ||
      !v.images.every(
        (id) => typeof id === "string" && /^sha256:[a-f0-9]{64}$/.test(id),
      ) ||
      new Set(v.images).size !== v.images.length ||
      !Array.isArray(v.mounts) ||
      v.mounts.length > 256
    )
      return false;
    for (const mount of v.mounts)
      if (
        !mount ||
        !fields(mount, ["service", "target", "sourceVolume", "readOnly"]) ||
        !name(mount.service) ||
        !name(mount.sourceVolume) ||
        !text(mount.target, 4096) ||
        !mount.target.startsWith("/") ||
        mount.target === "/" ||
        mount.target.includes("//") ||
        mount.target.endsWith("/") ||
        mount.target.split("/").some((p) => p === "." || p === "..") ||
        typeof mount.readOnly !== "boolean"
      )
        return false;
    if (v.mapping) {
      if (
        !v.mapping.selections ||
        typeof v.mapping.selections !== "object" ||
        Array.isArray(v.mapping.selections)
      )
        return false;
      if (
        !fields(v.mapping, ["namespace", "selections", "file"]) ||
        !fields(v.mapping.file, ["path", "sha256"]) ||
        !mappingPath(v.mapping.file?.path) ||
        !v.mapping.file.path.toLowerCase().endsWith(".json") ||
        !token(v.mapping.file.sha256)
      )
        return false;
      const keys = Object.keys(v.mapping.selections);
      if (
        keys.length > 256 ||
        keys.some((key) => !name(key) || !name(v.mapping!.selections[key]))
      )
        return false;
      migrationMountDraft(
        v.mounts,
        v.mapping.selections,
        v.mapping.namespace,
        Object.values(v.mapping.selections),
      );
    }
    return true;
  } catch {
    return false;
  }
}
export function migrationDraft(
  project: ReturnType<typeof migrationProjectReview>,
  mapping?: MigrationDraft["mapping"],
  mappingPath = windows,
): MigrationDraft {
  const ordinary = project.members.filter((member) => !member.oneOff);
  const result: MigrationDraft = {
    version: 1,
    source: {
      kind: project.source.kind,
      daemon: project.source.daemon,
      project: project.inputs.project,
      ...(project.source.wsl
        ? {
            wsl: {
              registration: project.source.wsl.registration,
              machine: project.source.wsl.machine,
            },
          }
        : {}),
    },
    target: project.target,
    members: ordinary.map((member) => member.container).sort(),
    images: project.images.map((image) => image.id).sort(),
    unknownImages: project.unknownImages,
    mounts: reviewMigrationMounts(project.members),
    ...(mapping ? { mapping: structuredClone(mapping) } : {}),
  };
  if (!validMigrationDraft(result, mappingPath))
    throw Error(
      "Migration provenance exceeds its supported bounds or is incomplete.",
    );
  return result;
}
export function draftMappingContents(
  draft: MigrationDraft,
  mappingPath = windows,
): string {
  if (!validMigrationDraft(draft, mappingPath) || !draft.mapping)
    throw Error(
      "Review and save the complete volume mapping before starting this migration draft.",
    );
  return migrationMountDraft(
    draft.mounts,
    draft.mapping.selections,
    draft.mapping.namespace,
    Object.values(draft.mapping.selections),
  );
}
export function assertMigrationInputs(
  draft: MigrationDraft,
  inputs: Pick<ComposeInputs, "file" | "overrides" | "directory">,
  action: string,
) {
  if (!["validate", "up"].includes(action))
    throw Error(
      "Saved migration drafts support separately reviewed Validate or Up only.",
    );
  if (
    !validMigrationDraft(draft) ||
    !windows(inputs.file) ||
    inputs.overrides.some((path) => !windows(path)) ||
    (inputs.directory !== "" && !windows(inputs.directory))
  )
    throw Error("Migration drafts require reviewed absolute Windows paths.");
  if (action !== "validate" && draft.unknownImages)
    throw Error(
      "Source image identity is incomplete. Refresh and review the project before Up.",
    );
  if (draft.mapping) {
    const paths = inputs.overrides.map(pathKey),
      expected = pathKey(draft.mapping.file.path);
    if (
      paths.at(-1) !== expected ||
      paths.filter((path) => path === expected).length !== 1 ||
      pathKey(inputs.file) === expected
    )
      throw Error(
        "The saved migration mapping must remain exactly once as the final override. Review and save a new draft after changing it.",
      );
  } else if (action !== "validate" && draft.mounts.length)
    throw Error(
      "This draft has no saved complete volume mapping. Return to source project review before Up.",
    );
}

export function recheckDraftSource(
  draft: MigrationDraft,
  text: string,
  mappingPath = windows,
) {
  if (!validMigrationDraft(draft, mappingPath))
    throw Error("Saved migration provenance is invalid.");
  const inventory = parseMigrationImages(text);
  if (
    inventory.source.kind !== draft.source.kind ||
    inventory.source.daemon !== draft.source.daemon ||
    inventory.target !== draft.target ||
    (draft.source.wsl &&
      (inventory.source.wsl?.registration !== draft.source.wsl.registration ||
        inventory.source.wsl.machine !== draft.source.wsl.machine))
  )
    throw Error(
      "Source or target identity changed. Inspect and review migration again.",
    );
  const current = migrationDraft(
    migrationProjectReview(inventory, draft.source.project),
  );
  if (
    JSON.stringify(current.members) !== JSON.stringify(draft.members) ||
    JSON.stringify(current.images) !== JSON.stringify(draft.images) ||
    current.unknownImages !== draft.unknownImages ||
    JSON.stringify(current.mounts) !== JSON.stringify(draft.mounts)
  )
    throw Error(
      "Observed source project members, images or mounts changed. Review and save a new draft; no workload was started.",
    );
}

export async function recheckMigrationDraft(
  draft: MigrationDraft,
  inputs: Pick<ComposeInputs, "file" | "overrides" | "directory">,
  action: string,
  readSource: () => Promise<string>,
  inspectMapping: (
    file: { path: string; sha256: string },
    contents: string,
    target: string,
  ) => Promise<void>,
  signal: AbortSignal,
) {
  const active = () => {
    if (signal.aborted)
      throw Error(
        "Migration draft check cancelled. No Compose request was dispatched.",
      );
  };
  active();
  assertMigrationInputs(draft, inputs, action);
  recheckDraftSource(draft, await readSource());
  active();
  if (draft.mapping)
    await inspectMapping(
      draft.mapping.file,
      draftMappingContents(draft),
      draft.target,
    );
  active();
}
