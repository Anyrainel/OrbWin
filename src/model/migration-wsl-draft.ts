import type { WslComposeInputs } from "./compose-inputs";
import type { MigrationDraft } from "./migration-draft";
import {
  validMigrationDraft,
  migrationDraft,
  draftMappingContents,
  recheckDraftSource,
} from "./migration-draft.ts";
import {
  parseProjectConnection,
  type ProjectConnection,
} from "./migration-wsl.ts";
import type { migrationProjectReview } from "./migration-projects";
const fields = (value: object, allowed: string[]) =>
  Object.keys(value).every((key) => allowed.includes(key));
export function wslProjectPath(
  connection: Pick<ProjectConnection, "source">,
  raw: unknown,
  directory = "",
): string {
  if (
    typeof raw !== "string" ||
    !raw ||
    raw.length > 4096 ||
    /[\p{Cc}\\]/u.test(raw) ||
    raw.split("/").some((part) => part === "." || part === "..") ||
    raw.includes("//") ||
    raw.endsWith("/")
  )
    throw Error("Use a canonical path inside the reviewed WSL project.");
  const root = connection.source;
  const base = directory ? wslProjectPath(connection, directory) : root;
  const result = raw.startsWith("/") ? raw : `${base}/${raw}`;
  if (result !== root && !result.startsWith(root + "/"))
    throw Error("The path is outside the reviewed WSL project.");
  return result;
}
export function wslDraftPath(connection: Pick<ProjectConnection, "source">) {
  return (path: unknown): path is string => {
    try {
      return (
        typeof path === "string" &&
        path.startsWith(connection.source + "/") &&
        wslProjectPath(connection, path) === path
      );
    } catch {
      return false;
    }
  };
}
export function validWslComposeInputs(
  value: unknown,
): value is WslComposeInputs {
  try {
    const v = value as WslComposeInputs;
    if (
      !v ||
      v.kind !== "wsl" ||
      !fields(v, [
        "kind",
        "distribution",
        "connection",
        "project",
        "file",
        "overrides",
        "envFiles",
        "profiles",
        "services",
        "directory",
        "migration",
      ]) ||
      !v.connection ||
      !fields(v.connection, ["registration", "source", "signature"]) ||
      !v.migration
    )
      return false;
    const connection = parseProjectConnection(
      {
        connected: true,
        export: {
          source: v.connection.source,
          signature: v.connection.signature,
        },
      },
      { id: v.connection.registration, name: v.distribution },
    );
    if (!validMigrationDraft(v.migration, wslDraftPath(connection)))
      return false;
    const text = (value: unknown): value is string =>
      typeof value === "string" &&
      value.length <= 4096 &&
      !/[\p{Cc}]/u.test(value);
    if (
      !text(v.project) ||
      !v.project ||
      v.project.length > 128 ||
      !text(v.file) ||
      !text(v.directory) ||
      !Array.isArray(v.overrides) ||
      v.overrides.length > 15 ||
      !v.overrides.every(text)
    )
      return false;
    for (const [items, limit] of [
      [v.envFiles ?? [], 16],
      [v.profiles, 16],
      [v.services ?? [], 64],
    ] as const)
      if (!Array.isArray(items) || items.length > limit || !items.every(text))
        return false;
    if (v.directory) wslProjectPath(connection, v.directory);
    for (const file of [v.file, ...v.overrides, ...(v.envFiles ?? [])])
      wslProjectPath(connection, file, v.directory);
    if (v.migration.mapping) {
      const file = v.migration.mapping.file.path;
      if (file.slice(connection.source.length + 1).includes("/")) return false;
    }
    return true;
  } catch {
    return false;
  }
}
export function wslMigrationDraft(
  review: ReturnType<typeof migrationProjectReview>,
  connection: ProjectConnection,
  mapping?: MigrationDraft["mapping"],
) {
  return migrationDraft(review, mapping, wslDraftPath(connection));
}
export function wslMappingReview(inputs: WslComposeInputs, action: string) {
  if (!validWslComposeInputs(inputs) || !["validate", "up"].includes(action))
    throw Error(
      "Reopen a valid reviewed WSL migration draft for Validate or Up.",
    );
  const d = inputs.migration!;
  if (action === "up" && (d.unknownImages || (!d.mapping && d.mounts.length)))
    throw Error("Complete source image and volume mapping review before Up.");
  if (!d.mapping) return undefined;
  const files = [inputs.file, ...inputs.overrides].map((file) =>
    wslProjectPath(inputs.connection, file, inputs.directory),
  );
  const expected = d.mapping.file.path;
  if (
    files.at(-1) !== expected ||
    files[0] === expected ||
    files.filter((file) => file === expected).length !== 1
  )
    throw Error("The WSL mapping must remain the unique final override.");
  return {
    file: expected.slice(inputs.connection.source.length + 1),
    sha256: d.mapping.file.sha256,
    contents: draftMappingContents(d, wslDraftPath(inputs.connection)),
  };
}
export async function recheckWslDraft(
  inputs: WslComposeInputs,
  action: string,
  inspect: () => Promise<unknown>,
  readSource: () => Promise<string>,
  signal: AbortSignal,
) {
  const active = () => {
    if (signal.aborted)
      throw Error(
        "WSL draft check cancelled; no Compose request was dispatched.",
      );
  };
  active();
  const mapping = wslMappingReview(inputs, action);
  const observation = await inspect();
  if (
    !observation ||
    typeof observation !== "object" ||
    (observation as { registration?: unknown }).registration !==
      inputs.connection.registration
  )
    throw Error(
      "WSL connection observation does not match the reviewed registration.",
    );
  const current = parseProjectConnection(observation, {
    id: inputs.connection.registration,
    name: inputs.distribution,
  });
  if (
    current.source !== inputs.connection.source ||
    current.signature !== inputs.connection.signature
  )
    throw Error("Connected WSL project changed; inspect and review it again.");
  active();
  recheckDraftSource(
    inputs.migration!,
    await readSource(),
    wslDraftPath(inputs.connection),
  );
  active();
  return mapping;
}
