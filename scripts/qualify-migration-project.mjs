// Opt-in nonempty Desktop/Rancher -> owned OrbWin project qualification. Only nonce
// fixtures are started/stopped/removed. Production SDK transactions own image
// and volume transfer; this harness does not change defaults or engine lifecycle.
// Current Rancher/native extension is SOURCE ONLY until reviewed runtime/nonce
// qualification. --plan performs no fixture/WSL/SDK/native work.
import { spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import {
  mkdirSync,
  readFileSync,
  writeFileSync,
  statSync,
  openSync,
  writeSync,
  ftruncateSync,
  fsyncSync,
  closeSync,
} from "node:fs";
import { resolve, isAbsolute } from "node:path";
import { fileURLToPath } from "node:url";
import { isDeepStrictEqual } from "node:util";
import { endpoint, verifyIdentity } from "./engine-identity.mjs";
import { verifyBuild } from "./build-identity.mjs";
import { parseMigrationImages } from "../src/model/migration-images.ts";
import { migrationProjectReview } from "../src/model/migration-projects.ts";
import {
  reviewMigrationMounts,
  migrationMountDraft,
} from "../src/model/migration-mounts.ts";

import { stableBytes } from "./qualify-migration-draft.mjs";
import {
  sourceProfile,
  dockerCommand,
  machineCommand,
  guardMachine,
  guardDaemon,
  guardReview,
  guardBase,
  sourceCopyPath,
  sdkSourceArgs,
  preservationSnapshot,
  projectConfigPaths,
  nativeApplicationEnvelope,
  guardNativeApplicationResponse,
  guardFixtureNamespace,
} from "./migration-project-source.mjs";
const selectedNonce = process.env.ORBWIN_QUALIFY_PROJECT_NONCE;
if (selectedNonce !== undefined && !/^[a-f0-9]{32}$/.test(selectedNonce))
  throw Error("Explicit fixture nonce must be32lowercasehex");
if (process.argv.includes("--plan")) {
  console.log(
    JSON.stringify(
      {
        scope: "source-only plan; no fixture/WSL/SDK/native execution",
        sourceKinds: ["docker-desktop", "rancher-desktop"],
        rancher: sourceProfile("rancher-desktop"),
        windowsProjectShape:
          "artifacts/qualification/orbwin-project-<32hex>/base.json + restored.compose.json",
        preservation:
          "all three canonical engines including full network inspect + six global/private/project config fingerprints",
        nativeAdapter:
          "optional explicitly pinned frozen test exe/hash; Validate/Up/Stop only",
        runtimeGate:
          "verifyBuild and native pin checks must pass; never bypass source/bundle drift",
      },
      null,
      2,
    ),
  );
  process.exit(0);
}
const profile = sourceProfile(process.env.ORBWIN_QUALIFY_MIGRATION_SOURCE);

if (
  process.env.ORBWIN_QUALIFY_OWNED !== "1" ||
  !["docker-desktop", "rancher-desktop"].includes(
    process.env.ORBWIN_QUALIFY_MIGRATION_SOURCE,
  )
)
  throw Error(
    "Explicit owned-engine and selected-source fixture opt-ins required",
  );
const root = fileURLToPath(new URL("../", import.meta.url));
const token = selectedNonce ?? randomUUID().replaceAll("-", ""),
  project = `orbwin-project-${token}`;
const folder = resolve(root, "artifacts/qualification", project);
mkdirSync(folder);
const config = resolve(folder, "docker");
const receiptDirectory = process.env.ORBWIN_QUALIFY_RECEIPT_TRANSIENT;
if (
  receiptDirectory &&
  (!isAbsolute(receiptDirectory) || !statSync(receiptDirectory).isDirectory())
)
  throw Error(
    "Explicit receipt qualification directory must already exist and be absolute",
  );
// Optional real app receipt storage supports native restart/history validation.
// These owning journals are intentionally retained after fixture cleanup.
const transient = receiptDirectory || resolve(folder, "transient");
mkdirSync(config);
if (!receiptDirectory) mkdirSync(transient);
const docker = resolve(root, "artifacts/runtime-tools/bin/docker.exe"),
  sdk = resolve(root, "artifacts/backend/orbwin-session.exe");
const source = profile.engine;
const assets = {
  docker,
  linuxDocker: resolve(root, "artifacts/runtime-tools/linux/docker"),
  wsl: resolve(process.env.SystemRoot, "System32/wsl.exe"),
  config,
};
const identity = JSON.parse(
  readFileSync(resolve(root, "artifacts/qualification/engine-identity.json")),
);
const sourceID = identity.qualification.engines.find(
  (v) => v.name === profile.engine,
)?.daemonId;
const targetID = identity.daemonId,
  baseImage = profile.baseImage;
if (
  sourceID !== profile.daemon ||
  targetID !== "d9c00194-ea9d-439a-8561-6a0ec580fbd5"
)
  throw Error("Frozen engine selection differs");
const sha = (bytes) => createHash("sha256").update(bytes).digest("hex");
const runtimeTools = JSON.parse(
  readFileSync(resolve(root, "runtime-tools.lock.json")),
);
const linuxDockerSHA = sha(readFileSync(assets.linuxDocker));
if (
  runtimeTools.tools.find((t) => t.path === "linux/docker")?.sha256 !==
  linuxDockerSHA
)
  throw Error("Pinned Linux Docker differs");
if (
  runtimeTools.tools.find((t) => t.path === "bin/docker.exe")?.sha256 !==
  sha(readFileSync(docker))
)
  throw Error("Pinned Windows Docker differs");
const pluginDirectory = resolve(root, "artifacts/runtime-tools/docker-cli-plugins");
const composePlugin = resolve(pluginDirectory, "docker-compose.exe");
const composeSHA = runtimeTools.tools.find(
  (tool) => tool.path === "docker-cli-plugins/docker-compose.exe",
)?.sha256;
function verifyComposePlugin() {
  if (!/^[a-f0-9]{64}$/.test(composeSHA ?? "") ||
      sha(stableBytes(composePlugin, 256 << 20)) !== composeSHA)
    throw Error("Pinned Windows Compose plugin differs");
}
verifyComposePlugin();
// The fixture's anonymous config must select the bundled plugin explicitly.
// A fixed Docker executable alone does not attest its discovered Compose plugin.
writeFileSync(resolve(config, "config.json"), JSON.stringify({
  cliPluginsExtraDirs: [pluginDirectory],
}), { flag: "wx" });
const native = process.env.ORBWIN_QUALIFY_APPLICATION_NATIVE;
const nativeSHA = process.env.ORBWIN_QUALIFY_APPLICATION_NATIVE_SHA256;
if (native || nativeSHA) {
  if (
    !native ||
    !isAbsolute(native) ||
    !/^[a-f0-9]{64}$/.test(nativeSHA ?? "") ||
    sha(stableBytes(native, 256 << 20)) !== nativeSHA
  )
    throw Error("Explicit frozen native adapter/hash required");
}

const sourceVolume = project + "-source",
  destination = project + "-destination",
  alias = project + ":app";
const fs = JSON.parse(
  readFileSync(resolve(root, "artifacts/backend/fs-manifest.json")),
);
const maintenance = JSON.parse(
  readFileSync(resolve(root, "maintenance-image.lock.json")),
);
const env = {
  ...process.env,
  DOCKER_HOST: "",
  DOCKER_CONTEXT: "",
  DOCKER_API_VERSION: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  DOCKER_CONFIG: config,
  DOCKER_AUTH_CONFIG: "",
  DOCKER_CUSTOM_HEADERS: "",
  BUILDKIT_HOST: "",
  BUILDX_BUILDER: "",
  BUILDX_CONFIG: "",
  COMPOSE_FILE: "",
  COMPOSE_PROJECT_NAME: "",
  COMPOSE_PROFILES: "",
  COMPOSE_ENV_FILES: "",
  ORBWIN_MIGRATION_DOCKER: assets.linuxDocker,
  ORBWIN_MIGRATION_DOCKER_SHA256: linuxDockerSHA,
  ORBWIN_TRANSIENT_DIR: transient,
  ORBWIN_FS_HELPER: resolve(root, "artifacts/backend/orbwin-fs"),
  ORBWIN_FS_HELPER_SHA256: fs.binarySha256,
  ORBWIN_MAINTENANCE_IMAGE: resolve(
    root,
    "artifacts/backend/orbwin-maintenance.tar",
  ),
  ORBWIN_MAINTENANCE_IMAGE_SHA256: maintenance.sha256,
};
const proof = {
  schema: 2,
  sourceKind: profile.kind,
  sourceWSL:
    profile.engine === "rancher"
      ? { registration: profile.registration, machine: profile.machine }
      : undefined,
  linuxDockerSHA,
  composePluginSHA256: composeSHA,
  applicationAdapter: native ? "rust-native" : "docker-compose-cli",
  nativeSHA,
  project,
  token,
  source: sourceID,
  target: targetID,
  sdkSha256: createHash("sha256").update(readFileSync(sdk)).digest("hex"),
  sourceFiles: Object.fromEntries(
    [
      "scripts/qualify-migration-project.mjs",
      "scripts/migration-project-source.mjs",
      "scripts/qualify-migration-draft.mjs",
    ].map((name) => [name, sha(readFileSync(resolve(root, name)))]),
  ),
  started: new Date().toISOString(),
  success: false,
  steps: [],
  cleanupErrors: [],
  receiptDirectory: transient,
};
const ownedContainers = [],
  targetContainers = new Map(),
  ownedReferences = [];
let sourceImage,
  sourceVolumeCreated = false,
  sourceVolumeMetadata,
  destinationMetadata,
  restoreRecord;
const proofFD = openSync(resolve(folder, "proof.json"), "wx");
const save = () => {
  const bytes = Buffer.from(JSON.stringify(proof, null, 2));
  ftruncateSync(proofFD, 0);
  let offset = 0;
  while (offset < bytes.length) {
    const n = writeSync(proofFD, bytes, offset, bytes.length - offset, offset);
    if (!n) throw Error("Short proof write");
    offset += n;
  }
  fsyncSync(proofFD);
};
const deadline = Date.now() + 15 * 60 * 1000;
let cleaning = false,
  cleanupEnds = 0,
  rechecking = false;
const mutationIntent = (engine, args) => {
  proof.intents ??= [];
  proof.intents.push({ engine, args, time: new Date().toISOString() });
  save();
};
const dockerMutation = (args) =>
  ["create", "start", "stop", "rm", "commit", "cp"].includes(args[0]) ||
  (["image", "volume", "network"].includes(args[0]) &&
    ["create", "rm", "tag"].includes(args[1])) ||
  (args[0] === "compose" &&
    !["config", "version"].some((v) => args.includes(v)));
function run(binary, args, input, allowFailure = false, extraEnv = {}) {
  const remaining = (cleaning ? cleanupEnds : deadline) - Date.now();
  if (remaining <= 0)
    throw Error("Bounded qualification/cleanup deadline reached");
  const result = spawnSync(binary, args, {
    cwd: root,
    env: { ...env, ...extraEnv },
    input,
    encoding: "utf8",
    windowsHide: true,
    timeout: Math.min(180000, remaining),
    maxBuffer: 20 << 20,
  });
  proof.steps.push({
    binary,
    args,
    status: result.status,
    error: result.error?.message,
    stdout: result.stdout,
    stderr: result.stderr,
  });
  save();
  if (result.error || result.signal) {
    proof.uncertainMutationOutcome = true;
    save();
    throw Error(
      "Client loss/timeout; unknown outcomes retained: " +
        String(result.error || result.signal),
    );
  }
  if (!allowFailure && result.status !== 0)
    throw Error(
      `Command failed: ${args.join(" ")}: ${result.stderr || result.error}`,
    );
  return result;
}
function engineFor(host) {
  if (host === endpoint) return "owned";
  if (["owned", "desktop", "rancher"].includes(host)) return host;
  throw Error("Foreign endpoint refused");
}
function d(host, args, allowFailure = false) {
  const engine = engineFor(host);
  if (dockerMutation(args)) {
    checkIdentities();
    mutationIntent(engine, args);
  }
  const command = dockerCommand(engine, assets, args);
  return run(command.binary, command.args, undefined, allowFailure);
}
function checkIdentities() {
  if (rechecking) return;
  rechecking = true;
  try {
    const command = machineCommand(assets);
    guardMachine(run(command.binary, command.args).stdout);
    for (const engine of ["owned", "desktop", "rancher"])
      guardDaemon(
        engine,
        JSON.parse(d(engine, ["info", "--format={{json .}}"]).stdout),
      );
  } finally {
    rechecking = false;
  }
}
const json = (host, args) => JSON.parse(d(host, args).stdout);
const sdkReads = new Set([
  "migration-review",
  "migration-rancher-review",
  "migration-volume-review",
  "migration-rancher-volume-review",
  "image-import-review",
  "migration-image-history",
  "migration-volume-history",
  "migration-destination-review",
]);
const sdkRun = (args, input) => {
  if (!sdkReads.has(args[0])) {
    checkIdentities();
    mutationIntent("production-sdk", args);
  }
  return JSON.parse(run(sdk, args, input).stdout);
};
const sourceArgs = (operation, extra = {}) =>
  sdkSourceArgs(profile, operation, {
    target: targetID,
    source: sourceID,
    ...extra,
  });
const configs = projectConfigPaths(
  process.env.USERPROFILE,
  process.env.LOCALAPPDATA,
);
const snapshot = () => {
  checkIdentities();
  return preservationSnapshot(
    (engine, args) => d(engine, args).stdout,
    configs,
  );
};
const volume = (host, name) => json(host, ["volume", "inspect", name])[0];
const image = (host, reference) =>
  json(host, ["image", "inspect", reference])[0];
const container = (host, id) => json(host, ["inspect", id])[0];
function fixtureContainer(host, name, extra, imageID, command) {
  const id = d(host, [
    "create",
    "--pull=never",
    "--network=none",
    "--read-only",
    "--cap-drop=ALL",
    "--security-opt=no-new-privileges:true",
    "--memory=128m",
    "--pids-limit=32",
    "--cpus=0.5",
    "--label",
    `dev.orbwin.qualification=${token}`,
    "--name",
    name,
    ...extra,
    imageID,
    ...command,
  ]).stdout.trim();
  if (!/^[a-f0-9]{64}$/.test(id))
    throw Error("Fixture create did not acknowledge an immutable ID");
  ownedContainers.push({ host, id, name, imageID });
  save();
  return id;
}
function checkFixture(host, id, name) {
  const v = container(host, id);
  const known = ownedContainers.find(
    (item) => item.id === id && item.host === host,
  );
  if (
    (known && v.Image !== known.imageID) ||
    v.Id !== id ||
    v.Name !== "/" + name ||
    v.Config.Labels?.["dev.orbwin.qualification"] !== token
  )
    throw Error("Fixture container identity/ownership changed");
  return v;
}
function removeFixture(host, id) {
  const item = ownedContainers.find((v) => v.id === id);
  if (!item) throw Error("Unknown fixture");
  const v = checkFixture(host, id, item.name);
  if (v.State.Running) throw Error("Fixture still running");
  d(host, ["rm", id]);
  ownedContainers.splice(ownedContainers.indexOf(item), 1);
}
function packageManifest(path) {
  const data = readFileSync(path),
    magic = Buffer.from("ORBWIN-VOLUME-1\n");
  if (data.length > 1 << 20 || !data.subarray(0, magic.length).equals(magic))
    throw Error("Fixture package bounds/magic differ");
  const length = Number(data.readBigUInt64LE(magic.length)),
    start = magic.length + 8 + length;
  if (!Number.isSafeInteger(length) || length > 65536 || start > data.length)
    throw Error("Fixture manifest bounds differ");
  const manifest = JSON.parse(data.subarray(magic.length + 8, start));
  if (
    manifest.bytes !== data.length - start ||
    sha(data.subarray(start)) !== manifest.sha256
  )
    throw Error("Fixture package payload differs");
  return manifest;
}
const tree = (manifest) =>
  Object.fromEntries(
    manifest.entries
      .map((e) => [e.path, e])
      .sort(([a], [b]) => a.localeCompare(b)),
  );
const composeBase = resolve(folder, "base.json"),
  mappingFile = resolve(folder, "restored.compose.json");
const cliCompose = (args, allowFailure = false) => {
  verifyComposePlugin();
  const result = d(
    endpoint,
    [
      "compose",
      "--project-name",
      project,
      "--project-directory",
      folder,
      "-f",
      composeBase,
      "-f",
      mappingFile,
      ...args,
    ],
    allowFailure,
  );
  verifyComposePlugin();
  return result;
};
let nativeSequence = 0;
function compose(args, allowFailure = false) {
  if (!native) return cliCompose(args, allowFailure);
  const action = args[0] === "config" ? "validate" : args[0];
  if (!["validate", "up", "stop"].includes(action))
    throw Error("Unsupported native fixture action");
  checkIdentities();
  if (sha(stableBytes(native, 256 << 20)) !== nativeSHA)
    throw Error("Frozen native adapter changed");
  const envelope = nativeApplicationEnvelope(
    token,
    action,
    proof.baseSha256,
    proof.mappingSha256,
  );
  const basename = `rust-${action}-${++nativeSequence}.json`,
    request = resolve(folder, basename);
  const bytes = Buffer.from(JSON.stringify(envelope));
  mutationIntent("native-application-request", {
    action,
    basename,
    sha256: sha(bytes),
  });
  const fd = openSync(request, "wx");
  try {
    writeFileSync(fd, bytes);
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
  try {
    run(
      native,
      [
        "--ignored",
        "--exact",
        "operations::qualification::migration_application_qualification::owned_migration_application_step",
        "--nocapture",
      ],
      undefined,
      false,
      {
        ORBWIN_QUALIFY_OWNED: "1",
        ORBWIN_QUALIFY_APPLICATION: "1",
        ORBWIN_QUALIFY_APPLICATION_NONCE: token,
        ORBWIN_QUALIFY_APPLICATION_REQUEST: basename,
        ORBWIN_QUALIFY_APPLICATION_REQUEST_SHA256: sha(bytes),
      },
    );
    const intent = JSON.parse(
      stableBytes(request.replace(/\.json$/, ".intent.json"), 1 << 20),
    );
    if (
      intent.nonce !== token ||
      intent.target !== targetID ||
      intent.phase !== "dispatch-intent" ||
      intent.requestSha256 !== sha(bytes) ||
      intent.action !== action[0].toUpperCase() + action.slice(1)
    )
      throw Error("Native dispatch intent differs");
    const result = JSON.parse(
      stableBytes(request.replace(/\.json$/, ".response.json"), 1 << 20),
    );
    guardNativeApplicationResponse(result, token);
    const nativeRecords = stableBytes(
      request.replace(/\.json$/, ".native-records.json"),
      4 << 20,
    );
    JSON.parse(nativeRecords);
    proof.nativeSteps ??= [];
    proof.nativeSteps.push({
      action,
      request: basename,
      requestSha256: sha(bytes),
      nativeRecordsSha256: sha(nativeRecords),
      responseSha256: sha(
        stableBytes(request.replace(/\.json$/, ".response.json"), 1 << 20),
      ),
      success: true,
    });
    save();
    return result;
  } catch (error) {
    proof.nativeUnknownOutcome = { action, request: basename };
    save();
    throw error;
  }
}
async function response(path, expected) {
  let error;
  for (let attempt = 0; attempt < 20; attempt++) {
    try {
      if (Date.now() >= deadline)
        throw Error("Scenario deadline before HTTP observation");
      const r = await fetch(proof.url + path, {
        signal: AbortSignal.timeout(
          Math.min(2000, Math.max(1, deadline - Date.now())),
        ),
      });
      if (!r.ok) throw Error(`HTTP ${r.status}`);
      const text = await r.text();
      if (text !== expected) throw Error("Served fixture bytes differ");
      return;
    } catch (e) {
      error = e;
      if (Date.now() >= deadline) throw error;
      await new Promise((r) =>
        setTimeout(r, Math.min(150, deadline - Date.now())),
      );
    }
  }
  throw error;
}
try {
  checkIdentities();
  verifyIdentity(
    identity,
    json(endpoint, ["info", "--format={{json .}}"]),
    verifyBuild(root),
  );
  if (
    !sourceID ||
    sourceID === targetID ||
    json(source, ["info", "--format={{json .}}"]).ID !== sourceID
  )
    throw Error("Source daemon differs");
  const base = image(source, baseImage);
  guardBase(profile, base);
  proof.baseline = snapshot();
  guardFixtureNamespace(proof.baseline, project, alias);
  proof.globalBefore = proof.baseline.configs;
  save();
  const script = resolve(folder, "application.go");
  const application = resolve(folder, "orbwin-server");
  writeFileSync(
    script,
    `package main
import("bytes";"log";"net/http";"os";"strconv";"strings")
func main(){
data,err:=os.ReadFile("/data/proof.txt");if err!=nil||!bytes.Equal(data,[]byte("source-${token}\\n")){log.Fatal("restored input differs")}
marker:=[]byte("target-${token}\\n");if data,err=os.ReadFile("/data/target-write.txt");os.IsNotExist(err){if err=os.WriteFile("/data/target-write.txt",marker,0640);err!=nil{log.Fatal(err)}}else if err!=nil||!bytes.Equal(data,marker){log.Fatal("target marker differs")}
count:=0;if data,err=os.ReadFile("/data/starts");err==nil{count,err=strconv.Atoi(strings.TrimSpace(string(data)));if err!=nil{log.Fatal(err)}}else if !os.IsNotExist(err){log.Fatal(err)}
if err=os.WriteFile("/data/starts",[]byte(strconv.Itoa(count+1)+"\\n"),0640);err!=nil{log.Fatal(err)}
log.Fatal(http.ListenAndServe(":8080",http.FileServer(http.Dir("/data"))))
}
`,
    { flag: "wx" },
  );
  const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
  const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
  const compiled = spawnSync(
    go,
    ["build", "-trimpath", "-o", application, script],
    {
      cwd: root,
      env: {
        ...env,
        GOOS: "linux",
        GOARCH: "amd64",
        CGO_ENABLED: "0",
        GOTOOLCHAIN: "local",
      },
      encoding: "utf8",
      windowsHide: true,
      timeout: 180000,
      maxBuffer: 2097152,
    },
  );
  if (compiled.status !== 0)
    throw Error(compiled.stderr || String(compiled.error));
  proof.applicationBinarySha256 = sha(readFileSync(application));
  save();
  const imageContainer = fixtureContainer(
    source,
    project + "-image",
    ["--read-only=false"],
    baseImage,
    ["sh", "-c", "chmod 0555 /orbwin-server"],
  );
  d(source, [
    "cp",
    sourceCopyPath(profile, application),
    imageContainer + ":/orbwin-server",
  ]);
  d(source, ["start", "--attach", imageContainer]);
  if (
    checkFixture(source, imageContainer, project + "-image").State.ExitCode !==
    0
  )
    throw Error("Application executable preparation failed");
  d(source, [
    "commit",
    "--change",
    'CMD ["/orbwin-server"]',
    "--change",
    `LABEL dev.orbwin.qualification=${token}`,
    imageContainer,
    alias,
  ]);
  sourceImage = image(source, alias);
  if (sourceImage.Config.Labels?.["dev.orbwin.qualification"] !== token)
    throw Error("Source image fixture ownership differs");
  proof.sourceImage = { id: sourceImage.Id, rootfs: sourceImage.RootFS };
  save();
  removeFixture(source, imageContainer);
  if (
    d(source, [
      "volume",
      "create",
      "--label",
      `dev.orbwin.qualification=${token}`,
      sourceVolume,
    ]).stdout.trim() !== sourceVolume
  )
    throw Error("Source volume differs");
  sourceVolumeCreated = true;
  sourceVolumeMetadata = volume(source, sourceVolume);
  if (
    sourceVolumeMetadata.Name !== sourceVolume ||
    sourceVolumeMetadata.Driver !== "local" ||
    sourceVolumeMetadata.Scope !== "local" ||
    Object.keys(sourceVolumeMetadata.Options ?? {}).length ||
    sourceVolumeMetadata.Labels?.["dev.orbwin.qualification"] !== token ||
    !sourceVolumeMetadata.CreatedAt
  )
    throw Error("New source volume metadata/ownership differs");
  proof.sourceVolumeMetadata = sourceVolumeMetadata;
  save();
  const seeder = fixtureContainer(
    source,
    project + "-seed",
    [
      "--cap-drop=ALL",
      "--mount",
      `type=volume,src=${sourceVolume},dst=/data,volume-nocopy`,
    ],
    baseImage,
    [
      "sh",
      "-c",
      `set -eu; printf 'source-${token}\\n' > /data/proof.txt; printf '\\000\\001\\377' > /data/binary.bin; chmod 0640 /data/proof.txt /data/binary.bin; ln /data/proof.txt /data/hard; ln -s proof.txt /data/link`,
    ],
  );
  d(source, ["start", "--attach", seeder]);
  if (checkFixture(source, seeder, project + "-seed").State.ExitCode !== 0)
    throw Error("Seeder failed");
  removeFixture(source, seeder);
  const baseConfig = {
    services: {
      web: {
        image: alias,
        pull_policy: "never",
        read_only: true,
        mem_limit: "128m",
        pids_limit: 32,
        cpus: 0.5,
        cap_drop: ["ALL"],
        security_opt: ["no-new-privileges:true"],
        labels: { "dev.orbwin.qualification": token },
        ports: ["127.0.0.1::8080"],
        healthcheck: {
          test: ["CMD", "test", "-f", "/data/target-write.txt"],
          interval: "1s",
          timeout: "1s",
          retries: 20,
        },
        volumes: [
          {
            type: "volume",
            source: "data",
            target: "/data",
            volume: { nocopy: true },
          },
        ],
      },
    },
    volumes: { data: { external: true, name: sourceVolume } },
    networks: { default: { labels: { "dev.orbwin.qualification": token } } },
  };
  writeFileSync(composeBase, JSON.stringify(baseConfig, null, 2), {
    flag: "wx",
  });
  proof.baseSha256 = sha(readFileSync(composeBase));
  const sourceMember = fixtureContainer(
    source,
    project + "-source",
    [
      "--label",
      `com.docker.compose.project=${project}`,
      "--label",
      "com.docker.compose.service=web",
      "--label",
      `com.docker.compose.project.working_dir=${folder}`,
      "--label",
      `com.docker.compose.project.config_files=${composeBase}`,
      "--mount",
      `type=volume,src=${sourceVolume},dst=/data,volume-nocopy`,
    ],
    sourceImage.Id,
    [],
  );
  const review = parseMigrationImages(
    JSON.stringify(sdkRun(sourceArgs("review"))),
  );
  guardReview(profile, review, targetID);
  const selected = migrationProjectReview(review, project);
  if (
    selected.members.length !== 1 ||
    selected.members[0].container !== sourceMember ||
    selected.inputs.file !== composeBase ||
    selected.images.length !== 1 ||
    selected.images[0].id !== sourceImage.Id ||
    !selected.images[0].available ||
    selected.unknownImages !== 0
  )
    throw Error("Reviewed source project identity/paths differ");
  proof.projectReview = selected;
  const imageArchive = resolve(folder, "application.tar");
  proof.imageExport = sdkRun(
    sourceArgs("image-export", {
      image: sourceImage.Id,
      reference: alias,
      destination: imageArchive,
    }),
  );
  const imageExportReceipt = readFileSync(proof.imageExport.receipt, "utf8")
    .trimEnd()
    .split("\n")
    .map(JSON.parse);
  if (
    imageExportReceipt.length !== 2 ||
    imageExportReceipt[0].transaction !== proof.imageExport.transaction ||
    imageExportReceipt[0].source.daemon !== sourceID ||
    imageExportReceipt[0].image !== sourceImage.Id ||
    imageExportReceipt[0].target !== targetID ||
    imageExportReceipt[1].phase !== "exported" ||
    imageExportReceipt[1].sha256 !== sha(readFileSync(imageArchive)) ||
    !isDeepStrictEqual(
      imageExportReceipt[1].configIds,
      proof.imageExport.images.map((x) => x.id),
    )
  )
    throw Error("Application source-image export receipt differs");
  guardReview(
    profile,
    {
      target: imageExportReceipt[0].target,
      source: imageExportReceipt[0].source,
    },
    targetID,
  );
  proof.sourceImageExportReceipt = imageExportReceipt;
  const preview = sdkRun(["image-import-review", targetID, imageArchive]);
  if (
    preview.images.length !== 1 ||
    preview.tags.length !== 1 ||
    preview.tags.some((t) => t.currentId !== "" || t.configIds.length !== 1)
  )
    throw Error("New fixture image conflicts or is ambiguous");
  proof.imageImport = sdkRun(
    ["import-images", targetID],
    JSON.stringify({
      source: imageArchive,
      sha256: preview.sha256,
      tags: preview.tags.map((t) => ({
        reference: t.reference,
        configId: t.configIds[0],
        expectedId: "",
        apply: true,
        replace: false,
      })),
    }),
  );
  for (const [ref, id] of Object.entries(proof.imageImport.tagged))
    ownedReferences.push({ ref, id });
  for (const stage of proof.imageImport.images)
    ownedReferences.push({
      ref: stage.reference,
      id: proof.imageImport.verified[stage.configId],
    });
  save();
  const targetImage = image(endpoint, alias);
  proof.targetImageId = targetImage.Id;
  proof.imageHistory = sdkRun(["migration-image-history", targetID]);
  const imageHistoryRow = proof.imageHistory.records.find(
    (r) => r.transaction === proof.imageExport.transaction,
  );
  if (
    !imageHistoryRow?.exportAcknowledged ||
    imageHistoryRow.sha256 !== proof.imageExport.sha256 ||
    !imageHistoryRow.imports?.some(
      (r) =>
        r.transaction === proof.imageImport.transaction &&
        r.phase === "complete" &&
        r.verifiedConfigs === preview.images.length,
    )
  )
    throw Error(
      "Combined application import receipt did not link to its source export",
    );
  if (
    !isDeepStrictEqual(targetImage.RootFS, sourceImage.RootFS) ||
    targetImage.Config.Labels?.["dev.orbwin.qualification"] !== token
  )
    throw Error("Imported application content/identity differs");
  proof.targetRootfsMatched = true;
  const volumeReview = sdkRun(
    sourceArgs("volume-review", { volume: sourceVolume }),
  );
  guardReview(profile, volumeReview, targetID);
  if (
    volumeReview.volume !== sourceVolume ||
    volumeReview.quiescentObserved !== true ||
    volumeReview.createdAt !== sourceVolumeMetadata.CreatedAt
  )
    throw Error("Reviewed source volume identity/quiescence differs");
  const volumePackage = resolve(folder, "source.orbvolume");
  proof.volumeExport = sdkRun(
    sourceArgs("volume-export", {
      volume: sourceVolume,
      fingerprint: volumeReview.fingerprint,
      destination: volumePackage,
    }),
  );
  restoreRecord = sdkRun([
    "volume-transfer",
    "restore",
    volumePackage,
    destination,
    targetID,
  ]);
  proof.volumeRestore = restoreRecord;
  destinationMetadata = volume(endpoint, destination);
  proof.destinationMetadata = destinationMetadata;
  save();
  proof.destinationReview = sdkRun([
    "migration-destination-review",
    targetID,
    sourceID,
    sourceVolume,
    destination,
  ]);
  const draft = migrationMountDraft(
    reviewMigrationMounts(selected.members),
    { [sourceVolume]: destination },
    token.slice(0, 16),
    [destination],
  );
  writeFileSync(mappingFile, draft, { flag: "wx" });
  proof.mappingSha256 = sha(readFileSync(mappingFile));
  compose(["config", "--quiet"]);
  proof.configurationValidated = true;
  if (
    d(endpoint, [
      "ps",
      "-aq",
      "--filter",
      `label=com.docker.compose.project=${project}`,
    ]).stdout.trim()
  )
    throw Error("Target project was not empty before startup");
  // Persist startup intent before Compose so even a failed/partial Up is audited.
  proof.startupIntent = true;
  save();
  compose([
    "up",
    "--detach",
    "--no-build",
    "--pull",
    "never",
    "--wait",
    "--wait-timeout",
    "30",
  ]);
  function discoverTarget() {
    const ids = d(endpoint, [
      "ps",
      "-aq",
      "--no-trunc",
      "--filter",
      `label=com.docker.compose.project=${project}`,
    ])
      .stdout.trim()
      .split("\n")
      .filter(Boolean);
    if (ids.length !== 1)
      throw Error("Target project has unexpected membership");
    const v = container(endpoint, ids[0]);
    if (
      v.Config.Labels?.["dev.orbwin.qualification"] !== token ||
      v.Config.Labels?.["com.docker.compose.service"] !== "web" ||
      v.Image !== targetImage.Id
    )
      throw Error("Target project identity/image differs");
    const mounted = v.Mounts.filter((m) => m.Destination === "/data");
    if (
      mounted.length !== 1 ||
      mounted[0].Name !== destination ||
      !mounted[0].RW
    )
      throw Error("Application did not mount the restored volume");
    targetContainers.set(v.Id, v.Name);
    proof.targetContainer = v.Id;
    const port = v.NetworkSettings.Ports["8080/tcp"];
    if (
      port?.length !== 1 ||
      port[0].HostIp !== "127.0.0.1" ||
      !/^\d+$/.test(port[0].HostPort)
    )
      throw Error("Application port was not loopback-only");
    proof.url = `http://127.0.0.1:${port[0].HostPort}`;
    save();
    return v;
  }
  discoverTarget();
  await response("/proof.txt", `source-${token}\n`);
  // Binary validation uses bytes, not text replacement decoding.
  const binary = Buffer.from(
    await (
      await fetch(proof.url + "/binary.bin", {
        signal: AbortSignal.timeout(3000),
      })
    ).arrayBuffer(),
  );
  if (!binary.equals(Buffer.from([0, 1, 255])))
    throw Error("Served binary data differs");
  await response("/target-write.txt", `target-${token}\n`);
  await response("/starts", "1\n");
  proof.applicationReadRestoredData = true;
  proof.destinationWriteServed = true;
  compose(["stop", "--timeout", "5"]);
  compose([
    "up",
    "--detach",
    "--no-build",
    "--pull",
    "never",
    "--wait",
    "--wait-timeout",
    "30",
  ]);
  discoverTarget();
  await response("/target-write.txt", `target-${token}\n`);
  await response("/starts", "2\n");
  proof.destinationRestartPreservedData = true;
  compose(["stop", "--timeout", "5"]);
  const sourceAfter = resolve(folder, "source-after.orbvolume");
  sdkRun(
    sourceArgs("volume-export", {
      volume: sourceVolume,
      fingerprint: volumeReview.fingerprint,
      destination: sourceAfter,
    }),
  );
  const before = packageManifest(volumePackage),
    after = packageManifest(sourceAfter);
  if (
    !isDeepStrictEqual(tree(before), tree(after)) ||
    before.sha256 !== after.sha256
  )
    throw Error("Source data/metadata changed after destination writes");
  if (
    after.entries.some((e) => ["target-write.txt", "starts"].includes(e.path))
  )
    throw Error("Destination writer reached source volume");
  if (
    image(source, alias).Id !== sourceImage.Id ||
    checkFixture(source, sourceMember, project + "-source").State.Status !==
      "created"
  )
    throw Error("Source image/workload changed");
  proof.sourceDataAndMetadataPreserved = true;
  proof.sourceWorkloadNeverStarted = true;
  proof.success = true;
} catch (error) {
  proof.error = error.stack;
  proof.success = false;
} finally {
  cleaning = true;
  cleanupEnds = Date.now() + 120000;
  let cleanupReady = false;
  try {
    checkIdentities();
    if (
      json(source, ["info", "--format={{json .}}"]).ID !== sourceID ||
      json(endpoint, ["info", "--format={{json .}}"]).ID !== targetID
    )
      throw Error("Engine identity changed; fixture cleanup refused");
    cleanupReady = true;
  } catch (error) {
    proof.cleanupErrors.push(error.message);
  }
  if (cleanupReady) {
    // A partial Up may have acknowledged containers before the client failed.
    if (proof.startupIntent)
      try {
        for (const id of d(endpoint, [
          "ps",
          "-aq",
          "--no-trunc",
          "--filter",
          `label=com.docker.compose.project=${project}`,
        ])
          .stdout.trim()
          .split("\n")
          .filter(Boolean)) {
          const v = container(endpoint, id);
          if (
            v.Config.Labels?.["dev.orbwin.qualification"] !== token ||
            !Object.values(proof.imageImport?.tagged ?? {}).includes(v.Image)
          )
            throw Error("Unowned target project member retained");
          targetContainers.set(id, v.Name);
        }
      } catch (e) {
        proof.cleanupErrors.push(e.message);
      }
    for (const [id, name] of targetContainers)
      try {
        const v = checkFixture(endpoint, id, name.slice(1));
        if (
          v.Image !== proof.targetImageId ||
          v.Config.Labels["com.docker.compose.project"] !== project
        )
          throw Error("Target adopted into another project");
        // Preserve fixture failure output before removing the acknowledged ID.
        d(endpoint, ["logs", "--tail", "100", id], true);
        if (v.State.Running) d(endpoint, ["stop", "--time", "5", id]);
        d(endpoint, ["rm", id]);
      } catch (e) {
        proof.cleanupErrors.push(e.message);
      }
    for (const item of [...ownedContainers].reverse())
      try {
        const v = checkFixture(item.host, item.id, item.name);
        if (
          item.name === project + "-source" &&
          (v.State.Status !== "created" ||
            !v.State.StartedAt.startsWith("0001-"))
        )
          throw Error("Source member was started or adopted; fixture retained");
        if (v.State.Running) d(item.host, ["stop", "--time", "5", item.id]);
        removeFixture(item.host, item.id);
      } catch (e) {
        proof.cleanupErrors.push(e.message);
      }
    try {
      for (const id of d(endpoint, [
        "network",
        "ls",
        "-q",
        "--no-trunc",
        "--filter",
        `label=com.docker.compose.project=${project}`,
      ])
        .stdout.trim()
        .split("\n")
        .filter(Boolean)) {
        const n = json(endpoint, ["network", "inspect", id])[0];
        if (
          !/^[a-f0-9]{64}$/.test(id) ||
          n.Driver !== "bridge" ||
          n.Scope !== "local" ||
          n.Id !== id ||
          n.Labels?.["dev.orbwin.qualification"] !== token ||
          n.Labels?.["com.docker.compose.project"] !== project ||
          n.Name !== project + "_default" ||
          Object.keys(n.Containers ?? {}).length
        )
          throw Error("Project network changed/adopted");
        d(endpoint, ["network", "rm", id]);
      }
    } catch (e) {
      proof.cleanupErrors.push(e.message);
    }
    if (restoreRecord)
      try {
        const v = volume(endpoint, destination);
        if (
          v.CreatedAt !== destinationMetadata?.CreatedAt ||
          v.Driver !== "local" ||
          v.Scope !== "local" ||
          Object.keys(v.Options ?? {}).length ||
          v.Labels?.["dev.orbwin.transfer"] !== restoreRecord.transaction ||
          v.Labels?.["dev.orbwin.source-sha256"] !== restoreRecord.sourceSha256
        )
          throw Error("Destination volume ownership differs");
        if (
          d(endpoint, [
            "ps",
            "-aq",
            "--filter",
            `volume=${destination}`,
          ]).stdout.trim()
        )
          throw Error("Restored volume adopted; retained");
        d(endpoint, ["volume", "rm", destination]);
      } catch (e) {
        proof.cleanupErrors.push(e.message);
      }
    if (sourceVolumeCreated)
      try {
        const v = volume(source, sourceVolume);
        if (
          v.CreatedAt !== sourceVolumeMetadata?.CreatedAt ||
          v.Driver !== "local" ||
          v.Scope !== "local" ||
          Object.keys(v.Options ?? {}).length ||
          v.Labels?.["dev.orbwin.qualification"] !== token
        )
          throw Error("Source volume ownership differs");
        if (
          d(source, [
            "ps",
            "-aq",
            "--filter",
            `volume=${sourceVolume}`,
          ]).stdout.trim()
        )
          throw Error("Source volume adopted; retained");
        d(source, ["volume", "rm", sourceVolume]);
      } catch (e) {
        proof.cleanupErrors.push(e.message);
      }
    for (const { ref, id } of ownedReferences)
      try {
        const v = image(endpoint, ref);
        if (
          v.Id !== id ||
          v.Config.Labels?.["dev.orbwin.qualification"] !== token
        )
          throw Error("Imported fixture image/reference changed");
        if (
          d(endpoint, ["ps", "-aq", "--filter", `ancestor=${id}`]).stdout.trim()
        )
          throw Error("Imported image adopted by a container");
        d(endpoint, ["image", "rm", ref]);
      } catch (e) {
        proof.cleanupErrors.push(e.message);
      }
    if (sourceImage)
      try {
        const v = image(source, alias);
        if (
          v.Id !== sourceImage.Id ||
          v.Config.Labels?.["dev.orbwin.qualification"] !== token
        )
          throw Error("Source fixture image changed");
        d(source, ["image", "rm", alias]);
      } catch (e) {
        proof.cleanupErrors.push(e.message);
      }
  }
  try {
    proof.after = snapshot();
    if (proof.baseline && !isDeepStrictEqual(proof.after, proof.baseline))
      throw Error(
        "All-three canonical inventories/networks/configs not restored",
      );
    proof.globalAfter = proof.after.configs;
    if (proof.baseSha256 && proof.baseSha256 !== sha(readFileSync(composeBase)))
      throw Error("Original source Compose file changed");
    if (
      proof.mappingSha256 &&
      proof.mappingSha256 !== sha(readFileSync(mappingFile))
    )
      throw Error("Reviewed mapping changed");
  } catch (e) {
    proof.cleanupErrors.push(e.message);
  }
  if (proof.cleanupErrors.length) proof.success = false;
  proof.finished = new Date().toISOString();
  save();
  closeSync(proofFD);
}
console.log(
  JSON.stringify({
    proof: resolve(folder, "proof.json"),
    success: proof.success,
    error: proof.error,
    cleanupErrors: proof.cleanupErrors,
  }),
);
if (!proof.success) process.exitCode = 1;
