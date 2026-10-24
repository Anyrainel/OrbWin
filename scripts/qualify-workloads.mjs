// Explicit opt-in qualification, never run by npm test. Requires a reviewed
// runtime identity record from the coexistence proof, not merely an open pipe.
import { spawnSync } from "node:child_process";
import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { randomUUID } from "node:crypto";
import { fileURLToPath } from "node:url";
import { endpoint, verifyIdentity } from "./engine-identity.mjs";
import { verifyBuild } from "./build-identity.mjs";

const root = fileURLToPath(new URL("../", import.meta.url));
const output = resolve(root, "artifacts/qualification");
const config = resolve(root, "artifacts/client/docker");
const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
const build = verifyBuild(root);
const identity = JSON.parse(
  readFileSync(resolve(output, "engine-identity.json")),
);
// Validate the record before even contacting the endpoint.
verifyIdentity(identity, { ID: identity.daemonId, OSType: "linux" }, build);
mkdirSync(config, { recursive: true });
mkdirSync(output, { recursive: true });
const project = `orbwin-qualification-${randomUUID().slice(0, 8)}`;
const results = {
  project,
  endpoint,
  sourceCommit: lock.commit,
  started: new Date().toISOString(),
  steps: [],
  success: false,
};
function docker(args, seconds = 30) {
  const started = performance.now();
  const child = spawnSync(
    "docker",
    ["--config", config, "--host", endpoint, ...args],
    {
      cwd: root,
      encoding: "utf8",
      timeout: seconds * 1000,
      maxBuffer: 2 * 1024 * 1024,
      windowsHide: true,
      // Clear competing selectors/TLS. Endpoint/config are always explicit.
      env: {
        ...process.env,
        DOCKER_HOST: endpoint,
        DOCKER_CONTEXT: "",
        DOCKER_TLS_VERIFY: "",
        DOCKER_CERT_PATH: "",
        DOCKER_CONFIG: config,
        BUILDX_BUILDER: "default",
        BUILDX_CONFIG: resolve(config, "buildx"),
        BUILDKIT_HOST: "",
      },
    },
  );
  results.steps.push({
    args,
    milliseconds: performance.now() - started,
    status: child.status,
    stdout: child.stdout,
    stderr: child.stderr,
    error: child.error?.message,
  });
  if (child.error || child.status !== 0)
    throw new Error(
      `${args.join(" ")}: ${child.error?.message ?? child.stderr ?? child.status}`,
    );
  return child.stdout.trim();
}
const compose = (...args) =>
  docker(
    ["compose", "-p", project, "-f", "tests/workloads/compose.yaml", ...args],
    150,
  );
let created = false;
let failure;
try {
  const info = JSON.parse(docker(["info", "--format", "{{json .}}"]));
  verifyIdentity(identity, info, build);
  docker(["version"]);
  docker(["compose", "version"]);
  docker(["buildx", "version"]);
  created = true; // Partial `up` failures also require project-scoped cleanup.
  compose("up", "-d", "--wait", "--wait-timeout", "100");
  if (compose("exec", "-T", "redis", "redis-cli", "ping") !== "PONG")
    throw new Error("Redis readiness response differed");
  compose(
    "exec",
    "-T",
    "redis",
    "redis-cli",
    "set",
    "orbwin-preservation",
    project,
  );
  compose(
    "exec",
    "-T",
    "postgres",
    "psql",
    "-U",
    "postgres",
    "-d",
    "orbwin_test",
    "-v",
    "ON_ERROR_STOP=1",
    "-c",
    "CREATE TABLE proof (value integer); INSERT INTO proof VALUES (42);",
  );
  compose("restart", "redis", "postgres");
  compose("up", "-d", "--wait", "--wait-timeout", "100");
  if (
    compose(
      "exec",
      "-T",
      "redis",
      "redis-cli",
      "get",
      "orbwin-preservation",
    ) !== project
  )
    throw new Error("Redis restart preservation failed");
  if (
    compose(
      "exec",
      "-T",
      "postgres",
      "psql",
      "-U",
      "postgres",
      "-d",
      "orbwin_test",
      "-Atc",
      "SELECT value FROM proof;",
    ) !== "42"
  )
    throw new Error("Postgres restart preservation failed");
  compose("logs", "--no-color", "--tail", "40");
  compose("ps", "--format", "json");
  results.success = true;
} catch (error) {
  failure = error;
} finally {
  if (created) {
    try {
      compose("down", "--volumes", "--remove-orphans", "--timeout", "15");
    } catch (error) {
      results.success = false;
      failure ??= error;
    }
  }
  results.finished = new Date().toISOString();
  writeFileSync(
    resolve(output, `${project}.json`),
    JSON.stringify(results, null, 2),
  );
}
if (failure) throw failure;
console.log(`Passed isolated workload run: ${project}`);
