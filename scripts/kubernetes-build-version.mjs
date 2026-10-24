// Kubernetes libraries use v0.<minor>.<patch> module versions for the matching
// v1.<minor>.<patch> kubectl release. A go.mod replacement owns the actual code.
export function kubernetesBuildVersion(module) {
  const actual = module.Replace?.Version ?? module.Version;
  const match = /^v0\.(\d+)\.(\d+)$/.exec(actual ?? "");
  if (!match)
    throw new Error(`Unsupported Kubernetes module version: ${actual}`);
  return {
    module: module.Path,
    sourceVersion: actual,
    version: `v1.${match[1]}.${match[2]}`,
    major: "1",
    minor: match[1],
  };
}
