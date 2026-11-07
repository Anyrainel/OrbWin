// Root-owned serial engine lane only. All mutations happen through the ignored
// Rust production-adapter test; this wrapper performs read-only baseline checks.
import { spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { endpoint, verifyIdentity } from "./engine-identity.mjs";
import { verifyBuild } from "./build-identity.mjs";

if (process.env.ORBWIN_QUALIFY_OWNED !== "1")
  throw new Error("Explicit owned-engine opt-in required");
const root = fileURLToPath(new URL("../", import.meta.url));
const image = process.env.ORBWIN_QUALIFY_FIXTURE_IMAGE;
if (!/^sha256:[a-f0-9]{64}$/.test(image ?? ""))
  throw new Error(
    "Select an already cached immutable Alpine amd64 image with sh, awk, cat, sleep and no image-declared volumes",
  );
const subnet = process.env.ORBWIN_QUALIFY_SUBNET;
if (
  !/^172\.(?:1[6-9]|2\d|3[01])\.(?:\d|[1-9]\d|1\d\d|2[0-4]\d|25[0-5])\.0\/24$/.test(
    subnet ?? "",
  )
)
  throw new Error(
    "Supply an explicit reviewed unused private 172.16–31.x.0/24 subnet; no automatic retries",
  );
const nonce = randomUUID().replaceAll("-", "");
const folder = resolve(
  root,
  "artifacts/qualification",
  `orbwin-inventory-${nonce}`,
);
const config = resolve(folder, "docker");
mkdirSync(config, { recursive: true });
writeFileSync(resolve(config, "config.json"), "{}\n", { flag: "wx" });
const dockerPath = resolve(root, "artifacts/runtime-tools/bin/docker.exe");
const linuxDocker = resolve(root, "artifacts/runtime-tools/linux/docker");
const build = verifyBuild(root);
const identity = JSON.parse(
  readFileSync(resolve(root, "artifacts/qualification/engine-identity.json")),
);
const wsl = "{231810b0-fa21-48a8-b0a7-86f9a1360029}";
const translate = (path) => {
  const match = path.match(/^([A-Za-z]):[\\/](.*)$/);
  if (!match) throw new Error("Qualification requires a local drive checkout");
  return `/mnt/${match[1].toLowerCase()}/${match[2].replaceAll("\\", "/")}`;
};
const env = {
  ...process.env,
  ORBWIN_INVENTORY_NONCE: nonce,
  DOCKER_HOST: "",
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  DOCKER_CONFIG: config,
  BUILDKIT_HOST: "",
  BUILDX_BUILDER: "default",
};
const proof = {
  schema: 1,
  fixture: `orbwin-inventory-${nonce}`,
  started: new Date().toISOString(),
  endpoint,
  image,
  subnet,
  commands: [],
  success: false,
  errors: [],
  before: null,
  after: null,
};
const save = () =>
  writeFileSync(resolve(folder, "proof.json"), JSON.stringify(proof, null, 2));
function command(executable, args, timeout = 30_000) {
  const result = spawnSync(executable, args, {
    cwd: root,
    env,
    encoding: "utf8",
    windowsHide: true,
    timeout,
    maxBuffer: 8 << 20,
  });
  proof.commands.push({
    executable,
    args,
    status: result.status,
    error: result.error?.message,
    stdout: result.stdout,
    stderr: result.stderr,
  });
  save();
  if (result.error || result.status !== 0)
    throw new Error(`Qualification client failed: ${args[0]}`);
  return result.stdout.trim();
}
const docker = (host, args) =>
  command(dockerPath, ["--config", config, "--host", host, ...args]);
const rancher = (args) =>
  command("wsl.exe", [
    "--distribution-id",
    wsl,
    "--user",
    "root",
    "--exec",
    translate(linuxDocker),
    "--config",
    translate(config),
    "--host",
    "unix:///mnt/wsl/rancher-desktop/run/docker.sock",
    ...args,
  ]);
const hash = (file) =>
  existsSync(file)
    ? createHash("sha256").update(readFileSync(file)).digest("hex")
    : null;
const lines = (text) => text.split(/\r?\n/).filter(Boolean).sort();
function snapshot() {
  const ownedInfo = JSON.parse(
    docker(endpoint, [
      "info",
      "--format",
      '{"ID":{{json .ID}},"OSType":{{json .OSType}}}',
    ]),
  );
  verifyIdentity(identity, ownedInfo, build);
  const ids = {
    owned: ownedInfo.ID,
    desktop: docker("npipe:////./pipe/dockerDesktopLinuxEngine", [
      "info",
      "--format",
      "{{.ID}}",
    ]),
    rancher: rancher(["info", "--format", "{{.ID}}"]),
  };
  if (
    ids.owned !== "d9c00194-ea9d-439a-8561-6a0ec580fbd5" ||
    ids.desktop !== "a307ff0e-581d-49d3-8aee-a0636dda9137" ||
    ids.rancher !== "741eb2cb-adf5-4676-bf28-f03f68d54f60"
  )
    throw new Error("Three-engine identity drift");
  const inventories = {};
  for (const [name, read] of [
    ["owned", (args) => docker(endpoint, args)],
    [
      "desktop",
      (args) => docker("npipe:////./pipe/dockerDesktopLinuxEngine", args),
    ],
    ["rancher", rancher],
  ]) {
    inventories[name] = {
      containers: lines(
        read(["ps", "--all", "--no-trunc", "--format", "{{.ID}}"]),
      ),
      volumes: lines(read(["volume", "ls", "--format", "{{.Name}}"])),
      images: [
        ...new Set(lines(read(["image", "ls", "--no-trunc", "--quiet"]))),
      ],
      networks: lines(
        read(["network", "ls", "--no-trunc", "--format", "{{.ID}}"]),
      ),
    };
    const ids = inventories[name].containers;
    if (ids.length > 512 || ids.some((id) => !/^[a-f0-9]{64}$/.test(id)))
      throw new Error(
        "Container-state baseline exceeds its bounded full-ID profile",
      );
    inventories[name].containerStates = ids.length
      ? lines(
          read([
            "container",
            "inspect",
            ...ids,
            "--format",
            "{{.Id}}|{{.State.Running}}|{{.State.Paused}}|{{.RestartCount}}",
          ]),
        )
      : [];
  }
  const home = resolve(process.env.LOCALAPPDATA, "OrbWinData/home");
  const hashes = {
    docker: hash(resolve(process.env.USERPROFILE, ".docker/config.json")),
    kube: hash(resolve(process.env.USERPROFILE, ".kube/config")),
    privateDocker: hash(resolve(home, ".rdorbwin/wsl-docker/config.json")),
    privateWindowsDocker: hash(resolve(home, ".rdorbwin/docker/config.json")),
    project: hash(
      resolve(
        home,
        ".rdorbwin/wsl-projects/710eb4f4d7f72b0e329a51d30a83377bc7ef6c0cca1a444bd8d2e70e72326e8f.json",
      ),
    ),
    globalWsl: hash(resolve(process.env.USERPROFILE, ".wslconfig")),
  };
  const elevated = command("powershell.exe", [
    "-NoProfile",
    "-NonInteractive",
    "-Command",
    "([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)",
  ]);
  if (elevated !== "False")
    throw new Error("Qualification must run without Windows elevation");
  return { ids, inventories, hashes, windowsElevated: false };
}
try {
  proof.before = snapshot();
  save();
  const inspected = JSON.parse(docker(endpoint, ["image", "inspect", image]));
  if (
    inspected.length !== 1 ||
    inspected[0].Id !== image ||
    inspected[0].Os !== "linux" ||
    inspected[0].Architecture !== "amd64" ||
    Object.keys(inspected[0].Config?.Volumes ?? {}).length
  )
    throw new Error("Cached fixture identity/platform/volume contract differs");
  // Refuse an overlap before dispatch. Docker remains authoritative for races;
  // the Rust test also proves a conflicting second create refuses.
  const ip = (address) =>
    address.split(".").reduce((n, part) => n * 256 + Number(part), 0) >>> 0;
  const [candidateAddress, candidateBits] = subnet.split("/");
  const candidate = ip(candidateAddress);
  for (const id of proof.before.inventories.owned.networks) {
    const network = JSON.parse(
      docker(endpoint, [
        "network",
        "inspect",
        id,
        "--format",
        "{{json .IPAM.Config}}",
      ]),
    );
    for (const entry of network ?? []) {
      if (!/^\d+\.\d+\.\d+\.\d+\/\d+$/.test(entry.Subnet ?? "")) continue;
      const [address, bits] = entry.Subnet.split("/");
      const prefix = Math.min(Number(candidateBits), Number(bits));
      const mask = prefix === 0 ? 0 : (0xffffffff << (32 - prefix)) >>> 0;
      if ((candidate & mask) >>> 0 === (ip(address) & mask) >>> 0)
        throw new Error(
          "Reviewed subnet overlaps an existing owned network; choose another before execution",
        );
    }
  }
  save();
  command(
    "cargo",
    [
      "test",
      "--manifest-path",
      "src-tauri/Cargo.toml",
      "owned_inventory_network_limits",
      "--",
      "--ignored",
      "--test-threads=1",
      "--nocapture",
    ],
    300_000,
  );
  const nativePath = resolve(folder, "native-records.json");
  if (!existsSync(nativePath))
    throw new Error(
      "Native test produced no evidence (registration may be missing)",
    );
  const native = JSON.parse(readFileSync(nativePath));
  if (native.at(-1)?.success !== true)
    throw new Error("Native fixture proof/cleanup did not succeed");
} catch (error) {
  proof.errors.push(String(error));
} finally {
  try {
    proof.after = snapshot();
  } catch (error) {
    proof.errors.push(`Postcheck: ${String(error)}`);
  }
  if (
    proof.before &&
    proof.after &&
    JSON.stringify(proof.before) !== JSON.stringify(proof.after)
  )
    proof.errors.push(
      "Three-engine inventory/config preservation mismatch; inspect retained fixture evidence. No broad cleanup is attempted.",
    );
  proof.success =
    proof.errors.length === 0 && proof.before !== null && proof.after !== null;
  proof.finished = new Date().toISOString();
  save();
}
console.log(
  JSON.stringify({
    proof: resolve(folder, "proof.json"),
    success: proof.success,
    errors: proof.errors,
  }),
);
if (!proof.success) process.exitCode = 1;
