import type { Operation } from "../backend";

export type MigrationProjectMember = {
  container: string;
  image?: string;
  project: string;
  service: string;
  workingDirectory: string;
  configFilesLabel: string;
  explicitEnvFiles: boolean;
  bindMounts: boolean;
  oneOff: boolean;
  volumes: string[];
  mounts?: {
    kind: "volume" | "bind" | "tmpfs" | "other";
    volume?: string;
    target: string;
    readOnly: boolean;
    mappingSupported?: boolean;
  }[];
};

export type MigrationImageReview = {
  version: 1;
  source: {
    kind: "docker-desktop" | "rancher-desktop";
    daemon: string;
    engineVersion: string;
    apiVersion: string;
    product: string;
    wsl?: { registration: string; name: string; machine: string };
  };
  target: string;
  images: { id: string; tags: string[]; bytes: number }[];
  volumes: {
    name: string;
    driver: string;
    scope: string;
    plainLocal: boolean;
  }[];
  projectMembers: MigrationProjectMember[];
  warnings: string[];
};
const bounded = (value: unknown, limit: number): value is string =>
  typeof value === "string" && value.length <= limit && !/[\0\r\n]/.test(value);
const id = (value: unknown): value is string =>
  typeof value === "string" && /^sha256:[a-f0-9]{64}$/.test(value);
const registration = (value: unknown): value is string =>
  typeof value === "string" &&
  /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(value);
export function parseMigrationWSLSources(
  text: string,
): { id: string; name: string }[] {
  const value = JSON.parse(text) as {
    version: number;
    registrations: { id: string; name: string }[];
  };
  if (
    !value ||
    value.version !== 1 ||
    !Array.isArray(value.registrations) ||
    value.registrations.length > 128
  )
    throw new Error("Invalid WSL source choices.");
  const ids = new Set<string>();
  for (const item of value.registrations) {
    if (
      !item ||
      !registration(item.id) ||
      ids.has(item.id) ||
      !bounded(item.name, 128) ||
      !item.name ||
      /^(orbwin|lima-orbwin|rancher-desktop|lima-rd|docker-desktop)/i.test(
        item.name,
      )
    )
      throw new Error("Invalid or protected WSL source choice.");
    ids.add(item.id);
  }
  return value.registrations;
}
export function parseMigrationImages(text: string): MigrationImageReview {
  const value = JSON.parse(text) as MigrationImageReview;
  if (
    !value ||
    value.version !== 1 ||
    !value.source ||
    !["docker-desktop", "rancher-desktop"].includes(value.source.kind) ||
    !bounded(value.source.daemon, 256) ||
    !value.source.daemon ||
    !bounded(value.target, 256) ||
    !value.target ||
    value.source.daemon === value.target ||
    value.source.engineVersion !==
      (value.source.kind === "docker-desktop" ? "29.8.0" : "29.5.3") ||
    !bounded(value.source.apiVersion, 32) ||
    !bounded(value.source.product, 512) ||
    !Array.isArray(value.images) ||
    value.images.length > 4096 ||
    !Array.isArray(value.volumes) ||
    value.volumes.length > 4096 ||
    !Array.isArray(value.projectMembers) ||
    value.projectMembers.length > 4096 ||
    !Array.isArray(value.warnings) ||
    value.warnings.length > 16 ||
    value.warnings.some((warning) => !bounded(warning, 4096))
  )
    throw new Error("Invalid or unsupported migration source review.");
  if (value.source.kind === "rancher-desktop") {
    const wsl = value.source.wsl;
    if (
      !wsl ||
      !registration(wsl.registration) ||
      !bounded(wsl.name, 128) ||
      !wsl.name ||
      !bounded(wsl.machine, 32) ||
      !/^[a-f0-9]{32}$/.test(wsl.machine)
    )
      throw new Error(
        "Rancher source requires its reviewed WSL machine identity.",
      );
  } else if (value.source.wsl !== undefined)
    throw new Error("Desktop source unexpectedly changed transport.");
  const ids = new Set<string>();
  for (const image of value.images) {
    if (
      !image ||
      !id(image.id) ||
      ids.has(image.id) ||
      !Number.isSafeInteger(image.bytes) ||
      image.bytes < 0 ||
      !Array.isArray(image.tags) ||
      image.tags.length > 512 ||
      image.tags.some((tag) => !bounded(tag, 512))
    )
      throw new Error("Invalid source image inventory.");
    ids.add(image.id);
  }
  const names = new Set<string>();
  for (const volume of value.volumes) {
    if (
      !volume ||
      !bounded(volume.name, 255) ||
      !volume.name ||
      names.has(volume.name) ||
      !bounded(volume.driver, 128) ||
      !bounded(volume.scope, 128) ||
      typeof volume.plainLocal !== "boolean" ||
      (volume.plainLocal &&
        (volume.driver !== "local" || volume.scope !== "local"))
    )
      throw new Error("Invalid source volume inventory.");
    names.add(volume.name);
  }
  const members = new Set<string>();
  for (const member of value.projectMembers) {
    if (
      !member ||
      !bounded(member.container, 64) ||
      !/^[a-f0-9]{64}$/.test(member.container) ||
      members.has(member.container) ||
      (member.image !== undefined &&
        member.image !== "" &&
        !id(member.image)) ||
      !bounded(member.project, 255) ||
      !member.project ||
      !bounded(member.service, 255) ||
      !bounded(member.workingDirectory, 4096) ||
      !bounded(member.configFilesLabel, 16384) ||
      typeof member.explicitEnvFiles !== "boolean" ||
      typeof member.bindMounts !== "boolean" ||
      typeof member.oneOff !== "boolean" ||
      !Array.isArray(member.volumes) ||
      member.volumes.length > 512 ||
      member.volumes.some((volume) => !bounded(volume, 255) || !volume)
    )
      throw new Error("Invalid source Compose project inventory.");
    if (
      member.mounts !== undefined &&
      (!Array.isArray(member.mounts) ||
        member.mounts.length > 512 ||
        member.mounts.some(
          (mount) =>
            !mount ||
            !["volume", "bind", "tmpfs", "other"].includes(mount.kind) ||
            !bounded(mount.target, 4096) ||
            !mount.target ||
            typeof mount.readOnly !== "boolean" ||
            (mount.mappingSupported !== undefined &&
              typeof mount.mappingSupported !== "boolean") ||
            (mount.kind === "volume"
              ? !bounded(mount.volume, 255) ||
                !mount.volume ||
                !member.volumes.includes(mount.volume)
              : mount.volume !== undefined),
        ))
    )
      throw new Error("Invalid source project mount inventory.");
    members.add(member.container);
  }
  return value;
}

export type MigrationVolumeReview = {
  version: 1;
  source: MigrationImageReview["source"];
  target: string;
  volume: string;
  fingerprint: string;
  createdAt: string;
  quiescentObserved: true;
  message: string;
};
export function parseMigrationVolumeReview(
  text: string,
  inventory: MigrationImageReview,
  volume: string,
): MigrationVolumeReview {
  const value = JSON.parse(text) as MigrationVolumeReview;
  if (
    !value ||
    value.version !== 1 ||
    !value.source ||
    value.source.daemon !== inventory.source.daemon ||
    value.source.kind !== inventory.source.kind ||
    value.source.engineVersion !== inventory.source.engineVersion ||
    (inventory.source.kind === "rancher-desktop"
      ? !value.source.wsl ||
        !inventory.source.wsl ||
        value.source.wsl.registration !== inventory.source.wsl.registration ||
        value.source.wsl.machine !== inventory.source.wsl.machine ||
        value.source.wsl.name !== inventory.source.wsl.name
      : value.source.wsl !== undefined) ||
    value.target !== inventory.target ||
    value.volume !== volume ||
    !inventory.volumes.some(
      (item) => item.name === volume && item.plainLocal,
    ) ||
    !bounded(value.fingerprint, 64) ||
    !/^[a-f0-9]{64}$/.test(value.fingerprint) ||
    !bounded(value.createdAt, 128) ||
    !value.createdAt ||
    value.quiescentObserved !== true ||
    !bounded(value.message, 4096)
  )
    throw new Error(
      "Source volume review changed or is not eligible for export.",
    );
  return value;
}
export function migrationVolumeExport(
  review: MigrationVolumeReview,
): Operation {
  if (review.source.kind === "rancher-desktop") {
    const wsl = review.source.wsl;
    if (!wsl) throw new Error("Review the Rancher WSL connection first.");
    return {
      kind: "exportRancherMigrationVolume",
      source: review.source.daemon,
      registration: wsl.registration,
      machine: wsl.machine,
      volume: review.volume,
      fingerprint: review.fingerprint,
      destination: "",
    };
  }
  return {
    kind: "exportDesktopMigrationVolume",
    source: review.source.daemon,
    volume: review.volume,
    fingerprint: review.fingerprint,
    destination: "",
  };
}
export function migrationImageExport(
  review: MigrationImageReview,
  imageID: string,
  reference: string,
): Operation {
  const image = review.images.find((image) => image.id === imageID);
  if (!image || (reference !== image.id && !image.tags.includes(reference)))
    throw new Error(
      "Select an image/reference from the current source review.",
    );
  if (review.source.kind === "rancher-desktop") {
    const wsl = review.source.wsl;
    if (!wsl) throw new Error("Review the Rancher WSL connection first.");
    return {
      kind: "exportRancherMigrationImage",
      source: review.source.daemon,
      registration: wsl.registration,
      machine: wsl.machine,
      image: image.id,
      reference,
      destination: "",
    };
  }
  return {
    kind: "exportDesktopMigrationImage",
    source: review.source.daemon,
    image: image.id,
    reference,
    destination: "",
  };
}
