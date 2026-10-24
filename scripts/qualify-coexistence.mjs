// Read actual daemon identities, maintain scoped workloads across an OrbWin
// restart, and remove only this run's resources. Never stop another engine.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { randomUUID, createHash } from "node:crypto";
import assert from "node:assert/strict";
import { createConnection } from "node:net";
import { verifyBuild } from "./build-identity.mjs";
if (process.env.ORBWIN_QUALIFY_OWNED !== "1")
  throw new Error("Owned-engine opt-in required");
const identity = JSON.parse(
  readFileSync("artifacts/qualification/runtime-identity.json"),
);
assert.equal(identity.binarySha256, verifyBuild(process.cwd()).binarySha256);
assert.equal(
  identity.endpoint,
  "npipe:////./pipe/docker-rancher-desktop-orbwin",
);
const run = `orbwin-coexist-${randomUUID().slice(0, 8)}`;
const folder = resolve("artifacts/coexistence", run);
mkdirSync(folder, { recursive: true });
const windowsConfig = resolve(folder, "docker");
mkdirSync(windowsConfig);
writeFileSync(
  resolve(windowsConfig, "config.json"),
  JSON.stringify({
    auths: { "https://index.docker.io/v1/": {} },
    cliPluginsExtraDirs: [
      resolve("artifacts/runtime-tools/docker-cli-plugins"),
    ],
  }),
);
const linuxConfig = `/mnt/f/Codes/Orbwin/artifacts/coexistence/${run}/docker-linux`;
mkdirSync(resolve(folder, "docker-linux"));
writeFileSync(
  resolve(folder, "docker-linux/config.json"),
  JSON.stringify({
    auths: { "https://index.docker.io/v1/": {} },
    cliPluginsExtraDirs: ["/mnt/f/Codes/Orbwin/artifacts/runtime-tools/linux"],
  }),
);
const env = {
  ...process.env,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  BUILDKIT_HOST: "",
  BUILDX_BUILDER: "default",
  BUILDX_CONFIG: resolve(windowsConfig, "buildx"),
};
const steps = [],
  checks = [];
function execute(binary, args, seconds = 30, extraEnv = {}) {
  try {
    const output = execFileSync(binary, args, {
      env: { ...env, ...extraEnv },
      encoding: "utf8",
      timeout: seconds * 1000,
      maxBuffer: 2 << 20,
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    steps.push({ binary, args, success: true, output });
    return output.trim();
  } catch (error) {
    steps.push({
      binary,
      args,
      success: false,
      error: String(error),
      stderr: String(error.stderr ?? ""),
    });
    throw error;
  }
}
const engines = [{ name: "orbwin", host: identity.endpoint }];
if (process.argv.includes("--desktop") || process.argv.includes("--all"))
  engines.push({
    name: "desktop",
    host: "npipe:////./pipe/dockerDesktopLinuxEngine",
  });
if (process.argv.includes("--rancher") || process.argv.includes("--all"))
  engines.push({
    name: "rancher",
    host: "unix:///mnt/wsl/rancher-desktop/run/docker.sock",
    linux: true,
  });
assert.ok(engines.length > 1, "Select --desktop, --rancher or --all");
function docker(engine, args, seconds = 30) {
  return engine.linux
    ? execute(
        "wsl.exe",
        [
          "-d",
          "Ubuntu",
          "-u",
          "root",
          "--exec",
          "env",
          "-u",
          "DOCKER_CONTEXT",
          "-u",
          "DOCKER_TLS_VERIFY",
          "-u",
          "DOCKER_CERT_PATH",
          "-u",
          "BUILDKIT_HOST",
          "BUILDX_BUILDER=default",
          `BUILDX_CONFIG=${linuxConfig}/buildx`,
          "/mnt/f/Codes/Orbwin/artifacts/runtime-tools/linux/docker",
          "--config",
          linuxConfig,
          "--host",
          engine.host,
          ...args,
        ],
        seconds,
      )
    : execute(
        resolve("artifacts/runtime-tools/bin/docker.exe"),
        ["--config", windowsConfig, "--host", engine.host, ...args],
        seconds,
      );
}
const protectedPaths = [
  resolve(process.env.USERPROFILE, ".docker/config.json"),
  resolve(process.env.USERPROFILE, ".kube/config"),
];
const hashes = () =>
  protectedPaths.map((path) => ({
    path,
    hash: existsSync(path)
      ? createHash("sha256").update(readFileSync(path)).digest("hex")
      : null,
  }));
const beforeHashes = hashes();
const owned = [];
const conflicts = [];
const image =
  "redis:7.4.2-alpine@sha256:02419de7eddf55aa5bcf49efb74e88fa8d931b4d77c07eff8a6b2144472b6952";
async function ping(engine) {
  const address = JSON.parse(
    docker(engine, [
      "inspect",
      "--format",
      "{{json .NetworkSettings.Ports}}",
      engine.container,
    ]),
  )["6379/tcp"][0];
  const observedPort = Number(address.HostPort);
  if (engine.port !== undefined)
    assert.equal(
      observedPort,
      engine.port,
      `${engine.name} published port changed across the owned restart`,
    );
  engine.port = observedPort;
  const deadline = Date.now() + 20000;
  let last;
  while (Date.now() < deadline) {
    try {
      const result = await new Promise((resolve, reject) => {
        const socket = createConnection({
          host: "127.0.0.1",
          port: Number(address.HostPort),
        });
        let bytes = "";
        socket.setTimeout(2000, () =>
          socket.destroy(new Error("TCP readiness timed out")),
        );
        socket.on("error", reject);
        socket.on("connect", () =>
          socket.write("*2\r\n$3\r\nGET\r\n$5\r\nproof\r\n"),
        );
        socket.on("data", (chunk) => {
          bytes += chunk;
          if (bytes.endsWith("\r\n") && bytes.split("\r\n").length >= 3) {
            socket.end();
            resolve(bytes);
          }
        });
      });
      assert.equal(result, `$${engine.name.length}\r\n${engine.name}\r\n`);
      return;
    } catch (error) {
      last = error;
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
  }
  throw last;
}
function rdd(args, seconds = 90) {
  return execute(resolve("artifacts/backend/rdd.exe"), args, seconds, {
    USERPROFILE: identity.home,
    HOME: identity.home,
    RDD_INSTANCE: "orbwin",
    RDD_DEVELOPER_MODE: "1",
    RDD_KEEP_LOGS: "1",
  });
}
let failure;
try {
  for (const engine of engines) {
    engine.id = docker(engine, ["info", "--format", "{{.ID}}"]);
    if (engine.name === "orbwin") assert.equal(engine.id, identity.daemonId);
    assert.ok(engine.id);
    engine.volume = `${run}-${engine.name}`;
    engine.container = `${run}-${engine.name}`;
    docker(engine, [
      "volume",
      "create",
      "--label",
      `org.orbwin.qualification=${run}`,
      engine.volume,
    ]);
    owned.push(engine);
    docker(
      engine,
      [
        "run",
        "-d",
        "--name",
        engine.container,
        "--label",
        `org.orbwin.qualification=${run}`,
        "--restart=unless-stopped",
        "--mount",
        `type=volume,source=${engine.volume},target=/data`,
        "-p",
        "127.0.0.1::6379",
        image,
        "redis-server",
        "--appendonly",
        "yes",
        "--dir",
        "/data",
      ],
      120,
    );
    let initialized = false;
    for (let attempt = 0; attempt < 30; attempt++) {
      try {
        assert.equal(
          docker(engine, [
            "exec",
            engine.container,
            "redis-cli",
            "SET",
            "proof",
            engine.name,
          ]),
          "OK",
        );
        initialized = true;
        break;
      } catch {
        await new Promise((resolve) => setTimeout(resolve, 250));
      }
    }
    assert.ok(initialized, `${engine.name} Redis did not become ready`);
    engine.started = docker(engine, [
      "inspect",
      "--format",
      "{{.State.StartedAt}}",
      engine.container,
    ]);
    await ping(engine);
  }
  assert.equal(
    new Set(engines.map((engine) => engine.id)).size,
    engines.length,
  );
  checks.push("distinct daemons and simultaneous Redis TCP workloads");
  for (const engine of engines)
    assert.equal(
      docker(engine, [
        "ps",
        "-aq",
        "--filter",
        `label=org.orbwin.qualification=${run}`,
      ]).split(/\r?\n/).length,
      1,
    );
  checks.push("each engine sees only its own labeled workload");
  for (const external of engines.slice(1)) {
    const conflict = `${run}-conflict-${external.name}`;
    docker(engines[0], [
      "create",
      "--name",
      conflict,
      "--label",
      `org.orbwin.qualification=${run}`,
      "-p",
      `127.0.0.1:${external.port}:6379`,
      image,
    ]);
    conflicts.push(conflict);
    assert.throws(
      () => docker(engines[0], ["start", conflict]),
      /unavailable|occupied|already in use|conflict/i,
    );
    assert.equal(
      docker(engines[0], [
        "inspect",
        "--format",
        "{{.State.Running}}",
        conflict,
      ]),
      "false",
    );
    await ping(external);
  }
  checks.push(
    "ports occupied by other engines fail before OrbWin container start; external traffic is preserved",
  );
  // Registry ownership is checked before the only lifecycle mutation.
  const base = execute("powershell.exe", [
    "-NoProfile",
    "-NonInteractive",
    "-Command",
    "$r=Get-ChildItem HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Lxss|Get-ItemProperty|Where-Object DistributionName -eq 'lima-orbwin'; if(@($r).Count -ne 1){throw 'ambiguous registration'}; $r.BasePath",
  ]);
  assert.ok(
    base
      .replace(/^\\\\\?\\/, "")
      .toLowerCase()
      .startsWith(identity.home.toLowerCase() + "\\"),
  );
  rdd(["service", "stop", "--timeout=60s"], 70);
  for (const engine of engines.slice(1)) {
    assert.equal(docker(engine, ["info", "--format", "{{.ID}}"]), engine.id);
    await ping(engine);
  }
  rdd(
    ["service", "start", "--controllers=*,-path-management", "--timeout=60s"],
    70,
  );
  rdd(
    [
      "ctl",
      "wait-condition",
      "limavms/orbwin",
      "Running=True",
      "--reason=Started",
      "--since=startup",
      "--namespace=rancher-desktop",
      "--timeout=180s",
    ],
    190,
  );
  let ready = false;
  for (let attempt = 0; attempt < 30; attempt++) {
    try {
      ready =
        docker(engines[0], ["info", "--format", "{{.ID}}"], 5) ===
        identity.daemonId;
      if (ready) break;
    } catch {}
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  assert.ok(ready, "owned daemon did not return after restart");
  for (const engine of engines) {
    assert.equal(docker(engine, ["info", "--format", "{{.ID}}"]), engine.id);
    await ping(engine);
    if (engine.name !== "orbwin")
      assert.equal(
        docker(engine, [
          "inspect",
          "--format",
          "{{.State.StartedAt}}",
          engine.container,
        ]),
        engine.started,
      );
  }
  checks.push(
    "OrbWin restart preserves external workloads, daemon identities and all sentinel data",
  );
} catch (error) {
  failure = String(error);
} finally {
  for (const container of conflicts) {
    try {
      docker(engines[0], ["rm", "-f", container]);
    } catch (error) {
      failure = `${failure ?? ""}\nConflict cleanup: ${error}`;
    }
  }
  for (const engine of owned) {
    try {
      docker(engine, ["rm", "-f", engine.container]);
      docker(engine, ["volume", "rm", engine.volume]);
    } catch (error) {
      failure = `${failure ?? ""}\nCleanup: ${error}`;
    }
  }
  const afterHashes = hashes();
  if (JSON.stringify(beforeHashes) !== JSON.stringify(afterHashes))
    failure = `${failure ?? ""}\nGlobal client config changed during the workload proof`;
  writeFileSync(
    resolve(folder, "result.json"),
    JSON.stringify(
      {
        run,
        time: new Date().toISOString(),
        engines,
        checks,
        beforeHashes,
        afterHashes,
        success: !failure,
        failure,
        steps,
      },
      null,
      2,
    ),
  );
}
if (failure) throw new Error(failure);
if (process.argv.includes("--publish-qualification")) {
  assert.ok(
    process.argv.includes("--all"),
    "Publishing qualification requires all three engines",
  );
  assert.equal(identity.binarySha256, verifyBuild(process.cwd()).binarySha256);
  const qualified = {
    ...identity,
    coexistenceQualified: true,
    qualification: {
      evidence: resolve(folder, "result.json"),
      engines: engines.map((engine) => ({
        name: engine.name,
        daemonId: engine.id,
      })),
      checks,
      time: new Date().toISOString(),
    },
  };
  writeFileSync(
    "artifacts/qualification/engine-identity.json",
    JSON.stringify(qualified, null, 2),
  );
}
console.log(checks.join("\n"));
