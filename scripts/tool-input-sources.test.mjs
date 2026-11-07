import test from "node:test";
import assert from "node:assert/strict";
import { resolve } from "node:path";
import { runtimeToolSource } from "./tool-input-sources.mjs";
test("fresh six-tool layout takes Docker only from actual verified candidate", () => {
  const cache = resolve("cache"),
    candidate = resolve("pinned-candidate");
  for (const [path, name, file] of [
    ["bin/docker.exe", "dockerCli", "docker.exe"],
    ["linux/docker", "dockerCli", "docker"],
    ["linux/docker-compose", "compose", "linux/docker-compose"],
    ["linux/docker-buildx", "buildx", "linux/docker-buildx"],
    [
      "docker-cli-plugins/docker-compose.exe",
      "compose",
      "docker-cli-plugins/docker-compose.exe",
    ],
    [
      "docker-cli-plugins/docker-buildx.exe",
      "buildx",
      "docker-cli-plugins/docker-buildx.exe",
    ],
  ])
    assert.equal(
      runtimeToolSource({ path, localSource: { name } }, cache, candidate),
      resolve(name === "dockerCli" ? candidate : cache, file),
    );
  assert.throws(() =>
    runtimeToolSource(
      { path: "bin/docker.exe", localSource: { name: "dockerCli" } },
      cache,
      null,
    ),
  );
  assert.throws(() =>
    runtimeToolSource(
      { path: "../foreign", localSource: { name: "dockerCli" } },
      cache,
      candidate,
    ),
  );
  assert.throws(() =>
    runtimeToolSource(
      { path: "bin/docker.exe", localSource: { name: "compose" } },
      cache,
      candidate,
    ),
  );
});

test("fresh verified Compose and Buildx candidates supply both roles without selected cache fallback", () => {
  const cache = resolve("old-cache"),
    docker = resolve("docker-candidate"),
    candidates = {
      compose: resolve("fresh-compose"),
      buildx: resolve("fresh-buildx"),
    };
  for (const name of ["compose", "buildx"])
    for (const target of ["windowsAmd64", "linuxAmd64"]) {
      const windows = target === "windowsAmd64",
        path = windows
          ? `docker-cli-plugins/docker-${name}.exe`
          : `linux/docker-${name}`;
      assert.equal(
        runtimeToolSource(
          { path, localSource: { name, target } },
          cache,
          docker,
          candidates,
        ),
        resolve(candidates[name], `docker-${name}${windows ? ".exe" : ""}`),
      );
    }
  assert.throws(() =>
    runtimeToolSource(
      {
        path: "linux/docker-compose",
        localSource: { name: "compose", target: "windowsAmd64" },
      },
      cache,
      docker,
      candidates,
    ),
  );
});
