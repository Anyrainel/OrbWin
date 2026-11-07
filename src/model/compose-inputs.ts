import { validWslComposeInputs } from "./migration-wsl-draft.ts";
// Device-local reviewed hints only. No YAML, environment values or engine state.
import { validMigrationDraft, type MigrationDraft } from "./migration-draft.ts";
export type ComposeInputs = {
  kind?: undefined;
  project: string;
  file: string;
  overrides: string[];
  envFiles?: string[];
  profiles: string[];
  services?: string[];
  directory: string;
  migration?: MigrationDraft;
};
export type WslComposeInputs = Omit<ComposeInputs, "kind"> & {
  kind: "wsl";
  distribution: string;
  connection: { registration: string; source: string; signature: string };
};
export type RememberedComposeInputs = ComposeInputs | WslComposeInputs;
const key = "orbwin.compose.inputs.v1";
function valid(value: unknown): value is RememberedComposeInputs {
  if (
    value &&
    typeof value === "object" &&
    (value as { kind?: unknown }).kind === "wsl"
  )
    return validWslComposeInputs(value);
  if (!value || typeof value !== "object") return false;
  const item = value as ComposeInputs;
  if (
    Object.keys(item).some(
      (key) =>
        ![
          "project",
          "file",
          "overrides",
          "envFiles",
          "profiles",
          "services",
          "directory",
          "migration",
        ].includes(key),
    )
  )
    return false;
  const text = (v: unknown) =>
    typeof v === "string" && v.length <= 32767 && !/[\r\n\0]/.test(v);
  const list = (v: unknown, max: number) =>
    Array.isArray(v) && v.length <= max && v.every(text);
  return (
    text(item.project) &&
    item.project.length > 0 &&
    item.project.length <= 256 &&
    text(item.file) &&
    text(item.directory) &&
    list(item.overrides, 15) &&
    (item.envFiles === undefined || list(item.envFiles, 16)) &&
    list(item.profiles, 16) &&
    (item.services === undefined || list(item.services, 64)) &&
    (item.migration === undefined || validMigrationDraft(item.migration))
  );
}
function readStored(): RememberedComposeInputs[] {
  const raw = localStorage.getItem(key);
  if (!raw) return [];
  if (raw.length > 2_000_000)
    throw Error(
      "Remembered project inputs exceed their storage bounds; existing bytes were retained.",
    );
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    throw Error("Remembered inputs are corrupt; existing bytes were retained.");
  }
  if (!Array.isArray(value) || value.length > 20 || !value.every(valid))
    throw Error(
      "Remembered inputs contain invalid records; existing bytes were retained.",
    );
  return value;
}
export function composeInputsProblem(): string {
  try {
    readStored();
    return "";
  } catch (error) {
    return String(error);
  }
}
export function loadComposeInputs(): RememberedComposeInputs[] {
  try {
    return structuredClone(readStored());
  } catch {
    return [];
  }
}
export function rememberComposeInputs(inputs: RememberedComposeInputs) {
  if (!valid(inputs)) throw Error("Project inputs could not be remembered.");
  const previous = readStored();
  const raw = JSON.stringify(
    [
      inputs,
      ...previous.filter(
        (item) => composeInputKey(item) !== composeInputKey(inputs),
      ),
    ].slice(0, 20),
  );
  if (raw.length > 2_000_000)
    throw new Error(
      "Remembered project inputs exceed the device storage limit. Forget older inputs first.",
    );
  localStorage.setItem(key, raw);
}
export function forgetComposeInputs(project: string) {
  const previous = readStored();
  localStorage.setItem(
    key,
    JSON.stringify(
      previous.filter(
        (item) => composeInputKey(item) !== project && item.project !== project,
      ),
    ),
  );
}

// A bookmark identity, never a transaction/engine authority. Same-named legacy
// projects and migrations from different source/target pairs cannot overwrite.
export function composeInputKey(inputs: RememberedComposeInputs) {
  const origin = inputs.migration;
  return JSON.stringify(
    origin
      ? [
          "migration-v1",
          origin.source.kind,
          origin.source.daemon,
          origin.source.project,
          origin.target,
          inputs.project,
          ...(inputs.kind === "wsl"
            ? [
                "wsl",
                inputs.connection.registration,
                inputs.connection.source,
                inputs.connection.signature,
              ]
            : []),
        ]
      : ["compose", inputs.project],
  );
}
