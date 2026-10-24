// Opt-in compatibility proof on the verified owned engine. Unique fixtures,
// project-scoped cleanup, no global client changes or user-distro mutations.
import { execFileSync } from "node:child_process";
import {
  mkdirSync,
  readFileSync,
  writeFileSync,
  rmSync,
  mkdtempSync,
} from "node:fs";
import { resolve, join } from "node:path";
import { tmpdir } from "node:os";
import { randomUUID } from "node:crypto";
import assert from "node:assert/strict";
import { verifyBuild } from "./build-identity.mjs";
import { endpoint } from "./engine-identity.mjs";
if (process.env.ORBWIN_QUALIFY_OWNED !== "1")
  throw new Error(
    "Set ORBWIN_QUALIFY_OWNED=1 to test the recorded owned engine.",
  );
const root = process.cwd();
const build = verifyBuild(root);
const identity = JSON.parse(
  readFileSync("artifacts/qualification/runtime-identity.json"),
);
assert.equal(identity.identityVerified, true);
assert.equal(identity.endpoint, endpoint);
assert.equal(identity.binarySha256, build.binarySha256);
const project = `orbwin-mounts-${randomUUID().slice(0, 8)}`;
const directory = resolve(root, "artifacts/qualification", project);
mkdirSync(directory);
const hostTemp = mkdtempSync(join(tmpdir(), project + "-"));
const fixtures = [
  join(directory, "drive space 例"),
  join(hostTemp, "drive space 例"),
];
const image =
  "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30";
const evidence = { project, steps: [], passed: false };
const env = {
  ...process.env,
  DOCKER_HOST: endpoint,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  BUILDKIT_HOST: "",
  BUILDX_BUILDER: "default",
};
const config = resolve(identity.home, ".rdorbwin/docker");
function docker(args, seconds = 60) {
  const started = performance.now();
  try {
    const output = execFileSync(
      "docker",
      ["--config", config, "--host", endpoint, ...args],
      {
        env,
        encoding: "utf8",
        timeout: seconds * 1000,
        windowsHide: true,
        maxBuffer: 2 << 20,
      },
    );
    evidence.steps.push({
      args,
      output,
      milliseconds: performance.now() - started,
    });
    return output.trim();
  } catch (error) {
    evidence.steps.push({
      args,
      error: String(error),
      stderr: String(error.stderr ?? ""),
    });
    throw error;
  }
}
const check = () =>
  assert.equal(
    JSON.parse(docker(["info", "--format", "{{json .}}"])).ID,
    identity.daemonId,
  );
const composeFile = join(directory, "compose.json");
const compose = (...args) =>
  docker(["compose", "-p", project, "-f", composeFile, ...args], 120);
let composeCreated = false,
  volumeCreated = false,
  failure;
try {
  check();
  for (const fixture of fixtures) {
    mkdirSync(fixture);
    const file = join(fixture, "file 例.txt");
    writeFileSync(file, "before");
    assert.equal(
      docker([
        "run",
        "--rm",
        "--mount",
        `type=bind,source=${fixture},target=/fixture,readonly`,
        image,
        "cat",
        "/fixture/file 例.txt",
      ]),
      "before",
    );
    assert.equal(
      docker([
        "run",
        "--rm",
        "-v",
        `${file}:/file:ro`,
        image,
        "sh",
        "-c",
        "cat /file; if printf forbidden >> /file; then exit 9; fi",
      ]),
      "before",
    );
    docker([
      "run",
      "--rm",
      "--mount",
      `type=bind,source=${file},target=/file`,
      image,
      "sh",
      "-c",
      "printf host-visible > /file",
    ]);
    assert.equal(readFileSync(file, "utf8"), "host-visible");
  }
  docker([
    "volume",
    "create",
    "--driver",
    "local",
    "--opt",
    "type=none",
    "--opt",
    "o=bind,ro",
    "--opt",
    `device=${fixtures[0]}`,
    project,
  ]);
  volumeCreated = true;
  assert.equal(
    docker([
      "run",
      "--rm",
      "--mount",
      `type=volume,source=${project},target=/fixture`,
      image,
      "cat",
      "/fixture/file 例.txt",
    ]),
    "host-visible",
  );
  writeFileSync(
    composeFile,
    JSON.stringify(
      {
        services: {
          probe: {
            image,
            command: [
              "sh",
              "-c",
              "cat '/fixture/file 例.txt'; cat /file; sleep 300",
            ],
            volumes: [
              {
                type: "bind",
                source: fixtures[0],
                target: "/fixture",
                read_only: true,
              },
              {
                type: "bind",
                source: join(fixtures[1], "file 例.txt"),
                target: "/file",
                read_only: true,
              },
            ],
          },
        },
      },
      null,
      2,
    ),
  );
  composeCreated = true;
  compose("up", "-d", "--wait", "--wait-timeout", "60");
  assert.equal(compose("exec", "-T", "probe", "cat", "/file"), "host-visible");
  compose("restart");
  assert.equal(
    compose("exec", "-T", "probe", "cat", "/fixture/file 例.txt"),
    "host-visible",
  );
  evidence.passed = true;
} catch (error) {
  failure = error;
  evidence.failure = String(error);
} finally {
  try {
    check();
    if (composeCreated) compose("down", "--volumes", "--timeout", "10");
    if (volumeCreated) docker(["volume", "rm", project]);
  } catch (error) {
    failure ??= error;
    evidence.cleanupFailure = String(error);
    evidence.passed = false;
  }
  writeFileSync(
    join(directory, "result.json"),
    JSON.stringify(evidence, null, 2),
  );
  // Only delete the exact newly-created temp fixture directory, never a parent.
  if (!failure) rmSync(hostTemp, { recursive: true });
}
console.log(
  `Mount proof ${evidence.passed ? "passed" : "FAILED"}: ${directory}`,
);
if (failure) throw failure;
