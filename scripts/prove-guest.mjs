// One-time isolated guest proof. Refuses an existing lima-orbwin registration.
// Owns a fresh home and stops only its rdd service after testing. Never deletes
// registrations/disks or changes another engine, distro, context or global PATH.
import { execFileSync } from "node:child_process";
import {
  readFileSync,
  writeFileSync,
  existsSync,
  mkdirSync,
  copyFileSync,
} from "node:fs";
import { resolve, relative, isAbsolute } from "node:path";
import { fileURLToPath } from "node:url";
import { randomUUID, createHash } from "node:crypto";
import { endpoint } from "./engine-identity.mjs";
import { conflictingRoutes } from "./network-preflight.mjs";
import { verifyBuild } from "./build-identity.mjs";
import { runBaseline } from "./service-baseline.mjs";
import { waitForEngine } from "./wait-engine.mjs";
const root = fileURLToPath(new URL("../", import.meta.url));
const runId = randomUUID();
const resume = process.argv
  .find((value) => value.startsWith("--resume="))
  ?.slice(9);
if (resume && !/^gp-[0-9a-f]{8}$/.test(resume))
  throw new Error("Resume requires a recorded gp-XXXXXXXX proof directory.");
const home = resolve(root, `artifacts/${resume ?? `gp-${runId.slice(0, 8)}`}`);
const executable = resolve(root, "artifacts/backend/rdd.exe");
const manifest = verifyBuild(root);
const previous = resume
  ? JSON.parse(readFileSync(resolve(home, "evidence.json")))
  : null;
const upgrade = process.argv.includes("--upgrade");
if (upgrade && !resume) throw new Error("Upgrade requires an explicitly selected recorded guest.");
if (
  previous &&
  (previous.home !== home ||
    (previous.manifest.binarySha256 !== manifest.binarySha256 && !upgrade))
)
  throw new Error(
    "Resume record is foreign or stale; preserve it for inspection.",
  );
if (upgrade && (!previous.passed || !previous.daemonId || previous.manifest.sourceCommit !== manifest.sourceCommit))
  throw new Error("Upgrade requires a successful owned guest proof at the same upstream pin. Preserve failed/foreign records for inspection.");
if (!manifest.distroEmbedded)
  throw new Error("Build the verified guest bundle first.");
const decode = (bytes) =>
  bytes.includes(0)
    ? bytes.toString("utf16le").replace(/^\uFEFF/, "")
    : bytes.toString("utf8");
const wsl = () =>
  decode(
    execFileSync("wsl.exe", ["--list", "--quiet"], {
      windowsHide: true,
      timeout: 15000,
    }),
  )
    .split(/\r?\n/)
    .map((name) => name.trim())
    .filter(Boolean);
const beforeDistros = wsl();
if (
  !resume &&
  beforeDistros.some((name) => name.toLowerCase() === "lima-orbwin")
)
  throw new Error(
    "lima-orbwin already exists. Preserve it and inspect its recorded owner before any reuse.",
  );
if (resume && !beforeDistros.includes("lima-orbwin"))
  throw new Error("Recorded guest is no longer registered.");
// The candidate bridge range must not overlap a specific host route. Treat
// broad corporate routes conservatively; runtime VPN coverage is still pending.
const routes = execFileSync(
  "powershell.exe",
  [
    "-NoProfile",
    "-NonInteractive",
    "-Command",
    "Get-NetRoute -AddressFamily IPv4 | Select-Object -ExpandProperty DestinationPrefix",
  ],
  { encoding: "utf8", windowsHide: true, timeout: 15000 },
)
  .trim()
  .split(/\r?\n/)
  .filter(Boolean);
const conflicts = conflictingRoutes(routes);
if (conflicts.length)
  throw new Error(
    `Candidate network conflicts with host routes: ${conflicts.join(", ")}`,
  );
const realHome = process.env.USERPROFILE;
const protectedFiles = [".docker/config.json", ".kube/config"].map((path) =>
  resolve(realHome, path),
);
const digest = (path) =>
  existsSync(path)
    ? createHash("sha256").update(readFileSync(path)).digest("hex")
    : null;
const before = protectedFiles.map(digest);
mkdirSync(home, { recursive: true });
if (resume)
  copyFileSync(
    resolve(home, "evidence.json"),
    resolve(home, `evidence-before-${runId}.json`),
  );
if (upgrade && existsSync(resolve(root, "artifacts/qualification/runtime-identity.json")))
  copyFileSync(resolve(root, "artifacts/qualification/runtime-identity.json"), resolve(home, `runtime-identity-before-upgrade-${runId}.json`));
const env = {
  ...process.env,
  USERPROFILE: home,
  HOME: home,
  RDD_INSTANCE: "orbwin",
  RDD_DEVELOPER_MODE: "1",
  RDD_KEEP_LOGS: "1",
  DOCKER_HOST: endpoint,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  BUILDX_BUILDER: "default",
  BUILDX_CONFIG: resolve(home, ".rdorbwin/docker/buildx"),
  BUILDKIT_HOST: "",
  DOCKER_CONFIG: resolve(home, ".rdorbwin/docker"),
  KUBECONFIG: resolve(home, ".rdorbwin/kube-client.config"),
};
const evidence = {
  runId,
  home,
  manifest,
  endpoint,
  steps: [],
  passed: false,
  coexistenceQualified: false,
  upgradeFrom: upgrade ? previous.manifest : undefined,
};
function run(binary, args, seconds = 30, input) {
  const start = performance.now();
  try {
    const output = execFileSync(binary, args, {
      env,
      windowsHide: true,
      encoding: "utf8",
      input,
      timeout: seconds * 1000,
      maxBuffer: 2 * 1024 * 1024,
      stdio: ["pipe", "pipe", "pipe"],
    });
    evidence.steps.push({
      args,
      milliseconds: performance.now() - start,
      output,
    });
    return output;
  } catch (error) {
    evidence.steps.push({
      args,
      milliseconds: performance.now() - start,
      error: error.message,
      stderr: String(error.stderr ?? ""),
    });
    throw error;
  } finally {
    writeFileSync(
      resolve(home, "evidence.json"),
      JSON.stringify(evidence, null, 2),
    );
  }
}
const rdd = (args, seconds, input) => run(executable, args, seconds, input);
const docker = (args) =>
  run(
    "docker.exe",
    ["--config", env.DOCKER_CONFIG, "--host", endpoint, ...args],
    120,
  );
function ownsRegistration() {
  const text = execFileSync(
    "powershell.exe",
    [
      "-NoProfile",
      "-NonInteractive",
      "-Command",
      "Get-ChildItem HKCU:/Software/Microsoft/Windows/CurrentVersion/Lxss | ForEach-Object { Get-ItemProperty -LiteralPath $_.PSPath } | Where-Object DistributionName -eq 'lima-orbwin' | Select-Object -ExpandProperty BasePath",
    ],
    { encoding: "utf8", windowsHide: true, timeout: 15000 },
  ).trim();
  if (!text) return true; // Startup may have failed before importing a guest.
  const base = text.replace(/^\\\\\?\\/, "");
  const child = relative(home, base);
  return child !== "" && !child.startsWith("..") && !isAbsolute(child);
}
let started = false;
let failure;
try {
  if (!ownsRegistration())
    throw new Error("Guest ownership differs from the selected proof home.");
  if (!resume) rdd(["service", "create", "--controllers=*,-path-management"]);
  started = true;
  rdd(
    ["service", "start", "--controllers=*,-path-management", "--timeout=60s"],
    70,
  );
  const app = {
    apiVersion: "app.rancherdesktop.io/v1alpha1",
    kind: "App",
    metadata: { name: "app" },
    spec: {
      running: true,
      namespace: "rancher-desktop",
      containerEngine: { name: "moby" },
      kubernetes: { enabled: false },
      virtualMachine: { cpus: 2, memory: "2Gi" },
      application: { addPath: "manual" },
    },
  };
  if (!resume) rdd(["ctl", "create", "-f", "-"], 30, JSON.stringify(app));
  rdd(
    [
      "ctl",
      "wait",
      "apps/app",
      "--for=condition=ContainerEngineReady=True",
      "--timeout=180s",
    ],
    190,
  );
  // Engine conditions can survive a service restart. Wait for this boot's Lima
  // completion, after provisioning has stopped/reconfigured the guest target.
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
  if (!ownsRegistration())
    throw new Error(
      "Guest registration ownership differs from this proof's fresh home.",
    );
  const info = await waitForEngine(() =>
    JSON.parse(docker(["info", "--format", "{{json .}}"])),
  );
  if (info.OSType !== "linux" || !info.ID)
    throw new Error("Owned endpoint did not identify a Linux daemon.");
  evidence.daemonId = info.ID;
  if (previous?.daemonId && previous.daemonId !== info.ID)
    throw new Error(
      "Owned daemon identity changed across restart; refusing workloads.",
    );
  const qualification = resolve(root, "artifacts/qualification");
  mkdirSync(qualification, { recursive: true });
  writeFileSync(
    resolve(qualification, "runtime-identity.json"),
    JSON.stringify(
      {
        endpoint,
        home,
        sourceCommit: manifest.sourceCommit,
        binarySha256: manifest.binarySha256,
        daemonId: info.ID,
        identityVerified: true,
        coexistenceQualified: false,
      },
      null,
      2,
    ),
  );
  docker(["version"]);
  if (
    docker([
      "run",
      "--rm",
      "--name",
      `orbwin-smoke-${runId.slice(0, 8)}`,
      "--label",
      `dev.orbwin.qualification=${runId}`,
      "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30",
      "uname",
      "-s",
    ]).trim() !== "Linux"
  )
    throw new Error("Linux container smoke output differed.");
  if (process.argv.includes("--suite"))
    await runBaseline({ root, runId, docker, evidence });
  if (process.argv.includes("--mounts")) {
    const bind = resolve(root, "tests/workloads/bind source");
    const linux = `/mnt/${bind[0].toLowerCase()}${bind.slice(2).replaceAll("\\", "/")}`;
    evidence.mountCases = [];
    for (const [kind, path] of [
      ["Windows drive path with spaces", bind],
      ["Explicit WSL drive path with spaces", linux],
    ]) {
      try {
        const output = docker([
          "run",
          "--rm",
          "--mount",
          `type=bind,src=${path},dst=/source,readonly`,
          "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30",
          "cat",
          "/source/hello.txt",
        ]).trim();
        if (output !== "orbwin-windows-bind-proof")
          throw new Error("Bind mount contents differed");
        evidence.mountCases.push({ kind, passed: true });
      } catch (error) {
        evidence.mountCases.push({ kind, passed: false, error: error.message });
      }
    }
    if (evidence.mountCases.some((item) => !item.passed))
      throw new Error(
        "One or more Windows bind-mount forms failed; see preserved mountCases evidence.",
      );
  }
  evidence.passed = true;
} catch (error) {
  failure = error;
} finally {
  if (
    started &&
    !(process.argv.includes("--keep-running") && !failure && evidence.passed)
  ) {
    try {
      if (!ownsRegistration())
        throw new Error(
          "Registration ownership changed; automatic stop refused. Inspect the proof home and owned service.",
        );
      rdd(["service", "stop", "--timeout=60s"], 70);
    } catch (error) {
      failure ??= error;
      evidence.passed = false;
    }
  }
  evidence.serviceLeftRunning =
    started &&
    process.argv.includes("--keep-running") &&
    !failure &&
    evidence.passed;
  evidence.globalClientConfigurationUnchanged = protectedFiles.every(
    (file, index) => digest(file) === before[index],
  );
  evidence.existingRegistrationsPreserved = beforeDistros.every((name) =>
    wsl().includes(name),
  );
  if (
    !evidence.globalClientConfigurationUnchanged ||
    !evidence.existingRegistrationsPreserved
  ) {
    failure ??= new Error("Host preservation check failed");
    evidence.passed = false;
  }
  writeFileSync(
    resolve(home, "evidence.json"),
    JSON.stringify(evidence, null, 2),
  );
}
if (failure) throw failure;
console.log(
  `Isolated guest proof passed; owned service ${evidence.serviceLeftRunning ? "left running for development" : "stopped"} and data retained: ${home}`,
);
