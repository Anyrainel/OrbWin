// SOURCE-ONLY until root reviews a concrete nonce and grants the mutation lane.
// Default `plan` prints a contract and performs no file/engine/native operation.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { isDeepStrictEqual } from "node:util";
import {
  qualifyDraftReopen,
  roundTripDraftInputs,
} from "./migration-draft-qualification.mjs";
import { migrationDraft } from "../src/model/migration-draft.ts";
import { migrationProjectReview } from "../src/model/migration-projects.ts";
import { parseMigrationImages } from "../src/model/migration-images.ts";
import {
  migrationMountDraft,
  reviewMigrationMounts,
} from "../src/model/migration-mounts.ts";
export const expectedIDs = {
  owned: "d9c00194-ea9d-439a-8561-6a0ec580fbd5",
  desktop: "a307ff0e-581d-49d3-8aee-a0636dda9137",
  rancher: "741eb2cb-adf5-4676-bf28-f03f68d54f60",
};
const root = fileURLToPath(new URL("../", import.meta.url));
const sha = (bytes) => createHash("sha256").update(bytes).digest("hex");
function signature(stat) {
  return [stat.dev, stat.ino, stat.mode, stat.size, stat.mtimeNs, stat.ctimeNs]
    .map(String)
    .join(":");
}
function directorySignature(stat) {
  // Sibling activity changes a directory's size/timestamps without redirecting
  // this target. Keep object/type identity, not unrelated directory contents.
  return [stat.dev, stat.ino, stat.mode, stat.birthtimeNs]
    .map(String)
    .join(":");
}
export function stableBytes(file, limit) {
  const absolute = path.resolve(file),
    parsed = path.parse(absolute);
  let component = parsed.root;
  for (const part of absolute.slice(parsed.root.length).split(path.sep)) {
    component = path.join(component, part);
    assert.equal(
      fs.lstatSync(component).isSymbolicLink(),
      false,
      "path links are refused",
    );
  }
  const before = fs.lstatSync(file, { bigint: true });
  assert.ok(before.isFile() && !before.isSymbolicLink());
  assert.ok(before.size <= BigInt(limit));
  const fd = fs.openSync(
    file,
    fs.constants.O_RDONLY | (fs.constants.O_NOFOLLOW ?? 0),
  );
  try {
    const opened = fs.fstatSync(fd, { bigint: true });
    assert.equal(signature(opened), signature(before));
    const chunks = [];
    let total = 0;
    for (;;) {
      const buffer = Buffer.alloc(Math.min(65536, limit + 1 - total));
      const count = fs.readSync(fd, buffer, 0, buffer.length, null);
      if (!count) break;
      chunks.push(buffer.subarray(0, count));
      total += count;
      assert.ok(total <= limit, "file exceeds bounded read");
    }
    assert.equal(
      signature(fs.fstatSync(fd, { bigint: true })),
      signature(opened),
    );
    assert.equal(
      signature(fs.lstatSync(file, { bigint: true })),
      signature(opened),
    );
    return Buffer.concat(chunks, total);
  } finally {
    fs.closeSync(fd);
  }
}
// Absence is a real snapshot state, not an empty hash. Only a missing leaf
// beneath stable, non-link directory ancestors may be recorded as absent.
export function configFingerprint(file, limit = 2 << 20) {
  const absolute = path.resolve(file),
    parsed = path.parse(absolute),
    parents = [];
  let component = parsed.root;
  for (const part of path
    .dirname(absolute)
    .slice(parsed.root.length)
    .split(path.sep)
    .filter(Boolean)) {
    component = path.join(component, part);
    const stat = fs.lstatSync(component, { bigint: true });
    assert.ok(
      stat.isDirectory() && !stat.isSymbolicLink(),
      "config parent must be a non-link directory",
    );
    parents.push({ path: component, signature: directorySignature(stat) });
  }
  const recheckParents = () => {
    for (const parent of parents) {
      const stat = fs.lstatSync(parent.path, { bigint: true });
      assert.ok(stat.isDirectory() && !stat.isSymbolicLink());
      assert.equal(
        directorySignature(stat),
        parent.signature,
        "config parent changed during snapshot",
      );
    }
  };
  let leaf;
  try {
    leaf = fs.lstatSync(absolute, { bigint: true });
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
    recheckParents();
    try {
      fs.lstatSync(absolute, { bigint: true });
      throw Error("config appeared during absence check");
    } catch (recheck) {
      if (recheck.code !== "ENOENT") throw recheck;
    }
    recheckParents();
    return { file: absolute, exists: false };
  }
  assert.ok(
    leaf.isFile() && !leaf.isSymbolicLink(),
    "config leaf must be a regular non-link file",
  );
  const value = {
    file: absolute,
    exists: true,
    sha256: sha(stableBytes(absolute, limit)),
  };
  recheckParents();
  return value;
}

// Docker returns these resource sets and resolved mount observations without an
// ordering contract. Preserve ordered command/env/layer/file-merge arrays.
export function canonicalEngineSnapshot(value) {
  const result = structuredClone(value),
    key = (item) => item.id ?? item.name;
  // Persisted JSON omits undefined object properties (for example an image
  // inspect response without Created). Use that same absence contract before
  // comparing a fresh snapshot to its receipt; retain every defined value.
  // Undefined array entries are not optional fields and must not become null.
  function omitAbsentFields(item) {
    if (item === null || typeof item !== "object") return;
    if (Array.isArray(item)) {
      for (const entry of item) {
        assert.notEqual(entry, undefined, "Undefined snapshot array entry");
        omitAbsentFields(entry);
      }
    } else {
      for (const name of Object.keys(item)) {
        if (item[name] === undefined) delete item[name];
        else omitAbsentFields(item[name]);
      }
    }
  }
  omitAbsentFields(result);
  for (const name of ["containers", "volumes", "images", "networks"])
    result[name].sort((a, b) => String(key(a)).localeCompare(String(key(b))));
  for (const container of result.containers)
    container.mounts.sort((a, b) =>
      JSON.stringify([a.Destination, a.Source, a.Name, a.Type]).localeCompare(
        JSON.stringify([b.Destination, b.Source, b.Name, b.Type]),
      ),
    );
  return result;
}

export function guardVolume(value, { name, nonce, createdAt }) {
  assert.equal(value.Name, name);
  assert.equal(value.Driver, "local");
  assert.equal(value.Scope, "local");
  assert.deepEqual(value.Options ?? {}, {});
  assert.equal(value.Labels?.["dev.orbwin.qualification"], nonce);
  assert.ok(value.CreatedAt);
  if (createdAt) assert.equal(value.CreatedAt, createdAt);
  return value;
}
export const cachedFixtureImage = {
  id: "sha256:02419de7eddf55aa5bcf49efb74e88fa8d931b4d77c07eff8a6b2144472b6952",
  configSha: "ab85bd23d4ca52fc23e5812c185a0dbd8bac789a63e16255326eb7998fae92f4",
};
export function guardCachedImagePair(a, b) {
  for (const value of [a, b]) {
    assert.equal(value.Id, cachedFixtureImage.id);
    assert.equal(value.Os, "linux");
    assert.equal(value.Architecture, "amd64");
    assert.equal(
      sha(JSON.stringify(value.Config)),
      cachedFixtureImage.configSha,
    );
    assert.deepEqual(value.Config.Volumes, { "/data": {} });
    assert.deepEqual(value.Config.Entrypoint, ["docker-entrypoint.sh"]);
    assert.deepEqual(value.Config.Cmd, ["redis-server"]);
    assert.equal(value.RootFS.Layers.length, 8);
  }
  assert.deepEqual(a.RootFS, b.RootFS);
  return cachedFixtureImage.id;
}

export function guardSource(
  value,
  { id, name, nonce, image, volume, mountpoint },
) {
  assert.equal(value.Id, id);
  assert.match(id, /^[a-f0-9]{64}$/);
  assert.equal(value.Name, "/" + name);
  assert.equal(value.Image, image);
  assert.equal(value.Config.Image, image);
  assert.deepEqual(value.Config.Entrypoint, ["/bin/true"]);
  assert.deepEqual(value.Config.Cmd, ["true"]);
  for (const destination of Object.keys(value.Config.Volumes ?? {}))
    assert.ok(
      value.Mounts.some(
        (mount) => mount.Destination === destination && mount.Type === "volume",
      ),
      "image volumes must be explicitly covered",
    );
  assert.equal(value.Config.Labels?.["dev.orbwin.qualification"], nonce);
  assert.equal(value.State.Running, false);
  assert.equal(value.State.Paused, false);
  assert.equal(value.HostConfig.NetworkMode, "none");
  assert.equal(value.HostConfig.ReadonlyRootfs, true);
  assert.equal(value.HostConfig.Privileged, false);
  assert.equal(value.Mounts.length, 1);
  const mount = value.Mounts[0];
  assert.equal(mount.Type, "volume");
  assert.equal(mount.Name, volume);
  assert.equal(mount.Source, mountpoint);
  assert.equal(mount.Destination, "/data");
  assert.equal(mount.RW, false);
  return value;
}
// Fixed read-only helper: exact expected nine-entry tree, no links or unbounded
// read_file shortcut. It does not create files or invoke an engine/project tool.
export const originalTreeReader = String.raw`import os,sys,json,stat,hashlib
assert os.getuid()==1000
expected=json.loads(sys.stdin.buffer.read(65537)); assert len(json.dumps(expected))<65536
root=expected['source']; fd=os.open(root,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
def sig(s): return (s.st_dev,s.st_ino,s.st_mode,s.st_uid,s.st_gid,s.st_size,s.st_mtime_ns,s.st_ctime_ns)
entries=[]; used=0; known={item['path']:item for item in expected['entries']}
def walk(directory,prefix):
 global used
 before=os.fstat(directory)
 names=sorted(os.listdir(directory)); assert len(names)<=256
 for name in names:
  assert name not in ('.','..') and '/' not in name
  relative=prefix+name; s=os.stat(name,dir_fd=directory,follow_symlinks=False)
  item={'path':relative,'uid':s.st_uid,'gid':s.st_gid,'mode':oct(stat.S_IMODE(s.st_mode)),'device':s.st_dev,'inode':s.st_ino,'size':s.st_size}
  assert len(entries)<256 and relative in known
  assert all(known[relative][key]==item[key] for key in item)
  if stat.S_ISDIR(s.st_mode):
   item['type']='directory'; entries.append(item); child=os.open(name,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=directory)
   try: assert sig(os.fstat(child))==sig(s); walk(child,relative+'/')
   finally: os.close(child)
  else:
   assert stat.S_ISREG(s.st_mode) and s.st_size<=2*1024*1024
   item['type']='file'; child=os.open(name,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=directory)
   try:
    assert sig(os.fstat(child))==sig(s); h=hashlib.sha256(); count=0
    while True:
     block=os.read(child,65536)
     if not block: break
     count+=len(block); used+=len(block); assert count<=2*1024*1024 and used<=2*1024*1024; h.update(block)
    assert count==s.st_size and sig(os.fstat(child))==sig(s); item['sha256']=h.hexdigest()
   finally: os.close(child)
   entries.append(item)
  assert sig(os.stat(name,dir_fd=directory,follow_symlinks=False))==sig(s)
 assert sig(os.fstat(directory))==sig(before)
try:
 s=os.fstat(fd); assert str(s.st_dev)+':'+str(s.st_ino)==expected['signature']; top=sorted(os.listdir(fd)); walk(fd,'')
 assert top==sorted(expected['topLevel']); assert sorted(entries,key=lambda x:x['path'])==sorted(expected['entries'],key=lambda x:x['path']); assert used==expected['fileBytes']
 print(json.dumps({'source':root,'signature':expected['signature'],'entries':entries,'topLevel':top,'fileBytes':used},sort_keys=True))
finally: os.close(fd)
`;
function lines(text) {
  return text.trim().split(/\r?\n/).filter(Boolean).sort();
}
export function runWindows({ nonce, native, nativeSha }) {
  assert.equal(process.platform, "win32");
  assert.match(nonce, /^[a-f0-9]{32}$/);
  assert.match(nativeSha, /^[a-f0-9]{64}$/);
  assert.equal(process.env.ORBWIN_QUALIFY_OWNED, "1");
  assert.equal(process.env.ORBWIN_QUALIFY_DRAFT, "1");
  assert.equal(process.env.ORBWIN_QUALIFY_DRAFT_RUN, "1");
  assert.ok(
    path
      .resolve(native)
      .startsWith(path.resolve(root, "artifacts/qualification") + path.sep),
  );
  assert.equal(fs.realpathSync(native), path.resolve(native));
  assert.equal(sha(stableBytes(native, 64 << 20)), nativeSha);
  const project = "orbwin-draft-" + nonce,
    folder = path.resolve(root, "artifacts/qualification", project);
  fs.mkdirSync(folder);
  assert.equal(fs.realpathSync(folder), folder);
  const journalFD = fs.openSync(path.join(folder, "intent.jsonl"), "wx"),
    proofFD = fs.openSync(path.join(folder, "proof.json"), "wx");
  const proof = {
    schema: 1,
    nonce,
    project,
    scope:
      "Windows Validate/reopen serialization only; no Save-dialog/app-restart/Up claim",
    started: new Date().toISOString(),
    nativeSha,
    steps: [],
    intents: [],
    cleanupErrors: [],
    fixture: {},
    fixtureFiles: [],
    success: false,
  };
  const save = () => {
    const bytes = Buffer.from(JSON.stringify(proof, null, 2));
    fs.ftruncateSync(proofFD, 0);
    fs.writeSync(proofFD, bytes, 0, bytes.length, 0);
    fs.fsyncSync(proofFD);
  };
  const intent = (kind, name) => {
    const record = { kind, name, time: new Date().toISOString() };
    fs.writeSync(journalFD, JSON.stringify(record) + "\n");
    fs.fsyncSync(journalFD);
    proof.intents.push(record);
    save();
    return record;
  };
  const writeNewFixture = (file, bytes) => {
    intent("create-exclusive-fixture-file", file);
    const fd = fs.openSync(file, "wx");
    try {
      fs.writeSync(fd, bytes);
      fs.fsyncSync(fd);
    } finally {
      fs.closeSync(fd);
    }
    proof.fixtureFiles.push({
      path: file,
      sha256: sha(stableBytes(file, 4 << 20)),
    });
    save();
  };
  const config = path.join(folder, "anonymous-docker");
  fs.mkdirSync(config);
  writeNewFixture(path.join(config, "config.json"), Buffer.from("{}"));
  const docker = path.resolve(root, "artifacts/runtime-tools/bin/docker.exe"),
    linuxDocker = path.resolve(root, "artifacts/runtime-tools/linux/docker"),
    wsl = path.resolve(process.env.SystemRoot, "System32/wsl.exe");
  const linux = (file) =>
    "/mnt/" + file[0].toLowerCase() + "/" + file.slice(3).replaceAll("\\", "/");
  const env = {
    ...process.env,
    DOCKER_CONFIG: config,
    DOCKER_HOST: "",
    DOCKER_CONTEXT: "",
    DOCKER_TLS_VERIFY: "",
    DOCKER_CERT_PATH: "",
    DOCKER_API_VERSION: "",
    DOCKER_AUTH_CONFIG: "",
    DOCKER_CUSTOM_HEADERS: "",
    BUILDKIT_HOST: "",
    BUILDX_BUILDER: "",
    COMPOSE_FILE: "",
    COMPOSE_PROJECT_NAME: "",
    COMPOSE_ENV_FILES: "",
  };
  const deadline = Date.now() + 180000;
  let cleanupEnds = 0;
  let cleaning = false,
    uncertain = false,
    nativeSequence = 0;
  const execute = (
    binary,
    args,
    { input, allowFailure = false, timeout = 45000 } = {},
  ) => {
    const remaining = cleaning
      ? cleanupEnds - Date.now()
      : deadline - Date.now();
    assert.ok(remaining > 0, "scenario deadline reached");
    const result = spawnSync(binary, args, {
      cwd: root,
      env,
      input,
      encoding: "utf8",
      windowsHide: true,
      timeout: Math.min(timeout, remaining),
      maxBuffer: 24 << 20,
    });
    proof.steps.push({
      binary: path.basename(binary),
      args,
      status: result.status,
      error: result.error?.message,
      stdoutSha: sha(result.stdout ?? ""),
      stderr: result.stderr?.slice(-8192),
    });
    save();
    if (result.error || result.signal) {
      uncertain = true;
      throw Error(
        "client timeout/loss; fixture outcomes uncertain: " +
          String(result.error ?? result.signal),
      );
    }
    if (!allowFailure) assert.equal(result.status, 0, result.stderr);
    return result;
  };
  const clients = {
    owned: (args, opt) =>
      execute(
        docker,
        [
          "--config",
          config,
          "--host",
          "npipe:////./pipe/docker-rancher-desktop-orbwin",
          ...args,
        ],
        opt,
      ),
    desktop: (args, opt) =>
      execute(
        docker,
        [
          "--config",
          config,
          "--host",
          "npipe:////./pipe/dockerDesktopLinuxEngine",
          ...args,
        ],
        opt,
      ),
    rancher: (args, opt) =>
      execute(
        wsl,
        [
          "--distribution-id",
          "{231810b0-fa21-48a8-b0a7-86f9a1360029}",
          "--user",
          "root",
          "--exec",
          "/usr/bin/env",
          "-i",
          "PATH=/usr/bin:/bin",
          linux(linuxDocker),
          "--config",
          linux(config),
          "--host",
          "unix:///mnt/wsl/rancher-desktop/run/docker.sock",
          ...args,
        ],
        opt,
      ),
  };
  const raw = (engine, args, opt) => clients[engine](args, opt).stdout.trim(),
    json = (engine, args) => JSON.parse(raw(engine, args));
  const mutate = (engine, args) => {
    assert.equal(
      raw(engine, ["info", "--format={{.ID}}"]),
      expectedIDs[engine],
    );
    return raw(engine, args);
  };
  const inspect = (engine, kind, id) => json(engine, [kind, "inspect", id])[0];
  const originalsFile = path.resolve(
      root,
      "artifacts/qualification/p2-migration-draft-wsl-originals-20261025.json",
    ),
    originals = JSON.parse(stableBytes(originalsFile, 65536));
  assert.equal(originals.signature, "2112:1374236");
  assert.equal(originals.entries.length, 9);
  assert.equal(originals.fileBytes, 1050);
  const wslOriginals = () => {
    const args = [
      "--distribution-id",
      "{231810b0-fa21-48a8-b0a7-86f9a1360029}",
      "--user",
      "vanyr",
      "--exec",
      "/usr/bin/env",
      "-i",
      "PATH=/usr/bin:/bin",
      "/usr/bin/python3",
      "-c",
      originalTreeReader,
    ];
    const value = JSON.parse(
      execute(wsl, args, { input: JSON.stringify(originals), timeout: 15000 })
        .stdout,
    );
    assert.equal(value.signature, originals.signature);
    return value;
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
  const snapshot = () => {
    const ids = {},
      engines = {};
    for (const engine of Object.keys(clients)) {
      const info = json(engine, ["info", "--format={{json .}}"]);
      ids[engine] = info.ID;
      assert.equal(info.ID, expectedIDs[engine]);
      assert.equal(info.OSType, "linux");
      const containers = lines(raw(engine, ["ps", "-aq", "--no-trunc"])),
        volumes = lines(raw(engine, ["volume", "ls", "-q"])),
        images = lines(raw(engine, ["image", "ls", "-aq", "--no-trunc"])),
        networks = lines(raw(engine, ["network", "ls", "-q", "--no-trunc"]));
      assert.ok(
        containers.length <= 256 &&
          volumes.length <= 256 &&
          images.length <= 512 &&
          networks.length <= 256,
      );
      engines[engine] = canonicalEngineSnapshot({
        containers: (containers.length
          ? json(engine, ["container", "inspect", ...containers])
          : []
        ).map((c) => {
          return {
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
          };
        }),
        volumes: (volumes.length
          ? json(engine, ["volume", "inspect", ...volumes])
          : []
        ).map((v) => {
          return {
            name: v.Name,
            created: v.CreatedAt,
            driver: v.Driver,
            scope: v.Scope,
            mountpoint: v.Mountpoint,
            labelsSha: sha(JSON.stringify(v.Labels)),
            optionsSha: sha(JSON.stringify(v.Options)),
          };
        }),
        images: (images.length
          ? json(engine, ["image", "inspect", ...new Set(images)])
          : []
        )
          .map((value) => ({
            id: value.Id,
            repoTags: [...(value.RepoTags ?? [])].sort(),
            repoDigests: [...(value.RepoDigests ?? [])].sort(),
            created: value.Created,
            rootfsSha: sha(JSON.stringify(value.RootFS)),
            configSha: sha(JSON.stringify(value.Config)),
          }))
          .sort((a, b) => a.id.localeCompare(b.id)),
        networks: (networks.length
          ? json(engine, ["network", "inspect", ...networks])
          : []
        ).map((n) => {
          return {
            id: n.Id,
            name: n.Name,
            driver: n.Driver,
            scope: n.Scope,
            ipamSha: sha(JSON.stringify(n.IPAM)),
            optionsSha: sha(JSON.stringify(n.Options)),
            labelsSha: sha(JSON.stringify(n.Labels)),
            containers: n.Containers,
          };
        }),
      });
    }
    return {
      ids,
      engines,
      configs: configPaths.map((file) => {
        const value = configFingerprint(file);
        assert.ok(
          value.exists ||
            file === path.resolve(process.env.USERPROFILE, ".wslconfig"),
          "unexpected required config absence",
        );
        return value;
      }),
      wslOriginals: wslOriginals(),
    };
  };
  const nativeStep = (step) => {
    assert.equal(sha(stableBytes(native, 64 << 20)), nativeSha);
    const name = `step-${String(++nativeSequence).padStart(4, "0")}.request.json`,
      file = path.join(folder, name),
      request = { schema: 1, nonce, expectedTarget: expectedIDs.owned, step };
    writeNewFixture(file, Buffer.from(JSON.stringify(request)));
    const requestSha = sha(stableBytes(file, 1 << 20));
    const previous = { ...env };
    Object.assign(env, {
      ORBWIN_QUALIFY_OWNED: "1",
      ORBWIN_QUALIFY_DRAFT: "1",
      ORBWIN_QUALIFY_DRAFT_NONCE: nonce,
      ORBWIN_QUALIFY_DRAFT_REQUEST: file,
      ORBWIN_QUALIFY_DRAFT_REQUEST_SHA256: requestSha,
    });
    try {
      execute(native, [
        "--exact",
        "operations::qualification::migration_draft_qualification::owned_migration_draft_step",
        "--ignored",
        "--nocapture",
        "--test-threads=1",
      ]);
    } finally {
      for (const key of Object.keys(env)) delete env[key];
      Object.assign(env, previous);
    }
    const response = JSON.parse(
      stableBytes(file.replace(/\.json$/, ".response.json"), 4 << 20),
    );
    assert.equal(response.nonce, nonce);
    assert.equal(response.targetAfter, expectedIDs.owned);
    if (response.result.Err) throw Error(response.result.Err);
    assert.ok(response.result.Ok);
    return response.result.Ok;
  };
  const sourceName = project + "-source",
    destination = project + "-destination",
    containerName = project + "-source",
    base = path.join(folder, "compose.json"),
    mapping = path.join(folder, "mapping.json");
  let sourceVolume,
    targetVolume,
    sourceContainer,
    imageID,
    baseBytes,
    mapBytes,
    mapSha,
    changedSha;
  const currentSource = () =>
    guardSource(inspect("desktop", "container", sourceContainer), {
      id: sourceContainer,
      name: containerName,
      nonce,
      image: imageID,
      volume: sourceName,
      mountpoint: sourceVolume.Mountpoint,
    });
  const removeSource = () => {
    guardVolume(inspect("desktop", "volume", sourceName), {
      name: sourceName,
      nonce,
      createdAt: sourceVolume.CreatedAt,
    });
    currentSource();
    intent("remove-stopped-source", sourceContainer);
    mutate("desktop", ["rm", sourceContainer]);
    sourceContainer = undefined;
    proof.fixture.sourceRemoved = true;
    save();
  };
  const retainedBaseline = (current) => {
    assert.deepEqual(current.ids, proof.before.ids);
    assert.deepEqual(current.configs, proof.before.configs);
    assert.deepEqual(current.wslOriginals, proof.before.wslOriginals);
    for (const engine of Object.keys(clients)) {
      for (const kind of ["containers", "volumes", "networks"]) {
        for (const original of proof.before.engines[engine][kind])
          assert.deepEqual(
            current.engines[engine][kind].find(
              (v) => (v.id ?? v.name) === (original.id ?? original.name),
            ),
            original,
          );
      }
      assert.deepEqual(
        current.engines[engine].images,
        proof.before.engines[engine].images,
      );
    }
  };
  const safeRewrite = (expected, bytes) => {
    assert.equal(sha(stableBytes(mapping, 256 << 10)), expected);
    intent("replace-only-owned-fixture-mapping", mapping);
    const reviewed = fs.lstatSync(mapping, { bigint: true });
    assert.equal(reviewed.nlink, 1n, "hard-linked fixture mapping retained");
    const fd = fs.openSync(
      mapping,
      fs.constants.O_RDWR | (fs.constants.O_NOFOLLOW ?? 0),
    );
    try {
      assert.equal(
        signature(fs.fstatSync(fd, { bigint: true })),
        signature(fs.lstatSync(mapping, { bigint: true })),
      );
      assert.equal(
        signature(fs.fstatSync(fd, { bigint: true })),
        signature(reviewed),
      );
      const chunks = [];
      let count = 0;
      for (;;) {
        const chunk = Buffer.alloc(65536);
        const got = fs.readSync(fd, chunk, 0, chunk.length, null);
        if (!got) break;
        count += got;
        assert.ok(count <= 256 << 10);
        chunks.push(chunk.subarray(0, got));
      }
      assert.equal(sha(Buffer.concat(chunks)), expected);
      assert.equal(
        signature(fs.fstatSync(fd, { bigint: true })),
        signature(reviewed),
      );
      fs.ftruncateSync(fd, 0);
      fs.writeSync(fd, bytes, 0, bytes.length, 0);
      fs.fsyncSync(fd);
    } finally {
      fs.closeSync(fd);
    }
    assert.equal(sha(stableBytes(mapping, 256 << 10)), sha(bytes));
  };
  return (async () => {
    try {
      proof.before = snapshot();
      proof.toolHashes = {
        docker: sha(stableBytes(docker, 128 << 20)),
        linuxDocker: sha(stableBytes(linuxDocker, 128 << 20)),
        sdk: sha(
          stableBytes(
            path.resolve(root, "artifacts/backend/orbwin-session.exe"),
            128 << 20,
          ),
        ),
        project: sha(
          stableBytes(
            path.resolve(root, "artifacts/backend/orbwin-project"),
            64 << 20,
          ),
        ),
      };
      const lock = JSON.parse(
        stableBytes(path.resolve(root, "runtime-tools.lock.json"), 1 << 20),
      );
      for (const [relative, actual] of [
        ["bin/docker.exe", proof.toolHashes.docker],
        ["linux/docker", proof.toolHashes.linuxDocker],
      ])
        assert.equal(
          actual,
          lock.tools.find((tool) => tool.path === relative)?.sha256,
        );
      for (const [name, key] of [
        ["session", "sdk"],
        ["project", "project"],
      ]) {
        const file = path.resolve(
          root,
          `artifacts/backend/${name}-manifest.json`,
        );
        const bytes = stableBytes(file, 2 << 20);
        assert.equal(JSON.parse(bytes).binarySha256, proof.toolHashes[key]);
        proof.toolHashes[name + "Manifest"] = sha(bytes);
      }
      proof.toolHashes.originalsBaseline = sha(
        stableBytes(originalsFile, 65536),
      );
      proof.toolHashes.runtimeLock = sha(
        stableBytes(path.resolve(root, "runtime-tools.lock.json"), 1 << 20),
      );
      save();
      const a = inspect("desktop", "image", cachedFixtureImage.id),
        b = inspect("owned", "image", cachedFixtureImage.id);
      imageID = guardCachedImagePair(a, b);
      proof.fixture.imageID = imageID;
      proof.fixture.sourceVolumeName = sourceName;
      proof.fixture.targetVolumeName = destination;
      save();
      for (const [engine, name] of [
        ["desktop", sourceName],
        ["owned", destination],
      ]) {
        assert.ok(
          !proof.before.engines[engine].volumes.some((v) => v.name === name),
        );
        intent("create-volume", `${engine}:${name}`);
        mutate(engine, [
          "volume",
          "create",
          "--label",
          `dev.orbwin.qualification=${nonce}`,
          name,
        ]);
        const v = guardVolume(inspect(engine, "volume", name), { name, nonce });
        if (engine === "desktop") sourceVolume = v;
        else targetVolume = v;
        proof.fixture[engine + "Volume"] = v;
        save();
      }
      baseBytes = Buffer.from(
        JSON.stringify(
          {
            services: {
              web: {
                image: imageID,
                entrypoint: ["/bin/true"],
                pull_policy: "never",
                network_mode: "none",
                command: ["true"],
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
            volumes: { data: { external: true, name: sourceName } },
          },
          null,
          2,
        ),
      );
      writeNewFixture(base, baseBytes);
      assert.ok(
        !proof.before.engines.desktop.containers.some(
          (c) => c.name === "/" + containerName,
        ),
      );
      intent("create-stopped-source", containerName);
      sourceContainer = mutate("desktop", [
        "create",
        "--pull=never",
        "--name",
        containerName,
        "--entrypoint",
        "/bin/true",
        "--network=none",
        "--read-only",
        "--memory=32m",
        "--cpus=0.1",
        "--pids-limit=16",
        "--cap-drop=ALL",
        "--security-opt=no-new-privileges:true",
        "--label",
        `dev.orbwin.qualification=${nonce}`,
        "--label",
        `com.docker.compose.project=${project}`,
        "--label",
        "com.docker.compose.service=web",
        "--label",
        `com.docker.compose.project.config_files=${base}`,
        "--label",
        `com.docker.compose.project.working_dir=${folder}`,
        "--mount",
        `type=volume,source=${sourceName},target=/data,readonly,volume-nocopy`,
        imageID,
        "true",
      ]);
      assert.match(sourceContainer, /^[a-f0-9]{64}$/);
      currentSource();
      assert.ok(
        !proof.before.engines.desktop.containers.some(
          (value) => value.id === sourceContainer,
        ),
      );
      proof.fixture.sourceContainer = sourceContainer;
      proof.fixture.imageID = imageID;
      save();
      const readSource = async () => {
        const result = nativeStep({ kind: "reviewSource" });
        assert.equal(result.success, true);
        assert.equal(result.mutation, false);
        const observed = parseMigrationImages(result.stdout);
        assert.equal(observed.source.kind, "docker-desktop");
        assert.equal(observed.source.daemon, expectedIDs.desktop);
        assert.equal(observed.target, expectedIDs.owned);
        if (
          !sourceContainer &&
          !observed.projectMembers.some((member) => member.project === project)
        )
          proof.staleSourceConfirmed = true;
        return result.stdout;
      };
      const review = migrationProjectReview(
        parseMigrationImages(await readSource()),
        project,
      );
      mapBytes = Buffer.from(
        migrationMountDraft(
          reviewMigrationMounts(review.members),
          { [sourceName]: destination },
          nonce.slice(0, 16),
          [destination],
        ),
      );
      mapSha = sha(mapBytes);
      writeNewFixture(mapping, mapBytes);
      proof.mappingEvidence =
        "harness-created narrow mapping; not Save-dialog proof";
      const inputs = {
        project,
        file: base,
        overrides: [mapping],
        directory: folder,
        profiles: [],
        migration: migrationDraft(review, {
          namespace: nonce.slice(0, 16),
          selections: { [sourceName]: destination },
          file: { path: mapping, sha256: mapSha },
        }),
      };
      const stored = roundTripDraftInputs(inputs);
      writeNewFixture(
        path.join(folder, "remembered-inputs.json"),
        Buffer.from(stored.bytes),
      );
      proof.storeEvidence = stored.evidenceKind;
      proof.scenario = await qualifyDraftReopen({
        inputs,
        readSource,
        inspectMapping: async (file, contents) => {
          try {
            return nativeStep({
              kind: "inspectMapping",
              path: file.path,
              sha256: file.sha256,
              contents,
            });
          } catch (error) {
            if (/saved mapping bytes changed/i.test(String(error)))
              proof.mappingByteRefusal = true;
            throw error;
          }
        },
        validate: async (value) =>
          nativeStep({
            kind: "validate",
            request: {
              kind: "compose",
              file: value.file,
              overrides: value.overrides,
              directory: value.directory,
              project: value.project,
              action: "validate",
            },
          }),
        changeMapping: async () => {
          const changed = Buffer.concat([mapBytes, Buffer.from("\n")]);
          changedSha = sha(changed);
          safeRewrite(mapSha, changed);
        },
        restoreMapping: async () => {
          const actual = sha(stableBytes(mapping, 256 << 10));
          if (actual === mapSha) return;
          assert.equal(actual, changedSha, "unknown mapping edit retained");
          safeRewrite(changedSha, mapBytes);
        },
        removeOwnedSource: async () => removeSource(),
        verifyBaselines: async (phase) => {
          const value = snapshot();
          retainedBaseline(value);
          if (phase === "before") assert.ok(sourceContainer);
          assert.equal(sha(stableBytes(base, 256 << 10)), sha(baseBytes));
        },
        signal: new AbortController().signal,
      });
      assert.equal(
        proof.mappingByteRefusal,
        true,
        "specific native mapping-byte refusal was not observed",
      );
      assert.equal(
        proof.staleSourceConfirmed,
        true,
        "successful native review did not confirm the removed source fixture",
      );
      proof.success = true;
    } catch (error) {
      proof.error = String(error);
      proof.success = false;
    } finally {
      cleaning = true;
      const cleanupDeadline = Date.now() + 60000;
      cleanupEnds = cleanupDeadline;
      const cleanupAttempt = (fn) => {
        try {
          assert.ok(Date.now() < cleanupDeadline);
          fn();
        } catch (error) {
          proof.cleanupErrors.push(String(error));
        }
      };
      if (!uncertain) {
        if (sourceContainer) cleanupAttempt(removeSource);
        for (const [engine, name, saved] of [
          ["desktop", sourceName, sourceVolume],
          ["owned", destination, targetVolume],
        ])
          if (saved)
            cleanupAttempt(() => {
              const value = guardVolume(inspect(engine, "volume", name), {
                name,
                nonce,
                createdAt: saved.CreatedAt,
              });
              assert.equal(value.Mountpoint, saved.Mountpoint);
              assert.equal(
                raw(engine, ["ps", "-aq", "--filter", `volume=${name}`]),
                "",
              );
              intent("remove-unadopted-volume", `${engine}:${name}`);
              mutate(engine, ["volume", "rm", name]);
            });
      } else
        proof.cleanupErrors.push(
          "Timed-out/lost client: retain uncertain fixture namespace for reviewed recovery",
        );
      cleanupAttempt(() => {
        proof.after = snapshot();
        assert.ok(
          isDeepStrictEqual(proof.after, proof.before),
          "three-engine/config/WSL baseline mismatch",
        );
        if (baseBytes)
          assert.equal(sha(stableBytes(base, 256 << 10)), sha(baseBytes));
        if (mapBytes)
          assert.equal(sha(stableBytes(mapping, 256 << 10)), mapSha);
      });
      proof.success = proof.success && proof.cleanupErrors.length === 0;
      proof.finished = new Date().toISOString();
      save();
      fs.closeSync(journalFD);
      fs.closeSync(proofFD);
    }
    return {
      folder,
      success: proof.success,
      error: proof.error,
      cleanupErrors: proof.cleanupErrors,
    };
  })();
}
if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const mode = process.argv[2] ?? "plan";
  if (mode === "plan")
    console.log(
      JSON.stringify({
        scope: "Windows-first source runner",
        requires: [
          "reviewed concrete nonce",
          "exact native test binary+SHA",
          "root engine lane",
          "reviewed fixed Redis cached ID/config on Desktop+owned",
          "unchanged supplied WSL originals",
        ],
        wslFixtureRun: "not implemented; separate review required",
        expectedIDs,
      }),
    );
  else if (mode === "--run") {
    assert.equal(process.env.ORBWIN_QUALIFY_DRAFT_RUN, "1");
    runWindows({
      nonce: process.env.ORBWIN_QUALIFY_DRAFT_NONCE,
      native: process.env.ORBWIN_QUALIFY_DRAFT_NATIVE,
      nativeSha: process.env.ORBWIN_QUALIFY_DRAFT_NATIVE_SHA256,
    })
      .then((value) => {
        console.log(JSON.stringify(value));
        if (!value.success) process.exitCode = 1;
      })
      .catch((error) => {
        console.error(String(error));
        process.exitCode = 1;
      });
  } else
    throw Error("Use plan or reviewed --run; WSL mutation is not implemented");
}
