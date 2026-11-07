import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import {
  nonce,
  project,
  locations,
  basePath,
  mappingPath,
  connection,
  baseBytes,
  expectedMapping,
  guardMapping,
  guardEmpty,
  guardFixtureVolume,
  guardFixtureSource,
  retainedBaseline,
  cleanupArguments,
  guardGuestReceipt,
  withGuestFixture,
  withoutGuestFixture,
  guestFixture,
  plan,
} from "./qualify-migration-wsl-ui-fixture.mjs";
const sha = (bytes) => createHash("sha256").update(bytes).digest("hex");
const namespace = "0123456789abcdef";
const sourceVolume = () => ({
  Name: locations.sourceVolume,
  Driver: "local",
  Scope: "local",
  Options: {},
  Labels: { "dev.orbwin.qualification": nonce },
  CreatedAt: "2026-10-26T01:00:00Z",
  Mountpoint: "/var/lib/docker/volumes/fixture/_data",
});
const id = "a".repeat(64);
function source() {
  return {
    Id: id,
    Name: "/" + locations.sourceContainer,
    Image: plan().image.id,
    RestartCount: 0,
    Config: {
      Image: plan().image.id,
      Entrypoint: ["/bin/true"],
      Cmd: ["true"],
      Volumes: { "/data": {} },
      Labels: {
        "dev.orbwin.qualification": nonce,
        "com.docker.compose.project": project,
        "com.docker.compose.service": "web",
        "com.docker.compose.project.config_files": basePath,
        "com.docker.compose.project.working_dir": plan().projectDirectory,
      },
    },
    State: { Status: "created", Running: false, Paused: false },
    HostConfig: {
      NetworkMode: "none",
      ReadonlyRootfs: true,
      Privileged: false,
      Memory: 32 << 20,
      NanoCpus: 100000000,
      PidsLimit: 16,
      CapDrop: ["ALL"],
      SecurityOpt: ["no-new-privileges:true"],
      Mounts: [
        {
          Type: "volume",
          Source: locations.sourceVolume,
          Target: "/data",
          ReadOnly: true,
          VolumeOptions: { NoCopy: true },
        },
      ],
    },
    Mounts: [
      {
        Type: "volume",
        Name: locations.sourceVolume,
        Source: sourceVolume().Mountpoint,
        Destination: "/data",
        RW: false,
      },
    ],
  };
}

const guestReceipt = () => ({
  signature: connection.signature,
  entries: [
    {
      path: project,
      device: 2112,
      inode: 10,
      uid: 1000,
      gid: 1000,
      type: "directory",
      mode: "0o700",
      size: 4096,
    },
    {
      path: project + "/compose.json",
      device: 2112,
      inode: 11,
      uid: 1000,
      gid: 1000,
      type: "file",
      mode: "0o600",
      size: baseBytes().length,
      sha256: sha(baseBytes()),
    },
  ],
});
const mappedReceipt = () => {
  const value = guestReceipt(),
    bytes = expectedMapping(namespace);
  value.entries.push({
    path: project + ".json",
    device: 2112,
    inode: 12,
    uid: 1000,
    gid: 1000,
    type: "file",
    mode: "0o600",
    size: bytes.length,
    sha256: sha(bytes),
  });
  value.mappingBase64 = bytes.toString("base64");
  return value;
};
test("fixed WSL UI plan uses connected root and leaves real mapping Save to UI", () => {
  const p = plan();
  assert.match(nonce, /^[a-f0-9]{32}$/);
  assert.equal(basePath, connection.source + "/" + project + "/compose.json");
  assert.equal(mappingPath, connection.source + "/" + project + ".json");
  assert.equal(p.mappingInitiallyAbsent, true);
  assert.equal(
    p.privateLinuxConfigSha,
    "77c380f608131ab1a9ea7710961205ca9171b65cf6d42f612371b478051a4f50",
  );
  const web = JSON.parse(baseBytes()).services.web;
  assert.equal(web.pull_policy, "never");
  assert.equal(web.network_mode, "none");
  assert.equal(web.read_only, true);
  assert.deepEqual(web.entrypoint, ["/bin/true"]);
  assert.equal(web.volumes.length, 1);
  assert.equal(web.volumes[0].volume.nocopy, true);
  assert.ok(p.optIn.includes("ORBWIN_QUALIFY_MIGRATION_WSL_UI_FIXTURE=1"));
});
test("guest receipt guards exact base identity and root-level mapping bytes/semantics", () => {
  const initial = guestReceipt();
  guardGuestReceipt(initial);
  guardGuestReceipt(mappedReceipt(), initial, sha(expectedMapping(namespace)));
  for (const mutate of [
    (v) => (v.entries[0].inode = 99),
    (v) => (v.entries[1].sha256 = "a".repeat(64)),
    (v) => (v.entries[1].path = "../original.json"),
    (v) => (v.entries[1].mode = "0o644"),
    (v) => (v.entries[0].uid = 0),
    (v) => v.entries.push({ ...v.entries[1], path: "unexpected" }),
  ]) {
    const value = guestReceipt();
    mutate(value);
    assert.throws(() => guardGuestReceipt(value, initial));
  }
  const changed = mappedReceipt();
  changed.entries[2].size++;
  assert.throws(() =>
    guardGuestReceipt(changed, initial, sha(expectedMapping(namespace))),
  );
  const forged = mappedReceipt();
  forged.mappingBase64 = Buffer.from("{}").toString("base64");
  assert.throws(() =>
    guardGuestReceipt(forged, initial, sha(expectedMapping(namespace))),
  );
  const unknown = expectedMapping(namespace)
    .toString()
    .replace(locations.targetVolume, "foreign");
  assert.throws(() =>
    guardMapping(Buffer.from(unknown), sha(Buffer.from(unknown))),
  );
});
test("original tree accounting excludes only exact acknowledged nonce names", () => {
  const originals = {
    source: connection.source,
    signature: connection.signature,
    topLevel: ["original"],
    entries: [
      { path: "original", type: "file", size: 1050, sha256: "original" },
    ],
    fileBytes: 1050,
  };
  const combined = withGuestFixture(originals, mappedReceipt());
  assert.deepEqual(withoutGuestFixture(combined), originals);
  const extra = structuredClone(combined);
  extra.entries.push({ path: project + "-other.json", type: "file", size: 1 });
  extra.fileBytes++;
  assert.notDeepEqual(withoutGuestFixture(extra), originals);
  const changed = structuredClone(combined);
  changed.entries[0].sha256 = "changed";
  assert.notDeepEqual(withoutGuestFixture(changed), originals);
  assert.equal(originals.entries.length, 1);
});
test("source fixture remains stopped immutable-image readonly/no-copy and bounded", () => {
  guardFixtureSource(source(), id, sourceVolume());
  guardFixtureVolume(sourceVolume(), sourceVolume());
  for (const mutate of [
    (v) => (v.State.Status = "exited"),
    (v) => (v.HostConfig.Mounts[0].VolumeOptions.NoCopy = false),
    (v) =>
      (v.Config.Labels["com.docker.compose.project.config_files"] =
        "C:\\foreign.json"),
    (v) => (v.HostConfig.Memory = 0),
  ]) {
    const value = source();
    mutate(value);
    assert.throws(() => guardFixtureSource(value, id, sourceVolume()));
  }
  const renamed = sourceVolume();
  renamed.CreatedAt = "other";
  assert.throws(() => guardFixtureVolume(renamed, sourceVolume()));
  guardEmpty(
    [{ Name: locations.targetVolume, Size: "0B", Links: "0" }],
    locations.targetVolume,
  );
  for (const row of [
    { Size: "1B", Links: "0" },
    { Size: "0B", Links: "1" },
  ])
    assert.throws(() =>
      guardEmpty(
        [{ Name: locations.targetVolume, ...row }],
        locations.targetVolume,
      ),
    );
});
test("cleanup requires reviewed receipt and exact mapping hash or explicit absence", () => {
  const receipt = "a".repeat(64),
    mapping = "b".repeat(64);
  assert.deepEqual(
    cleanupArguments([
      "--cleanup",
      "--receipt-sha=" + receipt,
      "--mapping-sha=" + mapping,
    ]),
    { receiptSha: receipt, mappingSha: mapping },
  );
  assert.equal(
    cleanupArguments([
      "--cleanup",
      "--receipt-sha=" + receipt,
      "--mapping-sha=absent",
    ]).mappingSha,
    "absent",
  );
  assert.throws(() =>
    cleanupArguments(["--cleanup", "--receipt-sha=" + receipt]),
  );
  assert.throws(() =>
    cleanupArguments([
      "--cleanup",
      "--receipt-sha=" + receipt,
      "--mapping-sha=current",
    ]),
  );
});
test("default plan and import do not dispatch WSL or create fixture state", () => {
  const result = spawnSync(
    process.execPath,
    [
      fileURLToPath(
        new URL("./qualify-migration-wsl-ui-fixture.mjs", import.meta.url),
      ),
    ],
    { encoding: "utf8", timeout: 10000, windowsHide: true },
  );
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(JSON.parse(result.stdout).nonce, nonce);
  // Compile the literal guest source, do not execute it or access guest files.
  const syntax = spawnSync(
    "python",
    ["-c", "import sys; compile(sys.stdin.read(), '<frozen-guest>', 'exec')"],
    {
      input: guestFixture,
      encoding: "utf8",
      timeout: 10000,
      windowsHide: true,
    },
  );
  assert.equal(syntax.status, 0, syntax.stderr);
});

test("retained baselines preserve all original engines/configs and reject unrelated additions", () => {
  const engines = Object.fromEntries(
    ["desktop", "owned", "rancher"].map((name) => [
      name,
      { containers: [], volumes: [], images: [{ id: name }], networks: [] },
    ]),
  );
  const originals = {
    source: connection.source,
    signature: connection.signature,
    topLevel: [],
    entries: [],
    fileBytes: 0,
  };
  const before = {
    engines,
    configs: [{ file: "private-wsl", sha256: "77c" }],
    originals,
  };
  const current = structuredClone(before);
  current.originals = withGuestFixture(originals, guestReceipt());
  current.engines.desktop.containers.push({ id });
  current.engines.desktop.volumes.push({ name: locations.sourceVolume });
  current.engines.owned.volumes.push({ name: locations.targetVolume });
  const owned = {
    sourceContainer: id,
    sourceVolume: sourceVolume(),
    targetVolume: { Name: locations.targetVolume },
  };
  retainedBaseline(current, before, owned);
  for (const mutate of [
    (v) => (v.configs[0].sha256 = "changed"),
    (v) => (v.engines.rancher.images[0].id = "changed"),
    (v) => v.engines.owned.volumes.push({ name: "unacknowledged" }),
    (v) =>
      v.originals.entries.push({ path: "unexpected", size: 0, type: "file" }),
  ]) {
    const changed = structuredClone(current);
    mutate(changed);
    assert.throws(() => retainedBaseline(changed, before, owned));
  }
});
