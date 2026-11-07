import assert from "node:assert/strict";
import { resolve } from "node:path";

// Source selection only, after the real provenance verifiers admitted their
// pinned source candidates. Never fall back to a predecessor Docker cache.
export function runtimeToolSource(
  tool,
  cache,
  dockerCandidate,
  candidates = {},
) {
  const roles = {
    "bin/docker.exe": ["dockerCli", "docker.exe"],
    "linux/docker": ["dockerCli", "docker"],
    "docker-cli-plugins/docker-compose.exe": ["compose"],
    "linux/docker-compose": ["compose"],
    "docker-cli-plugins/docker-buildx.exe": ["buildx"],
    "linux/docker-buildx": ["buildx"],
  };
  const role = roles[tool.path];
  assert.ok(
    role && tool.localSource?.name === role[0],
    "Closed local tool role required",
  );
  if (role[0] === "dockerCli") {
    assert.ok(dockerCandidate, "Verified Docker candidate required");
    return resolve(dockerCandidate, role[1]);
  }
  if (candidates[role[0]]) {
    assert.ok(
      ["windowsAmd64", "linuxAmd64"].includes(tool.localSource.target),
      "Closed local tool target required",
    );
    const windows = tool.localSource.target === "windowsAmd64";
    assert.equal(
      tool.path,
      windows
        ? `docker-cli-plugins/docker-${role[0]}.exe`
        : `linux/docker-${role[0]}`,
    );
    return resolve(
      candidates[role[0]],
      `docker-${role[0]}${windows ? ".exe" : ""}`,
    );
  }
  return resolve(cache, tool.path);
}
