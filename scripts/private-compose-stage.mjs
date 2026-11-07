// Opt-in synthetic-registry stage. Importing this module performs no work.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  closeSync,
  constants,
  fstatSync,
  lstatSync,
  mkdirSync,
  openSync,
  readdirSync,
  readFileSync,
  readSync,
  writeFileSync,
} from "node:fs";
import { basename, dirname, isAbsolute, resolve } from "node:path";

const label = "dev.orbwin.qualification";
const digest = (value) => createHash("sha256").update(value).digest("hex");
const id = /^[a-f0-9]{64}$/;
const imageId = /^sha256:[a-f0-9]{64}$/;
function records(text) {
  assert.ok(text.length <= 4 << 20, "Cache evidence exceeds bound");
  if (!text.trim()) return [];
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch {
    parsed = text
      .trim()
      .split(/\r?\n/)
      .map((line) => JSON.parse(line));
  }
  const values = Array.isArray(parsed) ? parsed : [parsed];
  assert.ok(values.length <= 4096);
  assert.ok(
    values.every(
      (value) => value && typeof value === "object" && !Array.isArray(value),
    ),
  );
  return values;
}
export function cacheEvidence(duText, historyText) {
  const du = records(duText).map((value) => {
    assert.match(value.ID, /^[a-zA-Z0-9_-]{1,128}$/);
    // Buildx du JSON exposes Size as a formatted decimal string, not bytes.
    assert.equal(typeof value.Size, "string");
    assert.match(value.Size, /^\d+(?:\.\d+)?(?:B|kB|MB|GB|TB)$/);
    assert.ok(Number.isSafeInteger(value.UsageCount) && value.UsageCount >= 0);
    for (const key of ["Mutable", "Reclaimable", "Shared"])
      assert.equal(typeof value[key], "boolean");
    return {
      id: value.ID,
      reportedSize: value.Size,
      usageCount: value.UsageCount,
      mutable: value.Mutable,
      reclaimable: value.Reclaimable,
      shared: value.Shared,
    };
  });
  const history = records(historyText).map((value) => {
    assert.equal(typeof value.ref, "string");
    assert.ok(value.ref.length > 0 && value.ref.length <= 512);
    assert.equal(typeof value.status, "string");
    assert.ok(
      ["completed", "error", "failed", "running", "canceled"].includes(
        value.status.toLowerCase(),
      ),
    );
    for (const key of ["cached_steps", "completed_steps", "total_steps"])
      assert.ok(Number.isSafeInteger(value[key]) && value[key] >= 0);
    return {
      refSha256: digest(value.ref),
      status: value.status,
      cachedSteps: value.cached_steps,
      completedSteps: value.completed_steps,
      totalSteps: value.total_steps,
    };
  });
  assert.equal(new Set(du.map((record) => record.id)).size, du.length);
  assert.equal(
    new Set(history.map((record) => record.refSha256)).size,
    history.length,
  );
  return {
    du: du.sort((a, b) => a.id.localeCompare(b.id)),
    history: history.sort((a, b) => a.refSha256.localeCompare(b.refSha256)),
  };
}
export function registryRequestObservations(text, nonce, manifestDigest) {
  assert.match(nonce, /^[a-f0-9]{32}$/);
  assert.ok(text.length <= 1 << 20, "Registry log evidence exceeds bound");
  if (manifestDigest !== undefined) assert.match(manifestDigest, imageId);
  const observations = [];
  for (let line of text.split(/\r?\n/)) {
    // Docker --timestamps may prefix either JSON or the registry's text logger.
    line = line.replace(/^\d{4}-\d\d-\d\dT\S+\s+/, "");
    let record,
      format = "json";
    try {
      record = JSON.parse(line);
    } catch {
      // Distribution3 access logger uses gorilla CombinedLoggingHandler on
      // stdout, separately from structured response logging on stderr.
      const clf = line.match(
        /^\S+\s+\S+\s+\S+\s+\[[^\]]+\]\s+"([A-Z]+) ([^" ]+) HTTP\/\d(?:\.\d)?"\s+([1-5]\d\d)(?:\s|$)/,
      );
      if (clf) {
        record = {
          "http.request.method": clf[1],
          "http.request.uri": clf[2],
          "http.response.status": clf[3],
        };
        format = "combined-access";
      } else {
        format = "structured-text";
        const field = (name) => {
          const match = line.match(
            new RegExp(
              `(?:^| )${name.replaceAll(".", "\\.")}=(?:"([^"\\\\]*)"|([^ ]+))`,
            ),
          );
          return match?.[1] ?? match?.[2];
        };
        record = {
          "http.request.uri": field("http.request.uri"),
          "http.request.method": field("http.request.method"),
          "http.response.status": field("http.response.status"),
        };
      }
    }
    const method = record?.["http.request.method"],
      uri = record?.["http.request.uri"],
      status = Number(record?.["http.response.status"]);
    if (
      !["GET", "HEAD", "PUT", "POST", "PATCH", "DELETE"].includes(method) ||
      typeof uri !== "string" ||
      uri.length > 4096 ||
      !Number.isInteger(status) ||
      status < 100 ||
      status > 599
    )
      continue;
    const path = uri.split("?", 1)[0];
    let pathClass;
    if (path === "/v2/") pathClass = "registry-base";
    else if (path.startsWith("/v2/fixture/image/")) {
      const suffix = path.slice("/v2/fixture/image/".length);
      if (suffix.startsWith("manifests/")) {
        let reference;
        try {
          reference = decodeURIComponent(suffix.slice("manifests/".length));
        } catch {
          continue;
        }
        if (/[\/\\]/.test(reference)) continue;
        pathClass =
          reference === nonce
            ? "nonce-manifest-tag"
            : manifestDigest && reference === manifestDigest
              ? "known-manifest-digest"
              : imageId.test(reference)
                ? "other-manifest-digest"
                : "other-manifest-tag";
      } else if (suffix.startsWith("blobs/"))
        pathClass = "fixture-repository-blob";
      else pathClass = "fixture-repository-other";
    } else continue;
    observations.push({
      format,
      method,
      pathClass,
      status,
      queryPresent: uri.includes("?"),
    });
    assert.ok(observations.length <= 4096);
  }
  return observations;
}
export function manifestRequests(text, nonce, manifestDigest) {
  const observations = registryRequestObservations(text, nonce, manifestDigest);
  const relevant = observations.filter(
    (record) =>
      ["GET", "HEAD"].includes(record.method) &&
      ["nonce-manifest-tag", "known-manifest-digest"].includes(
        record.pathClass,
      ),
  );
  return {
    success: relevant.filter((record) => record.status === 200).length,
    refused: relevant.filter((record) => record.status === 401).length,
  };
}
export function composePlan(state, negative = false) {
  assert.match(state.nonce, /^[a-f0-9]{32}$/);
  assert.equal(state.name, `orbwin-private-client-${state.nonce}`);
  assert.equal(state.image, `${state.server}/fixture/image:${state.nonce}`);
  assert.match(state.server, /^127\.0\.0\.1:[1-9]\d{0,4}$/);
  assert.ok(Number(state.server.split(":")[1]) <= 65535);
  const tag = `${state.name}:compose-${negative ? "negative" : "built"}`;
  const marker = `${state.nonce}-${negative ? "negative" : "positive"}`;
  return {
    tag,
    marker,
    dockerfile: `FROM ${state.image}\nLABEL ${label}="${state.nonce}" dev.orbwin.private-compose="${marker}"\nRUN ["/bin/busybox", "timeout", "-s", "KILL", "5", "/bin/sh", "-c", "printf '%s' '${marker}' > /orbwin-private-compose-proof"]\nCMD ["/bin/busybox", "timeout", "-s", "KILL", "120", "/bin/busybox", "sleep", "120"]\n`,
    compose: {
      name: state.name,
      services: {
        app: {
          image: tag,
          container_name: `${state.name}-compose`,
          build: {
            context: ".",
            dockerfile: "Dockerfile",
            pull: true,
            no_cache: true,
            network: "none",
            labels: { [label]: state.nonce },
          },
          pull_policy: "never",
          labels: { [label]: state.nonce },
          network_mode: "none",
          read_only: true,
          cpus: 0.25,
          mem_limit: "128m",
          pids_limit: 64,
          cap_drop: ["ALL"],
          security_opt: ["no-new-privileges:true"],
          restart: "no",
        },
      },
    },
  };
}
export function ownedBuiltImage(image, state, plan) {
  assert.match(image.Id, imageId);
  assert.ok(
    !state.before.inventories.owned.images.includes(image.Id),
    "Pre-existing image is not owned",
  );
  assert.equal(image.Config?.Labels?.[label], state.nonce);
  assert.equal(
    image.Config?.Labels?.["dev.orbwin.private-compose"],
    plan.marker,
  );
  assert.ok(
    (image.RepoTags ?? []).every((tag) => tag === plan.tag),
    "Unexpected image reference",
  );
  const base = state.committedRootFS.Layers,
    layers = image.RootFS?.Layers;
  assert.ok(Array.isArray(layers) && layers.length === base.length + 1);
  assert.deepEqual(layers.slice(0, base.length), base);
  assert.equal(image.Os, "linux");
  assert.equal(image.Architecture, "amd64");
  assert.deepEqual(image.Config.Cmd, [
    "/bin/busybox",
    "timeout",
    "-s",
    "KILL",
    "120",
    "/bin/busybox",
    "sleep",
    "120",
  ]);
  assert.ok(!image.Config.Entrypoint || image.Config.Entrypoint.length === 0);
  assert.equal(Object.keys(image.Config.Volumes ?? {}).length, 0);
  return image.Id;
}
export function ownedComposeContainer(value, image, state, plan) {
  assert.match(value.Id, id);
  assert.ok(!state.before.inventories.owned.containers.includes(value.Id));
  assert.equal(value.Name, `/${state.name}-compose`);
  assert.equal(value.Image, ownedBuiltImage(image, state, plan));
  assert.equal(value.Config?.Image, plan.tag);
  assert.equal(value.Config?.Labels?.[label], state.nonce);
  assert.deepEqual(value.Config?.Cmd, image.Config.Cmd);
  assert.ok(!value.Config?.Entrypoint || value.Config.Entrypoint.length === 0);
  assert.equal(value.HostConfig?.NetworkMode, "none");
  assert.equal(value.HostConfig?.ReadonlyRootfs, true);
  assert.equal(value.HostConfig?.Privileged, false);
  assert.equal(value.HostConfig?.NanoCpus, 250000000);
  assert.equal(value.HostConfig?.Memory, 134217728);
  assert.equal(value.HostConfig?.PidsLimit, 64);
  assert.ok(value.HostConfig?.CapDrop?.includes("ALL"));
  assert.ok(
    value.HostConfig?.SecurityOpt?.some((value) =>
      ["no-new-privileges", "no-new-privileges:true"].includes(value),
    ),
  );
  assert.equal(value.HostConfig?.RestartPolicy?.Name, "no");
  assert.deepEqual(value.Mounts, []);
  assert.equal(value.State?.Paused, false);
  return value.Id;
}

const cleared = Object.fromEntries(
  [
    "COMPOSE_BAKE",
    "BUILDX_BAKE_ENTITLEMENTS_FS",
    "BAKE_ALLOW_REMOTE_FS_ACCESS",
    "BUILDX_BAKE_FILE",
    "BUILDX_BAKE_FILE_RELATIVE_PATHS",
    "BUILDX_BAKE_PATH_SEPARATOR",
    "DOCKER_AUTH_CONFIG",
  ].map((key) => [key, undefined]),
);
function client(ctx, args, options = {}) {
  return ctx.execute(ctx.state.wrapper, args, {
    timeout: 90_000,
    extraEnv: cleared,
    ...options,
  });
}
function read(ctx, args) {
  return client(ctx, args, { timeout: 30_000 }).stdout.trim();
}
function cache(ctx) {
  // Instrumentation uses the verified raw client: product alias intentionally
  // restricts Buildx du/history. This is not a broadened product CLI claim.
  const raw = resolve(
    dirname(ctx.state.wrapper),
    "runtime/tools/bin/docker.exe",
  );
  const query = (args) =>
    ctx
      .execute(
        raw,
        [
          "--config",
          ctx.state.privateConfig,
          "--host",
          "npipe:////./pipe/docker-rancher-desktop-orbwin",
          "buildx",
          "--builder",
          "default",
          ...args,
        ],
        {
          timeout: 30_000,
          extraEnv: {
            ...cleared,
            DOCKER_CONFIG: ctx.state.privateConfig,
            DOCKER_HOST: "npipe:////./pipe/docker-rancher-desktop-orbwin",
            DOCKER_CONTEXT: "",
            BUILDX_BUILDER: "default",
            BUILDX_CONFIG: resolve(ctx.state.privateConfig, "buildx"),
          },
        },
      )
      .stdout.trim();
  return cacheEvidence(
    query(["du", "--format=json", "--timeout=10s"]),
    query(["history", "ls", "--format=json", "--no-trunc"]),
  );
}
function builderMetadata(ctx) {
  return builderMetadataAt(resolve(ctx.state.privateConfig, "buildx"));
}
function optionalStat(path) {
  try {
    return lstatSync(path);
  } catch (error) {
    if (error.code === "ENOENT") return null;
    throw error;
  }
}
export function metadataDirectoryChain(path) {
  // Docker plugin discovery returns Win32 namespace paths. path.dirname treats
  // their drive root differently; canonicalize only the local-drive prefix.
  if (path.startsWith("\\\\?\\") && /^[A-Za-z]:\\/.test(path.slice(4)))
    path = path.slice(4);
  const chain = [];
  let current = resolve(path);
  for (let count = 0; count < 64; count++) {
    chain.unshift(current);
    const parent = dirname(current);
    if (parent === current) break;
    current = parent;
    assert.ok(count < 63, "Metadata path depth exceeds bound");
  }
  return chain;
}
function safeDirectories(path) {
  for (const directory of metadataDirectoryChain(path)) {
    const info = optionalStat(directory);
    assert.ok(
      info && info.isDirectory() && !info.isSymbolicLink(),
      "Metadata directory link/non-directory refused",
    );
  }
}
function sameIdentity(a, b) {
  assert.equal(a.dev, b.dev);
  assert.equal(a.ino, b.ino);
  assert.equal(a.isFile(), b.isFile());
  assert.equal(a.isDirectory(), b.isDirectory());
  assert.equal(b.isSymbolicLink(), false);
}
function boundedSelection(path) {
  safeDirectories(dirname(path));
  const original = optionalStat(path);
  if (!original) return null;
  assert.ok(
    original.isFile() && !original.isSymbolicLink(),
    "Selection link/nonregular file refused",
  );
  assert.ok(original.size <= 65536, "Selection metadata exceeds64KiB");
  // Windows Node has no O_NOFOLLOW. Verify opened handle identity before reading
  // and path identity after it; never read a different symlink target's payload.
  const fd = openSync(path, constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0));
  try {
    const opened = fstatSync(fd);
    sameIdentity(original, opened);
    assert.ok(opened.isFile() && opened.size <= 65536);
    const bytes = Buffer.alloc(65537);
    let length = 0,
      count;
    while (
      length < bytes.length &&
      (count = readSync(fd, bytes, length, bytes.length - length, null)) > 0
    )
      length += count;
    assert.ok(length <= 65536, "Selection metadata grew beyond64KiB");
    sameIdentity(original, fstatSync(fd));
    const after = lstatSync(path);
    sameIdentity(original, after);
    assert.equal(after.size, original.size);
    assert.equal(after.mtimeMs, original.mtimeMs);
    safeDirectories(dirname(path));
    return digest(bytes.subarray(0, length));
  } finally {
    closeSync(fd);
  }
}
export function builderMetadataAt(root) {
  safeDirectories(root);
  const selection = Object.fromEntries(
    ["current", ".buildNodeID"].map((name) => [
      name,
      boundedSelection(resolve(root, name)),
    ]),
  );
  // Only names/stat metadata of ref/activity/instance directories: no certificate
  // payloads, instance credentials, config snapshots or arbitrary file contents.
  const entries = [];
  function walk(path, prefix, depth) {
    safeDirectories(dirname(path));
    const directory = optionalStat(path);
    if (!directory) return;
    assert.ok(
      directory.isDirectory() && !directory.isSymbolicLink(),
      "Metadata traversal link refused",
    );
    assert.ok(depth <= 5);
    for (const entry of readdirSync(path, { withFileTypes: true })) {
      assert.ok(!entry.isSymbolicLink(), "Buildx metadata symlink refused");
      const name = `${prefix}/${entry.name}`;
      const child = resolve(path, entry.name),
        stat = lstatSync(child);
      assert.ok(
        !stat.isSymbolicLink() && (stat.isFile() || stat.isDirectory()),
      );
      assert.ok(entries.length < 4096);
      entries.push({
        nameSha256: digest(name),
        group: prefix.split("/")[0],
        directory: stat.isDirectory(),
        bytes: stat.size,
        modifiedMs: stat.mtimeMs,
      });
      if (stat.isDirectory()) walk(child, name, depth + 1);
    }
    sameIdentity(directory, lstatSync(path));
    safeDirectories(path);
  }
  for (const name of ["defaults", "instances", "activity", "refs"])
    walk(resolve(root, name), name, 0);
  return {
    selection,
    entries: entries.sort((a, b) => a.nameSha256.localeCompare(b.nameSha256)),
  };
}
function requestCounts(ctx, phase) {
  const output = client(
    ctx,
    ["logs", "--tail=2000", "--timestamps", ctx.state.registryContainer],
    { timeout: 2_000 },
  );
  const text = (output.stdout ?? "") + (output.stderr ?? "");
  const records = registryRequestObservations(
    text,
    ctx.state.nonce,
    ctx.state.registryManifestDigest,
  );
  const counts = manifestRequests(
    text,
    ctx.state.nonce,
    ctx.state.registryManifestDigest,
  );
  ctx.state.privateCompose.observationSamples ??= [];
  ctx.state.privateCompose.observationSamples.push({
    phase,
    counts,
    sanitizedRequests: records,
  });
  ctx.save();
  requireEvidence(
    text.split(/\r?\n/).length < 2000,
    "MANIFEST_LOG_WINDOW_SATURATED",
  );
  return counts;
}
function requireEvidence(condition, code) {
  if (!condition)
    throw Object.assign(
      Error(
        "Required manifest evidence unavailable; see sanitized observations",
      ),
      { code },
    );
}
export async function pollManifestEvidence(ctx, before, negative) {
  let after;
  // At most five bounded2s log reads plus four150ms flush waits. No new request
  // to the registry is injected to satisfy this operation's evidence gate.
  for (let attempt = 0; attempt < 5; attempt++) {
    after = requestCounts(ctx, `after-build-${attempt + 1}`);
    if (
      negative ? after.refused > before.refused : after.success > before.success
    )
      break;
    if (attempt < 4) await new Promise((resolve) => setTimeout(resolve, 150));
  }
  return after;
}
function image(ctx, tag) {
  return JSON.parse(read(ctx, ["image", "inspect", tag]))[0];
}
export function selectedPluginEvidence(plugins, tools, hashFile) {
  assert.ok(Array.isArray(plugins) && plugins.length <= 64);
  return ["compose", "buildx"].map((name) => {
    const selected = plugins.filter((plugin) => plugin.Name === name);
    assert.equal(selected.length, 1, "Ambiguous/missing default plugin");
    const plugin = selected[0],
      path = `docker-cli-plugins/docker-${name}.exe`;
    const pin = tools.find((tool) => tool.path === path);
    assert.ok(pin?.sha256);
    assert.ok(typeof plugin.Path === "string" && isAbsolute(plugin.Path));
    assert.equal(basename(plugin.Path).toLowerCase(), `docker-${name}.exe`);
    assert.equal(plugin.Version?.replace(/^v/, ""), pin.version.replace(/^v/, ""));
    const sha256 = hashFile(plugin.Path);
    assert.equal(
      sha256,
      pin.sha256,
      "Discovered default plugin bytes differ from pin",
    );
    return {
      name,
      basename: basename(plugin.Path),
      sha256,
      version: pin.version,
      provenance:
        name === "compose"
          ? "Reviewed local Compose patch; current runtime-tools manifest"
          : pin.localSource?.name === "buildx"
            ? "Reviewed local Buildx dependency patch; current runtime-tools manifest"
            : "Pinned upstream Buildx; current runtime-tools manifest",
    };
  });
}
export function verifyTools(ctx) {
  const tools = JSON.parse(
    readFileSync(resolve(ctx.root, "artifacts/runtime-tools/manifest.json")),
  ).tools;
  assert.equal(
    digest(
      readFileSync(
        resolve(dirname(ctx.state.wrapper), "runtime/tools/bin/docker.exe"),
      ),
    ),
    tools.find((tool) => tool.path === "bin/docker.exe")?.sha256,
  );
  for (const name of ["compose", "buildx"]) {
    const path = `docker-cli-plugins/docker-${name}.exe`;
    const pin = tools.find((tool) => tool.path === path);
    assert.ok(pin?.sha256);
    assert.equal(
      digest(
        readFileSync(
          resolve(dirname(ctx.state.wrapper), "runtime/tools", path),
        ),
      ),
      pin.sha256,
    );
    if (name === "compose") {
      assert.equal(pin.version, "5.6.1-orbwin.2");
      assert.equal(read(ctx, ["compose", "version", "--short"]), pin.version);
    }
  }
  const plugins = JSON.parse(
    read(ctx, ["info", "--format", "{{json .ClientInfo.Plugins}}"]),
  );
  return selectedPluginEvidence(plugins, tools, (path) => {
    safeDirectories(dirname(path));
    const stat = lstatSync(path);
    assert.ok(
      stat.isFile() && !stat.isSymbolicLink() && stat.size <= 256 << 20,
    );
    return digest(readFileSync(path));
  });
}
function composeArgs(ctx, plan) {
  return [
    "compose",
    "--project-name",
    ctx.state.name,
    "--file",
    resolve(ctx.state.privateCompose.directory, plan.marker, "compose.json"),
  ];
}
function prepare(ctx, plan) {
  const directory = resolve(ctx.state.privateCompose.directory, plan.marker);
  mkdirSync(directory);
  writeFileSync(resolve(directory, "Dockerfile"), plan.dockerfile, {
    flag: "wx",
  });
  writeFileSync(
    resolve(directory, "compose.json"),
    JSON.stringify(plan.compose, null, 2),
    { flag: "wx" },
  );
  ctx.state.privateCompose.sources ??= [];
  ctx.state.privateCompose.sources.push({
    marker: plan.marker,
    dockerfileSha256: digest(plan.dockerfile),
    composeSha256: digest(JSON.stringify(plan.compose, null, 2)),
  });
  ctx.save();
}

function checkpoint(ctx, mode, stage) {
  ctx.state.privateComposeProgress = { mode, stage };
  ctx.save();
}
export function safeStageFailure(error, progress = {}) {
  const code =
    typeof error?.code === "string" && /^[A-Z][A-Z0-9_]{0,63}$/.test(error.code)
      ? error.code
      : "STAGE_FAILED";
  const source = String(error?.stack ?? "").match(
    /private-compose-stage\.mjs:(\d+):(\d+)/,
  );
  return {
    mode: progress.mode,
    stage: progress.stage ?? "unknown",
    code,
    source: source
      ? {
          file: "private-compose-stage.mjs",
          line: Number(source[1]),
          column: Number(source[2]),
        }
      : undefined,
  };
}
export async function runPrivateComposeStage(ctx, mode) {
  try {
    return await runComposeStage(ctx, mode);
  } catch (error) {
    ctx.state.privateComposeFailure = safeStageFailure(
      error,
      ctx.state.privateComposeProgress,
    );
    ctx.save();
    throw Error(
      `Private Compose stage failed (${ctx.state.privateComposeFailure.stage}/${ctx.state.privateComposeFailure.code}); inspect minimized stage receipt`,
    );
  }
}
export function preflightPrivateCompose(ctx) {
  return {
    selectedPlugins: verifyTools(ctx),
    cache: cache(ctx),
    metadata: builderMetadata(ctx),
  };
}
async function runComposeStage(ctx, mode) {
  assert.ok(["authenticated", "forgotten"].includes(mode));
  checkpoint(ctx, mode, "verify-tools");
  const selectedPlugins = verifyTools(ctx);
  const state = ctx.state,
    negative = mode === "forgotten",
    plan = composePlan(state, negative);
  if (!negative) {
    assert.equal(state.privateCompose, undefined);
    checkpoint(ctx, mode, "cache-baseline");
    const before = cache(ctx);
    checkpoint(ctx, mode, "metadata-baseline");
    const metadataBefore = builderMetadata(ctx);
    state.privateCompose = {
      directory: resolve(state.workspace, "private-compose"),
      before,
      metadataBefore,
      outputTags: [],
      requests: [],
      selectedPlugins,
    };
    mkdirSync(state.privateCompose.directory);
    ctx.save();
  }
  assert.ok(state.privateCompose);
  checkpoint(ctx, mode, "source-reference-preconditions");
  assert.deepEqual(selectedPlugins, state.privateCompose.selectedPlugins);
  assert.equal(
    read(ctx, ["image", "ls", "--filter", `reference=${state.image}`, "-q"]),
    "",
    "Private base reference must be absent",
  );
  assert.equal(
    read(ctx, ["image", "ls", "--filter", `reference=${plan.tag}`, "-q"]),
    "",
  );
  prepare(ctx, plan);
  ctx.intent("image", plan.tag);
  ctx.intent("buildkit-cache", plan.marker);
  state.privateCompose.outputTags.push(plan.tag);
  ctx.save();
  checkpoint(ctx, mode, "manifest-before");
  const before = requestCounts(ctx, "before-build");
  checkpoint(ctx, mode, "forced-build");
  const result = client(
    ctx,
    [...composeArgs(ctx, plan), "build", "--pull", "--no-cache", "app"],
    { allowedFailure: true },
  );
  checkpoint(ctx, mode, "manifest-after");
  const after = await pollManifestEvidence(ctx, before, negative);
  state.privateCompose.requests.push({ mode, before, after });
  checkpoint(ctx, mode, "cache-after");
  state.privateCompose.after = cache(ctx);
  ctx.save();
  assert.equal(result.error, undefined);
  if (negative) {
    checkpoint(ctx, mode, "negative-refusal");
    assert.notEqual(result.status, 0);
    assert.match(
      (result.stderr ?? "") + (result.stdout ?? ""),
      /unauthorized|authentication required|no basic auth credentials|unexpected status.*401/i,
    );
    requireEvidence(
      after.refused > before.refused,
      "NO_MANIFEST_REFUSAL_EVIDENCE",
    );
    assert.equal(
      read(ctx, ["image", "ls", "--filter", `reference=${plan.tag}`, "-q"]),
      "",
    );
    assert.equal(
      JSON.parse(
        read(ctx, ["container", "inspect", state.registryContainer]),
      )[0].State.Running,
      true,
    );
    state.steps.push(
      "default patched Compose forced private FROM build refused after native Forget; registry manifest401 observed",
    );
  } else {
    checkpoint(ctx, mode, "positive-build-verification");
    assert.equal(
      result.status,
      0,
      "Compose private build failed; no fallback permitted",
    );
    requireEvidence(
      after.success > before.success,
      "NO_MANIFEST_SUCCESS_EVIDENCE",
    );
    const built = image(ctx, plan.tag);
    state.privateCompose.builtId = ownedBuiltImage(built, state, plan);
    ctx.save();
    ctx.intent("container", `${state.name}-compose`);
    checkpoint(ctx, mode, "service-up");
    client(ctx, [
      ...composeArgs(ctx, plan),
      "up",
      "--no-build",
      "--pull",
      "never",
      "--detach",
      "app",
    ]);
    checkpoint(ctx, mode, "service-verification");
    const ids = read(ctx, [
      ...composeArgs(ctx, plan),
      "ps",
      "--all",
      "--quiet",
      "--no-trunc",
      "app",
    ])
      .split(/\r?\n/)
      .filter(Boolean);
    assert.equal(ids.length, 1);
    const value = JSON.parse(read(ctx, ["container", "inspect", ids[0]]))[0];
    state.privateCompose.containerId = ownedComposeContainer(
      value,
      built,
      state,
      plan,
    );
    ctx.save();
    assert.equal(
      read(ctx, [
        "exec",
        value.Id,
        "/bin/busybox",
        "timeout",
        "-s",
        "KILL",
        "5",
        "/bin/busybox",
        "cat",
        "/orbwin-private-compose-proof",
      ]),
      plan.marker,
    );
    checkpoint(ctx, mode, "service-cleanup");
    cleanupPrivateComposeStage(ctx, false);
    state.steps.push(
      "default patched Compose forced private FROM build observed manifest200; bounded service marker and limits verified",
    );
  }
  checkpoint(ctx, mode, "complete");
  ctx.save();
}

export function cleanupPrivateComposeStage(ctx, final = true) {
  const state = ctx.state,
    receipt = state.privateCompose;
  if (!receipt) return;
  const plan = composePlan(state);
  // Discover intent-owned name after uncertain up, never adopt by name alone.
  const found = read(ctx, [
    "ps",
    "-aq",
    "--no-trunc",
    "--filter",
    `name=^/${state.name}-compose$`,
  ])
    .split(/\r?\n/)
    .filter(Boolean);
  assert.ok(found.length <= 1);
  for (const fullId of found) {
    assert.ok(
      state.intents.some(
        (intent) =>
          intent.kind === "container" &&
          intent.name === `${state.name}-compose`,
      ),
    );
    const built = image(ctx, plan.tag);
    const value = JSON.parse(read(ctx, ["container", "inspect", fullId]))[0];
    ownedComposeContainer(value, built, state, plan);
    if (value.State.Running) client(ctx, ["stop", "--time", "5", fullId]);
    ownedComposeContainer(
      JSON.parse(read(ctx, ["container", "inspect", fullId]))[0],
      image(ctx, plan.tag),
      state,
      plan,
    );
    client(ctx, ["rm", fullId]);
  }
  for (const negative of [false, true]) {
    const current = composePlan(state, negative);
    const values = read(ctx, [
      "image",
      "ls",
      "--filter",
      `reference=${current.tag}`,
      "-q",
      "--no-trunc",
    ])
      .split(/\r?\n/)
      .filter(Boolean);
    for (const fullId of new Set(values)) {
      assert.ok(receipt.outputTags.includes(current.tag));
      const value = image(ctx, current.tag);
      assert.equal(value.Id, fullId);
      ownedBuiltImage(value, state, current);
      client(ctx, ["image", "rm", current.tag]);
    }
  }
  if (final) {
    receipt.after = cache(ctx);
    receipt.metadataAfter = builderMetadata(ctx);
    assert.deepEqual(
      receipt.metadataAfter.selection,
      receipt.metadataBefore.selection,
      "Buildx selection identity changed; preserved for review",
    );
    for (const group of ["defaults", "instances"])
      assert.deepEqual(
        receipt.metadataAfter.entries.filter((entry) => entry.group === group),
        receipt.metadataBefore.entries.filter((entry) => entry.group === group),
        "Buildx default/instance metadata changed; preserved for review",
      );
    receipt.cacheResidue =
      "BuildKit cache/history and product Buildx refs/activity retained, not pruned. Reported du sizes are formatted API metrics, not physical byte deltas.";
    ctx.save();
  }
}
