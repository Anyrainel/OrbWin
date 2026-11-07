// Disposable Desktop -> OrbWin image transfer. Source workload data is never
// selected. No pull, engine restart, context change, prune or forced removal.
import { spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { isDeepStrictEqual } from "node:util";
import { endpoint, verifyIdentity } from "./engine-identity.mjs";
import { verifyBuild } from "./build-identity.mjs";

if (
  process.env.ORBWIN_QUALIFY_OWNED !== "1" ||
  process.env.ORBWIN_QUALIFY_MIGRATION_SOURCE !== "docker-desktop"
)
  throw new Error(
    "Explicit owned and Docker Desktop fixture qualification opt-ins required",
  );
const root = fileURLToPath(new URL("../", import.meta.url));
const token = randomUUID().replaceAll("-", "");
const fixture = `orbwin-migration-${token}`;
const folder = resolve(root, "artifacts/qualification", fixture);
mkdirSync(folder, { recursive: false });
const config = resolve(folder, "docker");
mkdirSync(config);
const transient = resolve(folder, "transient");
mkdirSync(transient);
const sourceEndpoint = "npipe:////./pipe/dockerDesktopLinuxEngine";
const docker = resolve(root, "artifacts/runtime-tools/bin/docker.exe");
const sdk = resolve(root, "artifacts/backend/orbwin-session.exe");
const identity = JSON.parse(
  readFileSync(resolve(root, "artifacts/qualification/engine-identity.json")),
);
const build = verifyBuild(root);
const sourceExpected = identity.qualification.engines.find(
  (x) => x.name === "desktop",
).daemonId;
const alias = `${fixture}:source`;
const env = {
  ...process.env,
  DOCKER_HOST: "",
  DOCKER_CONTEXT: "",
  DOCKER_API_VERSION: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  DOCKER_CONFIG: config,
  ORBWIN_TRANSIENT_DIR: transient,
  ORBWIN_FS_HELPER: resolve(root, "artifacts/backend/orbwin-fs"),
  ORBWIN_FS_HELPER_SHA256: JSON.parse(
    readFileSync(resolve(root, "artifacts/backend/fs-manifest.json")),
  ).binarySha256,
  ORBWIN_MAINTENANCE_IMAGE: resolve(
    root,
    "artifacts/backend/orbwin-maintenance.tar",
  ),
  ORBWIN_MAINTENANCE_IMAGE_SHA256: JSON.parse(
    readFileSync(resolve(root, "maintenance-image.lock.json")),
  ).sha256,
};
const proof = {
  schema: 1,
  fixture,
  alias,
  started: new Date().toISOString(),
  sourceEndpoint,
  targetEndpoint: endpoint,
  sdkSha256: hash(sdk),
  steps: [],
  cleanupErrors: [],
  success: false,
};
const ownedRefs = [];
let sourceID;
function hash(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}
function save() {
  writeFileSync(resolve(folder, "proof.json"), JSON.stringify(proof, null, 2));
}
function run(executable, args, input, allowFailure = false) {
  const result = spawnSync(executable, args, {
    cwd: root,
    env,
    input,
    encoding: "utf8",
    windowsHide: true,
    timeout: 180_000,
    maxBuffer: 20 << 20,
  });
  proof.steps.push({
    executable,
    args,
    status: result.status,
    error: result.error?.message,
    stdout: result.stdout,
    stderr: result.stderr,
  });
  save();
  if (!allowFailure && (result.error || result.status !== 0))
    throw new Error(
      `Qualification command failed: ${args[0]}: ${result.stderr ?? result.error}`,
    );
  return result;
}
function d(host, args, allowFailure = false) {
  return run(docker, ["--host", host, ...args], undefined, allowFailure);
}
function inspect(host, ref) {
  return JSON.parse(
    d(host, ["image", "inspect", ref, "--format", "{{json .}}"]).stdout,
  );
}
function globals() {
  const result = {};
  for (const [name, path] of Object.entries({
    docker: resolve(process.env.USERPROFILE, ".docker/config.json"),
    kube: resolve(process.env.USERPROFILE, ".kube/config"),
  }))
    result[name] = existsSync(path) ? hash(path) : null;
  return result;
}
try {
  proof.globalBefore = globals();
  const target = JSON.parse(
    d(endpoint, ["info", "--format", "{{json .}}"]).stdout,
  );
  verifyIdentity(identity, target, build);
  const review = JSON.parse(
    run(sdk, ["migration-review", "docker-desktop", target.ID]).stdout,
  );
  if (review.source.daemon !== sourceExpected)
    throw new Error("Desktop source differs from coexistence record");
  proof.source = review.source;
  proof.target = target.ID;
  proof.inventoryCounts = {
    images: review.images.length,
    volumes: review.volumes.length,
    projectMembers: review.projectMembers.length,
  };
  if (
    !d(sourceEndpoint, ["image", "inspect", alias], true).status ||
    !d(endpoint, ["image", "inspect", alias], true).status
  )
    throw new Error("Unique fixture tag unexpectedly exists");
  const context = resolve(folder, "source");
  mkdirSync(context);
  const payload = `OrbWin disposable migration fixture ${token}\n`;
  writeFileSync(resolve(context, "proof.txt"), payload);
  const fixtureTar = resolve(folder, "fixture.tar");
  run("tar.exe", ["-cf", fixtureTar, "-C", context, "proof.txt"]);
  // Intent saved before creating the only fixture resource on Desktop.
  proof.sourceIntent = { alias, component: "migration-qualification", token };
  save();
  d(sourceEndpoint, [
    "image",
    "import",
    "--platform",
    "linux/amd64",
    "--change",
    "LABEL dev.orbwin.component=migration-qualification",
    "--change",
    `LABEL dev.orbwin.qualification=${token}`,
    fixtureTar,
    alias,
  ]);
  const before = inspect(sourceEndpoint, alias);
  if (before.Config?.Labels?.["dev.orbwin.qualification"] !== token)
    throw new Error("Source fixture ownership differs");
  sourceID = before.Id;
  proof.sourceImage = { id: sourceID, rootfs: before.RootFS };
  const archive = resolve(folder, "selected 例.tar");
  const args = [
    "migration-image-export",
    "docker-desktop",
    target.ID,
    sourceExpected,
    sourceID,
    alias,
    archive,
  ];
  const stale = [...args];
  stale[3] = "stale-source";
  if (run(sdk, stale, undefined, true).status === 0 || existsSync(archive))
    throw new Error("Stale source was accepted/published");
  const exported = JSON.parse(run(sdk, args).stdout);
  if (
    !exported.complete ||
    exported.source.daemon !== sourceExpected ||
    exported.sha256 !== hash(archive)
  )
    throw new Error("Export provenance/checksum differs");
  proof.export = exported;
  const sourceReceiptBytes = readFileSync(exported.receipt, "utf8");
  const sourceReceipt = sourceReceiptBytes
    .trimEnd()
    .split("\n")
    .map(JSON.parse);
  if (
    sourceReceipt.length !== 2 ||
    sourceReceipt[0].transaction !== exported.transaction ||
    sourceReceipt[0].source.daemon !== sourceExpected ||
    sourceReceipt[0].target !== target.ID ||
    sourceReceipt[0].image !== sourceID ||
    sourceReceipt[0].destination !== archive ||
    sourceReceipt[1].phase !== "exported" ||
    sourceReceipt[1].sha256 !== exported.sha256 ||
    sourceReceipt[1].bytes !== exported.bytes ||
    !isDeepStrictEqual(
      sourceReceipt[1].configIds,
      exported.images.map((x) => x.id),
    )
  )
    throw new Error("Durable source export acknowledgement differs");
  proof.sourceExportReceipt = sourceReceipt;
  if (
    run(sdk, args, undefined, true).status === 0 ||
    hash(archive) !== exported.sha256
  )
    throw new Error("Existing destination replaced");
  const preview = JSON.parse(
    run(sdk, ["image-import-review", target.ID, archive]).stdout,
  );
  if (
    preview.images.length !== 1 ||
    preview.tags.length !== 1 ||
    preview.tags[0].currentId !== "" ||
    preview.tags[0].configIds.length !== 1
  )
    throw new Error("Fixture import preview is not uniquely absent");
  const request = {
    source: archive,
    sha256: preview.sha256,
    tags: preview.tags.map((t) => ({
      reference: t.reference,
      configId: t.configIds[0],
      expectedId: "",
      apply: true,
      replace: false,
    })),
  };
  const importedRun = run(
    sdk,
    ["import-images", target.ID],
    JSON.stringify(request),
    true,
  );
  // Cleanup only acknowledged references. If CLI delivery fails, retain the
  // exact fixture receipt/resources for review instead of broad removal.
  if (importedRun.status !== 0)
    throw new Error(
      "Import did not complete; retained receipt requires review",
    );
  const imported = JSON.parse(importedRun.stdout);
  proof.import = imported;
  for (const [ref, id] of Object.entries(imported.tagged))
    ownedRefs.push({ ref, id });
  for (const stage of imported.images)
    ownedRefs.push({
      ref: stage.reference,
      id: imported.verified[stage.configId],
    });
  proof.imageHistory = JSON.parse(
    run(sdk, ["migration-image-history", target.ID]).stdout,
  );
  const linkedExport = proof.imageHistory.records.find(
    (x) => x.transaction === exported.transaction,
  );
  if (
    !linkedExport?.exportAcknowledged ||
    linkedExport.sha256 !== exported.sha256 ||
    !linkedExport.imports?.some(
      (x) =>
        x.transaction === imported.transaction &&
        x.phase === "complete" &&
        x.verifiedConfigs === exported.images.length,
    )
  )
    throw new Error(
      "Import owning receipt did not link to acknowledged source export",
    );
  const after = inspect(endpoint, alias);
  if (!isDeepStrictEqual(after.RootFS, before.RootFS))
    throw new Error("Source/target rootfs diff IDs differ");
  const retained = inspect(sourceEndpoint, alias);
  if (
    retained.Id !== sourceID ||
    !isDeepStrictEqual(retained.RootFS, before.RootFS)
  )
    throw new Error("Source image changed");
  proof.sourcePreserved = true;
  proof.targetRootfsVerified = true;
  proof.success = true;
} catch (error) {
  proof.error = error.stack;
} finally {
  for (const { ref, id } of ownedRefs) {
    try {
      if (!id || inspect(endpoint, ref).Id !== id)
        throw new Error("Target reference changed; retained");
      d(endpoint, ["image", "rm", ref]);
    } catch (error) {
      proof.cleanupErrors.push(`${ref}: ${error.message}`);
    }
  }
  if (sourceID) {
    try {
      const object = inspect(sourceEndpoint, alias);
      if (
        object.Id !== sourceID ||
        object.Config?.Labels?.["dev.orbwin.qualification"] !== token
      )
        throw new Error("Source fixture changed; retained");
      d(sourceEndpoint, ["image", "rm", alias]);
    } catch (error) {
      proof.cleanupErrors.push(`${alias}: ${error.message}`);
    }
  }
  proof.globalAfter = globals();
  if (
    JSON.stringify(proof.globalAfter) !== JSON.stringify(proof.globalBefore)
  ) {
    proof.success = false;
    proof.cleanupErrors.push("Global client configuration changed");
  }
  if (proof.cleanupErrors.length) proof.success = false;
  proof.finished = new Date().toISOString();
  save();
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
