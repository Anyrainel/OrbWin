import type { MigrationProjectMember } from "./migration-images";

export type MigrationMount = {
  service: string;
  target: string;
  sourceVolume: string;
  readOnly: boolean;
};
const key = (value: string) => /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$/.test(value);
function targetPath(value: string) {
  return (
    value.startsWith("/") &&
    value !== "/" &&
    !value.includes("//") &&
    !value.endsWith("/") &&
    value.length <= 4096 &&
    !/[\0-\x1f\x7f]/.test(value) &&
    !value.split("/").some((part) => part === "." || part === "..")
  );
}
// Repeated replicas must agree on each service/target tuple. Never choose one
// observation arbitrarily, infer anonymous data intent, or include one-off tasks.
export function reviewMigrationMounts(
  members: MigrationProjectMember[],
): MigrationMount[] {
  const result = new Map<string, MigrationMount>();
  const replicas = new Map<string, string>();
  const ordinary = members.filter((member) => !member.oneOff);
  if (!ordinary.length || ordinary.some((member) => !member.mounts))
    throw new Error(
      "Refresh the source review to obtain mount targets for ordinary services.",
    );
  for (const member of ordinary) {
    if (!key(member.service))
      throw new Error("A source member has no supported service identity.");
    const targets = new Set<string>();
    const signature = JSON.stringify(
      member
        .mounts!.map((mount) => {
          if (targets.has(mount.target))
            throw new Error("A source member repeats a mount target.");
          targets.add(mount.target);
          return [
            mount.target,
            mount.kind,
            mount.volume ?? "",
            mount.readOnly,
            mount.mappingSupported,
          ];
        })
        .sort((a, b) => String(a[0]).localeCompare(String(b[0]))),
    );
    if (
      replicas.has(member.service) &&
      replicas.get(member.service) !== signature
    )
      throw new Error(
        "Source replicas disagree on their mount configuration; review it manually.",
      );
    replicas.set(member.service, signature);
    for (const mount of member.mounts!) {
      const identity = `${member.service}\0${mount.target}`;
      if (mount.kind !== "volume") {
        if (result.has(identity))
          throw new Error(
            "Source replicas disagree on mount type; review their configuration manually.",
          );
        continue;
      }
      if (
        !mount.mappingSupported ||
        !mount.volume ||
        !key(mount.volume) ||
        !targetPath(mount.target)
      )
        throw new Error(
          "A source volume mount is not a verified ordinary full-volume mount. Driver options and subpath/unknown roots require manual configuration review.",
        );
      const next = {
        service: member.service,
        target: mount.target,
        sourceVolume: mount.volume,
        readOnly: mount.readOnly,
      };
      const previous = result.get(identity);
      if (previous && JSON.stringify(previous) !== JSON.stringify(next))
        throw new Error(
          "Source replicas disagree on volume mapping; review their configuration manually.",
        );
      result.set(identity, next);
    }
  }
  return [...result.values()].sort(
    (a, b) =>
      a.service.localeCompare(b.service) || a.target.localeCompare(b.target),
  );
}
export function migrationMountDraft(
  mounts: MigrationMount[],
  mapping: Record<string, string>,
  namespace: string,
  available: string[],
) {
  if (
    !/^[a-f0-9]{16}$/.test(namespace) ||
    !mounts.length ||
    mounts.length > 256
  )
    throw new Error("Select a supported set of observed volume mounts.");
  const services: Record<
    string,
    {
      volumes: {
        type: "volume";
        source: string;
        target: string;
        read_only: boolean;
        volume: { nocopy: true };
      }[];
    }
  > = Object.create(null);
  const volumes: Record<string, { external: true; name: string }> =
    Object.create(null);
  const aliases = new Map<string, string>();
  const owners = new Map<string, string>();
  const targets = new Set<string>();
  for (const mount of mounts) {
    if (
      !key(mount.service) ||
      !key(mount.sourceVolume) ||
      !targetPath(mount.target) ||
      typeof mount.readOnly !== "boolean"
    )
      throw new Error("Invalid reviewed volume mount.");
    const destination = mapping[mount.sourceVolume];
    if (!destination || !key(destination) || !available.includes(destination))
      throw new Error(
        `Choose an existing OrbWin volume for ${mount.sourceVolume}.`,
      );
    const identity = `${mount.service}\0${mount.target}`;
    if (targets.has(identity))
      throw new Error("A draft cannot repeat a service mount target.");
    targets.add(identity);
    if (
      owners.has(destination) &&
      owners.get(destination) !== mount.sourceVolume
    )
      throw new Error(
        "Distinct source volumes require distinct destination volumes.",
      );
    owners.set(destination, mount.sourceVolume);
    let alias = aliases.get(destination);
    if (!alias) {
      alias = `orbwin_migration_${namespace}_${aliases.size + 1}`;
      aliases.set(destination, alias);
      volumes[alias] = { external: true, name: destination };
    }
    (services[mount.service] ??= { volumes: [] }).volumes.push({
      type: "volume",
      source: alias,
      target: mount.target.replaceAll("$", () => "$$"),
      read_only: mount.readOnly,
      volume: { nocopy: true },
    });
  }
  const draft = JSON.stringify({ services, volumes }, null, 2) + "\n";
  if (draft.length > 128 * 1024)
    throw new Error("Mapping draft exceeds 128 KiB.");
  return draft;
}
