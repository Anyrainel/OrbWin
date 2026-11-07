// Non-elevated guest emulation proof. Never installs or changes binfmt handlers.
// Uses only uniquely labelled scratch fixtures on the qualified owned endpoint.
import { spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { endpoint, verifyIdentity } from "./engine-identity.mjs";
import { verifyBuild } from "./build-identity.mjs";

if (process.env.ORBWIN_QUALIFY_OWNED !== "1")
  throw new Error("Explicit owned-engine qualification opt-in required");
const root = fileURLToPath(new URL("../", import.meta.url));
const fixture = `orbwin-platform-${randomUUID().replaceAll("-", "").slice(0, 16)}`;
const folder = resolve(root, "artifacts/qualification", fixture);
const context = resolve(root, "tests/workloads/platform-probe");
const config = resolve(folder, "docker");
mkdirSync(config, { recursive: true });
const identity = JSON.parse(
  readFileSync(resolve(root, "artifacts/qualification/engine-identity.json")),
);
const build = verifyBuild(root);
verifyIdentity(identity, { ID: identity.daemonId, OSType: "linux" }, build);
const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
const dockerPath = resolve(root, "artifacts/runtime-tools/bin/docker.exe");
const env = {
  ...process.env,
  DOCKER_HOST: endpoint,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  DOCKER_CONFIG: config,
  BUILDX_BUILDER: "default",
  BUILDX_CONFIG: resolve(config, "buildx"),
  BUILDKIT_HOST: "",
};
const proof = {
  schema: 1,
  fixture,
  endpoint,
  started: new Date().toISOString(),
  steps: [],
  execution: {},
  binaries: {},
  cleanupErrors: [],
  success: false,
};
const hash = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");
function run(executable, args, options = {}) {
  const started = performance.now();
  const result = spawnSync(executable, args, {
    cwd: root,
    env,
    encoding: "utf8",
    timeout: 120_000,
    maxBuffer: 4 << 20,
    windowsHide: true,
    ...options,
  });
  proof.steps.push({
    executable,
    args,
    status: result.status,
    milliseconds: performance.now() - started,
    stdout: result.stdout,
    stderr: result.stderr,
    error: result.error?.message,
  });
  if (result.error || result.status !== 0)
    throw new Error(`Qualification command failed: ${args.join(" ")}`);
  return result.stdout;
}
const docker = (args, options) =>
  run(dockerPath, ["--config", config, "--host", endpoint, ...args], options);
const images = [],
  containers = [];
try {
  const info = JSON.parse(docker(["info", "--format", "{{json .}}"]));
  verifyIdentity(identity, info, build);
  proof.daemonId = info.ID;
  proof.guestArchitecture = info.Architecture;
  docker(["buildx", "version"]);
  for (const architecture of ["amd64", "arm64"]) {
    const binary = resolve(context, "bin", architecture, "probe");
    mkdirSync(resolve(context, "bin", architecture), { recursive: true });
    run(
      resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`),
      ["build", "-trimpath", "-o", binary, resolve(context, "main.go")],
      {
        env: { ...env, GOOS: "linux", GOARCH: architecture, CGO_ENABLED: "0" },
      },
    );
    proof.binaries[architecture] = hash(binary);
    const tag = `${fixture}:${architecture}`;
    images.push(tag);
    docker([
      "buildx",
      "build",
      "--builder",
      "default",
      "--progress=plain",
      "--platform",
      `linux/${architecture}`,
      "--load",
      "--label",
      `dev.orbwin.qualification=${fixture}`,
      "--tag",
      tag,
      context,
    ]);
    const inspected = JSON.parse(docker(["image", "inspect", tag]))[0];
    if (
      inspected.Os !== "linux" ||
      inspected.Architecture !== architecture ||
      inspected.Config?.Labels?.["dev.orbwin.qualification"] !== fixture
    )
      throw new Error("Built fixture platform or ownership differs");
    const name = `${fixture}-${architecture}`;
    containers.push(name);
    try {
      const execution = JSON.parse(
        docker(
          [
            "run",
            "--name",
            name,
            "--label",
            `dev.orbwin.qualification=${fixture}`,
            "--platform",
            `linux/${architecture}`,
            "--network",
            "none",
            "--read-only",
            "--cap-drop",
            "ALL",
            "--security-opt",
            "no-new-privileges:true",
            "--pids-limit",
            "32",
            "--memory",
            "64m",
            tag,
          ],
          { timeout: 30_000 },
        ),
      );
      if (
        execution.os !== "linux" ||
        execution.architecture !== architecture ||
        execution.result !== "executed"
      )
        throw new Error("Guest did not execute the requested platform probe");
      proof.execution[architecture] = { imageId: inspected.Id, ...execution };
    } catch (error) {
      // A failed platform must not prevent the independent Buildx proof. Keep
      // its raw failure and leave overall success false; never install handlers.
      proof.execution[architecture] = {
        imageId: inspected.Id,
        result: "failed",
        error: String(error),
      };
    }
  }
  // Local output verifies both Buildx platform targets without publishing to a
  // registry or assuming the engine's image store accepts a multi-platform index.
  const multi = resolve(folder, "multi-platform");
  docker([
    "buildx",
    "build",
    "--builder",
    "default",
    "--progress=plain",
    "--platform",
    "linux/amd64,linux/arm64",
    "--output",
    `type=local,dest=${multi}`,
    context,
  ]);
  for (const architecture of ["amd64", "arm64"])
    if (
      hash(resolve(multi, `linux_${architecture}`, "probe")) !==
      proof.binaries[architecture]
    )
      throw new Error(
        "Multi-platform Buildx output differs from selected platform binary",
      );
  proof.multiPlatformLocalOutput = true;
  proof.success = Object.values(proof.execution).every(
    (result) => result.result === "executed",
  );
} catch (error) {
  proof.failure = String(error);
} finally {
  for (const [kind, names] of [
    ["container", containers],
    ["image", images],
  ]) {
    for (const name of names.reverse()) {
      try {
        const before = spawnSync(
          dockerPath,
          ["--config", config, "--host", endpoint, kind, "inspect", name],
          { env, encoding: "utf8", timeout: 15_000, windowsHide: true },
        );
        if (before.status !== 0) {
          // Only a confirmed Docker absence is an ignorable cleanup outcome.
          if (!/No such (image|container|object)/i.test(before.stderr ?? ""))
            throw new Error(
              before.error?.message ?? before.stderr ?? "Inspection failed",
            );
          continue;
        }
        const inspected = JSON.parse(before.stdout)[0];
        if (inspected.Config?.Labels?.["dev.orbwin.qualification"] !== fixture)
          throw new Error(`Fixture ownership differs: ${name}`);
        docker(
          kind === "container"
            ? [kind, "rm", "--force", name]
            : [kind, "rm", name],
        );
      } catch (error) {
        proof.cleanupErrors.push(String(error));
      }
    }
  }
  try {
    verifyIdentity(
      identity,
      JSON.parse(docker(["info", "--format", "{{json .}}"])),
      build,
    );
  } catch (error) {
    proof.cleanupErrors.push(String(error));
  }
  proof.sourceHashes = Object.fromEntries(
    ["main.go", "Dockerfile", ".dockerignore"].map((name) => [
      name,
      hash(resolve(context, name)),
    ]),
  );
  proof.finished = new Date().toISOString();
  proof.success &&= proof.cleanupErrors.length === 0;
  writeFileSync(resolve(folder, "proof.json"), JSON.stringify(proof, null, 2));
  console.log(
    JSON.stringify({
      proof: resolve(folder, "proof.json"),
      success: proof.success,
      execution: proof.execution,
      failure: proof.failure,
      cleanupErrors: proof.cleanupErrors,
    }),
  );
  if (!proof.success) process.exitCode = 1;
}
