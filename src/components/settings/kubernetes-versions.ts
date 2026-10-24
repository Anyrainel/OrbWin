import { extractJson } from "./app-settings-model.ts";

export type KubernetesVersion = {
  version: string;
  release: string;
  channels: string[];
};
const concrete = /^(0|[1-9]\d{0,3})\.(0|[1-9]\d{0,3})\.(0|[1-9]\d{0,3})$/;
export function compareVersions(left: string, right: string) {
  const a = left.split(".").map(Number),
    b = right.split(".").map(Number);
  return a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
}
export function versionChangeUnavailable(
  current: string | undefined,
  next: string,
): string | undefined {
  if (!concrete.test(next) || (current && !concrete.test(current)))
    return "The concrete version could not be verified.";
  if (!current) return;
  if (current === next) return "This version is already selected.";
  const a = current.split(".").map(Number),
    b = next.split(".").map(Number);
  if (compareVersions(next, current) < 0)
    return "Downgrades are unsupported because existing cluster data is retained.";
  if (a[0] !== b[0] || b[1] > a[1] + 1)
    return "Upgrade one minor version at a time.";
}
export function parseKubernetesVersions(raw: string): KubernetesVersion[] {
  const doc = extractJson(raw) as {
    kind?: string;
    metadata?: { name?: string };
    data?: Record<string, string>;
  };
  if (doc?.kind !== "ConfigMap" || doc.metadata?.name !== "k3s-versions")
    throw new Error("The backend returned no Kubernetes version catalog.");
  const versions: unknown = JSON.parse(doc.data?.versions ?? "null");
  const channels: unknown = JSON.parse(doc.data?.channels ?? "{}");
  if (
    !versions ||
    typeof versions !== "object" ||
    Array.isArray(versions) ||
    Object.keys(versions).length > 2048
  )
    throw new Error("The Kubernetes version catalog is invalid or too large.");
  const aliases =
    channels && typeof channels === "object" && !Array.isArray(channels)
      ? Object.entries(channels)
      : [];
  const result = Object.entries(versions).map(([version, release]) => {
    if (
      !concrete.test(version) ||
      typeof release !== "string" ||
      !new RegExp(
        `^v${version.replaceAll(".", "\\.")}\\+k3s[1-9]\\d{0,3}$`,
      ).test(release)
    )
      throw new Error("The Kubernetes catalog contains an invalid release.");
    return {
      version,
      release,
      channels: aliases
        .filter(
          ([key, value]) =>
            key.length <= 64 &&
            /^[a-zA-Z0-9_.-]+$/.test(key) &&
            value === version,
        )
        .map(([key]) => key),
    };
  });
  if (!result.length)
    throw new Error("The Kubernetes version catalog is empty.");
  return result.sort((a, b) => compareVersions(b.version, a.version));
}
