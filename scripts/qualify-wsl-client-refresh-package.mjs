// Source-only until root reviews exact binary/attestation/nonce and runtime lane.
// Import/default plan performs no I/O. Never restores config or removes evidence.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";
import { packageTauriConfiguration } from "./package-input-config.mjs";
import {
  stableBytes,
  configFingerprint,
  canonicalEngineSnapshot,
  originalTreeReader,
  expectedIDs,
} from "./qualify-migration-draft.mjs";

const root = fileURLToPath(new URL("../", import.meta.url));
const sha = (bytes) => createHash("sha256").update(bytes).digest("hex");
// This wrapper qualifies current product state, never a historical debug proof.
// Clone at the final spawn boundary so per-step environment overrides cannot
// accidentally restore a case variant; never mutate the caller's environment.
export function productChildEnvironment(inherited) {
  const child = { ...inherited };
  for (const key of Object.keys(child))
    if (key.toUpperCase() === "ORBWIN_QUALIFICATION_IDENTITY") delete child[key];
  return child;
}
// These strings are observations from the same attested native test PE. They
// constrain baselines and are never an identity, directory lease or path grant.
export function nativeRootsContract(value, nonce) {
  assert.match(nonce, /^[a-f0-9]{32}$/);
  assert.ok(value && typeof value === "object" && !Array.isArray(value));
  assert.deepEqual(Object.keys(value).sort(), ["localAppData", "nonce", "profile", "schema"]);
  assert.equal(value.schema, 1);
  assert.equal(value.nonce, nonce);
  for (const key of ["localAppData", "profile"]) {
    const text = value[key];
    assert.equal(typeof text, "string");
    assert.ok(text.length >= 3 && text.length < 1024);
    assert.ok(Buffer.from(text, "utf8").toString("utf8") === text);
    assert.match(text, /^[a-zA-Z]:\\/);
    assert.ok(!/[\x00-\x1f\x7f-\x9f/]/.test(text));
    const components = text.slice(3).split("\\");
    assert.ok(components.every((part) => part && part !== "." && part !== ".." && !/[. :<>"|?*]$/.test(part) && !/[:<>"|?*]/.test(part)));
    assert.equal(path.win32.normalize(text), text);
  }
  return Object.freeze({ schema: 1, nonce, localAppData: value.localAppData, profile: value.profile });
}
export function nativeRootsPaths(value, nonce) {
  const roots = nativeRootsContract(value, nonce);
  const home = path.win32.join(roots.localAppData, "OrbWinData", "home");
  return Object.freeze({
    home,
    identityPath: path.win32.join(roots.localAppData, "OrbWinData", "runtime.json"),
    privateDirectory: path.win32.join(home, ".rdorbwin", "wsl-docker"),
    privateConfig: path.win32.join(home, ".rdorbwin", "wsl-docker", "config.json"),
    globalDocker: path.win32.join(roots.profile, ".docker", "config.json"),
    globalKube: path.win32.join(roots.profile, ".kube", "config"),
    globalWSL: path.win32.join(roots.profile, ".wslconfig"),
  });
}
/**
 * @param {Record<string, string | undefined>} inherited
 * @param {{nonce: string, roots?: {schema: number, nonce: string, localAppData: string, profile: string}, lockSha?: string}} options
 */
export function nativeQualificationEnvironment(inherited, { nonce, roots, lockSha }) {
  assert.match(nonce, /^[a-f0-9]{32}$/);
  const child = productChildEnvironment(inherited);
  const replaced = new Set([
    "ORBWIN_QUALIFY_WSL_CLIENT_ROOTS", "ORBWIN_QUALIFY_WSL_CLIENT_NONCE",
    "ORBWIN_QUALIFY_WSL_CLIENT_EXPECTED_ROOTS", "ORBWIN_QUALIFY_WSL_CLIENT_REFRESH",
    "ORBWIN_QUALIFY_WSL_CLIENT_OLD_SHA256", "ORBWIN_QUALIFY_WSL_CLIENT_LOCK_SHA256",
  ]);
  for (const key of Object.keys(child)) if (replaced.has(key.toUpperCase())) delete child[key];
  child.ORBWIN_QUALIFY_WSL_CLIENT_NONCE = nonce;
  if (roots === undefined) child.ORBWIN_QUALIFY_WSL_CLIENT_ROOTS = "1";
  else {
    assert.match(lockSha, /^[a-f0-9]{64}$/);
    child.ORBWIN_QUALIFY_WSL_CLIENT_EXPECTED_ROOTS = JSON.stringify(nativeRootsContract(roots, nonce));
    child.ORBWIN_QUALIFY_WSL_CLIENT_REFRESH = "1";
    child.ORBWIN_QUALIFY_WSL_CLIENT_OLD_SHA256 = pins.oldSha;
    child.ORBWIN_QUALIFY_WSL_CLIENT_LOCK_SHA256 = lockSha;
  }
  return child;
}
// The callback is the first permitted private-path read/evidence effect. A
// rejected/unknown probe cannot reach it. Tests inject only inert observations.
export function admitNativeRoots(nonce, probe, admitted) {
  assert.match(nonce, /^[a-f0-9]{32}$/);
  let selected;
  try {
    const result = probe();
    assert.ok(result && !result.error && !result.signal && result.status === 0);
    assert.ok(Buffer.isBuffer(result.stdout) && result.stdout.length <= 65536);
    assert.ok(Buffer.isBuffer(result.stderr) && result.stderr.length === 0);
    const text = result.stdout.toString("utf8");
    assert.ok(Buffer.from(text, "utf8").equals(result.stdout));
    const matches = [...text.matchAll(/^WSL_PRIVATE_CLIENT_ROOTS (\{[^\r\n]*\})$/gm)];
    assert.equal(matches.length, 1);
    const raw = matches[0][1];
    assert.ok(Buffer.byteLength(raw) <= 8192);
    const parsed = JSON.parse(raw);
    assert.equal(JSON.stringify(parsed), raw); // Refuse duplicate/noncanonical JSON.
    const roots = nativeRootsContract(parsed, nonce);
    // A marker in a zero-selected or failed harness run cannot admit roots.
    assert.match(text, /test result: ok\. 1 passed; 0 failed; 0 ignored;/);
    selected = {
      roots, paths: nativeRootsPaths(roots, nonce),
      probe: Object.freeze({ selector: pins.rootsTest, status: result.status,
        stdoutSha256: sha(result.stdout), stderrSha256: sha(result.stderr) }),
      raw: { stdout: result.stdout, stderr: result.stderr },
    };
  } catch {
    throw Error("Native roots observation refused or unknown; no private baseline or refresh was started. Raw content withheld.");
  }
  return admitted(selected);
}
const safeJson = (text) => {
  try {
    return JSON.parse(text);
  } catch {
    throw Error("Qualification JSON is invalid; content withheld.");
  }
};
export const pins = Object.freeze({
  oldSha: "29177adb30754d125ace81b1d0c0889c2299e56970654973d231cd03771f50ba",
  exactTest: "wsl_clients::qualification::owned_private_client_refresh",
  rootsTest: "wsl_clients::qualification::owned_private_client_roots",
  guid: "{231810b0-fa21-48a8-b0a7-86f9a1360029}",
  source: "/home/vanyr/orbwin-native-proof-20261024",
  signature: "2112:1374236",
});
export function parameters(value) {
  assert.match(value.nonce, /^[a-f0-9]{32}$/);
  for (const key of ["nativeSha", "attestationSha"])
    assert.match(value[key], /^[a-f0-9]{64}$/);
  for (const key of ["native", "attestation"]) {
    assert.ok(path.isAbsolute(value[key]));
    assert.ok(
      path
        .resolve(value[key])
        .startsWith(path.resolve(root, "artifacts/qualification") + path.sep),
    );
  }
  assert.equal(path.dirname(value.native), path.dirname(value.attestation));
  assert.notEqual(value.native, value.attestation);
  assert.ok(path.isAbsolute(value.packageInput));
  assert.equal(path.resolve(value.packageInput), path.dirname(value.native));
  assert.match(value.expectedPluginDirectory, /^[a-zA-Z]:[\\/]/);
  assert.ok(!/[\x00-\x1f]/.test(value.expectedPluginDirectory));
  assert.ok(!value.expectedPluginDirectory.replaceAll("\\", "/").split("/").some(p => p === "." || p === ".."));
  return value;
}
export function windowsLinux(file) {
  assert.match(file, /^[a-zA-Z]:[\\/]/);
  assert.ok(!file.includes("\0"));
  return (
    "/mnt/" + file[0].toLowerCase() + "/" + file.slice(3).replaceAll("\\", "/")
  );
}

// Locate raw top-level plugin bytes, never parse/re-encode unrelated numbers or
// secret values. Native strict JSON/duplicate validation remains authoritative.
export function pluginParts(bytes) {
  const text = bytes.toString("utf8");
  let at = 0;
  const spaces = () => {
    while (/[ \t\r\n]/.test(text[at] ?? "x")) at++;
  };
  const stringEnd = () => {
    assert.equal(text[at++], '"');
    while (at < text.length) {
      if (text[at] === "\\") {
        at += 2;
        continue;
      }
      if (text[at++] === '"') return at;
    }
    throw Error("Unterminated config string");
  };
  const valueEnd = () => {
    const open = text[at];
    if (open === '"') return stringEnd();
    if (open === "{" || open === "[") {
      const stack = [open === "{" ? "}" : "]"];
      at++;
      while (stack.length && at < text.length) {
        const ch = text[at];
        if (ch === '"') {
          stringEnd();
          continue;
        }
        if (ch === "{" || ch === "[") stack.push(ch === "{" ? "}" : "]");
        if (ch === "}" || ch === "]") assert.equal(ch, stack.pop());
        at++;
      }
      assert.equal(stack.length, 0);
      return at;
    }
    while (at < text.length && !/[ \t\r\n,}]/.test(text[at])) at++;
    return at;
  };
  spaces();
  assert.equal(text[at++], "{");
  const fields = new Set();
  let found;
  for (;;) {
    spaces();
    if (text[at] === "}") break;
    const keyStart = at,
      keyEnd = stringEnd(),
      key = safeJson(text.slice(keyStart, keyEnd));
    assert.ok(!fields.has(key), "duplicate top-level config key");
    fields.add(key);
    spaces();
    assert.equal(text[at++], ":");
    spaces();
    const start = at,
      end = valueEnd();
    if (key === "cliPluginsExtraDirs")
      found = {
        prefix: text.slice(0, start),
        value: safeJson(text.slice(start, end)),
        suffix: text.slice(end),
      };
    spaces();
    if (text[at] === "}") break;
    assert.equal(text[at++], ",");
  }
  assert.ok(found, "existing plugin field required by reviewed setup");
  at++;
  spaces();
  assert.equal(at, text.length);
  // UTF-8 validity must be exact; decoding replacement characters is refused.
  assert.ok(
    Buffer.from(text).equals(bytes),
    "Private config is not exact UTF-8; content withheld.",
  );
  return found;
}
export function assertPlannedEdit(before, after, expectedDirectory) {
  const a = pluginParts(before),
    b = pluginParts(after);
  assert.ok(
    a.prefix === b.prefix,
    "non-plugin prefix changed; content withheld",
  );
  assert.ok(
    a.suffix === b.suffix,
    "non-plugin suffix changed; content withheld",
  );
  assert.ok(
    Array.isArray(b.value) &&
      b.value.length === 1 &&
      b.value[0] === expectedDirectory,
    "Updated plugin directory differs; content withheld",
  );
  assert.ok(
    JSON.stringify(a.value) !== JSON.stringify(b.value),
    "expected real directory refresh",
  );
}
export function assertBackupDelta(before, after, oldSha) {
  const old = new Map(before.map((item) => [item.name, item]));
  const added = after.filter((item) => !old.has(item.name));
  assert.equal(added.length, 1, "only one new original-byte backup is allowed");
  const backup = added[0];
  assert.match(
    backup.name,
    /^\.orbwin-client-refresh-\d+-\d+-\d+\.original\.json$/,
  );
  assert.equal(backup.kind, "file");
  assert.equal(backup.sha256, oldSha);
  assert.ok(
    !after.some((item) => item.name.endsWith(".partial")),
    "temporary residue requires manual review",
  );
  assert.deepEqual(
    after.filter(
      (item) => item.name !== backup.name && item.name !== "config.json",
    ),
    before.filter((item) => item.name !== "config.json"),
  );
  assert.ok(
    after.some((item) => item.name === "config.json" && item.kind === "file"),
  );
  return backup;
}

const requiredSources = [
  "runtime-tools.lock.json",
  "src-tauri/Cargo.toml",
  "src-tauri/Cargo.lock",
  "src-tauri/src/main.rs",
  "src-tauri/src/engine.rs",
  "src-tauri/src/runtime.rs",
  "src-tauri/src/registry.rs",
  "src-tauri/src/wsl_integration.rs",
  "src-tauri/src/wsl_clients.rs",
  "src-tauri/src/wsl_clients_security.rs",
  "src-tauri/src/wsl_clients_qualification.rs",
  "src-tauri/build.rs",
  "src-tauri/src/runtime_cache.rs",
  "src-tauri/src/client_config.rs",
  "src-tauri/src/wsl_tools.rs",
  "scripts/package-input-config.mjs",
  "scripts/package-contract.mjs",
  "scripts/build-identity.mjs",
  "scripts/qualify-wsl-client-refresh-package.mjs",
];
export function attestationContract(value, native, nativeSha, lockSha, packageManifestSha) {
  assert.match(lockSha, /^[a-f0-9]{64}$/);
  assert.match(packageManifestSha, /^[a-f0-9]{64}$/);
  assert.equal(value.packageManifestSha256, packageManifestSha);
  assert.equal(value.schema, 1);
  assert.equal(path.resolve(value.binary), path.resolve(native));
  assert.equal(value.binarySha256, nativeSha);
  assert.equal(value.exactTest, pins.exactTest);
  assert.equal(value.rootsTest, pins.rootsTest);
  assert.equal(value.sources["runtime-tools.lock.json"], lockSha);
  for (const key of requiredSources)
    assert.match(value.sources[key], /^[a-f0-9]{64}$/);
  assert.ok(Object.keys(value.sources).length <= 512);
  for (const [relative, digest] of Object.entries(value.sources)) {
    assert.ok(
      !path.isAbsolute(relative) &&
        !relative.includes("\\") &&
        !relative
          .split("/")
          .some((part) => !part || part === "." || part === ".."),
    );
    assert.match(digest, /^[a-f0-9]{64}$/);
  }
  return value;
}

export function identityContract(value, home, build) {
  assert.equal(value.sourceCommit, build.sourceCommit);
  assert.equal(value.binarySha256, build.binarySha256);
  assert.equal(value.identityVerified, true);
  assert.equal(value.daemonId, expectedIDs.owned);
  assert.equal(
    value.endpoint,
    "npipe:////./pipe/docker-rancher-desktop-orbwin",
  );
  assert.equal(path.resolve(value.home), path.resolve(home));
}

export function expectedDirectory(value, lockSha) {
  assert.match(lockSha, /^[a-f0-9]{64}$/);
  const linux = windowsLinux(value);
  assert.ok(linux.split("/").slice(1).every(p => p && p !== "." && p !== ".."));
  assert.ok(linux.endsWith("/.orbwin-private/runtime/linux-docker-" + lockSha));
  return linux;
}

export function runRefresh(input) {
  const scenarioDeadline = Date.now() + 240000;
  assert.equal(process.platform, "win32");
  for (const key of [
    "ORBWIN_QUALIFY_OWNED",
    "ORBWIN_QUALIFY_WSL_CLIENT_REFRESH_RUN",
  ])
    assert.equal(process.env[key], "1");
  const { nonce, native, nativeSha, attestation, attestationSha } =
    parameters(input);
  const packageInput = path.resolve(input.packageInput);
  const packageManifest = path.join(packageInput, "runtime/package-manifest.json");
  const packageManifestSha = sha(stableBytes(packageManifest, 4 << 20));
  const selected = packageTauriConfiguration(root, packageInput);
  const build = safeJson(stableBytes(path.join(selected.componentInput, "build-manifest.json"), 4 << 20));
  const lockSha = sha(stableBytes(path.join(root, "runtime-tools.lock.json"), 1 << 20));
  const directory = expectedDirectory(input.expectedPluginDirectory, lockSha);
  assert.equal(sha(stableBytes(native, 64 << 20)), nativeSha);
  assert.equal(sha(stableBytes(attestation, 1 << 20)), attestationSha);
  const source = attestationContract(
    JSON.parse(stableBytes(attestation, 1 << 20)),
    native,
    nativeSha,
    lockSha,
    packageManifestSha,
  );
  for (const [relative, expected] of Object.entries(source.sources))
    assert.equal(
      sha(stableBytes(path.join(root, relative), 4 << 20)),
      expected,
      `source changed: ${relative}`,
    );
  const lock = JSON.parse(
    stableBytes(path.join(root, "runtime-tools.lock.json"), 1 << 20),
  );
  const tools = selected.toolsInput;
  for (const tool of lock.tools)
    if (tool.path.startsWith("linux/") || tool.path === "bin/docker.exe")
      assert.equal(
        sha(stableBytes(path.join(tools, tool.path), 128 << 20)),
        tool.sha256,
      );
  const preflight = admitNativeRoots(nonce, () => spawnSync(native, [
    "--exact", pins.rootsTest, "--ignored", "--nocapture", "--test-threads=1",
  ], {
    cwd: root, env: nativeQualificationEnvironment(process.env, { nonce }),
    encoding: null, windowsHide: true, maxBuffer: 65536,
    timeout: Math.min(15000, scenarioDeadline - Date.now()),
  }), (observation) => {
    const { home, identityPath, privateConfig } = observation.paths;
    const identityBytes = stableBytes(identityPath, 1 << 20);
    identityContract(safeJson(identityBytes), home, build);
    const old = stableBytes(privateConfig, 1 << 20);
    assert.equal(sha(old), pins.oldSha);
    assert.notDeepEqual(pluginParts(old).value, [directory]);
    return { ...observation, identityBytes, old };
  });
  const { home, identityPath, privateDirectory, privateConfig } = preflight.paths;
  const { identityBytes, old } = preflight;
  const folder = path.join(
    root,
    "artifacts/qualification/p2-wsl-client-refresh-package-" + nonce,
  );
  fs.mkdirSync(folder);
  assert.equal(fs.realpathSync(folder), folder);
  const proofFD = fs.openSync(path.join(folder, "proof.json"), "wx"),
    intentFD = fs.openSync(path.join(folder, "intent.jsonl"), "wx");
  const proof = {
    schema: 1,
    nonce,
    nativeSha,
    attestationSha,
    embeddedLockSha: lockSha,
    packageInput,
    packageManifestSha,
    identityPath,
    nativeRoots: preflight.roots,
    nativeRootsProbe: preflight.probe,
    expectedOldSha: pins.oldSha,
    directory,
    runnerSha: sha(stableBytes(fileURLToPath(import.meta.url), 1 << 20)),
    identitySha: sha(identityBytes),
    originalReaderSha: sha(originalTreeReader),
    scope:
      "Private client refresh plus explicit Docker/WSL read-only preservation snapshots; no reconnect/update/lifecycle mutation",
    started: new Date().toISOString(),
    steps: [],
    errors: [],
    postErrors: [],
    success: false,
    dispatch: "not-started",
  };
  const writeAll = (fd, bytes, position) => {
    if (typeof bytes === "string") bytes = Buffer.from(bytes);
    let offset = 0;
    while (offset < bytes.length) {
      const count = fs.writeSync(
        fd,
        bytes,
        offset,
        bytes.length - offset,
        position === undefined ? null : position + offset,
      );
      assert.ok(count > 0, "Evidence write made no progress");
      offset += count;
    }
  };
  const save = () => {
    const bytes = Buffer.from(JSON.stringify(proof, null, 2));
    fs.ftruncateSync(proofFD, 0);
    writeAll(proofFD, bytes, 0);
    fs.fsyncSync(proofFD);
  };
  for (const suffix of ["stdout", "stderr"]) {
    const fd = fs.openSync(path.join(folder, `roots-probe.${suffix}.bin`), "wx");
    writeAll(fd, preflight.raw[suffix]);
    fs.fsyncSync(fd);
    fs.closeSync(fd);
  }
  const intent = (kind) => {
    writeAll(
      intentFD,
      JSON.stringify({
        kind,
        time: new Date().toISOString(),
        nativeSha,
        oldSha: pins.oldSha,
        lockSha: lockSha,
      }) + "\n",
    );
    fs.fsyncSync(intentFD);
    save();
  };
  const anonymous = path.join(folder, "anonymous-docker");
  fs.mkdirSync(anonymous);
  const anonymousFD = fs.openSync(path.join(anonymous, "config.json"), "wx");
  writeAll(anonymousFD, "{}");
  fs.fsyncSync(anonymousFD);
  fs.closeSync(anonymousFD);
  const env = { ...process.env, DOCKER_CONFIG: anonymous };
  for (const key of [
    "DOCKER_HOST",
    "DOCKER_CONTEXT",
    "DOCKER_TLS_VERIFY",
    "DOCKER_CERT_PATH",
    "DOCKER_API_VERSION",
    "DOCKER_AUTH_CONFIG",
    "DOCKER_CUSTOM_HEADERS",
    "BUILDKIT_HOST",
    "BUILDX_BUILDER",
    "COMPOSE_FILE",
    "COMPOSE_PROJECT_NAME",
    "COMPOSE_ENV_FILES",
  ])
    delete env[key];
  let deadline = scenarioDeadline;
  const execute = (
    binary,
    args,
    { input, timeout = 45000, childEnv = env } = {},
  ) => {
    assert.ok(
      deadline > Date.now(),
      "cumulative qualification deadline reached",
    );
    const result = spawnSync(binary, args, {
      cwd: root,
      env: productChildEnvironment(childEnv),
      input,
      encoding: "utf8",
      windowsHide: true,
      timeout: Math.min(timeout, deadline - Date.now()),
      maxBuffer: 24 << 20,
    });
    proof.steps.push({
      binary: path.basename(binary),
      args,
      status: result.status,
      signal: result.signal,
      stdoutSha: sha(result.stdout ?? ""),
      stderrSha: sha(result.stderr ?? ""),
      error: result.error?.code,
    });
    save();
    assert.ok(
      !result.error && !result.signal,
      "client timeout/loss; preserve original backup and inspect uncertain outcome",
    );
    assert.equal(
      result.status,
      0,
      "qualification subprocess failed; retained hashes identify diagnostics without exposing secret output",
    );
    return result.stdout.trim();
  };
  const docker = path.join(tools, "bin/docker.exe"),
    wsl = path.resolve(process.env.SystemRoot, "System32/wsl.exe");
  const clients = {
    owned: (args) =>
      execute(docker, [
        "--config",
        anonymous,
        "--host",
        "npipe:////./pipe/docker-rancher-desktop-orbwin",
        ...args,
      ]),
    desktop: (args) =>
      execute(docker, [
        "--config",
        anonymous,
        "--host",
        "npipe:////./pipe/dockerDesktopLinuxEngine",
        ...args,
      ]),
    rancher: (args) =>
      execute(wsl, [
        "--distribution-id",
        pins.guid,
        "--user",
        "root",
        "--exec",
        "/usr/bin/env",
        "-i",
        "PATH=/usr/bin:/bin",
        windowsLinux(path.join(tools, "linux/docker")),
        "--config",
        windowsLinux(anonymous),
        "--host",
        "unix:///mnt/wsl/rancher-desktop/run/docker.sock",
        ...args,
      ]),
  };
  const lines = (text) =>
    [...new Set(text.split(/\r?\n/).filter(Boolean))].sort();
  const engineSnapshot = (engine) => {
    const read = (args) => clients[engine](args),
      json = (args) => safeJson(read(args));
    const info = json(["info", "--format={{json .}}"]);
    assert.equal(info.ID, expectedIDs[engine]);
    assert.equal(info.OSType, "linux");
    const containers = lines(read(["ps", "-aq", "--no-trunc"])),
      volumes = lines(read(["volume", "ls", "-q"])),
      images = lines(read(["image", "ls", "-aq", "--no-trunc"])),
      networks = lines(read(["network", "ls", "-q", "--no-trunc"]));
    assert.ok(
      containers.length <= 256 &&
        volumes.length <= 256 &&
        images.length <= 512 &&
        networks.length <= 256,
    );
    const inspect = (kind, ids) =>
      ids.length ? json([kind, "inspect", ...ids]) : [];
    const value = canonicalEngineSnapshot({
      containers: inspect("container", containers).map((c) => ({
        id: c.Id,
        name: c.Name,
        image: c.Image,
        created: c.Created,
        running: c.State.Running,
        paused: c.State.Paused,
        restarts: c.RestartCount,
        configSha: sha(JSON.stringify(c.Config)),
        hostConfigSha: sha(JSON.stringify(c.HostConfig)),
        mounts: c.Mounts,
      })),
      volumes: inspect("volume", volumes).map((v) => ({
        name: v.Name,
        created: v.CreatedAt,
        driver: v.Driver,
        scope: v.Scope,
        mountpoint: v.Mountpoint,
        labelsSha: sha(JSON.stringify(v.Labels)),
        optionsSha: sha(JSON.stringify(v.Options)),
      })),
      images: inspect("image", images).map((v) => ({
        id: v.Id,
        repoTags: [...(v.RepoTags ?? [])].sort(),
        repoDigests: [...(v.RepoDigests ?? [])].sort(),
        created: v.Created,
        rootfsSha: sha(JSON.stringify(v.RootFS)),
        configSha: sha(JSON.stringify(v.Config)),
      })),
      networks: inspect("network", networks).map((v) => ({
        id: v.Id,
        name: v.Name,
        driver: v.Driver,
        scope: v.Scope,
        ipamSha: sha(JSON.stringify(v.IPAM)),
        optionsSha: sha(JSON.stringify(v.Options)),
        labelsSha: sha(JSON.stringify(v.Labels)),
        containers: v.Containers,
      })),
    });
    assert.equal(json(["info", "--format={{json .}}"]).ID, expectedIDs[engine]);
    return value;
  };
  const tree = (base) => {
    const result = [];
    let total = 0;
    const walk = (directory, prefix) => {
      const stat = fs.lstatSync(directory);
      assert.ok(stat.isDirectory() && !stat.isSymbolicLink());
      for (const name of fs.readdirSync(directory).sort()) {
        assert.ok(result.length < 512);
        const file = path.join(directory, name),
          stat = fs.lstatSync(file),
          relative = prefix + name;
        assert.ok(!stat.isSymbolicLink());
        const item = {
          name: relative,
          kind: stat.isDirectory() ? "directory" : "file",
          device: stat.dev,
          inode: stat.ino,
          mode: stat.mode,
          size: stat.size,
          mtime: stat.mtimeMs,
          ctime: stat.ctimeMs,
        };
        if (stat.isDirectory()) {
          result.push(item);
          walk(file, relative + "/");
        } else {
          assert.ok(stat.isFile());
          const bytes = stableBytes(file, 4 << 20);
          total += bytes.length;
          assert.ok(total <= 64 << 20);
          item.sha256 = sha(bytes);
          result.push(item);
        }
      }
      const final = fs.lstatSync(directory);
      assert.ok(
        final.isDirectory() &&
          !final.isSymbolicLink() &&
          final.dev === stat.dev &&
          final.ino === stat.ino &&
          final.mtimeMs === stat.mtimeMs &&
          final.ctimeMs === stat.ctimeMs,
        "Private directory changed during snapshot",
      );
    };
    walk(base, "");
    return result;
  };
  const originals = JSON.parse(
    stableBytes(
      path.join(
        root,
        "artifacts/qualification/p2-migration-draft-wsl-originals-20261025.json",
      ),
      65536,
    ),
  );
  assert.equal(originals.source, pins.source);
  assert.equal(originals.signature, pins.signature);
  assert.equal(originals.entries.length, 9);
  assert.equal(originals.fileBytes, 1050);
  const configs = [
    preflight.paths.globalDocker,
    preflight.paths.globalKube,
    preflight.paths.globalWSL,
    path.join(home, ".rdorbwin/docker/config.json"),
  ];
  const snapshot = (label) => {
    const value = { engines: {}, configs: [] },
      errors = [];
    proof[label] = value;
    const capture = (name, action) => {
      try {
        action();
      } catch (error) {
        errors.push({ name, error: String(error.message ?? error) });
      }
      save();
    };
    for (const engine of Object.keys(clients))
      capture(engine, () => {
        value.engines[engine] = engineSnapshot(engine);
      });
    for (const file of configs)
      capture("config:" + path.basename(file), () => {
        const item = configFingerprint(file);
        assert.ok(
          item.exists ||
            file === preflight.paths.globalWSL,
        );
        value.configs.push(item);
      });
    capture("project-records", () => {
      value.projectRecords = tree(path.join(home, ".rdorbwin/wsl-projects"));
    });
    capture("private-wsl-client-tree", () => {
      value.privateTree = tree(privateDirectory);
      value.privateConfig = configFingerprint(privateConfig, 1 << 20);
    });
    capture("wsl-originals", () => {
      value.originals = safeJson(
        execute(
          wsl,
          [
            "--distribution-id",
            pins.guid,
            "--user",
            "vanyr",
            "--exec",
            "/usr/bin/env",
            "-i",
            "PATH=/usr/bin:/bin",
            "/usr/bin/python3",
            "-c",
            originalTreeReader,
          ],
          { input: JSON.stringify(originals), timeout: 15000 },
        ),
      );
    });
    return errors;
  };
  try {
    proof.errors.push(...snapshot("before"));
    assert.equal(
      proof.errors.length,
      0,
      "before baseline incomplete; native will not dispatch",
    );
    assert.equal(proof.before.privateConfig.sha256, pins.oldSha);
    assert.ok(
      !proof.before.privateTree.some((item) => item.name.endsWith(".partial")),
      "Existing refresh partial requires review before dispatch",
    );
    assert.equal(sha(stableBytes(native, 64 << 20)), nativeSha);
    assert.equal(
      sha(stableBytes(path.join(root, "runtime-tools.lock.json"), 1 << 20)),
      lockSha,
    );
    assert.equal(
      sha(
        stableBytes(
          identityPath,
          1 << 20,
        ),
      ),
      proof.identitySha,
      "Owned identity changed before dispatch",
    );
    packageTauriConfiguration(root, packageInput);
    assert.equal(sha(stableBytes(packageManifest, 4 << 20)), packageManifestSha);
    for (const [relative, digest] of Object.entries(source.sources))
      assert.equal(sha(stableBytes(path.join(root, relative), 4 << 20)), digest);
    intent("dispatch-exact-owned-private-client-refresh-once");
    proof.dispatch = "attempted-outcome-not-yet-known";
    save();
    const output = execute(
      native,
      [
        "--exact",
        pins.exactTest,
        "--ignored",
        "--nocapture",
        "--test-threads=1",
      ],
      {
        childEnv: nativeQualificationEnvironment(env, { nonce, roots: preflight.roots, lockSha }),
      },
    );
    const match = output.match(
      /WSL_PRIVATE_CLIENT_REFRESH changed=true idempotent=true oldSha256=([a-f0-9]{64}) newSha256=([a-f0-9]{64}) embeddedLockSha256=([a-f0-9]{64})/,
    );
    assert.ok(match, "native success/idempotence marker absent");
    assert.equal(match[1], pins.oldSha);
    assert.equal(match[3], lockSha);
    proof.nativeResult = {
      changed: true,
      idempotent: true,
      oldSha: match[1],
      newSha: match[2],
      ownerDaclGuard:
        "attested native creation-time owner/exact-DACL checks before writing secret bytes",
    };
    proof.dispatch = "returned-success";
  } catch (error) {
    proof.errors.push({
      name: "scenario",
      error: String(error.message ?? error),
    });
  } finally {
    deadline = Date.now() + 60000;
    try {
      proof.postErrors.push(...snapshot("after"));
    } catch (error) {
      proof.postErrors.push({
        name: "postchecks",
        error: String(error.message ?? error),
      });
    }
    proof.preservationChecks = {};
    const check = (name, action) => {
      try {
        action();
        proof.preservationChecks[name] = true;
      } catch (error) {
        proof.preservationChecks[name] = false;
        proof.postErrors.push({ name, error: String(error.message ?? error) });
      }
    };
    for (const key of ["engines", "configs", "projectRecords", "originals"])
      check("retained:" + key, () => {
        assert.ok(
          proof.before?.[key] !== undefined && proof.after?.[key] !== undefined,
          "Baseline section missing",
        );
        assert.deepEqual(proof.after[key], proof.before[key]);
      });
    check("private-config-raw-edit", () => {
      const actual = stableBytes(privateConfig, 1 << 20);
      proof.observedPrivateSha = sha(actual);
      proof.privateState =
        proof.observedPrivateSha === pins.oldSha
          ? "original-bytes-present"
          : "changed-inspect-result";
      assertPlannedEdit(old, actual, directory);
      proof.privateState = "planned-path-edit-present";
    });
    check("private-config-backup-and-residue", () => {
      proof.backup = assertBackupDelta(
        proof.before.privateTree,
        proof.after.privateTree,
        pins.oldSha,
      );
    });
    check("native-result-matches-observation", () => {
      assert.ok(
        proof.nativeResult,
        "Native success was not observed; outcome remains unqualified",
      );
      assert.equal(
        proof.after?.privateConfig?.sha256,
        proof.nativeResult?.newSha,
      );
    });
    check("current-package-source-binding", () => {
      packageTauriConfiguration(root, packageInput);
      assert.equal(sha(stableBytes(packageManifest, 4 << 20)), packageManifestSha);
      assert.equal(sha(stableBytes(native, 64 << 20)), nativeSha);
      assert.equal(sha(stableBytes(identityPath, 1 << 20)), proof.identitySha);
      for (const [relative, digest] of Object.entries(source.sources))
        assert.equal(sha(stableBytes(path.join(root, relative), 4 << 20)), digest);
    });
    proof.preservation = Object.values(proof.preservationChecks).every(Boolean);
    proof.success =
      proof.errors.length === 0 &&
      proof.postErrors.length === 0 &&
      proof.dispatch === "returned-success" &&
      proof.preservation === true;
    proof.finished = new Date().toISOString();
    proof.recovery =
      "Never restore automatically. Original exact bytes remain in the private sibling backup; inspect any uncertain outcome and retained partial files before retrying.";
    save();
    fs.closeSync(proofFD);
    fs.closeSync(intentFD);
  }
  return {
    proof: path.join(folder, "proof.json"),
    success: proof.success,
    dispatch: proof.dispatch,
    errors: proof.errors,
    postErrors: proof.postErrors,
  };
}

export const plan = () => ({
  scope:
    "Current-package private WSL client refresh with Docker/WSL read-only preservation snapshots; no reconnect or lifecycle changes",
  pins,
  required: [
    "--run",
    "ORBWIN_QUALIFY_OWNED=1",
    "ORBWIN_QUALIFY_WSL_CLIENT_REFRESH_RUN=1",
    "nonce + frozen native path/SHA + source attestation path/SHA",
    "canonical prepared package input beside native test; existing full package verifier",
    "same attested PE exact native roots query before private reads; no environment-root fallback",
    "closed native LocalAppData/Profile DATA rechecked by refresh; same actual Target used for both updates",
    "reviewed expected native-profile plugin directory; no namespace repair/retry",
    "current packaged runtime identity with unchanged fixed daemon; no rewrite",
  ],
  budgets: { scenarioSeconds: 240, stepSeconds: 45, postcheckSeconds: 60 },
  recovery: "retain backup/evidence; no automatic restore or cleanup",
});
if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  if (process.argv.length === 2) console.log(JSON.stringify(plan(), null, 2));
  else {
    assert.deepEqual(process.argv.slice(2), ["--run"]);
    const result = runRefresh({
      nonce: process.env.ORBWIN_QUALIFY_WSL_CLIENT_NONCE,
      packageInput: process.env.ORBWIN_QUALIFY_WSL_CLIENT_PACKAGE_INPUT,
      expectedPluginDirectory: process.env.ORBWIN_QUALIFY_WSL_CLIENT_PLUGIN_DIRECTORY,
      native: process.env.ORBWIN_QUALIFY_WSL_CLIENT_NATIVE,
      nativeSha: process.env.ORBWIN_QUALIFY_WSL_CLIENT_NATIVE_SHA256,
      attestation: process.env.ORBWIN_QUALIFY_WSL_CLIENT_ATTESTATION,
      attestationSha: process.env.ORBWIN_QUALIFY_WSL_CLIENT_ATTESTATION_SHA256,
    });
    console.log(JSON.stringify(result, null, 2));
    process.exitCode = result.success ? 0 : 1;
  }
}
