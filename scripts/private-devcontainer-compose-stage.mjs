// Separate opt-in Dev Containers/default Compose journey; no import side effects.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  closeSync,
  fstatSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  readSync,
  realpathSync,
  writeFileSync,
} from "node:fs";
import {
  basename,
  dirname,
  isAbsolute,
  relative,
  resolve,
  sep,
} from "node:path";
import { tmpdir } from "node:os";
import {
  metadataDirectoryChain,
  preflightPrivateCompose,
  registryRequestObservations,
  manifestRequests,
} from "./private-compose-stage.mjs";

const label = "dev.orbwin.qualification",
  markerLabel = "dev.orbwin.private-devcontainer-compose";
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
const fullId = /^[a-f0-9]{64}$/,
  imageId = /^sha256:[a-f0-9]{64}$/;
const finiteCommand = [
  "/bin/busybox",
  "timeout",
  "-s",
  "KILL",
  "300",
  "/bin/busybox",
  "sleep",
  "300",
];
const cleared = Object.fromEntries(
  [
    "COMPOSE_BAKE",
    "COMPOSE_LOG_LEVEL",
    "BUILDX_BAKE_ENTITLEMENTS_FS",
    "BAKE_ALLOW_REMOTE_FS_ACCESS",
    "BUILDX_BAKE_FILE",
    "BUILDX_BAKE_FILE_RELATIVE_PATHS",
    "BUILDX_BAKE_PATH_SEPARATOR",
    "DOCKER_AUTH_CONFIG",
  ].map((key) => [key, undefined]),
);

export function createCallbackBudget(
  totalMs,
  cleanupReserveMs = 20000,
  now = Date.now,
) {
  assert.ok(Number.isSafeInteger(totalMs) && totalMs > cleanupReserveMs);
  assert.ok(Number.isSafeInteger(cleanupReserveMs) && cleanupReserveMs >= 0);
  const started = now(),
    totalDeadline = started + totalMs,
    workDeadline = totalDeadline - cleanupReserveMs;
  let cleanup = false;
  const limitTimeout = (requested) => {
    assert.ok(Number.isSafeInteger(requested) && requested > 0);
    const remaining = Math.floor(
      (cleanup ? totalDeadline : workDeadline) - now(),
    );
    if (remaining <= 0)
      throw Object.assign(
        Error(
          "Dedicated callback deadline exhausted; accepted state may remain",
        ),
        { code: "DC_CALLBACK_DEADLINE_EXCEEDED" },
      );
    return Math.min(requested, remaining);
  };
  return {
    limitTimeout,
    assertCurrent: () => {
      limitTimeout(1);
    },
    phase: () => (cleanup ? "cleanup-reserve" : "work"),
    snapshot: () => ({
      startedAt: new Date(started).toISOString(),
      totalMs,
      cleanupReserveMs,
      workDeadlineAt: new Date(workDeadline).toISOString(),
      totalDeadlineAt: new Date(totalDeadline).toISOString(),
    }),
    withCleanupBudget(task) {
      const previous = cleanup;
      cleanup = true;
      try {
        const result = task();
        assert.ok(!result?.then, "Cleanup budget requires synchronous work");
        return result;
      } finally {
        cleanup = previous;
      }
    },
  };
}

export function devcontainerComposePlan(state, negative = false) {
  assert.match(state.nonce, /^[a-f0-9]{32}$/);
  assert.equal(state.name, `orbwin-private-client-${state.nonce}`);
  assert.match(state.server, /^127\.0\.0\.1:[1-9]\d{0,4}$/);
  assert.ok(Number(state.server.split(":")[1]) <= 65535);
  assert.equal(state.image, `${state.server}/fixture/image:${state.nonce}`);
  const mode = negative ? "negative" : "positive",
    marker = `${state.nonce}-dc-${mode}`;
  const workspace = resolve(state.workspace, `devcontainer-compose-${mode}`);
  const tag = `${state.name}:dc-compose-${mode}`,
    name = `${state.name}-dc-${mode}`;
  const dockerfile = `FROM ${state.image}\nLABEL ${label}="${state.nonce}" ${markerLabel}="${marker}"\nRUN ["/bin/busybox", "timeout", "-s", "KILL", "5", "/bin/sh", "-c", "printf '%s' '${marker}' > /orbwin-devcontainer-compose-proof"]\nCMD ${JSON.stringify(finiteCommand)}\n`;
  return {
    mode,
    marker,
    workspace,
    tag,
    name,
    dockerfile,
    config: {
      name,
      dockerComposeFile: "compose.json",
      service: "app",
      workspaceFolder: "/workspace",
      containerUser: "root",
      remoteUser: "root",
      userEnvProbe: "none",
      updateRemoteUserUID: false,
      overrideCommand: false,
      shutdownAction: "none",
      customizations: { vscode: { extensions: [] } },
    },
    compose: {
      name,
      services: {
        app: {
          image: tag,
          container_name: name,
          build: {
            context: "./build",
            dockerfile: "Dockerfile",
            pull: true,
            no_cache: true,
            network: "none",
            labels: { [label]: state.nonce },
          },
          pull_policy: "never",
          labels: { [label]: state.nonce },
          user: "root",
          command: finiteCommand,
          network_mode: "none",
          read_only: false,
          cpus: 0.25,
          mem_limit: "128m",
          pids_limit: 64,
          cap_drop: ["ALL"],
          security_opt: ["no-new-privileges:true"],
          restart: "no",
          volumes: [{ type: "bind", source: workspace, target: "/workspace" }],
        },
      },
    },
  };
}
function client(ctx, args, options = {}) {
  return ctx.execute(ctx.state.wrapper, args, {
    timeout: 30000,
    extraEnv: cleared,
    ...options,
  });
}
function read(ctx, args) {
  return client(ctx, args).stdout.trim();
}
function image(ctx, target) {
  return JSON.parse(read(ctx, ["image", "inspect", target]))[0];
}
function checkpoint(ctx, mode, stage) {
  ctx.state.privateDevcontainerComposeProgress = { mode, stage };
  ctx.save();
}
function failUnless(value, code) {
  if (!value)
    throw Object.assign(
      Error(
        "Dev Containers Compose proof condition failed; see minimized receipt",
      ),
      { code },
    );
}

export function generatedDockerfilePath(stderr) {
  assert.ok(stderr.length <= 8 << 20);
  const paths = [];
  for (const line of stderr.split(/\r?\n/)) {
    let record;
    try {
      record = JSON.parse(line);
    } catch {
      continue;
    }
    if (
      typeof record.text !== "string" ||
      !record.text.startsWith(
        "Docker Compose override file for building image:\n",
      )
    )
      continue;
    const matches = [
      ...record.text.matchAll(/^\s+dockerfile:\s+([^\r\n]+)\s*$/gm),
    ];
    assert.equal(matches.length, 1);
    let path = matches[0][1].trim();
    assert.ok(!/["'\0]/.test(path), "Unsupported generated path quoting");
    assert.ok(isAbsolute(path));
    path = resolve(path);
    assert.equal(basename(path), "Dockerfile-with-features");
    paths.push(path);
  }
  assert.equal(
    new Set(paths).size,
    1,
    "Missing/ambiguous actual generated Dockerfile event",
  );
  return paths[0];
}
export function defaultComposeCommandEvidence(
  stderr,
  wrapper,
  nonce,
  negative = false,
) {
  assert.match(nonce, /^[a-f0-9]{32}$/);
  const prefixes = [`Run: ${wrapper} compose `, `Run: "${wrapper}" compose `];
  const commands = [];
  for (const line of stderr.split(/\r?\n/)) {
    let record;
    try {
      record = JSON.parse(line);
    } catch {
      continue;
    }
    if (
      record.type !== "start" ||
      typeof record.text !== "string" ||
      !record.text.includes(nonce) ||
      !prefixes.some((prefix) => record.text.startsWith(prefix))
    )
      continue;
    commands.push(record.text);
  }
  const buildCommands = commands.filter((text) =>
    /(?:^| )build(?: |$)/.test(text),
  ).length;
  const upCommands = commands.filter((text) =>
    /(?:^| )up(?: |$)/.test(text),
  ).length;
  assert.ok(
    buildCommands >= 1,
    "Actual alias/default-plugin Compose build invocation not observed",
  );
  if (!negative)
    assert.ok(
      upCommands >= 1,
      "Actual alias/default-plugin Compose up invocation not observed",
    );
  return {
    defaultPlugin: true,
    buildCommands,
    upCommands,
    runtimeGrantArgvObserved: false,
  };
}
// Only exact selected-alias/source-image events can establish an early refusal.
// Persist counts/booleans, never arbitrary CLI text, paths or registry output.
export function negativeDevcontainerCommandEvidence(stderr, wrapper, state) {
  assert.ok(typeof stderr === "string" && stderr.length <= 8 << 20);
  assert.match(state.nonce, /^[a-f0-9]{32}$/);
  assert.equal(state.image, `${state.server}/fixture/image:${state.nonce}`);
  const aliases = [wrapper, `"${wrapper}"`],
    sourceArgs = [
      `pull ${state.image}`,
      `inspect --type image ${state.image}`,
      `image inspect ${state.image}`,
    ];
  const evidence = {
    jsonEvents: 0,
    unparsedLines: 0,
    startEvents: 0,
    aliasStartEvents: 0,
    sourcePullCommands: 0,
    sourceInspectCommands: 0,
    sourceFailedCommands: 0,
    sourceFailedPullCommands: 0,
    authRefusalEvents: 0,
    composeBuildCommands: 0,
    composeUpCommands: 0,
    generatedOverrideEvents: 0,
  };
  for (const line of stderr.split(/\r?\n/).filter(Boolean)) {
    let record;
    try {
      record = JSON.parse(line);
    } catch {
      evidence.unparsedLines++;
      continue;
    }
    evidence.jsonEvents++;
    if (typeof record?.text !== "string") continue;
    const text = record.text;
    if (
      /unauthorized|authentication required|no basic auth credentials|unexpected status.*401/i.test(
        text,
      )
    )
      evidence.authRefusalEvents++;
    if (text.startsWith("Docker Compose override file for building image:\n"))
      evidence.generatedOverrideEvents++;
    if (
      aliases.some((alias) =>
        sourceArgs.some((args) => text === `Command failed: ${alias} ${args}`),
      )
    )
      evidence.sourceFailedCommands++;
    if (
      aliases.some(
        (alias) => text === `Command failed: ${alias} ${sourceArgs[0]}`,
      )
    )
      evidence.sourceFailedPullCommands++;
    if (record.type !== "start") continue;
    evidence.startEvents++;
    const alias = aliases.find((candidate) =>
      text.startsWith(`Run: ${candidate} `),
    );
    if (!alias) continue;
    evidence.aliasStartEvents++;
    const args = text.slice(`Run: ${alias} `.length);
    if (args === sourceArgs[0]) evidence.sourcePullCommands++;
    if (sourceArgs.slice(1).includes(args)) evidence.sourceInspectCommands++;
    if (args.startsWith("compose ") && args.includes(state.nonce)) {
      if (/(?:^| )build(?: |$)/.test(args)) evidence.composeBuildCommands++;
      if (/(?:^| )up(?: |$)/.test(args)) evidence.composeUpCommands++;
    }
  }
  return evidence;
}
export function classifyNegativeDevcontainerRefusal(evidence, status) {
  failUnless(
    Number.isInteger(status) && status !== 0,
    "DC_NEGATIVE_UP_UNEXPECTED_SUCCESS",
  );
  failUnless(
    evidence.authRefusalEvents > 0,
    "DC_NEGATIVE_AUTH_REFUSAL_NOT_OBSERVED",
  );
  if (evidence.composeBuildCommands > 0)
    return { phase: "compose-build-auth-refusal", composeBuildObserved: true };
  failUnless(
    evidence.sourcePullCommands > 0 &&
      evidence.sourceFailedPullCommands > 0 &&
      evidence.composeUpCommands === 0,
    "DC_NEGATIVE_SOURCE_REFUSAL_NOT_OBSERVED",
  );
  return {
    phase: "prebuild-private-source-auth-refusal",
    composeBuildObserved: false,
  };
}
function safeAncestors(path) {
  for (const directory of metadataDirectoryChain(path)) {
    const info = lstatSync(directory);
    assert.ok(
      info.isDirectory() && !info.isSymbolicLink(),
      "Generated path directory link refused",
    );
  }
}
function under(root, path) {
  const part = relative(root, path);
  return (
    part !== "" &&
    !part.startsWith(`..${sep}`) &&
    part !== ".." &&
    !isAbsolute(part)
  );
}
function readGenerated(path) {
  safeAncestors(dirname(path));
  const original = lstatSync(path);
  assert.ok(
    original.isFile() && !original.isSymbolicLink() && original.size <= 262144,
  );
  const fd = openSync(path, "r");
  try {
    const opened = fstatSync(fd);
    assert.ok(
      opened.isFile() &&
        opened.dev === original.dev &&
        opened.ino === original.ino &&
        opened.size <= 262144,
    );
    const bytes = Buffer.alloc(262145);
    let length = 0,
      count;
    while (
      length < bytes.length &&
      (count = readSync(fd, bytes, length, bytes.length - length, null)) > 0
    )
      length += count;
    assert.ok(length <= 262144);
    const after = lstatSync(path),
      final = fstatSync(fd);
    assert.ok(
      !after.isSymbolicLink() &&
        after.dev === opened.dev &&
        after.ino === opened.ino &&
        after.size === original.size &&
        after.mtimeMs === original.mtimeMs &&
        final.size === original.size &&
        final.mtimeMs === original.mtimeMs,
    );
    safeAncestors(dirname(path));
    return bytes.subarray(0, length);
  } finally {
    closeSync(fd);
  }
}
export function verifyGeneratedDockerfile(text, plan, state) {
  const normalized = text.replaceAll("\r\n", "\n");
  const from = [...normalized.matchAll(/^\s*FROM\s+(.+)$/gim)].map((match) =>
    match[1].trim(),
  );
  assert.deepEqual(from, [
    `${state.image} AS dev_container_auto_added_stage_label`,
    "$_DEV_CONTAINERS_BASE_IMAGE AS dev_containers_target_stage",
  ]);
  const expected = plan.dockerfile.replace(
    `FROM ${state.image}\n`,
    `FROM ${state.image} AS dev_container_auto_added_stage_label\n`,
  );
  assert.ok(normalized.includes(expected));
  assert.equal([...normalized.matchAll(/^\s*RUN\s+/gim)].length, 1);
  assert.deepEqual(
    [...normalized.matchAll(/^\s*([a-z]+)\s+/gim)].map((match) =>
      match[1].toUpperCase(),
    ),
    ["ARG", "FROM", "LABEL", "RUN", "CMD", "FROM", "LABEL"],
  );
  assert.ok(
    normalized
      .trimStart()
      .startsWith("ARG _DEV_CONTAINERS_BASE_IMAGE=scratch\n"),
  );
  assert.ok(!/^\s*(?:ADD|COPY|ONBUILD|HEALTHCHECK)\s+/im.test(normalized));
  assert.ok(!/^\s*#\s*syntax=/im.test(normalized));
}
function observeGenerated(ctx, plan, stderr) {
  const path = generatedDockerfilePath(stderr),
    context = resolve(plan.workspace, ".devcontainer/build");
  assert.ok(
    under(resolve(tmpdir(), "devcontainercli/container-features"), path),
    "Generated file is outside the ordinary Dev Containers temporary subtree",
  );
  assert.ok(
    !under(context, path) && path !== context,
    "Generated Dockerfile must be outside its context",
  );
  assert.equal(realpathSync(path).toLowerCase(), path.toLowerCase());
  const bytes = readGenerated(path);
  verifyGeneratedDockerfile(bytes.toString("utf8"), plan, ctx.state);
  ctx.state.privateDevcontainerCompose.generatedFiles.push({
    mode: plan.mode,
    path,
    sha256: digest(bytes),
    bytes: bytes.length,
    outsideContext: true,
    evidence:
      "Actual Dev Containers build-override event; patched default Compose source + successful build imply selected regular-file read grant. Runtime grant argv not observed.",
  });
  ctx.save();
}
function prepare(ctx, plan) {
  mkdirSync(resolve(plan.workspace, ".devcontainer/build"), {
    recursive: true,
  });
  for (const [path, text] of [
    [".devcontainer/devcontainer.json", JSON.stringify(plan.config, null, 2)],
    [".devcontainer/compose.json", JSON.stringify(plan.compose, null, 2)],
    [".devcontainer/build/Dockerfile", plan.dockerfile],
    ["seed.txt", plan.marker],
  ])
    writeFileSync(resolve(plan.workspace, path), text, { flag: "wx" });
  ctx.state.privateDevcontainerCompose.sources.push({
    mode: plan.mode,
    dockerfileSha256: digest(plan.dockerfile),
    composeSha256: digest(JSON.stringify(plan.compose, null, 2)),
    configSha256: digest(JSON.stringify(plan.config, null, 2)),
  });
  ctx.save();
}
function dev(ctx, plan, action, extra = [], allowedFailure = false) {
  assert.equal(digest(readFileSync(ctx.state.devCli)), ctx.state.devCliSha256);
  return ctx.execute(
    process.execPath,
    [
      ctx.state.devCli,
      action,
      "--docker-path",
      ctx.state.wrapper,
      "--workspace-folder",
      plan.workspace,
      "--mount-workspace-git-root",
      "false",
      "--user-data-folder",
      resolve(plan.workspace, "client-data"),
      "--default-user-env-probe",
      "none",
      "--log-level",
      "trace",
      "--log-format",
      "json",
      ...extra,
    ],
    {
      timeout:
        action === "exec"
          ? 20000
          : extra.includes("--expect-existing-container")
            ? 45000
            : 120000,
      cwd: plan.workspace,
      allowedFailure,
      extraEnv: {
        ...cleared,
        DOCKER_CONFIG: ctx.state.privateConfig,
        PATH:
          resolve(dirname(ctx.state.wrapper), "runtime/backend") +
          ";" +
          process.env.PATH,
      },
    },
  );
}
function observe(ctx, phase) {
  const output = client(
    ctx,
    ["logs", "--tail=2000", "--timestamps", ctx.state.registryContainer],
    { timeout: 2000 },
  );
  const text = (output.stdout ?? "") + (output.stderr ?? "");
  const counts = manifestRequests(
    text,
    ctx.state.nonce,
    ctx.state.registryManifestDigest,
  );
  const records = registryRequestObservations(
    text,
    ctx.state.nonce,
    ctx.state.registryManifestDigest,
  );
  ctx.state.privateDevcontainerCompose.observations.push({
    phase,
    counts,
    sanitizedRequests: records,
  });
  ctx.save();
  failUnless(
    text.split(/\r?\n/).length < 2000,
    "DC_MANIFEST_LOG_WINDOW_SATURATED",
  );
  return counts;
}
async function afterBuild(ctx, before, negative) {
  let after;
  for (let attempt = 0; attempt < 5; attempt++) {
    after = observe(
      ctx,
      `after-${negative ? "negative" : "positive"}-up-${attempt + 1}`,
    );
    if (
      negative ? after.refused > before.refused : after.success > before.success
    )
      break;
    if (attempt < 4) await new Promise((resolve) => setTimeout(resolve, 150));
  }
  return after;
}
export function ownedDevcontainerComposeImage(value, state, plan) {
  assert.match(value.Id, imageId);
  assert.ok(!state.before.inventories.owned.images.includes(value.Id));
  assert.equal(value.Config?.Labels?.[label], state.nonce);
  assert.equal(value.Config?.Labels?.[markerLabel], plan.marker);
  assert.ok((value.RepoTags ?? []).every((tag) => tag === plan.tag));
  assert.equal(value.Os, "linux");
  assert.equal(value.Architecture, "amd64");
  assert.deepEqual(value.Config.Cmd, finiteCommand);
  assert.ok(!value.Config.Entrypoint || value.Config.Entrypoint.length === 0);
  assert.equal(Object.keys(value.Config.Volumes ?? {}).length, 0);
  const layers = value.RootFS?.Layers,
    base = state.committedRootFS.Layers;
  assert.ok(Array.isArray(layers) && layers.length === base.length + 1);
  assert.deepEqual(layers.slice(0, base.length), base);
  return value.Id;
}
// Dev Containers may pull the base into the engine store while inspecting it.
// Only this newly committed nonce fixture can be removed before Forget.
export function ownedDevcontainerComposeBase(value, state) {
  assert.match(state.committedId, imageId);
  assert.equal(value.Id, state.committedId);
  assert.ok(!state.before.inventories.owned.images.includes(value.Id));
  assert.equal(value.Config?.Labels?.[label], state.nonce);
  assert.deepEqual(value.RepoTags, [state.image]);
  assert.deepEqual(value.Config?.Cmd, finiteCommand);
  assert.ok(!value.Config?.Entrypoint || value.Config.Entrypoint.length === 0);
  assert.equal(Object.keys(value.Config?.Volumes ?? {}).length, 0);
  assert.deepEqual(value.RootFS, state.committedRootFS);
  assert.equal(value.Os, "linux");
  assert.equal(value.Architecture, "amd64");
  return value.Id;
}
function linuxPath(path) {
  assert.match(path, /^[A-Za-z]:\\/);
  return `/mnt/${path[0].toLowerCase()}/${path.slice(3).replaceAll("\\", "/")}`;
}
export function ownedDevcontainerComposeContainer(value, built, state, plan) {
  assert.match(value.Id, fullId);
  assert.ok(!state.before.inventories.owned.containers.includes(value.Id));
  assert.equal(value.Name, `/${plan.name}`);
  assert.equal(value.Image, ownedDevcontainerComposeImage(built, state, plan));
  assert.equal(value.Config?.Image, plan.tag);
  assert.equal(value.Config?.Labels?.[label], state.nonce);
  assert.deepEqual(value.Config.Cmd, finiteCommand);
  const entry = value.Config.Entrypoint;
  assert.ok(
    Array.isArray(entry) &&
      entry.length === 4 &&
      entry[0] === "/bin/sh" &&
      entry[1] === "-c" &&
      entry[3] === "-",
  );
  // The wrapper's fallback is unreachable after successful exec of a verified
  // nonempty BusyBox timeout command. Reject any different setup script.
  assert.equal(
    entry[2].replace(/\s+/g, " ").trim(),
    'echo Container started trap "exit 0" 15 exec "$@" while sleep 1 & wait $!; do :; done',
  );
  assert.equal(value.HostConfig?.NetworkMode, "none");
  assert.equal(value.HostConfig?.Privileged, false);
  assert.equal(value.HostConfig?.ReadonlyRootfs, false);
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
  assert.equal(value.State?.Paused, false);
  assert.ok(Array.isArray(value.Mounts) && value.Mounts.length === 1);
  const mount = value.Mounts[0];
  assert.equal(mount.Type, "bind");
  assert.equal(mount.Destination, "/workspace");
  assert.equal(mount.RW, true);
  assert.ok(
    mount.Source === linuxPath(plan.workspace) ||
      (typeof mount.Source === "string" &&
        resolve(mount.Source).toLowerCase() === plan.workspace.toLowerCase()),
  );
  return value.Id;
}
function finishMetadata(ctx) {
  const receipt = ctx.state.privateDevcontainerCompose;
  const after = preflightPrivateCompose(ctx);
  receipt.after = after;
  assert.deepEqual(after.selectedPlugins, receipt.before.selectedPlugins);
  assert.deepEqual(after.metadata.selection, receipt.before.metadata.selection);
  for (const group of ["defaults", "instances"])
    assert.deepEqual(
      after.metadata.entries.filter((value) => value.group === group),
      receipt.before.metadata.entries.filter((value) => value.group === group),
    );
  receipt.retainedResidue =
    "BuildKit cache/history and product Buildx refs/activity remain; no prune or byte-delta claim. Source contexts and generated-file receipts retained.";
  ctx.save();
}
export async function runPrivateDevcontainerCompose(ctx, mode) {
  assert.ok(["authenticated", "forgotten"].includes(mode));
  const state = ctx.state,
    negative = mode === "forgotten",
    plan = devcontainerComposePlan(state, negative);
  try {
    checkpoint(ctx, mode, "preflight");
    if (!negative) {
      assert.equal(state.privateDevcontainerCompose, undefined);
      const before = preflightPrivateCompose(ctx);
      const provenance = JSON.parse(
        readFileSync(
          resolve(ctx.root, "artifacts/compose/compose-manifest.json"),
        ),
      );
      const patchSha256 = digest(
        readFileSync(
          resolve(
            ctx.root,
            "patches/compose/0001-explicit-local-dockerfile-read.patch",
          ),
        ),
      );
      assert.equal(provenance.patchSha256, patchSha256);
      assert.equal(
        before.selectedPlugins.find((plugin) => plugin.name === "compose")
          .sha256,
        provenance.binarySha256,
      );
      state.privateDevcontainerCompose = {
        before,
        patchSha256,
        developerCliSha256: state.devCliSha256,
        temporaryOverrides: false,
        forcedStandaloneAdapter: false,
        sources: [],
        generatedFiles: [],
        observations: [],
        imageIds: [],
        outputTags: [],
        workspaces: [],
      };
    }
    const receipt = state.privateDevcontainerCompose;
    assert.ok(receipt);
    assert.equal(
      read(ctx, ["image", "ls", "--filter", `reference=${state.image}`, "-q"]),
      "",
    );
    assert.equal(
      read(ctx, ["image", "ls", "--filter", `reference=${plan.tag}`, "-q"]),
      "",
    );
    prepare(ctx, plan);
    receipt.workspaces.push(plan.workspace);
    receipt.outputTags.push(plan.tag);
    ctx.save();
    ctx.intent("image", plan.tag);
    ctx.intent("container", plan.name);
    ctx.intent("buildkit-cache", plan.marker);
    const before = observe(ctx, `before-${plan.mode}-up`);
    checkpoint(ctx, mode, "actual-devcontainers-up");
    const result = dev(
      ctx,
      plan,
      "up",
      [
        "--build-no-cache",
        "--update-remote-user-uid-default",
        "never",
        "--skip-post-create",
      ],
      true,
    );
    const after = await afterBuild(ctx, before, negative);
    receipt.requests ??= [];
    receipt.requests.push({ mode, before, after });
    ctx.save();
    receipt.commandEvidence ??= [];
    if (negative) {
      checkpoint(ctx, mode, "negative-refusal");
      const events = negativeDevcontainerCommandEvidence(
        result.stderr ?? "",
        state.wrapper,
        state,
      );
      // Save minimized diagnostics before any assertion so an unexpected actual
      // CLI format can be reviewed without retaining arbitrary child output.
      const commandEvidence = { mode, events, childStatus: result.status };
      receipt.commandEvidence.push(commandEvidence);
      ctx.save();
      assert.equal(result.error, undefined);
      const classification = classifyNegativeDevcontainerRefusal(
        events,
        result.status,
      );
      Object.assign(commandEvidence, classification);
      ctx.save();
      if (classification.composeBuildObserved) {
        const commands = defaultComposeCommandEvidence(
          result.stderr ?? "",
          state.wrapper,
          state.nonce,
          true,
        );
        Object.assign(commandEvidence, {
          buildCommands: commands.buildCommands,
          upCommands: commands.upCommands,
          runtimeGrantArgvObserved: false,
        });
        ctx.save();
        observeGenerated(ctx, plan, result.stderr ?? "");
      }
      failUnless(
        after.refused > before.refused && after.success === before.success,
        "DC_NO_MANIFEST_REFUSAL_EVIDENCE",
      );
      assert.equal(
        read(ctx, [
          "ps",
          "-aq",
          "--no-trunc",
          "--filter",
          `name=^/${plan.name}$`,
        ]),
        "",
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
        classification.composeBuildObserved
          ? "actual Dev Containers Compose private FROM build refused after native Forget; new manifest401/no200 observed"
          : "actual Dev Containers private source pull/inspection refused before Compose build after native Forget; new manifest401/no200 observed; negative Compose build not exercised",
      );
    } else {
      assert.equal(result.error, undefined);
      checkpoint(ctx, mode, "generated-source-verification");
      receipt.commandEvidence.push({
        mode,
        ...defaultComposeCommandEvidence(
          result.stderr ?? "",
          state.wrapper,
          state.nonce,
        ),
      });
      ctx.save();
      observeGenerated(ctx, plan, result.stderr ?? "");
      checkpoint(ctx, mode, "positive-generated-source");
      failUnless(result.status === 0, "DC_POSITIVE_UP_FAILED");
      failUnless(
        after.success > before.success,
        "DC_NO_MANIFEST_SUCCESS_EVIDENCE",
      );
      const resultValue = JSON.parse(
        result.stdout.trim().split(/\r?\n/).at(-1),
      );
      assert.match(resultValue.containerId ?? "", fullId);
      const value = JSON.parse(
          read(ctx, ["container", "inspect", resultValue.containerId]),
        )[0],
        built = image(ctx, value.Image);
      receipt.containerId = ownedDevcontainerComposeContainer(
        value,
        built,
        state,
        plan,
      );
      receipt.imageIds.push(ownedDevcontainerComposeImage(built, state, plan));
      ctx.save();
      checkpoint(ctx, mode, "workspace-roundtrip");
      dev(ctx, plan, "exec", [
        "--container-id",
        value.Id,
        "/bin/busybox",
        "timeout",
        "-s",
        "KILL",
        "5",
        "/bin/sh",
        "-c",
        `test "$(cat /orbwin-devcontainer-compose-proof)" = '${plan.marker}' && test "$(cat /workspace/seed.txt)" = '${plan.marker}' && printf '%s' '${plan.marker}-container' > /workspace/from-container.txt`,
      ]);
      assert.equal(
        readFileSync(resolve(plan.workspace, "from-container.txt"), "utf8"),
        `${plan.marker}-container`,
      );
      writeFileSync(
        resolve(plan.workspace, "from-host.txt"),
        `${plan.marker}-host`,
        { flag: "wx" },
      );
      dev(ctx, plan, "exec", [
        "--container-id",
        value.Id,
        "/bin/busybox",
        "timeout",
        "-s",
        "KILL",
        "5",
        "/bin/sh",
        "-c",
        `test "$(cat /workspace/from-host.txt)" = '${plan.marker}-host'`,
      ]);
      checkpoint(ctx, mode, "reopen-same-container");
      const reopened = dev(ctx, plan, "up", [
        "--expect-existing-container",
        "--update-remote-user-uid-default",
        "never",
        "--skip-post-create",
      ]);
      assert.equal(
        JSON.parse(reopened.stdout.trim().split(/\r?\n/).at(-1)).containerId,
        value.Id,
      );
      ownedDevcontainerComposeContainer(
        JSON.parse(read(ctx, ["container", "inspect", value.Id]))[0],
        image(ctx, value.Image),
        state,
        plan,
      );
      const cleanupPositive = () => {
        cleanupPrivateDevcontainerCompose(ctx, false);
        if (
          read(ctx, [
            "image",
            "ls",
            "--filter",
            `reference=${state.image}`,
            "-q",
          ])
        ) {
          const id = ownedDevcontainerComposeBase(
            image(ctx, state.image),
            state,
          );
          client(ctx, ["image", "rm", id]);
          receipt.baseReferenceRemoved = id;
          ctx.save();
        }
        assert.equal(
          read(ctx, [
            "image",
            "ls",
            "--filter",
            `reference=${state.image}`,
            "-q",
          ]),
          "",
        );
      };
      if (ctx.withCleanupBudget) ctx.withCleanupBudget(cleanupPositive);
      else cleanupPositive();
      state.steps.push(
        "actual Dev Containers default Compose private FROM generated outside-context Dockerfile build/up/marker/workspace writes/reopen passed; manifest200 observed",
      );
    }
    finishMetadata(ctx);
    checkpoint(ctx, mode, "complete");
  } catch (error) {
    const source = String(error?.stack ?? "").match(
      /private-devcontainer-compose-stage\.mjs:(\d+):(\d+)/,
    );
    state.privateDevcontainerComposeFailure = {
      ...state.privateDevcontainerComposeProgress,
      code:
        typeof error?.code === "string" &&
        /^[A-Z][A-Z0-9_]{0,63}$/.test(error.code)
          ? error.code
          : "DC_STAGE_FAILED",
      source: source
        ? {
            file: "private-devcontainer-compose-stage.mjs",
            line: Number(source[1]),
            column: Number(source[2]),
          }
        : undefined,
    };
    ctx.save();
    if (ctx.withCleanupBudget && state.privateDevcontainerCompose) {
      try {
        ctx.withCleanupBudget(() =>
          cleanupPrivateDevcontainerCompose(ctx, false),
        );
      } catch (cleanupError) {
        state.privateDevcontainerComposeFailure.cleanup = {
          code:
            typeof cleanupError?.code === "string" &&
            /^[A-Z][A-Z0-9_]{0,63}$/.test(cleanupError.code)
              ? cleanupError.code
              : "DC_PARTIAL_CLEANUP_REFUSED",
        };
        ctx.save();
      }
    }
    throw Error(
      "Dev Containers Compose stage failed; inspect minimized stage receipt",
    );
  }
}
export function cleanupPrivateDevcontainerCompose(ctx, final = true) {
  const state = ctx.state,
    receipt = state.privateDevcontainerCompose;
  if (!receipt) return;
  for (const negative of [false, true]) {
    const plan = devcontainerComposePlan(state, negative);
    const ids = read(ctx, [
      "ps",
      "-aq",
      "--no-trunc",
      "--filter",
      `name=^/${plan.name}$`,
    ])
      .split(/\r?\n/)
      .filter(Boolean);
    assert.ok(ids.length <= 1);
    for (const id of ids) {
      assert.ok(
        state.intents.some(
          (intent) => intent.kind === "container" && intent.name === plan.name,
        ),
      );
      let value = JSON.parse(read(ctx, ["container", "inspect", id]))[0],
        built = image(ctx, value.Image);
      ownedDevcontainerComposeContainer(value, built, state, plan);
      if (!receipt.imageIds.includes(value.Image)) {
        receipt.imageIds.push(value.Image);
        ctx.save();
      }
      if (value.State.Running) client(ctx, ["stop", "--time", "5", id]);
      value = JSON.parse(read(ctx, ["container", "inspect", id]))[0];
      ownedDevcontainerComposeContainer(
        value,
        image(ctx, value.Image),
        state,
        plan,
      );
      client(ctx, ["rm", id]);
    }
    const images = read(ctx, [
      "image",
      "ls",
      "--filter",
      `reference=${plan.tag}`,
      "-q",
      "--no-trunc",
    ])
      .split(/\r?\n/)
      .filter(Boolean);
    for (const id of new Set([...images, ...receipt.imageIds])) {
      if (!images.includes(id) && !receipt.imageIds.includes(id)) continue;
      const candidate = client(ctx, ["image", "inspect", id], {
        allowedFailure: true,
      });
      if (candidate.error) throw candidate.error;
      if (candidate.status !== 0) {
        failUnless(
          /No such image/i.test(candidate.stderr ?? ""),
          "DC_IMAGE_INSPECT_UNKNOWN_FAILURE",
        );
        continue;
      }
      const built = JSON.parse(candidate.stdout)[0];
      if (built.Config?.Labels?.[markerLabel] !== plan.marker) {
        failUnless(!images.includes(id), "DC_OUTPUT_IMAGE_OWNERSHIP_CHANGED");
        continue;
      }
      assert.ok(receipt.outputTags.includes(plan.tag));
      ownedDevcontainerComposeImage(built, state, plan);
      client(ctx, ["image", "rm", built.Id]);
    }
  }
  if (final) finishMetadata(ctx);
}
