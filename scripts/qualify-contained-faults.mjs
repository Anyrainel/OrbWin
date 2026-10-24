// Container-scoped pressure only. Never fill the host/guest data disk or apply
// memory pressure to another engine, WSL distribution or the shared WSL VM.
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { resolve } from "node:path";
import { randomUUID, createHash } from "node:crypto";
import { verifyBuild } from "./build-identity.mjs";
import { verifyIdentity } from "./engine-identity.mjs";
assert.equal(process.env.ORBWIN_QUALIFY_OWNED, "1");
const identity = JSON.parse(
  readFileSync("artifacts/qualification/engine-identity.json"),
);
const build = verifyBuild(process.cwd());
verifyIdentity(identity, { ID: identity.daemonId, OSType: "linux" }, build);
const run = randomUUID(),
  name = `orbwin-faults-${run.slice(0, 8)}`,
  directory = resolve("artifacts/qualification", name);
mkdirSync(directory);
const docker = resolve("artifacts/runtime-tools/bin/docker.exe"),
  config = resolve(identity.home, ".rdorbwin/docker");
const env = {
  ...process.env,
  DOCKER_HOST: identity.endpoint,
  DOCKER_CONFIG: config,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  PATH: resolve("artifacts/backend") + ";" + process.env.PATH,
};
const result = {
  run,
  name,
  identity,
  kind: "contained pressure; not whole-engine disk exhaustion",
  checks: [],
  containers: [],
  passed: false,
  cleaned: false,
};
const cli = (args, seconds = 30) =>
  execFileSync(
    docker,
    ["--config", config, "--host", identity.endpoint, ...args],
    {
      env,
      encoding: "utf8",
      windowsHide: true,
      timeout: seconds * 1000,
      maxBuffer: 1 << 20,
    },
  ).trim();
const hash = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");
const globalConfigs = [
  resolve(process.env.USERPROFILE, ".docker/config.json"),
  resolve(process.env.USERPROFILE, ".kube/config"),
];
const before = globalConfigs.map(hash);
const pin = JSON.parse(readFileSync("tests/registry/images.lock.json")).base;
assert.equal(cli(["info", "--format", "{{.ID}}"]), identity.daemonId);
const lock = JSON.parse(readFileSync("upstream.lock.json"));
const binary = resolve(directory, "oom");
execFileSync(
  resolve(`.tools/go${lock.go.version}/go/bin/go.exe`),
  ["build", "-trimpath", "-o", binary, "tests/faults/oom.go"],
  {
    env: { ...process.env, GOOS: "linux", GOARCH: "amd64", CGO_ENABLED: "0" },
    windowsHide: true,
    stdio: "inherit",
  },
);
result.workloadSha256 = hash(binary);
let volume;
function inspected(id) {
  const item = JSON.parse(cli(["inspect", id]))[0];
  assert.equal(item.Id, id);
  assert.equal(item.Config.Labels["org.orbwin.qualification"], run);
  return item;
}
function create(suffix, args) {
  const id = cli([
    "create",
    "--name",
    `${name}-${suffix}`,
    "--label",
    `org.orbwin.qualification=${run}`,
    ...args,
  ]);
  result.containers.push(id);
  inspected(id);
  return id;
}
function marker() {
  const id = create(`marker-${result.containers.length}`, [
    "--mount",
    `type=volume,source=${volume},target=/data,readonly`,
    pin,
    "cat",
    "/data/marker",
  ]);
  cli(["start", id]);
  assert.equal(cli(["wait", id]), "0");
  assert.equal(cli(["logs", id]), run);
}
try {
  volume = cli([
    "volume",
    "create",
    "--label",
    `org.orbwin.qualification=${run}`,
    name,
  ]);
  const seed = create("seed", [
    "--mount",
    `type=volume,source=${volume},target=/data`,
    pin,
    "sh",
    "-c",
    `printf '%s' '${run}' > /data/marker`,
  ]);
  cli(["start", seed]);
  assert.equal(cli(["wait", seed]), "0");
  marker();
  const disk = create("tmpfs-full", [
    "--tmpfs",
    "/limited:rw,size=16m",
    pin,
    "sh",
    "-c",
    "dd if=/dev/zero of=/limited/full bs=1M count=32",
  ]);
  cli(["start", disk]);
  assert.equal(cli(["wait", disk]), "1");
  assert.equal(inspected(disk).State.OOMKilled, false);
  const output = spawnSync(
    docker,
    ["--config", config, "--host", identity.endpoint, "logs", disk],
    { env, encoding: "utf8", windowsHide: true, timeout: 10000 },
  );
  assert.match(output.stderr, /No space left on device/);
  marker();
  result.checks.push(
    "container tmpfs ENOSPC exits nonzero without OOM or named-volume damage",
  );
  const oom = create("oom", [
    "--memory",
    "32m",
    "--memory-swap",
    "32m",
    "--mount",
    `type=bind,source=${binary},target=/oom,readonly`,
    pin,
    "/oom",
  ]);
  cli(["start", oom]);
  assert.equal(cli(["wait", oom]), "137");
  assert.equal(inspected(oom).State.OOMKilled, true);
  marker();
  result.checks.push(
    "cgroup-scoped OOM is reported as OOMKilled/137 and named-volume data survives",
  );
  assert.equal(cli(["info", "--format", "{{.ID}}"]), identity.daemonId);
  result.passed = true;
} catch (error) {
  result.error = String(error);
  throw error;
} finally {
  try {
    for (const id of result.containers) {
      const item = inspected(id);
      if (item.State.Running) cli(["stop", "--time=1", id]);
      cli(["rm", id]);
    }
    if (volume) {
      assert.equal(
        JSON.parse(cli(["volume", "inspect", volume]))[0].Labels[
          "org.orbwin.qualification"
        ],
        run,
      );
      cli(["volume", "rm", volume]);
    }
    globalConfigs.forEach((path, index) =>
      assert.equal(hash(path), before[index]),
    );
    result.cleaned = true;
  } catch (cleanupError) {
    result.cleaned = false;
    result.cleanupError = String(cleanupError);
    result.passed = false;
    process.exitCode = 1;
  }
  result.finishedAt = new Date().toISOString();
  writeFileSync(
    resolve(directory, "result.json"),
    JSON.stringify(result, null, 2),
  );
}
console.log(`Contained fault qualification passed: ${name}`);
