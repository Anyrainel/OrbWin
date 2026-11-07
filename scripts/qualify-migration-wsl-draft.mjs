// Source-only: default plan/import performs no filesystem or engine operations.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";
import {
  expectedIDs,
  stableBytes,
  configFingerprint,
  canonicalEngineSnapshot,
  guardVolume,
  guardSource,
  guardCachedImagePair,
  cachedFixtureImage,
  originalTreeReader,
} from "./qualify-migration-draft.mjs";
import {
  qualifyDraftReopen,
  roundTripDraftInputs,
} from "./migration-draft-qualification.mjs";
import { migrationProjectReview } from "../src/model/migration-projects.ts";
import { parseMigrationImages } from "../src/model/migration-images.ts";
import {
  migrationMountDraft,
  reviewMigrationMounts,
} from "../src/model/migration-mounts.ts";
import { wslMigrationDraft } from "../src/model/migration-wsl-draft.ts";
const root = fileURLToPath(new URL("../", import.meta.url)),
  sha = (bytes) => createHash("sha256").update(bytes).digest("hex");
export const pins = {
  registration: "231810b0-fa21-48a8-b0a7-86f9a1360029",
  name: "Ubuntu",
  source: "/home/vanyr/orbwin-native-proof-20261024",
  signature: "2112:1374236",
  uid: 1000,
  nativeSha: "aa4239f86123d529cf2bcca6415364366cf3839b55e97424fb667e58c99912ca",
};
export function fixturePaths(nonce) {
  assert.match(nonce, /^[a-f0-9]{32}$/);
  const project = "orbwin-draft-" + nonce;
  return {
    project,
    child: pins.source + "/" + project,
    base: pins.source + "/" + project + "/compose.json",
    mapping: pins.source + "/" + project + ".json",
  };
}
export function extendedOriginals(originals, receipt, nonce) {
  const p = fixturePaths(nonce),
    names = [
      p.project,
      p.project + "/compose.json",
      p.project + ".json",
    ].sort();
  assert.deepEqual(receipt.entries.map((item) => item.path).sort(), names);
  assert.equal(originals.source, pins.source);
  assert.equal(receipt.signature, originals.signature);
  const value = structuredClone(originals);
  for (const item of receipt.entries) {
    assert.equal(item.type, item.path === p.project ? "directory" : "file");
    assert.equal(item.device, 2112);
    assert.ok(Number.isInteger(item.inode) && item.inode > 0);
    assert.ok(Number.isInteger(item.size) && item.size >= 0);
    if (item.type === "file") {
      assert.ok(item.size <= 262144);
      assert.match(item.sha256, /^[a-f0-9]{64}$/);
    }
    assert.equal(item.uid, 1000);
    assert.equal(item.gid, 1000);
    assert.equal(item.mode, item.type === "directory" ? "0o700" : "0o600");
    assert.ok(!value.entries.some((entry) => entry.path === item.path));
    value.entries.push(item);
  }
  value.topLevel.push(p.project, p.project + ".json");
  value.fileBytes += receipt.entries
    .filter((item) => item.type === "file")
    .reduce((total, item) => total + item.size, 0);
  return value;
}
// Fixture identity comes only from the acknowledged exclusive create receipt.
export function mappingMutationIdentity(receipt, nonce) {
  const expected = fixturePaths(nonce).project + ".json",
    entries = receipt.entries.filter((item) => item.path === expected);
  assert.equal(entries.length, 1);
  const item = entries[0];
  assert.equal(item.type, "file");
  assert.equal(item.device, 2112);
  assert.ok(Number.isInteger(item.inode) && item.inode > 0);
  assert.equal(item.uid, 1000);
  assert.equal(item.gid, 1000);
  assert.equal(item.mode, "0o600");
  return { device: item.device, inode: item.inode };
}
// This exact pure predicate is included by the guest writer and exercised by
// host-only tests; no separate test-only approximation of the guard is used.
export const guestMappingIdentityGuard = String.raw`def assert_mapping_identity(before,expected):
 assert type(expected['device']) is int and type(expected['inode']) is int and expected['inode']>0
 assert before.st_dev==expected['device'] and before.st_ino==expected['inode'], 'mapping replacement retained'
`;
// Creates/removes only the exact nonce child/base and root mapping. No recursive
// deletion, link following, generic path argument, or project/export mutation.
export const guestFixture = String.raw`import os,sys,json,hashlib,stat
v=json.loads(sys.stdin.buffer.read(1048577)); assert len(json.dumps(v))<=1048576
assert os.getuid()==1000 and v['source']=='/home/vanyr/orbwin-native-proof-20261024' and v['signature']=='2112:1374236'
nonce=v['nonce']; assert len(nonce)==32 and all(c in '0123456789abcdef' for c in nonce)
name='orbwin-draft-'+nonce; mapping=name+'.json'; root=os.open(v['source'],os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
${guestMappingIdentityGuard}
def sig(s): return (s.st_dev,s.st_ino,s.st_mode,s.st_uid,s.st_gid,s.st_size,s.st_mtime_ns,s.st_ctime_ns)
def rootcheck():
 s=os.fstat(root); n=os.stat(v['source'],follow_symlinks=False); assert s.st_dev==2112 and s.st_ino==1374236 and sig(s)==sig(n)
def read(parent,file,expected):
 before=os.stat(file,dir_fd=parent,follow_symlinks=False); assert stat.S_ISREG(before.st_mode) and before.st_uid==1000 and stat.S_IMODE(before.st_mode)==0o600 and before.st_nlink==1 and before.st_size<=262144
 fd=os.open(file,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=parent)
 try:
  assert sig(os.fstat(fd))==sig(before); h=hashlib.sha256(); total=0
  while True:
   block=os.read(fd,65536)
   if not block: break
   total+=len(block); assert total<=262144; h.update(block)
  assert total==before.st_size and sig(os.fstat(fd))==sig(before) and sig(os.stat(file,dir_fd=parent,follow_symlinks=False))==sig(before)
  assert h.hexdigest()==expected; return before
 finally: os.close(fd)
def write_new(parent,file,contents):
 data=contents.encode(); assert len(data)<=262144
 fd=os.open(file,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600,dir_fd=parent)
 try:
  offset=0
  while offset<len(data): offset+=os.write(fd,data[offset:])
  os.fsync(fd)
 finally: os.close(fd)
 os.fsync(parent)
def entry(parent,file,relative):
 s=os.stat(file,dir_fd=parent,follow_symlinks=False); result={'path':relative,'uid':s.st_uid,'gid':s.st_gid,'mode':oct(stat.S_IMODE(s.st_mode)),'device':s.st_dev,'inode':s.st_ino,'size':s.st_size,'type':'directory' if stat.S_ISDIR(s.st_mode) else 'file'}
 if result['type']=='file': result['sha256']=v['baseSha'] if file=='compose.json' else v['mappingSha']; read(parent,file,result['sha256'])
 return result
try:
 rootcheck(); mode=v['mode']
 if mode=='create':
  os.mkdir(name,0o700,dir_fd=root); os.fsync(root); child=os.open(name,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=root)
  try:
   write_new(child,'compose.json',v['base']); write_new(root,mapping,v['mapping']); rootcheck(); print(json.dumps({'signature':v['signature'],'entries':[entry(root,name,name),entry(child,'compose.json',name+'/compose.json'),entry(root,mapping,mapping)]}))
  finally: os.close(child)
 elif mode in ('change','restore'):
  before=read(root,mapping,v['expectedSha']); assert_mapping_identity(before,v['mappingIdentity']); data=v['contents'].encode(); assert len(data)<=262144; fd=os.open(mapping,os.O_RDWR|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=root)
  try:
   assert sig(os.fstat(fd))==sig(before); os.ftruncate(fd,0); offset=0
   while offset<len(data): offset+=os.write(fd,data[offset:])
   os.fsync(fd); after=os.fstat(fd); named=os.stat(mapping,dir_fd=root,follow_symlinks=False); assert after.st_dev==before.st_dev and after.st_ino==before.st_ino and sig(after)==sig(named)
  finally: os.close(fd)
  read(root,mapping,hashlib.sha256(data).hexdigest()); rootcheck(); print(json.dumps({'sha256':hashlib.sha256(data).hexdigest()}))
 elif mode=='cleanup':
  known={item['path']:item for item in v['receipt']['entries']}; child=os.open(name,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=root)
  try:
   childstat=os.fstat(child); s=childstat; assert s.st_dev==known[name]['device'] and s.st_ino==known[name]['inode'] and s.st_uid==1000 and stat.S_IMODE(s.st_mode)==0o700
   assert os.listdir(child)==['compose.json']
   for parent,file,relative,digest in [(child,'compose.json',name+'/compose.json',v['baseSha']),(root,mapping,mapping,v['mappingSha'])]:
    s=read(parent,file,digest); assert s.st_dev==known[relative]['device'] and s.st_ino==known[relative]['inode']
   named=os.stat(name,dir_fd=root,follow_symlinks=False); assert named.st_dev==childstat.st_dev and named.st_ino==childstat.st_ino
   rootcheck(); read(child,'compose.json',v['baseSha']); read(root,mapping,v['mappingSha']); os.unlink('compose.json',dir_fd=child); os.fsync(child); os.unlink(mapping,dir_fd=root); os.fsync(root)
  finally: os.close(child)
  os.rmdir(name,dir_fd=root); os.fsync(root); rootcheck(); print(json.dumps({'cleaned':True}))
 else: raise ValueError('unsupported fixture action')
finally: os.close(root)
`;
const lines = (text) => text.trim().split(/\r?\n/).filter(Boolean).sort();
export function runWsl({ nonce, native, nativeSha = pins.nativeSha }) {
  assert.equal(process.platform, "win32");
  for (const key of [
    "ORBWIN_QUALIFY_OWNED",
    "ORBWIN_QUALIFY_DRAFT",
    "ORBWIN_QUALIFY_WSL_DRAFT_RUN",
  ])
    assert.equal(process.env[key], "1");
  assert.match(nativeSha, /^[a-f0-9]{64}$/);
  const p = fixturePaths(nonce),
    folder = path.resolve(root, "artifacts/qualification", p.project);
  assert.equal(fs.realpathSync(native), path.resolve(native));
  assert.ok(
    path
      .resolve(native)
      .startsWith(path.resolve(root, "artifacts/qualification") + path.sep),
  );
  assert.equal(sha(stableBytes(native, 64 << 20)), nativeSha);
  fs.mkdirSync(folder);
  assert.equal(fs.realpathSync(folder), folder);
  const proofFD = fs.openSync(path.join(folder, "proof.json"), "wx"),
    journalFD = fs.openSync(path.join(folder, "intent.jsonl"), "wx");
  const proof = {
    schema: 1,
    nonce,
    scope:
      "WSL Validate/reopen serialization only; not Save-helper/UI/app-restart/Up",
    nativeSha,
    intents: [],
    steps: [],
    fixture: {},
    cleanupErrors: [],
    success: false,
    started: new Date().toISOString(),
  };
  const save = () => {
    const b = Buffer.from(JSON.stringify(proof, null, 2));
    fs.ftruncateSync(proofFD, 0);
    fs.writeSync(proofFD, b, 0, b.length, 0);
    fs.fsyncSync(proofFD);
  };
  const intent = (kind, name) => {
    const value = { kind, name, time: new Date().toISOString() };
    fs.writeSync(journalFD, JSON.stringify(value) + "\n");
    fs.fsyncSync(journalFD);
    proof.intents.push(value);
    save();
  };
  const write = (file, contents) => {
    intent("create-exclusive-artifact", file);
    const fd = fs.openSync(file, "wx");
    try {
      fs.writeSync(fd, contents);
      fs.fsyncSync(fd);
    } finally {
      fs.closeSync(fd);
    }
  };
  const config = path.join(folder, "anonymous-docker");
  fs.mkdirSync(config);
  write(path.join(config, "config.json"), Buffer.from("{}"));
  const docker = path.resolve(root, "artifacts/runtime-tools/bin/docker.exe"),
    linuxDocker = path.resolve(root, "artifacts/runtime-tools/linux/docker"),
    wsl = path.resolve(process.env.SystemRoot, "System32/wsl.exe"),
    linux = (file) =>
      "/mnt/" +
      file[0].toLowerCase() +
      "/" +
      file.slice(3).replaceAll("\\", "/");
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
  let cleanupEnds = 0,
    uncertain = false,
    sequence = 0,
    activeOriginals,
    guestReceipt;
  const execute = (binary, args, { input, timeout = 45000 } = {}) => {
    const left = (cleanupEnds || deadline) - Date.now();
    assert.ok(left > 0, "WSL fixture deadline reached");
    const result = spawnSync(binary, args, {
      cwd: root,
      env,
      input,
      encoding: "utf8",
      windowsHide: true,
      timeout: Math.min(left, timeout),
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
      throw Error("lost/timed-out client; retain uncertain namespace");
    }
    assert.equal(result.status, 0, result.stderr);
    return result.stdout.trim();
  };
  const guid = "{" + pins.registration + "}";
  const guest = (script, input) =>
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
        script,
      ],
      { input: JSON.stringify(input), timeout: 15000 },
    );
  const clients = {
    owned: (args) =>
      execute(docker, [
        "--config",
        config,
        "--host",
        "npipe:////./pipe/docker-rancher-desktop-orbwin",
        ...args,
      ]),
    desktop: (args) =>
      execute(docker, [
        "--config",
        config,
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
        linux(linuxDocker),
        "--config",
        linux(config),
        "--host",
        "unix:///mnt/wsl/rancher-desktop/run/docker.sock",
        ...args,
      ]),
  };
  const raw = (engine, args) => clients[engine](args),
    json = (engine, args) => JSON.parse(raw(engine, args)),
    inspect = (engine, kind, id) => json(engine, [kind, "inspect", id])[0],
    mutate = (engine, args) => {
      assert.equal(
        raw(engine, ["info", "--format={{.ID}}"]),
        expectedIDs[engine],
      );
      return raw(engine, args);
    };
  const originals = JSON.parse(
    stableBytes(
      path.resolve(
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
  activeOriginals = originals;
  const home = path.resolve(process.env.LOCALAPPDATA, "OrbWinData/home"),
    configs = [
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
    const engines = {},
      ids = {};
    for (const engine of Object.keys(clients)) {
      const info = json(engine, ["info", "--format={{json .}}"]);
      assert.equal(info.ID, expectedIDs[engine]);
      assert.equal(info.OSType, "linux");
      ids[engine] = info.ID;
      const cs = lines(raw(engine, ["ps", "-aq", "--no-trunc"])),
        vs = lines(raw(engine, ["volume", "ls", "-q"])),
        ns = lines(raw(engine, ["network", "ls", "-q", "--no-trunc"])),
        is = [
          ...new Set(lines(raw(engine, ["image", "ls", "-aq", "--no-trunc"]))),
        ];
      assert.ok(
        cs.length <= 256 &&
          vs.length <= 256 &&
          ns.length <= 256 &&
          is.length <= 512,
      );
      engines[engine] = canonicalEngineSnapshot({
        containers: (cs.length
          ? json(engine, ["container", "inspect", ...cs])
          : []
        ).map((c) => ({
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
        volumes: (vs.length
          ? json(engine, ["volume", "inspect", ...vs])
          : []
        ).map((v) => ({
          name: v.Name,
          created: v.CreatedAt,
          driver: v.Driver,
          scope: v.Scope,
          mountpoint: v.Mountpoint,
          labelsSha: sha(JSON.stringify(v.Labels)),
          optionsSha: sha(JSON.stringify(v.Options)),
        })),
        images: (is.length
          ? json(engine, ["image", "inspect", ...is])
          : []
        ).map((v) => ({
          id: v.Id,
          repoTags: [...(v.RepoTags ?? [])].sort(),
          repoDigests: [...(v.RepoDigests ?? [])].sort(),
          created: v.Created,
          rootfsSha: sha(JSON.stringify(v.RootFS)),
          configSha: sha(JSON.stringify(v.Config)),
        })),
        networks: (ns.length
          ? json(engine, ["network", "inspect", ...ns])
          : []
        ).map((n) => ({
          id: n.Id,
          name: n.Name,
          driver: n.Driver,
          scope: n.Scope,
          ipamSha: sha(JSON.stringify(n.IPAM)),
          optionsSha: sha(JSON.stringify(n.Options)),
          labelsSha: sha(JSON.stringify(n.Labels)),
          containers: n.Containers,
        })),
      });
    }
    return {
      ids,
      engines,
      configs: configs.map((file) => {
        const value = configFingerprint(file);
        assert.ok(
          value.exists ||
            file === path.resolve(process.env.USERPROFILE, ".wslconfig"),
        );
        return value;
      }),
      originals: JSON.parse(guest(originalTreeReader, activeOriginals)),
    };
  };
  const nativeStep = (step) => {
    assert.equal(sha(stableBytes(native, 64 << 20)), nativeSha);
    const file = path.join(
      folder,
      "step-" + String(++sequence).padStart(4, "0") + ".request.json",
    );
    write(
      file,
      Buffer.from(
        JSON.stringify({
          schema: 1,
          nonce,
          expectedTarget: expectedIDs.owned,
          step,
        }),
      ),
    );
    const saved = { ...env };
    Object.assign(env, {
      ORBWIN_QUALIFY_OWNED: "1",
      ORBWIN_QUALIFY_DRAFT: "1",
      ORBWIN_QUALIFY_DRAFT_NONCE: nonce,
      ORBWIN_QUALIFY_DRAFT_REQUEST: file,
      ORBWIN_QUALIFY_DRAFT_REQUEST_SHA256: sha(stableBytes(file, 1 << 20)),
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
      Object.assign(env, saved);
    }
    const value = JSON.parse(
      stableBytes(file.replace(/\.json$/, ".response.json"), 4 << 20),
    );
    assert.equal(value.nonce, nonce);
    assert.equal(value.targetAfter, expectedIDs.owned);
    if (value.result.Err) throw Error(value.result.Err);
    assert.ok(value.result.Ok);
    return value.result.Ok;
  };
  const sourceName = p.project + "-source",
    destination = p.project + "-destination";
  let sourceVolume,
    targetVolume,
    sourceContainer,
    imageID,
    base,
    mapping,
    mapSha,
    changedSha;
  const verifyConnection = () =>
    nativeStep({
      kind: "inspectConnection",
      name: pins.name,
      registration: pins.registration,
      source: pins.source,
      signature: pins.signature,
    });
  const guestCall = (mode, extra = {}) => {
    verifyConnection();
    return JSON.parse(
      guest(guestFixture, {
        mode,
        nonce,
        source: pins.source,
        signature: pins.signature,
        ...extra,
      }),
    );
  };
  const checkSource = () =>
    guardSource(inspect("desktop", "container", sourceContainer), {
      id: sourceContainer,
      name: sourceName,
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
    checkSource();
    intent("remove-stopped-source", sourceContainer);
    mutate("desktop", ["rm", sourceContainer]);
    sourceContainer = undefined;
    proof.fixture.sourceRemoved = true;
    save();
  };
  const preserve = (current) => {
    assert.deepEqual(current.ids, proof.before.ids);
    assert.deepEqual(current.configs, proof.before.configs);
    for (const engine of Object.keys(clients)) {
      for (const kind of ["containers", "volumes", "networks"])
        for (const original of proof.before.engines[engine][kind])
          assert.deepEqual(
            current.engines[engine][kind].find(
              (v) => (v.id ?? v.name) === (original.id ?? original.name),
            ),
            original,
          );
      assert.deepEqual(
        current.engines[engine].images,
        proof.before.engines[engine].images,
      );
    }
  };
  return (async () => {
    try {
      proof.before = snapshot();
      proof.toolHashes = {
        sharedWindowsHarness: sha(
          stableBytes(
            path.resolve(root, "scripts/qualify-migration-draft.mjs"),
            1 << 20,
          ),
        ),
        guestScript: sha(guestFixture),
        originalsReader: sha(originalTreeReader),
        originalsBaseline: sha(
          stableBytes(
            path.resolve(
              root,
              "artifacts/qualification/p2-migration-draft-wsl-originals-20261025.json",
            ),
            65536,
          ),
        ),
      };
      for (const [name, file, limit] of [
        ["docker", docker, 128 << 20],
        ["linuxDocker", linuxDocker, 128 << 20],
        [
          "sdk",
          path.resolve(root, "artifacts/backend/orbwin-session.exe"),
          128 << 20,
        ],
        [
          "project",
          path.resolve(root, "artifacts/backend/orbwin-project"),
          64 << 20,
        ],
      ])
        proof.toolHashes[name] = sha(stableBytes(file, limit));
      const lock = JSON.parse(
        stableBytes(path.resolve(root, "runtime-tools.lock.json"), 1 << 20),
      );
      for (const [name, file] of [
        ["docker", "bin/docker.exe"],
        ["linuxDocker", "linux/docker"],
      ])
        assert.equal(
          proof.toolHashes[name],
          lock.tools.find((v) => v.path === file)?.sha256,
        );
      for (const [name, key] of [
        ["session", "sdk"],
        ["project", "project"],
      ]) {
        const bytes = stableBytes(
          path.resolve(root, "artifacts/backend/" + name + "-manifest.json"),
          2 << 20,
        );
        assert.equal(JSON.parse(bytes).binarySha256, proof.toolHashes[key]);
        proof.toolHashes[name + "Manifest"] = sha(bytes);
      }
      imageID = guardCachedImagePair(
        inspect("desktop", "image", cachedFixtureImage.id),
        inspect("owned", "image", cachedFixtureImage.id),
      );
      proof.fixture.imageID = imageID;
      save();
      nativeStep({
        kind: "inspectConnection",
        name: "Ubuntu",
        registration: pins.registration,
        source: pins.source,
        signature: pins.signature,
      });
      for (const [engine, name] of [
        ["desktop", sourceName],
        ["owned", destination],
      ]) {
        assert.ok(
          !proof.before.engines[engine].volumes.some((v) => v.name === name),
        );
        intent("create-volume", engine + ":" + name);
        mutate(engine, [
          "volume",
          "create",
          "--label",
          "dev.orbwin.qualification=" + nonce,
          name,
        ]);
        const value = guardVolume(inspect(engine, "volume", name), {
          name,
          nonce,
        });
        if (engine === "desktop") sourceVolume = value;
        else targetVolume = value;
        proof.fixture[engine + "Volume"] = value;
        save();
      }
      assert.ok(
        !proof.before.engines.desktop.containers.some(
          (v) => v.name === "/" + sourceName,
        ),
      );
      intent("create-stopped-source", sourceName);
      sourceContainer = mutate("desktop", [
        "create",
        "--pull=never",
        "--name",
        sourceName,
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
        "dev.orbwin.qualification=" + nonce,
        "--label",
        "com.docker.compose.project=" + p.project,
        "--label",
        "com.docker.compose.service=web",
        "--label",
        "com.docker.compose.project.config_files=" + p.base,
        "--label",
        "com.docker.compose.project.working_dir=" + p.child,
        "--mount",
        "type=volume,source=" +
          sourceName +
          ",target=/data,readonly,volume-nocopy",
        imageID,
        "true",
      ]);
      checkSource();
      assert.ok(
        !proof.before.engines.desktop.containers.some(
          (v) => v.id === sourceContainer,
        ),
      );
      proof.fixture.sourceContainer = sourceContainer;
      save();
      const readSource = async () => {
        const result = nativeStep({ kind: "reviewSource" });
        assert.equal(result.success, true);
        assert.equal(result.mutation, false);
        const value = parseMigrationImages(result.stdout);
        assert.equal(value.source.kind, "docker-desktop");
        assert.equal(value.source.daemon, expectedIDs.desktop);
        assert.equal(value.target, expectedIDs.owned);
        if (
          !sourceContainer &&
          !value.projectMembers.some((v) => v.project === p.project)
        )
          proof.staleSourceConfirmed = true;
        return result.stdout;
      };
      const review = migrationProjectReview(
        parseMigrationImages(await readSource()),
        p.project,
      );
      base = JSON.stringify({
        services: {
          web: {
            image: imageID,
            entrypoint: ["/bin/true"],
            command: ["true"],
            pull_policy: "never",
            network_mode: "none",
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
      });
      mapping = migrationMountDraft(
        reviewMigrationMounts(review.members),
        { [sourceName]: destination },
        nonce.slice(0, 16),
        [destination],
      );
      mapSha = sha(mapping);
      intent("create-only-guest-nonce-paths", JSON.stringify(p));
      guestReceipt = guestCall("create", {
        base,
        mapping,
        baseSha: sha(base),
        mappingSha: mapSha,
      });
      activeOriginals = extendedOriginals(originals, guestReceipt, nonce);
      proof.fixture.guest = guestReceipt;
      save();
      const connection = {
          registration: pins.registration,
          source: pins.source,
          signature: pins.signature,
        },
        inputs = {
          kind: "wsl",
          distribution: "Ubuntu",
          connection,
          project: p.project,
          file: p.base,
          overrides: [p.mapping],
          directory: p.child,
          profiles: [],
          services: [],
          migration: wslMigrationDraft(
            review,
            { ...connection, name: "Ubuntu" },
            {
              namespace: nonce.slice(0, 16),
              selections: { [sourceName]: destination },
              file: { path: p.mapping, sha256: mapSha },
            },
          ),
        };
      const stored = roundTripDraftInputs(inputs);
      write(
        path.join(folder, "remembered-inputs.json"),
        Buffer.from(stored.bytes),
      );
      proof.storeEvidence = stored.evidenceKind;
      proof.scenario = await qualifyDraftReopen({
        inputs,
        readSource,
        inspectConnection: async () =>
          nativeStep({
            kind: "inspectConnection",
            name: "Ubuntu",
            registration: pins.registration,
            source: pins.source,
            signature: pins.signature,
          }),
        validate: async (value, mappingReview) => {
          const result = nativeStep({
            kind: "validate",
            request: {
              kind: "wslCompose",
              distribution: "Ubuntu",
              connection: value.connection,
              project: value.project,
              file: value.file,
              overrides: value.overrides,
              directory: value.directory,
              reviewedDaemon: expectedIDs.owned,
              mappingReview,
              action: "validate",
            },
          });
          if (
            !result.success &&
            /saved mapping bytes changed/i.test(result.stdout)
          )
            proof.mappingByteRefusal = true;
          return result;
        },
        changeMapping: async () => {
          changedSha = sha(mapping + "\n");
          intent("edit-only-owned-guest-mapping", p.mapping);
          assert.equal(
            guestCall("change", {
              expectedSha: mapSha,
              mappingIdentity: mappingMutationIdentity(guestReceipt, nonce),
              contents: mapping + "\n",
            }).sha256,
            changedSha,
          );
        },
        restoreMapping: async () => {
          intent("restore-only-known-guest-mapping", p.mapping);
          assert.equal(
            guestCall("restore", {
              expectedSha: changedSha,
              mappingIdentity: mappingMutationIdentity(guestReceipt, nonce),
              contents: mapping,
            }).sha256,
            mapSha,
          );
        },
        removeOwnedSource: async () => removeSource(),
        verifyBaselines: async () => preserve(snapshot()),
        signal: new AbortController().signal,
      });
      assert.equal(proof.mappingByteRefusal, true);
      assert.equal(proof.staleSourceConfirmed, true);
      proof.success = true;
    } catch (error) {
      proof.error = String(error);
    } finally {
      cleanupEnds = Date.now() + 60000;
      const attempt = (fn) => {
        try {
          assert.ok(Date.now() < cleanupEnds);
          fn();
        } catch (error) {
          proof.cleanupErrors.push(String(error));
        }
      };
      if (!uncertain) {
        if (sourceContainer) attempt(removeSource);
        for (const [engine, name, saved] of [
          ["desktop", sourceName, sourceVolume],
          ["owned", destination, targetVolume],
        ])
          if (saved)
            attempt(() => {
              const v = guardVolume(inspect(engine, "volume", name), {
                name,
                nonce,
                createdAt: saved.CreatedAt,
              });
              assert.equal(v.Mountpoint, saved.Mountpoint);
              assert.equal(
                raw(engine, ["ps", "-aq", "--filter", "volume=" + name]),
                "",
              );
              intent("remove-unadopted-volume", engine + ":" + name);
              mutate(engine, ["volume", "rm", name]);
            });
        if (guestReceipt)
          attempt(() => {
            intent("remove-only-receipted-guest-paths", JSON.stringify(p));
            assert.equal(
              guestCall("cleanup", {
                receipt: guestReceipt,
                baseSha: sha(base),
                mappingSha: mapSha,
              }).cleaned,
              true,
            );
            activeOriginals = originals;
          });
      } else
        proof.cleanupErrors.push(
          "Unknown client outcome: retain nonce namespace for reviewed recovery",
        );
      attempt(() => {
        proof.after = snapshot();
        if (proof.before) assert.deepEqual(proof.after, proof.before);
        else {
          // A failed read-only preflight has no complete baseline to compare.
          // Keep the original failure; never present this as preservation proof.
          assert.ok(proof.intents.every((item) => item.kind === "create-exclusive-artifact"));
          proof.preservation = "unqualified: initial read-only snapshot failed";
        }
      });
      proof.success = proof.success && proof.cleanupErrors.length === 0;
      proof.finished = new Date().toISOString();
      save();
      fs.closeSync(proofFD);
      fs.closeSync(journalFD);
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
        scope: "WSL source-only fixture runner",
        requires: [
          "reviewed concrete nonce",
          "root engine lane",
          "exact frozen native test",
          "unchanged connected root/record/originals",
          "reviewed cached Redis ID/config bothengines",
        ],
        pins,
      }),
    );
  else if (mode === "--run")
    runWsl({
      nonce: process.env.ORBWIN_QUALIFY_DRAFT_NONCE,
      native: process.env.ORBWIN_QUALIFY_DRAFT_NATIVE,
      nativeSha:
        process.env.ORBWIN_QUALIFY_DRAFT_NATIVE_SHA256 ?? pins.nativeSha,
    })
      .then((value) => {
        console.log(JSON.stringify(value));
        if (!value.success) process.exitCode = 1;
      })
      .catch((error) => {
        console.error(String(error));
        process.exitCode = 1;
      });
  else throw Error("Use plan or explicitly reviewed --run");
}
