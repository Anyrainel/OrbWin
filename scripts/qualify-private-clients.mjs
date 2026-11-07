// Synthetic loopback registry only. Root reviews and grants the engine lane.
// No dependency acquisition: the exact registry/base pins must already be cached.
import { spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  openSync,
  writeSync,
  fsyncSync,
  closeSync,
  statSync,
  lstatSync,
  realpathSync,
  renameSync,
} from "node:fs";
import { resolve, dirname, basename } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";
import {
  cleanupPrivateComposeStage,
  runPrivateComposeStage,
} from "./private-compose-stage.mjs";
import {
  cleanupPrivateDevcontainerCompose,
  createCallbackBudget,
  runPrivateDevcontainerCompose,
} from "./private-devcontainer-compose-stage.mjs";

if (
  process.env.ORBWIN_QUALIFY_OWNED !== "1" ||
  process.env.ORBWIN_QUALIFY_PRIVATE_CLIENTS !== "1"
)
  throw Error("Explicit private-client and owned-engine opt-in required");
const root = fileURLToPath(new URL("../", import.meta.url)),
  mode = process.argv[2] ?? "run";
const pins = JSON.parse(
  readFileSync(resolve(root, "tests/registry/images.lock.json")),
);
const ownEndpoint = "npipe:////./pipe/docker-rancher-desktop-orbwin",
  desktop = "npipe:////./pipe/dockerDesktopLinuxEngine";
const expected = {
  owned: "d9c00194-ea9d-439a-8561-6a0ec580fbd5",
  desktop: "a307ff0e-581d-49d3-8aee-a0636dda9137",
  rancher: "741eb2cb-adf5-4676-bf28-f03f68d54f60",
};
const label = "dev.orbwin.qualification",
  password = "orbwin-disposable-proof-only";
let state, file;
let callbackBudget;
function noteBudgetFailure(error) {
  if (callbackBudget && error?.code === "DC_CALLBACK_DEADLINE_EXCEEDED") {
    state.devcontainerComposeCallbackFailure = {
      mode,
      phase: callbackBudget.phase(),
      code: error.code,
    };
    save();
  }
  throw error;
}
function deadlineTimeout(requested) {
  try {
    return callbackBudget ? callbackBudget.limitTimeout(requested) : requested;
  } catch (error) {
    return noteBudgetFailure(error);
  }
}
function checkCurrentBudget() {
  try {
    callbackBudget?.assertCurrent();
  } catch (error) {
    noteBudgetFailure(error);
  }
}
function devcontainerComposeContext() {
  return {
    root,
    state,
    execute,
    intent,
    save,
    withCleanupBudget: (task) =>
      callbackBudget ? callbackBudget.withCleanupBudget(task) : task(),
  };
}
const hash = (path) =>
  existsSync(path)
    ? createHash("sha256").update(readFileSync(path)).digest("hex")
    : null;
const bytesHash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const lines = (text) => text.trim().split(/\r?\n/).filter(Boolean).sort();
function save() {
  const temporary = `${file}.partial-${process.pid}-${randomUUID()}`;
  const fd = openSync(temporary, "wx");
  try {
    writeSync(fd, JSON.stringify(state, null, 2));
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
  renameSync(temporary, file);
}
function journal(value) {
  const fd = openSync(resolve(state.folder, "resource-intent.jsonl"), "a");
  try {
    writeSync(fd, JSON.stringify({ nonce: state.nonce, ...value }) + "\n");
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
}
const env = {
  ...process.env,
  DOCKER_HOST: ownEndpoint,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  DOCKER_API_VERSION: "",
  BUILDKIT_HOST: "",
  BUILDX_BUILDER: "default",
  HTTP_PROXY: "",
  HTTPS_PROXY: "",
  ALL_PROXY: "",
  NO_PROXY: "127.0.0.1,localhost",
  COMPOSE_FILE: "",
  COMPOSE_PROFILES: "",
  COMPOSE_PROJECT_NAME: "",
  COMPOSE_ENV_FILES: "",
  COMPOSE_REMOVE_ORPHANS: "false",
};
function execute(
  binary,
  args,
  {
    timeout = 120_000,
    input,
    cwd = root,
    allowedFailure = false,
    extraEnv = {},
  } = {},
) {
  timeout = deadlineTimeout(timeout);
  const started = Date.now();
  const out = spawnSync(binary, args, {
    cwd,
    env: { ...env, ...extraEnv },
    encoding: "utf8",
    windowsHide: true,
    timeout,
    maxBuffer: 8 << 20,
    input,
  });
  if (binary === "cargo" || binary === state.frozenNativeTest?.path)
    state = JSON.parse(readFileSync(file));
  state.events.push({
    binary,
    args,
    status: out.status,
    milliseconds: Date.now() - started,
    timeoutMs: timeout,
    callbackBudgetPhase: callbackBudget?.phase(),
    error: out.error?.message,
    stdoutSha256: bytesHash(out.stdout ?? ""),
    stderrSha256: bytesHash(out.stderr ?? ""),
  });
  save();
  checkCurrentBudget();
  if (!allowedFailure && (out.error || out.status !== 0))
    throw Error(
      `Fixture command failed (${basename(binary)} ${args[0]}, ${out.status}); raw output deliberately omitted`,
    );
  return out;
}
const cli = (...args) => execute(state.wrapper, args).stdout.trim();
const raw = (host, ...args) =>
  execute(resolve(root, "artifacts/runtime-tools/bin/docker.exe"), [
    "--config",
    state.emptyConfig,
    "--host",
    host,
    ...args,
  ]).stdout.trim();
const linuxPath = (path) => {
  const m = path.match(/^([A-Za-z]):[\\/](.*)$/);
  if (!m) throw Error("Local drive path required");
  return `/mnt/${m[1].toLowerCase()}/${m[2].replaceAll("\\", "/")}`;
};
// Explicit existing ordinary-user registration. No registration/default changes.
const wslRegistration = "{231810b0-fa21-48a8-b0a7-86f9a1360029}";
function wslShell(script, args = [], options = {}) {
  return execute(
    resolve(process.env.SystemRoot, "System32/wsl.exe"),
    [
      "--distribution-id",
      wslRegistration,
      "--user",
      "vanyr",
      "--exec",
      "/bin/sh",
      "-c",
      script,
      "orbwin-private-client",
      ...args,
    ],
    options,
  );
}
function wslGlobalHash() {
  const out = wslShell(
    'test "$(/usr/bin/id -u)" = 1000 && test "$HOME" = /home/vanyr && test "$WSL_DISTRO_NAME" = Ubuntu || exit 21; if test -f "$HOME/.docker/config.json"; then /usr/bin/sha256sum "$HOME/.docker/config.json"; elif test ! -e "$HOME/.docker/config.json"; then printf absent; else exit 22; fi',
  ).stdout.trim();
  if (out === "absent") return null;
  assert.match(out, /^[a-f0-9]{64}  \/home\/vanyr\/\.docker\/config\.json$/);
  return out.slice(0, 64);
}
function wslDocker(args, options = {}) {
  checkWrapper();
  const manifest = JSON.parse(
    readFileSync(resolve(root, "artifacts/runtime-tools/manifest.json")),
  );
  const docker = resolve(root, "artifacts/runtime-tools/linux/docker");
  assert.equal(
    hash(docker),
    manifest.tools.find((tool) => tool.path === "linux/docker")?.sha256,
  );
  return wslShell(
    'test "$(/usr/bin/id -u)" = 1000 && test "$HOME" = /home/vanyr && test "$WSL_DISTRO_NAME" = Ubuntu && test -S /mnt/wsl/orbwin/docker.sock && test -n "$WSL_INTEROP" || exit 23; helper=$1; config=$2; shift 2; exec /usr/bin/env -i HOME=/home/vanyr USER=vanyr LOGNAME=vanyr WSL_INTEROP="$WSL_INTEROP" WSL_DISTRO_NAME=Ubuntu PATH="$helper:/usr/bin:/bin" DOCKER_CONFIG="$config" "$@"',
    [
      linuxPath(resolve(dirname(state.wrapper), "runtime/backend")),
      linuxPath(
        resolve(
          process.env.LOCALAPPDATA,
          "OrbWinData/home/.rdorbwin/wsl-docker",
        ),
      ),
      linuxPath(docker),
      "--config",
      linuxPath(
        resolve(
          process.env.LOCALAPPDATA,
          "OrbWinData/home/.rdorbwin/wsl-docker",
        ),
      ),
      "--host",
      "unix:///mnt/wsl/orbwin/docker.sock",
      ...args,
    ],
    { timeout: 45_000, ...options },
  );
}
const rancher = (...args) =>
  execute(resolve(process.env.SystemRoot, "System32/wsl.exe"), [
    "--distribution-id",
    "{231810b0-fa21-48a8-b0a7-86f9a1360029}",
    "--user",
    "root",
    "--exec",
    linuxPath(resolve(root, "artifacts/runtime-tools/linux/docker")),
    "--config",
    linuxPath(state.emptyConfig),
    "--host",
    "unix:///mnt/wsl/rancher-desktop/run/docker.sock",
    ...args,
  ]).stdout.trim();
function inspect(kind, id) {
  return JSON.parse(cli(kind, "inspect", id))[0];
}
function intent(kind, name) {
  const value = { kind, name, time: new Date().toISOString() };
  journal(value);
  state.intents.push(value);
  save();
}
function identity() {
  const ids = {
    owned: raw(ownEndpoint, "info", "--format", "{{.ID}}"),
    desktop: raw(desktop, "info", "--format", "{{.ID}}"),
    rancher: rancher("info", "--format", "{{.ID}}"),
  };
  assert.deepEqual(ids, expected);
  return ids;
}
function snapshot() {
  const ids = identity(),
    inventories = {};
  for (const [name, read] of [
    ["owned", (...a) => raw(ownEndpoint, ...a)],
    ["desktop", (...a) => raw(desktop, ...a)],
    ["rancher", rancher],
  ]) {
    const containers = lines(
      read("ps", "-a", "--no-trunc", "--format", "{{.ID}}"),
    );
    assert.ok(
      containers.length <= 256 &&
        containers.every((v) => /^[a-f0-9]{64}$/.test(v)),
    );
    inventories[name] = {
      containers,
      states: containers.length
        ? lines(
            read(
              "container",
              "inspect",
              ...containers,
              "--format",
              "{{.Id}}|{{.State.Running}}|{{.State.Paused}}|{{.RestartCount}}",
            ),
          )
        : [],
      images: [...new Set(lines(read("image", "ls", "-aq", "--no-trunc")))],
      volumes: lines(read("volume", "ls", "-q")),
      networks: lines(read("network", "ls", "--no-trunc", "-q")),
    };
  }
  const home = resolve(process.env.LOCALAPPDATA, "OrbWinData/home");
  const configs = {
    docker: hash(resolve(process.env.USERPROFILE, ".docker/config.json")),
    kube: hash(resolve(process.env.USERPROFILE, ".kube/config")),
    wsl: hash(resolve(process.env.USERPROFILE, ".wslconfig")),
    windowsPrivate: hash(resolve(home, ".rdorbwin/docker/config.json")),
    wslPrivate: hash(resolve(home, ".rdorbwin/wsl-docker/config.json")),
    ...(state.wslPrivateEnabled ? { wslOrdinaryGlobal: wslGlobalHash() } : {}),
    project: hash(
      resolve(
        home,
        ".rdorbwin/wsl-projects/710eb4f4d7f72b0e329a51d30a83377bc7ef6c0cca1a444bd8d2e70e72326e8f.json",
      ),
    ),
  };
  assert.equal(
    execute("powershell.exe", [
      "-NoProfile",
      "-NonInteractive",
      "-Command",
      "([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)",
    ]).stdout.trim(),
    "False",
  );
  return { ids, inventories, configs, windowsElevated: false };
}
function checkWrapper() {
  const manifest = JSON.parse(
    readFileSync(resolve(root, "artifacts/cli/cli-manifest.json")),
  );
  assert.equal(basename(state.wrapper).toLowerCase(), "orbwin-docker.exe");
  assert.equal(hash(state.wrapper), manifest.alias.sha256);
  const helper = JSON.parse(
    readFileSync(resolve(root, "artifacts/backend/credentials-manifest.json")),
  );
  assert.equal(
    hash(
      resolve(
        dirname(state.wrapper),
        "runtime/backend/docker-credential-orbwin.exe",
      ),
    ),
    helper.binarySha256,
  );
  assert.equal(
    hash(resolve(dirname(state.wrapper), "runtime/tools/bin/docker.exe")),
    manifest.dockerSha256,
  );
}
function ownedContainer(id) {
  const value = inspect("container", id);
  assert.equal(value.Id, id);
  assert.ok(/^[a-f0-9]{64}$/.test(id));
  assert.ok(!state.before.inventories.owned.containers.includes(id));
  assert.equal(value.Config.Labels?.[label], state.nonce);
  assert.ok(
    state.intents.some(
      (x) => x.kind === "container" && value.Name === `/${x.name}`,
    ),
  );
  if (value.Name.endsWith("-registry") || value.Name.endsWith("-probe"))
    assert.equal(value.Image, state.registryImageId);
  else if (value.Name.endsWith("-seed"))
    assert.equal(value.Image, state.baseImageId);
  else {
    assert.equal(value.Config.Image, state.image);
    const image = inspect("image", value.Image);
    assert.equal(image.Config.Labels?.[label], state.nonce);
    assert.deepEqual(image.RootFS, state.committedRootFS);
  }
  assert.equal(value.HostConfig.Privileged, false);
  if (!value.Name.endsWith("-dev"))
    assert.equal(value.HostConfig.ReadonlyRootfs, true);
  assert.ok(
    value.HostConfig.Memory > 0 && value.HostConfig.Memory <= 134217728,
  );
  assert.ok(
    value.HostConfig.NanoCpus > 0 && value.HostConfig.NanoCpus <= 250000000,
  );
  assert.ok(value.HostConfig.PidsLimit > 0 && value.HostConfig.PidsLimit <= 64);
  assert.ok(value.HostConfig.CapDrop?.includes("ALL"));
  assert.ok(
    value.HostConfig.SecurityOpt?.includes("no-new-privileges:true") ||
      value.HostConfig.SecurityOpt?.includes("no-new-privileges"),
  );
  for (const mount of value.Mounts) {
    assert.ok(
      (mount.Type === "volume" && mount.Name === state.volume) ||
        (mount.Type === "bind" &&
          mount.Source.includes(state.name) &&
          ["/auth/htpasswd", "/workspace"].includes(mount.Destination)) ||
        (mount.Type === "tmpfs" &&
          mount.Destination === "/tmp" &&
          value.Name === `/${state.name}-registry`),
    );
    if (mount.Type === "volume")
      assert.equal(mount.Destination, "/var/lib/registry");
    if (mount.Type === "bind" && mount.Destination === "/auth/htpasswd")
      assert.equal(mount.RW, false);
  }
  return value;
}
function cleanup() {
  identity();
  cleanupPrivateDevcontainerCompose(devcontainerComposeContext());
  cleanupPrivateComposeStage({ root, state, execute, intent, save });
  for (const id of lines(
    cli("ps", "-aq", "--no-trunc", "--filter", `label=${label}=${state.nonce}`),
  )) {
    const value = ownedContainer(id);
    if (value.State.Paused)
      throw Error("Unexpected paused fixture; retain for review");
    if (value.State.Running) cli("stop", "--time", "5", id);
    ownedContainer(id);
    cli("rm", id);
  }
  for (const id of [
    ...new Set(
      lines(
        cli(
          "image",
          "ls",
          "--filter",
          `label=${label}=${state.nonce}`,
          "-q",
          "--no-trunc",
        ),
      ),
    ),
  ]) {
    const image = inspect("image", id);
    assert.equal(image.Id, id);
    assert.equal(image.Config.Labels?.[label], state.nonce);
    assert.ok(!state.before.inventories.owned.images.includes(id));
    const tags = image.RepoTags ?? [];
    assert.ok(
      tags.every(
        (tag) => tag === state.image || tag.startsWith(`${state.name}:`),
      ),
    );
    if (tags.length) for (const tag of tags) cli("image", "rm", tag);
    else cli("image", "rm", id);
  }
  if (state.volume) {
    const volume = JSON.parse(cli("volume", "inspect", state.volume))[0];
    assert.equal(volume.Name, state.volume);
    assert.equal(volume.Labels?.[label], state.nonce);
    assert.equal(volume.Driver, "local");
    assert.equal(
      lines(cli("ps", "-aq", "--filter", `volume=${state.volume}`)).length,
      0,
    );
    cli("volume", "rm", state.volume);
  }
}
function dev(action, extra = []) {
  const out = execute(
    process.execPath,
    [
      state.devCli,
      action,
      "--docker-path",
      state.wrapper,
      "--workspace-folder",
      state.workspace,
      "--mount-workspace-git-root",
      "false",
      "--user-data-folder",
      resolve(state.workspace, "client-data"),
      "--default-user-env-probe",
      "none",
      "--log-level",
      "info",
      "--log-format",
      "json",
      ...extra,
    ],
    {
      timeout: 180_000,
      cwd: state.workspace,
      extraEnv: {
        DOCKER_CONFIG: state.privateConfig,
        PATH:
          resolve(dirname(state.wrapper), "runtime/backend") +
          ";" +
          process.env.PATH,
      },
    },
  );
  return out.stdout;
}
async function authenticated() {
  checkWrapper();
  identity();
  intent("image", state.image);
  const pushed = state.devcontainerComposeOnly
    ? execute(state.wrapper, ["push", state.image], {
        timeout: 30000,
      }).stdout.trim()
    : cli("push", state.image);
  const pushedDigest = pushed.match(/digest:\s*(sha256:[a-f0-9]{64})/)?.[1];
  assert.ok(pushedDigest);
  const response = await fetch(
    `http://${state.server}/v2/fixture/image/manifests/${state.nonce}`,
    {
      headers: {
        Authorization: `Basic ${Buffer.from(`${state.username}:${password}`).toString("base64")}`,
        Accept:
          "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json",
      },
      signal: AbortSignal.timeout(deadlineTimeout(10_000)),
    },
  );
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("docker-content-digest"), pushedDigest);
  const manifest = await response.json();
  assert.match(manifest.config.digest, /^sha256:[a-f0-9]{64}$/);
  state.registryManifestDigest = pushedDigest;
  state.registryConfigDigest = manifest.config.digest;
  save();
  const configResponse = await fetch(
    `http://${state.server}/v2/fixture/image/blobs/${manifest.config.digest}`,
    {
      headers: {
        Authorization: `Basic ${Buffer.from(`${state.username}:${password}`).toString("base64")}`,
      },
      signal: AbortSignal.timeout(deadlineTimeout(10_000)),
    },
  );
  assert.equal(configResponse.status, 200);
  const configText = await configResponse.text();
  assert.ok(configText.length <= 1 << 20);
  assert.equal(`sha256:${bytesHash(configText)}`, manifest.config.digest);
  const imageConfiguration = JSON.parse(configText);
  assert.equal(imageConfiguration.config.Labels?.[label], state.nonce);
  assert.deepEqual(
    imageConfiguration.rootfs.diff_ids,
    state.committedRootFS.Layers,
  );
  state.steps.push(
    "independent registry config bytes matched content digest, nonce and layer identities",
  );
  save();
  cli("image", "rm", state.image);
  if (state.devcontainerComposeOnly) {
    await runPrivateDevcontainerCompose(
      devcontainerComposeContext(),
      "authenticated",
    );
    checkCurrentBudget();
    return;
  }
  cli("pull", state.image);
  let pulled = inspect("image", state.image);
  assert.ok(
    pulled.RepoDigests?.includes(
      `${state.server}/fixture/image@${pushedDigest}`,
    ),
  );
  assert.equal(pulled.Config.Labels?.[label], state.nonce);
  assert.deepEqual(pulled.RootFS, state.committedRootFS);
  assert.equal(pulled.Os, "linux");
  assert.equal(pulled.Architecture, "amd64");
  state.steps.push(
    "current alias forced private pull matched independent registry digest and nonce content",
  );
  save();
  cli("image", "rm", state.image);
  const config = {
    name: state.name,
    image: state.image,
    overrideCommand: false,
    workspaceMount: `type=bind,source=${state.workspace},target=/workspace`,
    workspaceFolder: "/workspace",
    containerUser: "root",
    remoteUser: "root",
    userEnvProbe: "none",
    updateRemoteUserUID: false,
    runArgs: [
      "--name",
      `${state.name}-dev`,
      "--network=none",
      "--cpus=0.25",
      "--memory=128m",
      "--pids-limit=64",
      "--cap-drop=ALL",
      "--security-opt=no-new-privileges:true",
      `--label=${label}=${state.nonce}`,
    ],
    customizations: { vscode: { extensions: [] } },
  };
  mkdirSync(resolve(state.workspace, ".devcontainer"), { recursive: true });
  writeFileSync(
    resolve(state.workspace, ".devcontainer/devcontainer.json"),
    JSON.stringify(config, null, 2),
    { flag: "wx" },
  );
  writeFileSync(resolve(state.workspace, "seed.txt"), state.nonce, {
    flag: "wx",
  });
  intent("container", `${state.name}-dev`);
  const output = dev("up", [
    "--update-remote-user-uid-default",
    "never",
    "--skip-post-create",
  ]);
  const result = JSON.parse(output.trim().split(/\r?\n/).at(-1));
  assert.match(result.containerId ?? "", /^[a-f0-9]{64}$/);
  state.devContainer = result.containerId;
  save();
  const value = ownedContainer(result.containerId);
  assert.equal(value.Config.Image, state.image);
  assert.equal(value.HostConfig.NetworkMode, "none");
  assert.ok(
    value.Mounts.every(
      (m) =>
        m.Type === "bind" &&
        m.Destination === "/workspace" &&
        m.Source.includes(state.name),
    ),
  );
  assert.ok(
    inspect("image", state.image).RepoDigests?.includes(
      `${state.server}/fixture/image@${pushedDigest}`,
    ),
  );
  dev("exec", [
    "--container-id",
    result.containerId,
    "/bin/sh",
    "-c",
    `test "$(cat /workspace/seed.txt)" = '${state.nonce}' && printf '%s' '${state.nonce}-roundtrip' > /workspace/result.txt`,
  ]);
  assert.equal(
    readFileSync(resolve(state.workspace, "result.txt"), "utf8"),
    `${state.nonce}-roundtrip`,
  );
  state.steps.push(
    "actual Dev Containers up forced private pull through opt-in alias; exec/workspace roundtrip verified",
  );
  cli("stop", "--time", "5", result.containerId);
  ownedContainer(result.containerId);
  cli("rm", result.containerId);
  cli("image", "rm", state.image);
  if (state.wslPrivateEnabled) {
    assert.equal(wslGlobalHash(), state.before.configs.wslOrdinaryGlobal);
    const config = JSON.parse(
      readFileSync(
        resolve(
          process.env.LOCALAPPDATA,
          "OrbWinData/home/.rdorbwin/wsl-docker/config.json",
        ),
      ),
    );
    assert.equal(config.credHelpers?.[state.server], "orbwin.exe");
    assert.equal(
      wslDocker(["info", "--format", "{{.ID}}"]).stdout.trim(),
      expected.owned,
    );
    assert.equal(
      cli("image", "ls", "--filter", `reference=${state.image}`, "-q"),
      "",
    );
    intent("image", state.image);
    wslDocker(["pull", state.image]);
    const image = JSON.parse(
      wslDocker(["image", "inspect", state.image]).stdout,
    )[0];
    assert.ok(
      image.RepoDigests?.includes(
        `${state.server}/fixture/image@${pushedDigest}`,
      ),
    );
    assert.equal(image.Config.Labels?.[label], state.nonce);
    assert.deepEqual(image.RootFS, state.committedRootFS);
    assert.equal(image.Os, "linux");
    assert.equal(image.Architecture, "amd64");
    assert.equal(wslGlobalHash(), state.before.configs.wslOrdinaryGlobal);
    cli("image", "rm", state.image);
    state.steps.push(
      "explicit Ubuntu UID1000 forced private pull via Windows helper matched registry digest, nonce and rootfs; ordinary global config preserved",
    );
  }
  if (state.privateComposeEnabled)
    await runPrivateComposeStage(
      { root, state, execute, intent, save },
      "authenticated",
    );
  save();
}
async function forgotten() {
  checkWrapper();
  identity();
  if (state.devcontainerComposeOnly) {
    await runPrivateDevcontainerCompose(
      devcontainerComposeContext(),
      "forgotten",
    );
    checkCurrentBudget();
    return;
  }
  const result = execute(state.wrapper, ["pull", state.image], {
    allowedFailure: true,
  });
  assert.equal(result.error, undefined);
  assert.notEqual(result.status, 0);
  assert.match(
    result.stderr ?? "",
    /unauthorized|authentication required|no basic auth credentials/i,
  );
  assert.equal(ownedContainer(state.registryContainer).State.Running, true);
  state.steps.push(
    "forced private alias pull refused after native Forget while registry stayed running",
  );
  if (state.wslPrivateEnabled) {
    assert.equal(
      wslDocker(["info", "--format", "{{.ID}}"]).stdout.trim(),
      expected.owned,
    );
    assert.equal(
      cli("image", "ls", "--filter", `reference=${state.image}`, "-q"),
      "",
    );
    const refusal = wslDocker(["pull", state.image], { allowedFailure: true });
    assert.equal(refusal.error, undefined);
    assert.notEqual(refusal.status, 0);
    assert.match(
      refusal.stderr ?? "",
      /unauthorized|authentication required|no basic auth credentials/i,
    );
    assert.equal(ownedContainer(state.registryContainer).State.Running, true);
    assert.equal(wslGlobalHash(), state.before.configs.wslOrdinaryGlobal);
    state.steps.push(
      "explicit Ubuntu UID1000 forced private pull refused after native Forget while registry stayed running; ordinary global config preserved",
    );
  }
  if (state.privateComposeEnabled)
    await runPrivateComposeStage(
      { root, state, execute, intent, save },
      "forgotten",
    );
  save();
}

if (mode !== "run") {
  assert.ok(["authenticated", "forgotten"].includes(mode));
  file = resolve(process.argv[3] ?? "");
  state = JSON.parse(readFileSync(file));
  assert.match(state.nonce, /^[a-f0-9]{32}$/);
  assert.equal(
    dirname(file),
    resolve(root, "artifacts/qualification", state.name),
  );
  assert.equal(state.name, `orbwin-private-client-${state.nonce}`);
  if (state.devcontainerComposeOnly) {
    assert.equal(state.wslPrivateEnabled, false);
    assert.equal(state.privateComposeEnabled, false);
    callbackBudget = createCallbackBudget(
      mode === "authenticated" ? 180000 : 90000,
    );
    state.devcontainerComposeCallbackBudgets ??= {};
    state.devcontainerComposeCallbackBudgets[mode] = callbackBudget.snapshot();
    save();
  }
  if (mode === "authenticated") await authenticated();
  else await forgotten();
  checkCurrentBudget();
  if (callbackBudget) {
    state.devcontainerComposeCallbackBudgets[mode].completedAt =
      new Date().toISOString();
    save();
    checkCurrentBudget();
  }
} else {
  const nonce = randomUUID().replaceAll("-", "");
  const name = `orbwin-private-client-${nonce}`,
    folder = resolve(root, "artifacts/qualification", name);
  mkdirSync(folder);
  file = resolve(folder, "state.json");
  const workspace = resolve(tmpdir(), name);
  mkdirSync(workspace);
  const emptyConfig = resolve(folder, "empty-docker");
  mkdirSync(emptyConfig);
  writeFileSync(resolve(emptyConfig, "config.json"), "{}", { flag: "wx" });
  const wrapper = resolve(process.env.ORBWIN_QUALIFY_DOCKER_WRAPPER ?? "");
  assert.notEqual(
    wrapper,
    root,
    "Supply the reviewed staged Docker alias path",
  );
  state = {
    schema: 1,
    nonce,
    name,
    folder,
    workspace,
    emptyConfig,
    wrapper,
    privateConfig: resolve(
      process.env.LOCALAPPDATA,
      "OrbWinData/home/.rdorbwin/docker",
    ),
    username: `orbwin-proof-${nonce.slice(0, 12)}`,
    wslPrivateEnabled: process.env.ORBWIN_QUALIFY_WSL_PRIVATE === "1",
    privateComposeEnabled: process.env.ORBWIN_QUALIFY_COMPOSE_PRIVATE === "1",
    devcontainerComposeOnly:
      process.env.ORBWIN_QUALIFY_DEVCONTAINER_COMPOSE_ONLY === "1",
    selectedClientScope:
      process.env.ORBWIN_QUALIFY_DEVCONTAINER_COMPOSE_ONLY === "1"
        ? "Dev Containers default Compose private FROM only; prior Windows/image-only/WSL/plain Compose journeys skipped"
        : "Existing selected client stages",
    events: [],
    intents: [],
    steps: [],
    errors: [],
    success: false,
    cleaned: false,
    pins,
    started: new Date().toISOString(),
  };
  save();
  try {
    if (state.devcontainerComposeOnly) {
      assert.equal(
        state.wslPrivateEnabled,
        false,
        "Dedicated run cannot combine WSL replay",
      );
      assert.equal(
        state.privateComposeEnabled,
        false,
        "Dedicated run cannot combine plain Compose replay",
      );
    }
    checkWrapper();
    if (process.env.ORBWIN_PRIVATE_NATIVE_TEST) {
      const path = resolve(process.env.ORBWIN_PRIVATE_NATIVE_TEST);
      const sha256 = process.env.ORBWIN_PRIVATE_NATIVE_TEST_SHA256;
      assert.match(sha256 ?? "", /^[a-f0-9]{64}$/);
      assert.ok(
        path.startsWith(resolve(root, "artifacts/qualification") + "\\"),
      );
      assert.equal(basename(path), "orbwin-native-test.exe");
      assert.equal(realpathSync(path), path);
      assert.ok(lstatSync(path).isFile() && !lstatSync(path).isSymbolicLink());
      assert.ok(statSync(path).size <= 64 << 20);
      assert.equal(hash(path), sha256);
      state.frozenNativeTest = {
        path,
        sha256,
        scope:
          "Reviewed exact existing native test binary; this run does not compile in-progress application sources",
      };
      save();
    }
    state.before = snapshot();
    save();
    const registry = JSON.parse(
        raw(ownEndpoint, "image", "inspect", pins.registry),
      )[0],
      base = JSON.parse(raw(ownEndpoint, "image", "inspect", pins.base))[0];
    assert.equal(registry.Os, "linux");
    assert.equal(registry.Architecture, "amd64");
    assert.equal(base.Os, "linux");
    assert.equal(base.Architecture, "amd64");
    assert.equal(Object.keys(base.Config.Volumes ?? {}).length, 0);
    assert.ok(
      Object.keys(registry.Config.Volumes ?? {}).every(
        (v) => v === "/var/lib/registry",
      ),
    );
    state.registryImageId = registry.Id;
    state.baseImageId = base.Id;
    state.cachedDependency = {
      requested: pins.registry,
      resolvedId: registry.Id,
      reportedUncompressedBytes: registry.Size,
      networkTransferBytes: null,
      acquisition:
        "pre-cached prerequisite; no network acquisition by this harness",
    };
    save();
    const registryArchive = resolve(folder, "registry-source.tar");
    cli("image", "save", "--output", registryArchive, pins.registry);
    assert.ok(statSync(registryArchive).size <= 256 * 1024 * 1024);
    const archiveManifest = JSON.parse(
      execute(resolve(process.env.SystemRoot, "System32/tar.exe"), [
        "-xOf",
        registryArchive,
        "manifest.json",
      ]).stdout,
    );
    assert.ok(
      Array.isArray(archiveManifest) &&
        archiveManifest.length > 0 &&
        archiveManifest.length <= 128,
    );
    const configIds = [];
    for (const record of archiveManifest) {
      assert.match(
        record.Config,
        /^(?:blobs\/sha256\/[a-f0-9]{64}|[a-f0-9]{64}\.json)$/,
      );
      const digest = record.Config.replace(/^blobs\/sha256\//, "").replace(
        /\.json$/,
        "",
      );
      const configBytes = execute(
        resolve(process.env.SystemRoot, "System32/tar.exe"),
        ["-xOf", registryArchive, record.Config],
      ).stdout;
      assert.equal(bytesHash(configBytes), digest);
      configIds.push(`sha256:${digest}`);
    }
    state.cachedDependency.sourceArchiveBytes = statSync(registryArchive).size;
    state.cachedDependency.sourceArchiveSha256 = hash(registryArchive);
    state.cachedDependency.configIds = [...new Set(configIds)];
    save();
    const go = JSON.parse(readFileSync(resolve(root, "upstream.lock.json"))).go
      .version;
    const hashExe = resolve(folder, "registry-fixture-hash.exe");
    execute(
      resolve(root, `.tools/go${go}/go/bin/go.exe`),
      ["build", "-mod=readonly", "-trimpath", "-o", hashExe, "."],
      {
        cwd: resolve(root, "tests/registry"),
        extraEnv: {
          GOTOOLCHAIN: "local",
          GOFLAGS: "",
          GOPROXY: "off",
          GOSUMDB: "off",
          GOOS: "windows",
          GOARCH: "amd64",
          CGO_ENABLED: "0",
        },
      },
    );
    state.hashHelper = {
      binarySha256: hash(hashExe),
      sourceSha256: hash(resolve(root, "tests/registry/main.go")),
      go,
      goModSha256: hash(resolve(root, "tests/registry/go.mod")),
      goSumSha256: hash(resolve(root, "tests/registry/go.sum")),
    };
    const hashed = execute(hashExe, [], { input: password }).stdout.trim();
    assert.match(hashed, /^\$2[aby]\$10\$/);
    const auth = resolve(workspace, "htpasswd");
    writeFileSync(auth, `${state.username}:${hashed}\n`, { flag: "wx" });
    state.authPath = auth;
    const volume = `${name}-data`;
    assert.ok(!state.before.inventories.owned.volumes.includes(volume));
    intent("volume", volume);
    state.volume = cli(
      "volume",
      "create",
      "--label",
      `${label}=${nonce}`,
      volume,
    );
    const inspectedVolume = JSON.parse(
      cli("volume", "inspect", state.volume),
    )[0];
    assert.equal(inspectedVolume.Labels?.[label], nonce);
    assert.equal(inspectedVolume.Driver, "local");
    assert.equal(Object.keys(inspectedVolume.Options ?? {}).length, 0);
    save();
    const common = [
      "--pull=never",
      "--label",
      `${label}=${nonce}`,
      "--cpus=0.25",
      "--memory=128m",
      "--pids-limit=64",
      "--cap-drop=ALL",
      "--security-opt=no-new-privileges:true",
      "--read-only",
    ];
    const probeName = `${name}-probe`;
    intent("container", probeName);
    state.probeContainer = cli(
      "create",
      "--name",
      probeName,
      ...common,
      "--network=none",
      "--mount",
      `type=volume,source=${volume},target=/var/lib/registry,readonly`,
      "--entrypoint",
      "/bin/busybox",
      pins.registry,
      "timeout",
      "-s",
      "KILL",
      "2",
      "/bin/busybox",
      "true",
    );
    save();
    cli("start", "--attach", state.probeContainer);
    assert.equal(ownedContainer(state.probeContainer).State.ExitCode, 0);
    cli("rm", state.probeContainer);
    const entry = [
      ...(registry.Config.Entrypoint ?? []),
      ...(registry.Config.Cmd ?? []),
    ];
    assert.ok(
      entry.length > 0 &&
        entry.length <= 8 &&
        entry.every(
          (v) =>
            typeof v === "string" && v.length < 4096 && !/[\r\n\0]/.test(v),
        ),
    );
    const registryName = `${name}-registry`;
    intent("container", registryName);
    state.registryContainer = cli(
      "create",
      "--name",
      registryName,
      ...common,
      "--publish",
      "127.0.0.1::5000",
      "--mount",
      `type=bind,source=${auth},target=/auth/htpasswd,readonly`,
      "--mount",
      `type=volume,source=${volume},target=/var/lib/registry`,
      "--tmpfs",
      "/tmp:rw,nosuid,noexec,size=16m",
      "--env",
      "REGISTRY_AUTH=htpasswd",
      "--env",
      "REGISTRY_AUTH_HTPASSWD_REALM=OrbWin synthetic qualification",
      "--env",
      "REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd",
      "--env",
      "GOMAXPROCS=2",
      "--env",
      "GOMEMLIMIT=96MiB",
      "--entrypoint",
      "/bin/busybox",
      pins.registry,
      "timeout",
      "-s",
      "KILL",
      "600",
      ...entry,
    );
    save();
    cli("start", state.registryContainer);
    const value = ownedContainer(state.registryContainer);
    const binding = value.NetworkSettings.Ports["5000/tcp"];
    assert.equal(binding.length, 1);
    assert.equal(binding[0].HostIp, "127.0.0.1");
    state.server = `127.0.0.1:${Number(binding[0].HostPort)}`;
    state.image = `${state.server}/fixture/image:${nonce}`;
    save();
    const deadline = Date.now() + 20_000;
    for (;;) {
      try {
        const res = await fetch(`http://${state.server}/v2/`, {
          signal: AbortSignal.timeout(2000),
        });
        assert.equal(res.status, 401);
        break;
      } catch (e) {
        if (Date.now() > deadline) throw e;
        await new Promise((r) => setTimeout(r, 100));
      }
    }
    state.steps.push("loopback registry unauthenticated request refused401");
    save();
    const seedName = `${name}-seed`;
    intent("container", seedName);
    state.seedContainer = cli(
      "create",
      "--name",
      seedName,
      ...common,
      "--network=none",
      pins.base,
    );
    save();
    ownedContainer(state.seedContainer);
    intent("image", state.image);
    const cmd = [
      "/bin/busybox",
      "timeout",
      "-s",
      "KILL",
      "300",
      "/bin/busybox",
      "sleep",
      "300",
    ];
    state.committedId = cli(
      "commit",
      "--change",
      `LABEL ${label}=${nonce}`,
      "--change",
      `CMD ${JSON.stringify(cmd)}`,
      state.seedContainer,
      state.image,
    );
    const committed = inspect("image", state.image);
    assert.equal(committed.Config.Labels?.[label], nonce);
    assert.deepEqual(committed.Config.Cmd, cmd);
    state.committedRootFS = committed.RootFS;
    save();
    cli("rm", state.seedContainer);
    state.devCli = resolve(
      process.env.ORBWIN_QUALIFY_DEVCONTAINERS_CLI ??
        resolve(
          process.env.USERPROFILE,
          ".vscode/extensions/ms-vscode-remote.remote-containers-0.469.0/dist/spec-node/devContainersSpecCLI.js",
        ),
    );
    assert.ok(existsSync(state.devCli));
    state.devCliSha256 = hash(state.devCli);
    state.devClientVersion =
      "VS Code Dev Containers entrypoint; exact file hash recorded; explicit child-only product DOCKER_CONFIG/helper PATH opt-in";
    save();
    const nativeOptions = {
      timeout: 480_000,
      extraEnv: {
        ORBWIN_PRIVATE_CLIENT_STATE: file,
        ORBWIN_QUALIFY_NODE: process.execPath,
      },
    };
    if (state.frozenNativeTest) {
      assert.equal(
        hash(state.frozenNativeTest.path),
        state.frozenNativeTest.sha256,
      );
      execute(
        state.frozenNativeTest.path,
        [
          "owned_private_clients",
          "--ignored",
          "--test-threads=1",
          "--nocapture",
        ],
        nativeOptions,
      );
      assert.equal(
        hash(state.frozenNativeTest.path),
        state.frozenNativeTest.sha256,
      );
    } else
      execute(
        "cargo",
        [
          "test",
          "--manifest-path",
          "src-tauri/Cargo.toml",
          "owned_private_clients",
          "--",
          "--ignored",
          "--test-threads=1",
          "--nocapture",
        ],
        nativeOptions,
      );
    state = JSON.parse(readFileSync(file));
    const native = JSON.parse(
      readFileSync(resolve(folder, "native-credentials.json")),
    );
    assert.equal(native.at(-1)?.success, true);
    state.success = true;
    save();
  } catch (error) {
    state = JSON.parse(readFileSync(file));
    state.errors.push(String(error));
    save();
  } finally {
    try {
      state = JSON.parse(readFileSync(file));
      if (state.before) {
        cleanup();
        state.after = snapshot();
        assert.deepEqual(state.after, state.before);
        state.cleaned = true;
      }
    } catch (error) {
      state.errors.push(`Cleanup/preservation: ${String(error)}`);
    }
    state.retainedArtifacts = [state.folder, state.workspace];
    state.finished = new Date().toISOString();
    save();
    console.log(
      JSON.stringify({
        state: file,
        success: state.success,
        cleaned: state.cleaned,
        errors: state.errors,
      }),
    );
    if (!state.success || !state.cleaned || state.errors.length)
      process.exitCode = 1;
  }
}
