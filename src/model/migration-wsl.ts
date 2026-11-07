import { parseMigrationWSLSources } from "./migration-images.ts";
import type { migrationProjectReview } from "./migration-projects";
import type { Operation } from "../backend";

export type ProjectConnection = {
  name: string;
  registration: string;
  source: string;
  signature: string;
};
const path = (value: unknown): value is string =>
  typeof value === "string" &&
  value.length <= 4096 &&
  value.startsWith("/home/") &&
  !/[\p{Cc}\\]/u.test(value) &&
  value
    .split("/")
    .slice(1)
    .every((part) => part !== "" && part !== "." && part !== "..");
export function parseProjectConnection(
  value: unknown,
  choice: { id: string; name: string },
): ProjectConnection {
  parseMigrationWSLSources(
    JSON.stringify({ version: 1, registrations: [choice] }),
  );
  const status = value as {
    connected?: boolean;
    export?: { source?: string; signature?: string };
  } | null;
  if (
    status?.connected !== true ||
    !path(status.export?.source) ||
    typeof status.export?.signature !== "string" ||
    status.export.signature.length > 64 ||
    !/^[0-9]+:[0-9]+$/.test(status.export.signature)
  )
    throw new Error(
      "No connected project was verified. Connect the intended directory from WSL distributions, then inspect it again here.",
    );
  return {
    name: choice.name,
    registration: choice.id,
    source: status.export.source,
    signature: status.export.signature,
  };
}
export function migrationWslReview(
  review: ReturnType<typeof migrationProjectReview>,
  connection: ProjectConnection,
) {
  // The socket connection's distro is irrelevant to project ownership. Only an
  // explicit project-distro choice and its current connected export are used.
  parseProjectConnection(
    {
      connected: true,
      export: { source: connection.source, signature: connection.signature },
    },
    { id: connection.registration, name: connection.name },
  );
  const inside = (value: string) =>
    path(value) &&
    (value === connection.source || value.startsWith(connection.source + "/"));
  const first = review.members[0];
  const files = first.configFilesLabel.split(",");
  const coherent =
    review.members.every(
      (member) =>
        member.configFilesLabel === first.configFilesLabel &&
        member.workingDirectory === first.workingDirectory &&
        !member.explicitEnvFiles,
    ) &&
    files.length <= 16 &&
    files.every(inside) &&
    inside(first.workingDirectory);
  const inputs: Extract<Operation, { kind: "wslCompose" }> = {
    kind: "wslCompose",
    action: "validate",
    distribution: connection.name,
    connection: {
      registration: connection.registration,
      source: connection.source,
      signature: connection.signature,
    },
    project: review.inputs.project,
    file: coherent ? files[0] : "",
    overrides: coherent ? files.slice(1) : [],
    directory: coherent ? first.workingDirectory : "",
    profiles: [],
    services: [],
  };
  return {
    inputs,
    mappingFile: undefined as { path: string; sha256: string } | undefined,
    suggestedFiles: coherent,
    message: coherent
      ? "The ordered Linux paths agree and lie inside the explicitly selected project. Review the files before validation."
      : "Source labels do not establish a coherent file set inside this connected root, or indicate explicit env files. Enter reviewed Linux Compose and interpolation env-file paths manually; no file set was guessed.",
  };
}
export function appendWslMapping(
  prepared: ReturnType<typeof migrationWslReview>,
  connection: ProjectConnection,
  file: string,
  sha256?: string,
) {
  const overrides = prepared.inputs.overrides ?? [];
  if (
    !path(file) ||
    !file.startsWith(connection.source + "/") ||
    prepared.inputs.connection?.registration !== connection.registration ||
    prepared.inputs.connection.source !== connection.source ||
    prepared.inputs.connection.signature !== connection.signature ||
    overrides.length >= 15 ||
    prepared.inputs.file === file ||
    overrides.includes(file)
  )
    throw new Error(
      "Mapping file does not match the current project review or exceeds its ordered file limit.",
    );
  return {
    ...prepared,
    mappingFile: sha256 ? { path: file, sha256 } : undefined,
    inputs: {
      ...prepared.inputs,
      overrides: [...overrides, file],
    },
    message:
      prepared.message +
      " The newly saved mapping override is appended last; review and Validate before any separate Up.",
  };
}
export function parseWslMappingSave(
  text: string,
  connection: ProjectConnection,
  filename: string,
) {
  if (text.length > 65536)
    throw new Error("Mapping save response exceeded its limit.");
  const value = JSON.parse(text);
  if (
    !value ||
    value.version !== 1 ||
    value.path !== `${connection.source}/${filename}` ||
    value.signature !== connection.signature ||
    typeof value.sha256 !== "string" ||
    !/^[a-f0-9]{64}$/.test(value.sha256)
  )
    throw new Error(
      "Mapping save response does not match the reviewed project.",
    );
  return { path: value.path as string, sha256: value.sha256 as string };
}
