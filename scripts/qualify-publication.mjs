// Explicit, scoped runtime proof. No lifecycle changes or global client config.
import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { randomUUID } from "node:crypto";
import { createServer, createConnection } from "node:net";
import { verifyBuild } from "./build-identity.mjs";

assert.equal(
  process.env.ORBWIN_QUALIFY_OWNED,
  "1",
  "Explicit owned-engine opt-in required",
);
const identity = JSON.parse(
  readFileSync("artifacts/qualification/runtime-identity.json"),
);
assert.equal(identity.identityVerified, true);
assert.equal(identity.binarySha256, verifyBuild(process.cwd()).binarySha256);
assert.equal(
  identity.endpoint,
  "npipe:////./pipe/docker-rancher-desktop-orbwin",
);
const run = `orbwin-publish-${randomUUID().slice(0, 8)}`;
const directory = resolve("artifacts/qualification", run);
mkdirSync(directory);
writeFileSync(
  resolve(directory, "config.json"),
  JSON.stringify({ auths: { "https://index.docker.io/v1/": {} } }),
);
const linux = (path) =>
  resolve(path)
    .replace(/\\/g, "/")
    .replace(/^([a-z]):/i, (_, drive) => `/mnt/${drive.toLowerCase()}`);
const cli = resolve("artifacts/runtime-tools/bin/docker.exe");
const linuxCli = linux("artifacts/runtime-tools/linux/docker");
const execute = promisify(execFile);
const evidence = {
  run,
  daemonId: identity.daemonId,
  binarySha256: identity.binarySha256,
  steps: [],
  passed: false,
};
const owned = [];
const env = {
  ...process.env,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  DOCKER_HOST: identity.endpoint,
};
async function docker(args, source = "windows", timeout = 40000) {
  const command = source === "windows" ? cli : "wsl.exe";
  const prefix =
    source === "windows"
      ? ["--config", directory, "--host", identity.endpoint]
      : [
          "-d",
          "Ubuntu",
          "-u",
          "root",
          "--exec",
          linuxCli,
          "--config",
          linux(directory),
          "--host",
          "unix:///mnt/wsl/orbwin/docker.sock",
        ];
  try {
    const { stdout } = await execute(command, [...prefix, ...args], {
      env,
      timeout,
      windowsHide: true,
      maxBuffer: 2 << 20,
    });
    evidence.steps.push({ source, args, ok: true, output: stdout.trim() });
    return stdout.trim();
  } catch (error) {
    evidence.steps.push({
      source,
      args,
      ok: false,
      error: String(error),
      stderr: String(error.stderr ?? ""),
    });
    throw error;
  }
}
async function ping(port) {
  return new Promise((resolve, reject) => {
    const socket = createConnection({ host: "127.0.0.1", port });
    let response = "";
    socket.setTimeout(3000, () =>
      socket.destroy(new Error("published Redis timed out")),
    );
    socket.on("connect", () => socket.write("*1\r\n$4\r\nPING\r\n"));
    socket.on("error", reject);
    socket.on("data", (bytes) => {
      response += bytes;
      if (response.includes("\r\n")) {
        socket.destroy();
        resolve(response);
      }
    });
  });
}
const image =
  "redis:7.4.2-alpine@sha256:02419de7eddf55aa5bcf49efb74e88fa8d931b4d77c07eff8a6b2144472b6952";
const heldClients = new Set();
const held = createServer((socket) => {
  heldClients.add(socket);
  socket.on("close", () => heldClients.delete(socket));
  socket.on("error", () => {});
  socket.end("foreign-listener\r\n", () => socket.destroy());
});
await new Promise((done, reject) => {
  held.once("error", reject);
  held.listen(0, "127.0.0.1", done);
});
const busy = held.address().port;
try {
  assert.equal(
    await docker(["info", "--format", "{{.ID}}"]),
    identity.daemonId,
  );
  const conflict = `${run}-conflict`;
  owned.push(conflict);
  await assert.rejects(
    docker([
      "run",
      "-d",
      "--name",
      conflict,
      "--label",
      `dev.orbwin.proof=${run}`,
      "-p",
      `127.0.0.1:${busy}:6379`,
      image,
    ]),
    /occupied|unavailable|already in use|conflict/i,
  );
  const conflictState = JSON.parse(await docker(["inspect", conflict]))[0]
    .State;
  assert.equal(
    conflictState.Running,
    false,
    "busy host port must fail before starting the container",
  );
  assert.equal(
    await ping(busy),
    "foreign-listener\r\n",
    "foreign listener remains intact",
  );
  const fixtures = Array.from({ length: 6 }, (_, index) => ({
    name: `${run}-${index}`,
    source: index < 3 ? "windows" : "wsl",
    all: index === 2,
  }));
  for (const fixture of fixtures) owned.push(fixture.name);
  const starts = await Promise.allSettled(
    fixtures.map((fixture) =>
      docker(
        [
          "run",
          "-d",
          "--name",
          fixture.name,
          "--label",
          `dev.orbwin.proof=${run}`,
          ...(fixture.all ? ["-P"] : ["-p", "127.0.0.1::6379"]),
          image,
        ],
        fixture.source,
      ),
    ),
  );
  for (const result of starts)
    if (result.status === "rejected") throw result.reason;
  // Exercise a socket explicitly delegated to a workload. It must reach the
  // same adapter, rather than bypassing Windows allocation via Moby's socket.
  const nested = `${run}-delegated`;
  owned.push(nested);
  await docker([
    "run",
    "--rm",
    "--label",
    `dev.orbwin.proof=${run}`,
    "--mount",
    "type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock",
    "--mount",
    `type=bind,src=${linuxCli},dst=/docker,readonly`,
    "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30",
    "/docker",
    "--host",
    "unix:///var/run/docker.sock",
    "run",
    "-d",
    "--name",
    nested,
    "--label",
    `dev.orbwin.proof=${run}`,
    "-p",
    "127.0.0.1::6379",
    image,
  ]);
  fixtures.push({ name: nested, source: "delegated", all: false });
  const ports = [];
  for (const fixture of fixtures) {
    const data = JSON.parse(await docker(["inspect", fixture.name]))[0];
    const port = Number(data.NetworkSettings.Ports["6379/tcp"][0].HostPort);
    ports.push(port);
    assert.notEqual(port, busy);
    assert.equal(await ping(port), "+PONG\r\n");
    await docker(["restart", fixture.name]);
    const after = JSON.parse(await docker(["inspect", fixture.name]))[0];
    assert.equal(
      Number(after.NetworkSettings.Ports["6379/tcp"][0].HostPort),
      port,
    );
    assert.equal(await ping(port), "+PONG\r\n");
  }
  assert.equal(
    new Set(ports).size,
    ports.length,
    "concurrent clients received distinct Windows ports",
  );
  evidence.ports = ports;
  evidence.passed = true;
} catch (error) {
  evidence.error = String(error);
  throw error;
} finally {
  for (const name of owned) {
    try {
      await docker(["rm", "-f", name]);
    } catch {
      evidence.passed = false;
    }
  }
  for (const socket of heldClients) socket.destroy();
  await new Promise((done) => held.close(done));
  writeFileSync(
    resolve(directory, "result.json"),
    JSON.stringify(evidence, null, 2),
  );
}
assert.equal(
  evidence.passed,
  true,
  "publication/cleanup failed; inspect the retained evidence",
);
console.log(`Windows, WSL and delegated publication passed: ${directory}`);
