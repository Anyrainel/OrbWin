// Explicit installed-engine measurements. Shared WSL VM resources are not
// attributed to OrbWin. This is an initial baseline, not a comparative claim.
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { randomUUID, createHash } from "node:crypto";
import { verifyBuild } from "./build-identity.mjs";
import { verifyIdentity } from "./engine-identity.mjs";
assert.equal(process.env.ORBWIN_QUALIFY_OWNED, "1");
const identity = JSON.parse(
  readFileSync("artifacts/qualification/engine-identity.json"),
);
verifyIdentity(
  identity,
  { ID: identity.daemonId, OSType: "linux" },
  verifyBuild(process.cwd()),
);
const installed = resolve(process.env.LOCALAPPDATA, "OrbWin"),
  profile = resolve(process.env.LOCALAPPDATA, "OrbWinData");
assert.equal(
  identity.home,
  resolve(profile, "home"),
  "Use the ordinary Windows installed profile",
);
const run = randomUUID(),
  name = `orbwin-baseline-${run.slice(0, 8)}`,
  directory = resolve("artifacts/qualification", name);
mkdirSync(directory);
const file = resolve(directory, "result.json");
const budgets = {
  warmServiceAndGuestStartupMs: 180000,
  cachedBuildMs: 30000,
  baseCachedNoRunCacheBuildMs: 60000,
  filesystem16MiBMs: 60000,
  idleBackendMeanCpuCores: 1,
  idleUiMeanCpuCores: 1,
  idleBackendPrivateBytes: 1536 << 20,
  idleUiPrivateBytes: 1536 << 20,
};
const result = {
  schema: 1,
  run,
  name,
  identity,
  installed,
  startedAt: new Date().toISOString(),
  budgets,
  conditions: {
    kernel: "shared WSL VM remains warm; other engines are not stopped",
    build: "base image cached; explicit cached versus --no-cache RUN layers",
    filesystem:
      "16 MiB zeros, sync and SHA-256; includes container/client startup",
    idle: "Windows owned process trees only; shared vmmemWSL is not attributed",
  },
  stages: [],
  startupMs: [],
  buildMs: [],
  filesystem: [],
  passed: false,
  cleaned: false,
};
const save = (stage) => {
  result.stage = stage;
  result.stages.push({ stage, time: new Date().toISOString() });
  writeFileSync(file, JSON.stringify(result, null, 2));
  console.log(stage);
};
const docker = resolve(installed, "runtime/tools/bin/docker.exe"),
  rdd = resolve(installed, "runtime/backend/rdd.exe"),
  config = resolve(identity.home, ".rdorbwin/docker");
assert.equal(
  createHash("sha256").update(readFileSync(rdd)).digest("hex"),
  identity.binarySha256,
);
const env = {
  ...process.env,
  DOCKER_HOST: identity.endpoint,
  DOCKER_CONFIG: config,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  BUILDKIT_HOST: "",
  BUILDX_BUILDER: "default",
  BUILDX_CONFIG: resolve(config, "buildx"),
  PATH: resolve(installed, "runtime/backend") + ";" + process.env.PATH,
};
const execute = (binary, args, seconds = 60, extra = {}) =>
  execFileSync(binary, args, {
    env: { ...env, ...extra },
    encoding: "utf8",
    windowsHide: true,
    timeout: seconds * 1000,
    maxBuffer: 4 << 20,
  }).trim();
const cli = (args, seconds) =>
  execute(
    docker,
    ["--config", config, "--host", identity.endpoint, ...args],
    seconds,
  );
const service = (args, seconds = 80) =>
  execute(rdd, args, seconds, {
    USERPROFILE: identity.home,
    HOME: identity.home,
    RDD_INSTANCE: "orbwin",
    RDD_DEVELOPER_MODE: "1",
  });
// Fixed read-only CIM commands are supported even where the host disallows
// unsigned .ps1 files. Do not change execution policy or interpolate paths into
// command text; the selected installation is an environment value.
function idleSnapshot() {
  const command = String.raw`$ErrorActionPreference='Stop'; $installation=$env:ORBWIN_MEASURE_INSTALL; $all=@(Get-CimInstance Win32_Process); $groups=@{}; foreach($item in $all){if($item.ExecutablePath -eq (Join-Path $installation 'orbwin.exe')){$groups[[int]$item.ProcessId]='ui'}elseif($item.ExecutablePath -eq (Join-Path $installation 'runtime/backend/rdd.exe')){$groups[[int]$item.ProcessId]='backend'}}; if(-not($groups.Values -contains 'ui') -or -not($groups.Values -contains 'backend')){throw 'Installed UI/backend missing'}; do{$added=$false;foreach($item in $all){if(-not $groups.ContainsKey([int]$item.ProcessId) -and $groups.ContainsKey([int]$item.ParentProcessId)){$groups[[int]$item.ProcessId]=$groups[[int]$item.ParentProcessId];$added=$true}}}while($added); $values=@();foreach($item in $all){$key=[int]$item.ProcessId;if($groups.ContainsKey($key)){$live=Get-Process -Id $key -ErrorAction Stop;$values += [pscustomobject]@{id=$key;group=$groups[$key];created=$item.CreationDate.ToUniversalTime().ToString('o');cpuSeconds=$live.CPU;privateBytes=$live.PrivateMemorySize64;workingSetBytes=$live.WorkingSet64}}}; ConvertTo-Json -InputObject $values -Depth 4 -Compress`;
  return JSON.parse(
    execute(
      "powershell.exe",
      ["-NoProfile", "-NonInteractive", "-Command", command],
      15,
      { ORBWIN_MEASURE_INSTALL: installed },
    ),
  );
}
const globalConfigs = [
  resolve(process.env.USERPROFILE, ".docker/config.json"),
  resolve(process.env.USERPROFILE, ".kube/config"),
];
const hashes = () =>
  globalConfigs.map((path) =>
    createHash("sha256").update(readFileSync(path)).digest("hex"),
  );
const before = hashes();
const pin = JSON.parse(readFileSync("tests/registry/images.lock.json")).base;
assert.equal(cli(["info", "--format", "{{.ID}}"]), identity.daemonId);
assert.equal(
  cli(["ps", "--quiet"]),
  "",
  "Baseline restarts require an engine without running workloads",
);
const control = JSON.parse(
  execute(
    rdd,
    [
      "kubectl",
      "--kubeconfig",
      resolve(
        identity.home,
        "AppData/Local/rancher-desktop-orbwin/config.yaml",
      ),
      "get",
      "apps/app",
      "-o",
      "json",
    ],
    15,
    {
      USERPROFILE: identity.home,
      HOME: identity.home,
      RDD_INSTANCE: "orbwin",
      RDD_DEVELOPER_MODE: "1",
    },
  ),
);
assert.equal(
  control.spec.kubernetes.enabled,
  false,
  "Record the empty, Kubernetes-disabled baseline first",
);
let volume, buildTag;
const containers = [];
function fixture(args) {
  const container = `${name}-${containers.length}`;
  containers.push(container);
  return cli(
    [
      "run",
      "--rm",
      "--name",
      container,
      "--label",
      `org.orbwin.qualification=${run}`,
      ...args,
    ],
    75,
  );
}
function marker() {
  assert.equal(
    fixture([
      "--mount",
      `type=volume,source=${volume},target=/data,readonly`,
      pin,
      "cat",
      "/data/marker",
    ]),
    run,
  );
}
try {
  save("Collecting host/version metadata");
  const system = JSON.parse(
    execute(
      "powershell.exe",
      [
        "-NoProfile",
        "-NonInteractive",
        "-Command",
        "$os=Get-CimInstance Win32_OperatingSystem; $cpu=Get-CimInstance Win32_Processor; [pscustomobject]@{caption=$os.Caption;version=$os.Version;build=$os.BuildNumber;ramBytes=[int64]$os.TotalVisibleMemorySize*1024;cpu=($cpu.Name -join ', ');logicalCpus=($cpu.NumberOfLogicalProcessors | Measure-Object -Sum).Sum} | ConvertTo-Json -Compress",
      ],
      15,
    ),
  );
  result.host = system;
  result.wsl = execute("wsl.exe", ["--version"], 15).replaceAll("\0", "");
  result.docker = JSON.parse(cli(["info", "--format", "{{json .}}"]));
  // Do not retain labels, environment, registry configuration or workload files.
  result.docker = Object.fromEntries(
    [
      "ID",
      "ServerVersion",
      "OSType",
      "Architecture",
      "NCPU",
      "MemTotal",
      "Driver",
      "OperatingSystem",
      "KernelVersion",
    ].map((key) => [key, result.docker[key]]),
  );
  save("Measuring 30 seconds of empty-engine/UI Windows idle processes");
  const idleBefore = idleSnapshot(),
    idleStart = performance.now();
  await new Promise((resolve) => setTimeout(resolve, 30000));
  const idleAfter = idleSnapshot(),
    durationSeconds = (performance.now() - idleStart) / 1000;
  const keys = (values) =>
    values.map((item) => `${item.id}:${item.created}`).sort();
  const groups = {};
  for (const name of ["backend", "ui"]) {
    const before = idleBefore.filter((item) => item.group === name),
      after = idleAfter.filter((item) => item.group === name);
    const sum = (values, key) =>
      values.reduce((total, item) => total + item[key], 0);
    groups[name] = {
      meanCpuCores:
        (sum(after, "cpuSeconds") - sum(before, "cpuSeconds")) /
        durationSeconds,
      privateBytes: sum(after, "privateBytes"),
      workingSetBytes: sum(after, "workingSetBytes"),
    };
  }
  result.idle = {
    durationSeconds,
    processSetStable:
      JSON.stringify(keys(idleBefore)) === JSON.stringify(keys(idleAfter)),
    groups,
    before: idleBefore,
    after: idleAfter,
    workingSetCaveat:
      "Working-set sums include shared pages; privateBytes is private committed memory. Shared vmmemWSL is excluded.",
  };
  assert.equal(
    result.idle.processSetStable,
    true,
    "Idle process sets changed; inspect and repeat a stable sample",
  );
  assert(
    result.idle.groups.backend.meanCpuCores <= budgets.idleBackendMeanCpuCores,
  );
  assert(result.idle.groups.ui.meanCpuCores <= budgets.idleUiMeanCpuCores);
  assert(
    result.idle.groups.backend.privateBytes <= budgets.idleBackendPrivateBytes,
  );
  assert(result.idle.groups.ui.privateBytes <= budgets.idleUiPrivateBytes);
  try {
    cli(["image", "inspect", pin]);
  } catch {
    cli(["pull", pin], 120);
  }
  volume = cli([
    "volume",
    "create",
    "--label",
    `org.orbwin.qualification=${run}`,
    name,
  ]);
  fixture([
    "--mount",
    `type=volume,source=${volume},target=/data`,
    pin,
    "sh",
    "-c",
    `printf '%s' '${run}' > /data/marker`,
  ]);
  for (let sample = 0; sample < 3; sample++) {
    save(`Warm control-plane and guest restart ${sample + 1}/3`);
    assert.equal(cli(["ps", "--quiet"]), "");
    service(["service", "stop", "--timeout=60s"]);
    const stopped = spawnSync(
      docker,
      [
        "--config",
        config,
        "--host",
        identity.endpoint,
        "info",
        "--format",
        "{{.ID}}",
      ],
      { env, encoding: "utf8", windowsHide: true, timeout: 5000 },
    );
    assert.notEqual(
      stopped.status,
      0,
      "The owned endpoint must stop before measuring a new start",
    );
    const started = performance.now();
    service([
      "service",
      "start",
      "--controllers=*,-path-management",
      "--timeout=60s",
    ]);
    service(
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
      200,
    );
    let observed;
    const deadline = Date.now() + 120000;
    for (;;) {
      try {
        observed = cli(["info", "--format", "{{.ID}}"], 5);
        break;
      } catch (error) {
        if (Date.now() >= deadline) throw error;
        await new Promise((resolve) => setTimeout(resolve, 250));
      }
    }
    assert.equal(observed, identity.daemonId);
    const elapsed = performance.now() - started;
    assert(elapsed <= budgets.warmServiceAndGuestStartupMs);
    result.startupMs.push(elapsed);
    marker();
  }
  const context = resolve(directory, "build");
  mkdirSync(context);
  writeFileSync(
    resolve(context, "Dockerfile"),
    `FROM ${pin}\nRUN printf '${run}' > /baseline-marker\n`,
  );
  buildTag = `${name}:build`;
  for (const cached of [false, true])
    for (let sample = 0; sample < 3; sample++) {
      save(
        `${cached ? "Cached" : "Base cached, RUN uncached"} build ${sample + 1}/3`,
      );
      const started = performance.now();
      cli(
        [
          "buildx",
          "build",
          "--load",
          ...(cached ? [] : ["--no-cache"]),
          "--tag",
          buildTag,
          context,
        ],
        90,
      );
      const elapsed = performance.now() - started;
      assert(
        elapsed <=
          (cached
            ? budgets.cachedBuildMs
            : budgets.baseCachedNoRunCacheBuildMs),
      );
      result.buildMs.push({ cached, milliseconds: elapsed });
      assert.equal(fixture([buildTag, "cat", "/baseline-marker"]), run);
    }
  const cSource = resolve(profile, "qualification", name);
  mkdirSync(cSource, { recursive: true });
  const fSource = resolve(directory, "filesystem");
  mkdirSync(fSource);
  const sources = [
    { name: "Windows C drive", source: cSource },
    { name: "Windows F drive", source: fSource },
  ];
  // The record filename is derived to avoid accepting an arbitrary Linux path.
  const ubuntuId = createHash("sha256").update("Ubuntu").digest("hex");
  const recordPath = resolve(
    identity.home,
    ".rdorbwin/wsl-projects",
    `${ubuntuId}.json`,
  );
  if (existsSync(recordPath)) {
    const record = JSON.parse(readFileSync(recordPath));
    assert.equal(record.name, "Ubuntu");
    assert.match(
      record.sharedSource,
      /^\/mnt\/wsl\/orbwin\/projects\/[a-f0-9]{24}$/,
    );
    const signature = execute(
      "wsl.exe",
      [
        "-d",
        "Ubuntu",
        "-u",
        "root",
        "--exec",
        "stat",
        "-Lc",
        "%d:%i",
        record.sharedSource,
      ],
      10,
    );
    assert.equal(signature, record.signature);
    const child = `baseline-${run}`;
    execute(
      "wsl.exe",
      ["-d", "Ubuntu", "--exec", "mkdir", "--", `${record.source}/${child}`],
      10,
    );
    sources.push({
      name: "Selected Ubuntu source",
      source: `${record.sharedSource}/${child}`,
    });
  }
  const expected = createHash("sha256")
    .update(Buffer.alloc(16 << 20))
    .digest("hex");
  for (const source of sources)
    for (let sample = 0; sample < 3; sample++) {
      save(`${source.name} sync/write/read ${sample + 1}/3`);
      const started = performance.now();
      const output = fixture([
        "--mount",
        `type=bind,source=${source.source},target=/data`,
        pin,
        "sh",
        "-c",
        "dd if=/dev/zero of=/data/blob bs=1M count=16 2>/dev/null && sync /data/blob && sha256sum /data/blob && rm /data/blob",
      ]);
      assert.equal(output.split(/\s+/)[0], expected);
      const elapsed = performance.now() - started;
      assert(elapsed <= budgets.filesystem16MiBMs);
      result.filesystem.push({ source: source.name, milliseconds: elapsed });
    }
  save("Measurements complete; checking retained data and cleanup");
  marker();
  assert.equal(cli(["info", "--format", "{{.ID}}"]), identity.daemonId);
  assert.deepEqual(hashes(), before);
  result.passed = true;
} catch (error) {
  result.error = String(error);
  throw error;
} finally {
  try {
    for (const container of containers) {
      const found = cli([
        "ps",
        "--all",
        "--filter",
        `name=^/${container}$`,
        "--quiet",
        "--no-trunc",
      ]);
      if (found) {
        const item = JSON.parse(cli(["inspect", found]))[0];
        assert.equal(item.Config.Labels["org.orbwin.qualification"], run);
        cli(["rm", "--force", item.Id]);
      }
    }
    if (
      buildTag &&
      cli(["image", "ls", "--filter", `reference=${buildTag}`, "--quiet"])
    )
      cli(["image", "rm", buildTag]);
    if (volume) {
      assert.equal(
        JSON.parse(cli(["volume", "inspect", volume]))[0].Labels[
          "org.orbwin.qualification"
        ],
        run,
      );
      cli(["volume", "rm", volume]);
    }
    result.cleaned = true;
  } catch (cleanupError) {
    result.cleaned = false;
    result.cleanupError = String(cleanupError);
    result.passed = false;
    process.exitCode = 1;
  }
  result.finishedAt = new Date().toISOString();
  writeFileSync(file, JSON.stringify(result, null, 2));
}
console.log(`Baseline recorded: ${file}`);
