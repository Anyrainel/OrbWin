// SOURCE ONLY until root reviews and grants its serial mutation lane.
// Default/import is I/O-free. This owns nonce fixtures, never a migration job.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { isDeepStrictEqual } from "node:util";
import {
  expectedIDs,
  stableBytes,
  configFingerprint,
  canonicalEngineSnapshot,
  originalTreeReader,
  cachedFixtureImage,
  guardCachedImagePair,
  guardSource,
  guardVolume,
} from "./qualify-migration-draft.mjs";
import { migrationMountDraft } from "../src/model/migration-mounts.ts";

const root = fileURLToPath(new URL("../", import.meta.url));
const sha = (bytes) => createHash("sha256").update(bytes).digest("hex");
const json = (text) => {
  try {
    return JSON.parse(text);
  } catch {
    throw Error("Invalid qualification JSON; raw content withheld.");
  }
};
export const nonce = "1b6f4a402f304a278629ea729b7d5712";
export const project = "orbwin-draft-ui-" + nonce;
export const locations = Object.freeze({
  folder: path.join(root, "artifacts/qualification", project),
  sourceVolume: project + "-source",
  targetVolume: project + "-target",
  sourceContainer: project + "-source",
});
const projectDirectory = path.join(locations.folder, "project");
export const basePath = path.join(projectDirectory, "compose.json");
export const mappingPath = path.join(
  projectDirectory,
  "orbwin-ui-map-" + nonce + ".json",
);
const lockSha =
  "37528c5f2505535c07bb90f65347aca6576be9be7d038b7b091aef85adc7a0e1";
const guid = "{231810b0-fa21-48a8-b0a7-86f9a1360029}";

export function baseBytes() {
  return Buffer.from(
    JSON.stringify(
      {
        services: {
          web: {
            image: cachedFixtureImage.id,
            entrypoint: ["/bin/true"],
            command: ["true"],
            pull_policy: "never",
            network_mode: "none",
            read_only: true,
            volumes: [
              {
                type: "volume",
                source: "data",
                target: "/data",
                read_only: true,
                volume: { nocopy: true },
              },
            ],
          },
        },
        volumes: { data: { external: true, name: locations.sourceVolume } },
      },
      null,
      2,
    ),
  );
}
export function expectedMapping(namespace) {
  return Buffer.from(
    migrationMountDraft(
      [
        {
          service: "web",
          target: "/data",
          sourceVolume: locations.sourceVolume,
          readOnly: true,
        },
      ],
      { [locations.sourceVolume]: locations.targetVolume },
      namespace,
      [locations.targetVolume],
    ).trimEnd(),
  );
}
export function guardMapping(bytes, reviewedSha) {
  assert.match(reviewedSha, /^[a-f0-9]{64}$/);
  assert.equal(
    sha(bytes),
    reviewedSha,
    "UI mapping differs from root's reviewed SHA",
  );
  const value = json(bytes),
    aliases = Object.keys(value.volumes ?? {});
  assert.equal(aliases.length, 1);
  const match = /^orbwin_migration_([a-f0-9]{16})_1$/.exec(aliases[0]);
  assert.ok(match, "UI mapping alias must carry one reviewed namespace");
  assert.ok(
    bytes.equals(expectedMapping(match[1])),
    "UI mapping bytes/semantics differ from the sole fixed production selection; raw content withheld",
  );
  return { namespace: match[1], sha256: reviewedSha };
}
export function guardEmpty(rows, name, references = 0) {
  assert.ok(Array.isArray(rows) && rows.length <= 4096);
  const selected = rows.filter((item) => item.Name === name);
  assert.equal(
    selected.length,
    1,
    "exact one selected volume usage observation required",
  );
  assert.ok(
    selected[0].Size === "0B",
    "nonempty/unsupported volume must be retained",
  );
  assert.ok(
    selected[0].Links === String(references),
    "volume was adopted or usage is unknown",
  );
}
const volumeTuple = (v) => ({
  Name: v.Name,
  Driver: v.Driver,
  Scope: v.Scope,
  Options: v.Options ?? {},
  Labels: v.Labels ?? {},
  CreatedAt: v.CreatedAt,
  Mountpoint: v.Mountpoint,
});
// Ordinary Docker volumes have no immutable ID. These are point-in-time name,
// label, creation-time and mountpoint guards, not proof against indistinguishable
// same-user replacement between the final check and the request.
export function guardFixtureVolume(value, acknowledged) {
  guardVolume(value, {
    name: acknowledged.Name,
    nonce,
    createdAt: acknowledged.CreatedAt,
  });
  assert.ok(
    isDeepStrictEqual(volumeTuple(value), volumeTuple(acknowledged)),
    "acknowledged volume metadata changed; raw labels withheld",
  );
}
export function guardFixtureSource(value, id, sourceVolume) {
  guardSource(value, {
    id,
    name: locations.sourceContainer,
    nonce,
    image: cachedFixtureImage.id,
    volume: locations.sourceVolume,
    mountpoint: sourceVolume.Mountpoint,
  });
  const hc = value.HostConfig;
  assert.equal(
    value.State.Status,
    "created",
    "fixture must never have started",
  );
  assert.equal(value.RestartCount, 0);
  assert.equal(hc.Memory, 32 << 20);
  assert.equal(hc.NanoCpus, 100000000);
  assert.equal(hc.PidsLimit, 16);
  assert.deepEqual(hc.CapDrop, ["ALL"]);
  assert.ok(
    hc.SecurityOpt?.length === 1 &&
      ["no-new-privileges", "no-new-privileges:true"].includes(
        hc.SecurityOpt[0],
      ),
  );
  assert.ok(
    !hc.AutoRemove && ["", "no"].includes(hc.RestartPolicy?.Name ?? ""),
  );
  for (const key of ["Binds", "VolumesFrom", "Devices", "DeviceRequests"])
    assert.ok(!hc[key]?.length);
  assert.ok(
    !Object.keys(hc.PortBindings ?? {}).length &&
      !Object.keys(hc.Tmpfs ?? {}).length,
  );
  assert.equal(hc.Mounts?.length, 1);
  const mount = hc.Mounts[0];
  assert.ok(
    mount.Type === "volume" &&
      mount.Source === locations.sourceVolume &&
      mount.Target === "/data" &&
      mount.ReadOnly === true &&
      mount.VolumeOptions?.NoCopy === true,
    "only the explicit readonly no-copy source mount is allowed",
  );
  assert.ok(
    isDeepStrictEqual(value.Config.Labels, {
      "dev.orbwin.qualification": nonce,
      "com.docker.compose.project": project,
      "com.docker.compose.service": "web",
      "com.docker.compose.project.config_files": basePath,
      "com.docker.compose.project.working_dir": projectDirectory,
    }),
    "fixture source labels changed; raw labels withheld",
  );
}
export function retainedBaseline(current, before, fixture = {}) {
  assert.deepEqual(current.configs, before.configs);
  assert.deepEqual(current.originals, before.originals);
  for (const engine of Object.keys(expectedIDs)) {
    const value = structuredClone(current.engines[engine]);
    if (engine === "desktop") {
      value.containers = value.containers.filter(
        (item) => item.id !== fixture.sourceContainer,
      );
      value.volumes = value.volumes.filter(
        (item) => !fixture.sourceVolume || item.name !== locations.sourceVolume,
      );
    }
    if (engine === "owned")
      value.volumes = value.volumes.filter(
        (item) => !fixture.targetVolume || item.name !== locations.targetVolume,
      );
    assert.deepEqual(
      value,
      before.engines[engine],
      "original engine inventory changed or unacknowledged extra resource appeared",
    );
  }
}
export function cleanupArguments(args) {
  assert.equal(args.length, 3);
  assert.equal(args[0], "--cleanup");
  const receipt = args
    .find((arg) => arg.startsWith("--receipt-sha="))
    ?.slice(14);
  const mapping = args
    .find((arg) => arg.startsWith("--mapping-sha="))
    ?.slice(14);
  assert.match(receipt, /^[a-f0-9]{64}$/);
  assert.ok(mapping === "absent" || /^[a-f0-9]{64}$/.test(mapping ?? ""));
  return { receiptSha: receipt, mappingSha: mapping };
}
const linux = (file) => {
  assert.match(file, /^[A-Za-z]:[\\/]/);
  return (
    "/mnt/" + file[0].toLowerCase() + "/" + file.slice(3).replaceAll("\\", "/")
  );
};
function fileIdentity(file) {
  const s = fs.lstatSync(file, { bigint: true });
  assert.ok(s.isFile() && !s.isSymbolicLink() && s.nlink === 1n);
  return [s.dev, s.ino, s.mode, s.size, s.mtimeNs, s.ctimeNs]
    .map(String)
    .join(":");
}

export function guardProjectFiles(directory, expected) {
  for (const name of [expected.baseName, expected.mappingName])
    assert.ok(
      typeof name === "string" && /^[a-zA-Z0-9_.-]+\.json$/.test(name),
      "only exact fixture JSON basenames may be reviewed",
    );
  const stat = fs.lstatSync(directory);
  assert.ok(stat.isDirectory() && !stat.isSymbolicLink());
  const names =
    expected.mappingSha === "absent"
      ? [expected.baseName]
      : [expected.baseName, expected.mappingName].sort();
  assert.deepEqual(
    fs.readdirSync(directory).sort(),
    names,
    "Unexpected project files must be retained",
  );
  const base = path.join(directory, expected.baseName);
  assert.equal(fileIdentity(base), expected.baseIdentity);
  assert.equal(sha(stableBytes(base, 256 << 10)), expected.baseSha);
  const result = { base: fileIdentity(base) };
  if (expected.mappingSha !== "absent") {
    const mapping = path.join(directory, expected.mappingName);
    const bytes = stableBytes(mapping, 256 << 10);
    result.mapping = fileIdentity(mapping);
    result.review = guardMapping(bytes, expected.mappingSha);
  }
  return result;
}

export function run(mode, reviewed = {}) {
  assert.equal(process.platform, "win32");
  assert.ok(["prepare", "cleanup"].includes(mode));
  for (const key of [
    "ORBWIN_QUALIFY_OWNED",
    "ORBWIN_QUALIFY_MIGRATION_UI_FIXTURE",
  ])
    assert.equal(process.env[key], "1");
  const tools = path.join(root, "artifacts/runtime-tools"),
    lockBytes = stableBytes(
      path.join(root, "runtime-tools.lock.json"),
      1 << 20,
    );
  assert.equal(sha(lockBytes), lockSha);
  const lock = json(lockBytes),
    toolHashes = {};
  for (const relative of ["bin/docker.exe", "linux/docker"]) {
    toolHashes[relative] = sha(
      stableBytes(path.join(tools, relative), 128 << 20),
    );
    assert.equal(
      toolHashes[relative],
      lock.tools.find((item) => item.path === relative)?.sha256,
    );
  }
  const receiptFile = path.join(locations.folder, "prepare.json");
  let prepared;
  if (mode === "prepare") {
    fs.mkdirSync(locations.folder);
    assert.equal(fs.realpathSync(locations.folder), locations.folder);
  } else {
    assert.match(reviewed.receiptSha, /^[a-f0-9]{64}$/);
    const bytes = stableBytes(receiptFile, 8 << 20);
    assert.equal(sha(bytes), reviewed.receiptSha);
    prepared = json(bytes);
    assert.equal(prepared.schema, 1);
    assert.equal(prepared.nonce, nonce);
    assert.equal(prepared.state, "prepared");
    assert.equal(prepared.success, true);
    assert.equal(prepared.baselineChecked, true);
    assert.equal(prepared.uncertain, false);
    assert.deepEqual(prepared.toolHashes, toolHashes);
    assert.equal(prepared.lockSha, lockSha);
    assert.equal(prepared.baseSha, sha(baseBytes()));
  }
  const proofFD = fs.openSync(
      path.join(locations.folder, mode + ".json"),
      "wx",
    ),
    journalFD = fs.openSync(
      path.join(locations.folder, mode + ".intent.jsonl"),
      "wx",
    );
  const proof = {
    schema: 1,
    nonce,
    mode,
    state: "initializing",
    toolHashes,
    lockSha,
    runnerSha: sha(stableBytes(fileURLToPath(import.meta.url), 1 << 20)),
    baseSha: sha(baseBytes()),
    started: new Date().toISOString(),
    steps: [],
    intents: [],
    fixture: prepared?.fixture ?? {},
    uncertain: false,
    errors: [],
    success: false,
  };
  const writeAll = (fd, bytes, position) => {
    if (typeof bytes === "string") bytes = Buffer.from(bytes);
    let at = 0;
    while (at < bytes.length) {
      const count = fs.writeSync(
        fd,
        bytes,
        at,
        bytes.length - at,
        position === undefined ? null : position + at,
      );
      assert.ok(count > 0);
      at += count;
    }
  };
  const save = () => {
    fs.ftruncateSync(proofFD, 0);
    writeAll(proofFD, Buffer.from(JSON.stringify(proof, null, 2)), 0);
    fs.fsyncSync(proofFD);
  };
  const intent = (kind, target) => {
    const value = { kind, target, time: new Date().toISOString() };
    writeAll(journalFD, JSON.stringify(value) + "\n");
    fs.fsyncSync(journalFD);
    proof.intents.push(value);
    save();
  };
  save();
  const anonymous = path.join(locations.folder, "anonymous-docker");
  if (mode === "prepare") {
    intent("create-anonymous-client-config", anonymous);
    fs.mkdirSync(anonymous);
    const fd = fs.openSync(path.join(anonymous, "config.json"), "wx");
    writeAll(fd, "{}");
    fs.fsyncSync(fd);
    fs.closeSync(fd);
  } else
    assert.ok(
      stableBytes(path.join(anonymous, "config.json"), 1024).equals(
        Buffer.from("{}"),
      ),
    );
  const env = { ...process.env, DOCKER_CONFIG: anonymous };
  for (const key of [
    "DOCKER_CONTEXT",
    "DOCKER_HOST",
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
  let deadline = Date.now() + 240000,
    pendingMutation;
  const execute = (binary, args, { input, timeout = 45000 } = {}) => {
    assert.ok(deadline > Date.now(), "cumulative fixture deadline reached");
    const result = spawnSync(binary, args, {
      env,
      cwd: root,
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
      error: result.error?.code,
      stdoutSha: sha(result.stdout ?? ""),
      stderrSha: sha(result.stderr ?? ""),
    });
    save();
    if (result.error || result.signal || result.status !== 0) {
      if (pendingMutation) proof.uncertain = true;
      throw Error(
        "Fixture client failed; retain unacknowledged outcomes. Raw diagnostics withheld.",
      );
    }
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
        guid,
        "--user",
        "root",
        "--exec",
        "/usr/bin/env",
        "-i",
        "PATH=/usr/bin:/bin",
        linux(path.join(tools, "linux/docker")),
        "--config",
        linux(anonymous),
        "--host",
        "unix:///mnt/wsl/rancher-desktop/run/docker.sock",
        ...args,
      ]),
  };
  const read = (engine, args) => clients[engine](args),
    readJSON = (engine, args) => json(read(engine, args));
  const identity = (engine) => {
    const info = readJSON(engine, ["info", "--format={{json .}}"]);
    assert.equal(info.ID, expectedIDs[engine]);
    assert.equal(info.OSType, "linux");
  };
  const mutate = (engine, args) => {
    identity(engine);
    pendingMutation = { engine, args };
    proof.pendingMutation = pendingMutation;
    intent("dispatch-engine-mutation", pendingMutation);
    return read(engine, args);
  };
  const acknowledge = () => {
    pendingMutation = undefined;
    proof.pendingMutation = null;
    save();
  };
  const inspect = (engine, kind, id) => {
    identity(engine);
    return readJSON(engine, [kind, "inspect", id])[0];
  };
  // Read-only engine-wide df (not a helper container). This exact template uses
  // the upstream CLI verbose Volumes context; unknown/missing counters refuse.
  const usageRows = (engine) => {
    identity(engine);
    const text = read(engine, [
      "system",
      "df",
      "--verbose",
      "--format",
      "{{range .Volumes}}{{json .}}{{println}}{{end}}",
    ]);
    const rows = text.split(/\r?\n/).filter(Boolean).map(json);
    assert.ok(
      rows.length <= 4096 &&
        rows.every(
          (row) =>
            typeof row.Name === "string" &&
            typeof row.Size === "string" &&
            typeof row.Links === "string",
        ),
      "pinned CLI volume formatter is unsupported; no missing counter is assumed empty",
    );
    identity(engine);
    return rows;
  };
  const empty = (engine, name, references = 0) =>
    guardEmpty(usageRows(engine), name, references);
  const lines = (text) =>
    [...new Set(text.split(/\r?\n/).filter(Boolean))].sort();
  const engineSnapshot = (engine) => {
    identity(engine);
    const containers = lines(read(engine, ["ps", "-aq", "--no-trunc"])),
      volumes = lines(read(engine, ["volume", "ls", "-q"])),
      images = lines(read(engine, ["image", "ls", "-aq", "--no-trunc"])),
      networks = lines(read(engine, ["network", "ls", "-q", "--no-trunc"]));
    assert.ok(
      containers.length <= 256 &&
        volumes.length <= 256 &&
        images.length <= 512 &&
        networks.length <= 256,
    );
    const many = (kind, ids) =>
      ids.length ? readJSON(engine, [kind, "inspect", ...ids]) : [];
    const result = canonicalEngineSnapshot({
      containers: many("container", containers).map((c) => ({
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
      volumes: many("volume", volumes).map((v) => ({
        name: v.Name,
        created: v.CreatedAt,
        driver: v.Driver,
        scope: v.Scope,
        mountpoint: v.Mountpoint,
        labelsSha: sha(JSON.stringify(v.Labels)),
        optionsSha: sha(JSON.stringify(v.Options)),
      })),
      images: many("image", images).map((v) => ({
        id: v.Id,
        repoTags: [...(v.RepoTags ?? [])].sort(),
        repoDigests: [...(v.RepoDigests ?? [])].sort(),
        created: v.Created,
        rootfsSha: sha(JSON.stringify(v.RootFS)),
        configSha: sha(JSON.stringify(v.Config)),
      })),
      networks: many("network", networks).map((v) => ({
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
    identity(engine);
    return result;
  };
  const home = path.resolve(process.env.LOCALAPPDATA, "OrbWinData/home");
  const configPaths = [
    path.resolve(process.env.USERPROFILE, ".docker/config.json"),
    path.resolve(process.env.USERPROFILE, ".kube/config"),
    path.resolve(process.env.USERPROFILE, ".wslconfig"),
    path.join(home, ".rdorbwin/docker/config.json"),
    path.join(home, ".rdorbwin/wsl-docker/config.json"),
    path.join(
      home,
      ".rdorbwin/wsl-projects/710eb4f4d7f72b0e329a51d30a83377bc7ef6c0cca1a444bd8d2e70e72326e8f.json",
    ),
  ];
  const originalsBytes = stableBytes(
      path.join(
        root,
        "artifacts/qualification/p2-migration-draft-wsl-originals-20261025.json",
      ),
      65536,
    ),
    originals = json(originalsBytes);
  assert.equal(originals.source, "/home/vanyr/orbwin-native-proof-20261024");
  assert.equal(originals.signature, "2112:1374236");
  assert.equal(originals.entries.length, 9);
  assert.equal(originals.fileBytes, 1050);
  proof.originalsBaselineSha = sha(originalsBytes);
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
    for (const file of configPaths)
      capture("config:" + file, () => {
        const item = configFingerprint(file);
        assert.ok(
          item.exists ||
            file === path.resolve(process.env.USERPROFILE, ".wslconfig"),
        );
        value.configs.push(item);
      });
    capture("wsl-originals", () => {
      value.originals = json(
        execute(
          wsl,
          [
            "--distribution-id",
            guid,
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
    value.snapshotErrors = errors;
    assert.equal(
      errors.length,
      0,
      "baseline incomplete; inspect recorded section errors",
    );
    return value;
  };
  const source = () =>
    guardFixtureSource(
      inspect("desktop", "container", proof.fixture.sourceContainer),
      proof.fixture.sourceContainer,
      proof.fixture.sourceVolume,
    );
  const volume = (engine, key) => {
    const saved = proof.fixture[key];
    const actual = inspect(engine, "volume", saved.Name);
    guardFixtureVolume(actual, saved);
    return actual;
  };
  let fileReview;
  const reviewFiles = () =>
    guardProjectFiles(projectDirectory, {
      baseName: path.basename(basePath),
      mappingName: path.basename(mappingPath),
      baseIdentity: prepared.baseIdentity,
      baseSha: prepared.baseSha,
      mappingSha: reviewed.mappingSha,
    });
  try {
    proof.state = "checking-baseline";
    if (mode === "cleanup") fileReview = reviewFiles();
    proof.before = snapshot("before");
    save();
    if (mode === "prepare") {
      for (const engine of ["desktop", "owned"])
        assert.ok(
          !proof.before.engines[engine].volumes.some((v) =>
            [locations.sourceVolume, locations.targetVolume].includes(v.name),
          ),
        );
      assert.ok(
        !proof.before.engines.desktop.containers.some(
          (c) => c.name === "/" + locations.sourceContainer,
        ),
      );
      guardCachedImagePair(
        inspect("desktop", "image", cachedFixtureImage.id),
        inspect("owned", "image", cachedFixtureImage.id),
      );
      // Establish formatter support before any engine fixture mutation.
      for (const engine of ["desktop", "owned"]) usageRows(engine);
      intent("create-exclusive-project-directory", projectDirectory);
      fs.mkdirSync(projectDirectory);
      intent("create-exclusive-base-file", basePath);
      const fd = fs.openSync(basePath, "wx");
      writeAll(fd, baseBytes());
      fs.fsyncSync(fd);
      fs.closeSync(fd);
      proof.baseIdentity = fileIdentity(basePath);
      save();
      assert.equal(
        configFingerprint(mappingPath).exists,
        false,
        "UI mapping must remain absent until actual Save",
      );
      for (const [engine, name, key] of [
        ["desktop", locations.sourceVolume, "sourceVolume"],
        ["owned", locations.targetVolume, "targetVolume"],
      ]) {
        assert.equal(
          mutate(engine, [
            "volume",
            "create",
            "--label",
            "dev.orbwin.qualification=" + nonce,
            name,
          ]),
          name,
        );
        const value = guardVolume(inspect(engine, "volume", name), {
          name,
          nonce,
        });
        assert.ok(
          isDeepStrictEqual(value.Labels, {
            "dev.orbwin.qualification": nonce,
          }),
          "fresh fixture volume has unexpected labels; retain without adoption",
        );
        proof.fixture[key] = volumeTuple(value);
        acknowledge();
        empty(engine, name);
      }
      const args = [
        "create",
        "--pull=never",
        "--name",
        locations.sourceContainer,
        "--entrypoint",
        "/bin/true",
        "--network=none",
        "--read-only",
        "--memory=32m",
        "--cpus=0.1",
        "--pids-limit=16",
        "--cap-drop=ALL",
        "--security-opt=no-new-privileges:true",
      ];
      for (const label of [
        "dev.orbwin.qualification=" + nonce,
        "com.docker.compose.project=" + project,
        "com.docker.compose.service=web",
        "com.docker.compose.project.config_files=" + basePath,
        "com.docker.compose.project.working_dir=" + projectDirectory,
      ])
        args.push("--label", label);
      args.push(
        "--mount",
        "type=volume,source=" +
          locations.sourceVolume +
          ",target=/data,readonly,volume-nocopy",
        cachedFixtureImage.id,
        "true",
      );
      proof.fixture.sourceContainer = mutate("desktop", args);
      assert.match(proof.fixture.sourceContainer, /^[a-f0-9]{64}$/);
      source();
      acknowledge();
      empty("desktop", locations.sourceVolume, 1);
      empty("owned", locations.targetVolume);
      proof.after = snapshot("after");
      retainedBaseline(proof.after, proof.before, proof.fixture);
      assert.equal(sha(stableBytes(basePath, 256 << 10)), proof.baseSha);
      assert.equal(configFingerprint(mappingPath).exists, false);
      proof.state = "prepared";
      proof.success = true;
    } else {
      retainedBaseline(proof.before, prepared.before, proof.fixture);
      const present = proof.before.engines.desktop.containers.some(
        (c) => c.id === proof.fixture.sourceContainer,
      );
      if (present) source();
      volume("desktop", "sourceVolume");
      volume("owned", "targetVolume");
      empty("desktop", locations.sourceVolume, present ? 1 : 0);
      empty("owned", locations.targetVolume);
      assert.deepEqual(reviewFiles(), fileReview);
      if (present) {
        source();
        mutate("desktop", ["rm", proof.fixture.sourceContainer]);
        acknowledge();
      }
      for (const [engine, key] of [
        ["desktop", "sourceVolume"],
        ["owned", "targetVolume"],
      ]) {
        const v = volume(engine, key);
        assert.equal(
          read(engine, ["ps", "-aq", "--filter", "volume=" + v.Name]),
          "",
        );
        empty(engine, v.Name);
        volume(engine, key);
        mutate(engine, ["volume", "rm", v.Name]);
        acknowledge();
      }
      assert.deepEqual(reviewFiles(), fileReview);
      // Stable reads + identity rechecks, not exclusion of hostile same-UID writes
      // between the final check and unlink. No recursive deletion or force flags.
      for (const [file, expected] of [
        [mappingPath, fileReview.mapping],
        [basePath, fileReview.base],
      ])
        if (expected) {
          assert.equal(fileIdentity(file), expected);
          assert.equal(
            sha(stableBytes(file, 256 << 10)),
            file === basePath ? prepared.baseSha : reviewed.mappingSha,
          );
          assert.equal(fileIdentity(file), expected);
          intent("unlink-exact-reviewed-fixture-file", file);
          fs.unlinkSync(file);
        }
      assert.deepEqual(fs.readdirSync(projectDirectory), []);
      intent("remove-empty-owned-project-directory", projectDirectory);
      fs.rmdirSync(projectDirectory);
      proof.state = "cleaned";
      proof.success = true;
    }
  } catch (error) {
    if (pendingMutation) proof.uncertain = true;
    proof.errors.push(String(error.message ?? error));
    proof.state = "retained-review-required";
  } finally {
    deadline = Date.now() + 60000;
    try {
      proof.final = snapshot("final");
      if (mode === "cleanup" && proof.success)
        assert.deepEqual(proof.final, prepared.before);
      else
        retainedBaseline(
          proof.final,
          mode === "prepare" ? proof.before : prepared.before,
          proof.fixture,
        );
      proof.baselineChecked = true;
    } catch (error) {
      proof.errors.push("Final baseline: " + String(error.message ?? error));
      proof.success = false;
    }
    if (proof.errors.length) proof.success = false;
    proof.finished = new Date().toISOString();
    proof.recovery =
      "Retain evidence and uncertain resources. No automatic adoption, restart, forced removal, prune, file restore, or recursive deletion.";
    save();
    fs.closeSync(proofFD);
    fs.closeSync(journalFD);
  }
  return {
    proof: path.join(locations.folder, mode + ".json"),
    success: proof.success,
    state: proof.state,
    uncertain: proof.uncertain,
    errors: proof.errors,
  };
}

export function plan() {
  return {
    nonce,
    project,
    locations,
    projectDirectory,
    basePath,
    mappingPath,
    mappingInitiallyAbsent: true,
    mappingNamespace:
      "Actual UI review UUID-derived namespace; cleanup regenerates exact production selection and requires root-reviewed SHA",
    image: cachedFixtureImage,
    expectedIDs,
    optIn: ["ORBWIN_QUALIFY_OWNED=1", "ORBWIN_QUALIFY_MIGRATION_UI_FIXTURE=1"],
    modes: [
      "--prepare",
      "--cleanup --receipt-sha=<prepare.json SHA> --mapping-sha=<reviewed SHA|absent>",
    ],
    mutations:
      "two labelled empty local volumes; one stopped bounded Desktop container; known nonce project files only",
    excluded:
      "No Up/start/stop/pull/build/reconnect/lifecycle/prune/force; no user project/profile/config writes",
  };
}
if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const args = process.argv.slice(2);
  if (!args.length) console.log(JSON.stringify(plan(), null, 2));
  else {
    const mode = args[0] === "--prepare" ? "prepare" : "cleanup";
    const reviewed =
      mode === "prepare"
        ? (assert.deepEqual(args, ["--prepare"]), {})
        : cleanupArguments(args);
    const result = run(mode, reviewed);
    console.log(JSON.stringify(result, null, 2));
    process.exitCode = result.success ? 0 : 1;
  }
}
