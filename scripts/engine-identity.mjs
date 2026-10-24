export const endpoint = "npipe:////./pipe/docker-rancher-desktop-orbwin";
export function verifyIdentity(record, info, build) {
  if (
    record?.endpoint !== endpoint ||
    record?.sourceCommit !== build.sourceCommit ||
    !record?.binarySha256 ||
    record.binarySha256 !== build.binarySha256 ||
    record?.coexistenceQualified !== true
  ) {
    throw new Error(
      "Engine qualification record is missing or does not match the pinned, isolated endpoint.",
    );
  }
  if (
    typeof record.daemonId !== "string" ||
    !record.daemonId ||
    info?.ID !== record.daemonId
  ) {
    throw new Error(
      "Docker daemon identity differs from the qualified engine. No workloads were started.",
    );
  }
  if (info?.OSType !== "linux")
    throw new Error("Qualification requires the owned Linux engine.");
}
