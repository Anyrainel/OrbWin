import type { Resource } from "../../backend";
import { isKubernetesContainer } from "../../model/resources.ts";

// Cleanup candidates are derived only from the current inventory snapshot. No
// broad prune exists in the native adapter; each selected item is removed with
// the existing single-resource `remove` operation, which the engine may reject
// (for example a volume still attached to a container).

export type CleanupKind = "containers" | "images" | "networks" | "volumes";
export type Candidate = {
  kind: CleanupKind;
  target: string;
  label: string;
  detail: string;
};

const builtInNetworks = new Set(["bridge", "host", "none"]);

function text(value: unknown) {
  return typeof value === "string" ? value : "";
}

export function stoppedContainers(rows: Resource[]): Candidate[] {
  return rows
    .filter(
      (row) =>
        !isKubernetesContainer(row) &&
        ["exited", "created", "dead"].includes(text(row.State).toLowerCase()),
    )
    .map((row) => ({
      kind: "containers",
      target: text(row.ID),
      label: text(row.Names) || text(row.ID).slice(0, 12),
      detail: [text(row.Image), text(row.Status)].filter(Boolean).join(" · "),
    }));
}

export function danglingImages(rows: Resource[]): Candidate[] {
  return rows
    .filter(
      (row) => text(row.Repository) === "<none>" && text(row.Tag) === "<none>",
    )
    .map((row) => ({
      kind: "images",
      target: text(row.ID),
      label: text(row.ID)
        .replace(/^sha256:/, "")
        .slice(0, 12),
      detail: [text(row.Size), text(row.CreatedSince)]
        .filter(Boolean)
        .join(" · "),
    }));
}

export function customNetworks(rows: Resource[]): Candidate[] {
  return rows
    .filter((row) => !builtInNetworks.has(text(row.Name)))
    .map((row) => ({
      kind: "networks",
      target: text(row.ID),
      label: text(row.Name),
      detail: [text(row.Driver), text(row.Scope)].filter(Boolean).join(" · "),
    }));
}

// `docker ps --format {{json .}}` reports Mounts as a comma-separated list of
// volume names (bind sources are paths). Volumes absent from every container's
// list are unattached in this snapshot; the engine still enforces in-use checks.
export function attachedVolumes(containers: Resource[]): Set<string> {
  const names = new Set<string>();
  for (const row of containers)
    for (const mount of text(row.Mounts).split(","))
      if (mount.trim()) names.add(mount.trim());
  return names;
}
export function unattachedVolumes(
  volumes: Resource[],
  containers: Resource[],
): Candidate[] {
  const attached = attachedVolumes(containers);
  return volumes
    .filter((row) => text(row.Name) && !attached.has(text(row.Name)))
    .map((row) => ({
      kind: "volumes",
      target: text(row.Name),
      label: text(row.Name),
      detail: text(row.Driver),
    }));
}

const units: Record<string, number> = {
  b: 1,
  kb: 1e3,
  mb: 1e6,
  gb: 1e9,
  tb: 1e12,
};
// Docker CLI renders image sizes with decimal units ("15.2MB").
export function parseSize(value: unknown): number | undefined {
  const match = text(value)
    .trim()
    .match(/^([\d.]+)\s*([kmgt]?b)$/i);
  if (!match) return undefined;
  return Number(match[1]) * units[match[2].toLowerCase()];
}
export function formatSize(bytes: number) {
  const steps = ["B", "kB", "MB", "GB", "TB"];
  let value = bytes;
  let index = 0;
  while (value >= 1000 && index < steps.length - 1) {
    value /= 1000;
    index++;
  }
  return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${steps[index]}`;
}
export function imageSizeTotal(images: Resource[]) {
  let total = 0;
  let unknown = 0;
  for (const row of images) {
    const size = parseSize(row.Size);
    if (size === undefined) unknown++;
    else total += size;
  }
  return { total, unknown };
}
