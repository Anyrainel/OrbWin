// Opt-in real framework workloads. Verify the framework-selected daemon before
// GenericContainer.start can create a Ryuk helper or any workload. No default
// context changes, competitor lifecycle, broad cleanup or disabled reaper.
import { spawn, execFileSync } from "node:child_process";
import { createConnection } from "node:net";
import { mkdirSync, readFileSync, writeFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { randomUUID, createHash } from "node:crypto";
import assert from "node:assert/strict";
import { verifyBuild } from "../../scripts/build-identity.mjs";
import { endpoint } from "../../scripts/engine-identity.mjs";
import { nodeTestcontainersEndpoint as frameworkEndpoint } from "../../src/model/client-targeting.ts";

const root = fileURLToPath(new URL("../../", import.meta.url));
const label = "org.orbwin.qualification";
// Testcontainers 12.2.1 parses DOCKER_HOST through WHATWG URL. The canonical
// CLI form loses its /./ segment there. Backslash pipe separators preserve the
// native Windows pipe path; keep this client-specific and verify its daemon.
assert.equal(
  new URL(frameworkEndpoint).pathname,
  String.raw`//.\pipe\docker-rancher-desktop-orbwin`,
);
const images = {
  redis:
    "redis:7.4.2-alpine@sha256:02419de7eddf55aa5bcf49efb74e88fa8d931b4d77c07eff8a6b2144472b6952",
  postgres:
    "postgres:17.4-alpine@sha256:7062a2109c4b51f3c792c7ea01e83ed12ef9a980886e3b3d380a7d2e5f6ce3f5",
  ryuk: "testcontainers/ryuk:0.14.0@sha256:7c1a8a9a47c780ed0f983770a662f80deb115d95cce3e2daa3d12115b8cd28f0",
};
const identity = JSON.parse(
  readFileSync(resolve(root, "artifacts/qualification/runtime-identity.json")),
);
const build = verifyBuild(root);
assert.equal(identity.identityVerified, true);
assert.equal(identity.endpoint, endpoint);
assert.equal(identity.binarySha256, build.binarySha256);
assert.equal(identity.sourceCommit, build.sourceCommit);
function redisPing(host, port) {
  return new Promise((accept, reject) => {
    const socket = createConnection({ host, port });
    let data = "";
    const timer = setTimeout(() => {
      socket.destroy();
      reject(new Error("Published Redis endpoint timed out"));
    }, 5000);
    socket.once("connect", () => socket.write("*1\r\n$4\r\nPING\r\n"));
    socket.on("data", (chunk) => {
      data += chunk;
      if (data.includes("\r\n")) {
        clearTimeout(timer);
        socket.destroy();
        accept(data);
      }
    });
    socket.once("error", (error) => {
      clearTimeout(timer);
      socket.destroy();
      reject(error);
    });
  });
}
async function worker(mode, runId) {
  const { GenericContainer, Wait, getContainerRuntimeClient } = await import(
    "testcontainers"
  );
  const client = await getContainerRuntimeClient();
  const info = await client.container.dockerode.info();
  assert.equal(
    info.ID,
    identity.daemonId,
    "Framework selected a different engine; refusing all mutations",
  );
  assert.equal(info.OSType, "linux");
  const resources = [];
  const progress = (event) =>
    process.stdout.write(JSON.stringify({ runId, ...event }) + "\n");
  const startRedis = async () => {
    const container = await new GenericContainer(images.redis)
      .withLabels({ [label]: runId })
      .withExposedPorts(6379)
      .withResourcesQuota({ memory: 128 * 1024 * 1024 })
      .withWaitStrategy(Wait.forLogMessage(/Ready to accept connections/))
      .withStartupTimeout(90000)
      .start();
    resources.push(container);
    const sessionId = container.getLabels()["org.testcontainers.session-id"];
    const helpers = (
      await client.container.dockerode.listContainers({ all: true })
    )
      .filter(
        (c) =>
          c.Labels["org.testcontainers.ryuk"] === "true" &&
          c.Labels["org.testcontainers.session-id"] === sessionId,
      )
      .map((c) => c.Id);
    progress({ stage: "started", id: container.getId(), helpers, sessionId });
    return container;
  };
  const redis = await startRedis();
  assert.equal(
    await redisPing(redis.getHost(), redis.getMappedPort(6379)),
    "+PONG\r\n",
  );
  const set = await redis.exec(["redis-cli", "set", "proof", runId]);
  assert.equal(set.exitCode, 0);
  if (mode === "assertion") {
    progress({ stage: "intentional-assertion-failure" });
    process.exit(23);
  }
  if (mode === "cancel") {
    progress({ stage: "ready-for-cancellation" });
    await new Promise(() => {});
  }
  const postgres = await new GenericContainer(images.postgres)
    .withLabels({ [label]: runId })
    .withEnvironment({
      POSTGRES_PASSWORD: "disposable-framework-proof",
      POSTGRES_DB: "orbwin_test",
    })
    .withExposedPorts(5432)
    .withResourcesQuota({ memory: 256 * 1024 * 1024 })
    .withWaitStrategy(
      Wait.forLogMessage(/database system is ready to accept connections/, 2),
    )
    .withStartupTimeout(90000)
    .start();
  resources.push(postgres);
  progress({ stage: "postgres-started", id: postgres.getId() });
  const query = await postgres.exec([
    "psql",
    "-U",
    "postgres",
    "-d",
    "orbwin_test",
    "-Atc",
    "SELECT 42",
  ]);
  assert.equal(query.exitCode, 0);
  assert.equal(query.output.trim(), "42");
  await redis.restart({ timeout: 10000 });
  assert.equal(
    await redisPing(redis.getHost(), redis.getMappedPort(6379)),
    "+PONG\r\n",
  );
  const get = await redis.exec(["redis-cli", "get", "proof"]);
  assert.equal(get.output.trim(), runId);
  const expected = await redis.exec(["sh", "-c", "exit 7"]);
  assert.equal(expected.exitCode, 7);
  const stream = await redis.logs();
  const log = await new Promise((accept, reject) => {
    const timer = setTimeout(() => {
      stream.destroy();
      reject(new Error("Framework logs timed out"));
    }, 5000);
    stream.once("data", (chunk) => {
      clearTimeout(timer);
      stream.destroy();
      accept(chunk.toString());
    });
    stream.once("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
  });
  assert.ok(log.length > 0);
  for (const container of resources.reverse()) await container.stop();
  progress({
    stage: "passed",
    daemonId: info.ID,
    framework: "12.2.1",
    logBytes: log.length,
  });
  process.exit(0);
}
if (process.argv[2] === "worker") {
  await worker(process.argv[3], process.argv[4]);
} else {
  if (process.env.ORBWIN_QUALIFY_OWNED !== "1")
    throw new Error(
      "Set ORBWIN_QUALIFY_OWNED=1 for this real owned-engine proof.",
    );
  const runId = randomUUID();
  const directory = resolve(
    root,
    `artifacts/qualification/testcontainers-${runId}`,
  );
  mkdirSync(directory, { recursive: true });
  const docker = resolve(
    process.env.ProgramFiles ?? "C:/Program Files",
    "Docker/Docker/resources/bin/docker.exe",
  );
  const config = resolve(identity.home, ".rdorbwin/docker");
  const cli = (args) =>
    execFileSync(docker, ["--config", config, "--host", endpoint, ...args], {
      encoding: "utf8",
      windowsHide: true,
      timeout: 30000,
      maxBuffer: 1024 * 1024,
      env: {
        ...process.env,
        DOCKER_CONTEXT: "",
        DOCKER_TLS_VERIFY: "",
        DOCKER_CERT_PATH: "",
      },
    }).trim();
  const check = () =>
    assert.equal(
      JSON.parse(cli(["info", "--format", "{{json .}}"])).ID,
      identity.daemonId,
    );
  check();
  const protectedFiles = [".docker/config.json", ".kube/config"].map((path) =>
    resolve(process.env.USERPROFILE, path),
  );
  const digest = (path) =>
    existsSync(path)
      ? createHash("sha256").update(readFileSync(path)).digest("hex")
      : null;
  const before = protectedFiles.map(digest);
  const records = [];
  const helpers = new Map();
  const runIds = [];
  async function child(mode) {
    const caseId = `${runId}-${mode}-${randomUUID().slice(0, 8)}`;
    runIds.push(caseId);
    const profile = resolve(directory, caseId);
    mkdirSync(profile, { recursive: true });
    let stdout = "",
      stderr = "",
      killed = false;
    const started = performance.now();
    const processChild = spawn(
      process.execPath,
      [fileURLToPath(import.meta.url), "worker", mode, caseId],
      {
        cwd: root,
        windowsHide: true,
        env: {
          ...process.env,
          USERPROFILE: profile,
          HOME: profile,
          DOCKER_HOST: frameworkEndpoint,
          DOCKER_CONFIG: config,
          DOCKER_CONTEXT: "",
          DOCKER_TLS_VERIFY: "",
          DOCKER_CERT_PATH: "",
          TESTCONTAINERS_HOST_OVERRIDE: "127.0.0.1",
          TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE: "/var/run/docker.sock",
          TESTCONTAINERS_RYUK_DISABLED: "false",
          TESTCONTAINERS_RYUK_TEST_LABEL: "true",
          TESTCONTAINERS_RYUK_RECONNECTION_TIMEOUT: "5s",
          RYUK_CONTAINER_IMAGE: images.ryuk,
          DEBUG: "",
        },
        stdio: ["ignore", "pipe", "pipe"],
      },
    );
    const timer = setTimeout(() => {
      killed = true;
      processChild.kill();
    }, 180000);
    processChild.stdout.on("data", (chunk) => {
      stdout = (stdout + chunk).slice(-262144);
      for (const line of stdout.split("\n")) {
        try {
          const event = JSON.parse(line);
          for (const id of event.helpers ?? [])
            helpers.set(id, event.sessionId);
          if (
            mode === "cancel" &&
            event.stage === "ready-for-cancellation" &&
            !killed
          ) {
            killed = true;
            processChild.kill();
          }
        } catch {}
      }
    });
    processChild.stderr.on("data", (chunk) => {
      stderr = (stderr + chunk).slice(-262144);
    });
    const status = await new Promise((accept, reject) => {
      processChild.once("error", reject);
      processChild.once("exit", (code, signal) => accept({ code, signal }));
    });
    clearTimeout(timer);
    const passed =
      mode === "normal"
        ? status.code === 0
        : mode === "assertion"
          ? status.code === 23
          : killed && stdout.includes("ready-for-cancellation");
    const record = {
      caseId,
      mode,
      status,
      passed,
      milliseconds: performance.now() - started,
      stdout,
      stderr,
    };
    records.push(record);
    writeFileSync(
      resolve(directory, `${caseId}.json`),
      JSON.stringify(record, null, 2),
    );
    console.log(`${mode}: ${passed ? "passed" : "FAILED"}`);
    if (!passed)
      throw new Error(`Framework ${mode} case failed: ${stderr || stdout}`);
  }
  let failure;
  try {
    await child("normal");
    const concurrent = await Promise.allSettled([
      child("normal"),
      child("normal"),
    ]);
    const rejected = concurrent.find((result) => result.status === "rejected");
    if (rejected) throw rejected.reason;
    await child("assertion");
    await child("cancel");
  } catch (error) {
    failure = String(error);
  }
  // Wait for the real helper's cleanup after each process disconnects. Any
  // fallback cleanup is recorded as failure, never counted as a Ryuk pass.
  const list = () =>
    cli(["ps", "--all", "--no-trunc", "--format", "{{json .}}"])
      .split("\n")
      .filter(Boolean)
      .map((line) => JSON.parse(line));
  let remaining = [];
  for (let attempt = 0; attempt < 30; attempt++) {
    check();
    remaining = list().filter(
      (row) =>
        runIds.some((id) => String(row.Labels).includes(`${label}=${id}`)) ||
        helpers.has(row.ID),
    );
    if (!remaining.length) break;
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  const cleanup = [];
  if (remaining.length) {
    failure ??=
      "Ryuk did not clean all recorded test resources within 30 seconds";
    for (const row of remaining) {
      check();
      try {
        cli(["stop", "--time=10", row.ID]);
        cli(["rm", "--volumes", row.ID]);
        cleanup.push({ id: row.ID, removed: true });
      } catch (error) {
        cleanup.push({ id: row.ID, error: String(error) });
      }
    }
  }
  assert.deepEqual(
    protectedFiles.map(digest),
    before,
    "Global client configuration changed",
  );
  check();
  writeFileSync(
    resolve(directory, "result.json"),
    JSON.stringify(
      {
        runId,
        endpoint,
        frameworkEndpoint,
        daemonId: identity.daemonId,
        images,
        records,
        remaining,
        cleanup,
        globalConfigsUnchanged: true,
        passed: !failure,
        failure,
      },
      null,
      2,
    ),
  );
  console.log(`Evidence: ${directory}`);
  if (failure) throw new Error(failure);
}
