// Build history/storage smoke on the sequential proof guest only. All resources
// in this guest are disposable; it never selects the main product/default pipe.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import assert from "node:assert/strict";
import { verifyBuild } from "./build-identity.mjs";
if (process.env.ORBWIN_QUALIFY_OWNED !== "1")
  throw new Error("Explicit owned-engine opt-in required");
const root = process.cwd();
const build = verifyBuild(root);
const identity = JSON.parse(readFileSync("artifacts/setup-proof/runtime.json"));
assert.equal(
  identity.endpoint,
  "npipe:////./pipe/docker-rancher-desktop-orbwin-proof",
);
assert.equal(identity.binarySha256, build.binarySha256);
const env = {
  ...process.env,
  DOCKER_HOST: identity.endpoint,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  BUILDX_BUILDER: "default",
  BUILDKIT_HOST: "",
};
const records = [];
const tag = `orbwin-history:${Date.now()}`;
function docker(args, seconds = 120) {
  try {
    const output = execFileSync(
      resolve("artifacts/runtime-tools/bin/docker.exe"),
      [
        "--config",
        resolve(identity.home, ".rdorbwin-proof/docker"),
        "--host",
        identity.endpoint,
        ...args,
      ],
      {
        env,
        encoding: "utf8",
        timeout: seconds * 1000,
        maxBuffer: 2 << 20,
        windowsHide: true,
      },
    );
    records.push({ args, output });
    return output.trim();
  } catch (error) {
    records.push({
      args,
      error: String(error),
      stderr: String(error.stderr ?? ""),
    });
    throw error;
  }
}
let built = false;
let failure;
try {
  assert.equal(
    JSON.parse(docker(["info", "--format", "{{json .}}"])).ID,
    identity.daemonId,
  );
  assert.equal(
    docker([
      "run",
      "--rm",
      "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30",
      "uname",
      "-s",
    ]),
    "Linux",
  );
  docker([
    "buildx",
    "build",
    "--progress=plain",
    "--load",
    "--tag",
    tag,
    resolve("tests/workloads/build"),
  ]);
  built = true;
  const raw = docker(["buildx", "history", "ls", "--format", "json"]);
  console.log(raw.slice(0, 3000));
  const rows = raw
    .split(/\r?\n/)
    .filter(Boolean)
    .map((row) => JSON.parse(row));
  assert.ok(rows.length, "Buildx did not retain the submitted build");
  const ref = (rows[0].Ref ?? rows[0].ref)?.split("/").at(-1);
  assert.equal(typeof ref, "string");
  docker(["buildx", "history", "inspect", "--format", "json", ref]);
  docker(["buildx", "history", "logs", ref, "--progress=plain"]);
  docker(["system", "df", "--format", "json"]);
} catch (error) {
  failure = String(error);
} finally {
  if (built) {
    try {
      docker(["image", "rm", tag]);
    } catch (error) {
      failure ??= String(error);
    }
  }
  writeFileSync(
    resolve("artifacts/setup-proof/history.json"),
    JSON.stringify({ passed: !failure, failure, records }, null, 2),
  );
}
if (failure) throw new Error(failure);
console.log(
  "Build history, logs, disk usage and cleanup passed on the proof guest.",
);
