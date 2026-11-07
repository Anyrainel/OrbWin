import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {
  nonce,
  project,
  locations,
  basePath,
  mappingPath,
  baseBytes,
  expectedMapping,
  guardMapping,
  guardEmpty,
  guardFixtureVolume,
  guardFixtureSource,
  retainedBaseline,
  cleanupArguments,
  guardProjectFiles,
  plan,
} from "./qualify-migration-ui-fixture.mjs";
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
test("fixed plan has no initial mapping file and covers cached image volume without anonymous data", () => {
  assert.match(nonce, /^[a-f0-9]{32}$/);
  assert.ok(!plan().locations.folder.includes("p2-migration-ui-"));
  const value = JSON.parse(baseBytes());
  const web = value.services.web;
  assert.equal(web.image, plan().image.id);
  assert.equal(web.pull_policy, "never");
  assert.equal(web.network_mode, "none");
  assert.equal(web.read_only, true);
  assert.deepEqual(web.volumes, [
    {
      type: "volume",
      source: "data",
      target: "/data",
      read_only: true,
      volume: { nocopy: true },
    },
  ]);
  assert.equal(value.volumes.data.name, locations.sourceVolume);
  assert.ok(mappingPath.endsWith(nonce + ".json"));
  assert.equal(plan().mappingInitiallyAbsent, true);
});
test("real UI namespace may vary but only exact canonical production selection and reviewed SHA qualify", () => {
  for (const ns of [namespace, "f".repeat(16)]) {
    const bytes = expectedMapping(ns);
    assert.deepEqual(guardMapping(bytes, sha(bytes)), {
      namespace: ns,
      sha256: sha(bytes),
    });
  }
  const good = expectedMapping(namespace);
  for (const changed of [
    Buffer.concat([good, Buffer.from("\n")]),
    Buffer.from(good.toString().replace(locations.targetVolume, "user-data")),
    Buffer.from(
      good.toString().replace('"read_only": true', '"read_only": false'),
    ),
    Buffer.from(good.toString().replace('"nocopy": true', '"nocopy": false')),
    Buffer.from(
      good.toString().replace('"services": {', '"extra":true,"services": {'),
    ),
  ])
    assert.throws(() => guardMapping(changed, sha(changed)));
  assert.throws(() => guardMapping(good, "0".repeat(64)));
});
test("unknown/nonempty/duplicate or adopted volume accounting is never interpreted as empty", () => {
  const row = { Name: locations.targetVolume, Size: "0B", Links: "0" };
  guardEmpty([row], row.Name);
  guardEmpty([{ ...row, Links: "1" }], row.Name, 1);
  for (const rows of [
    [],
    [row, row],
    [{ ...row, Size: "N/A" }],
    [{ ...row, Size: "1B" }],
    [{ ...row, Size: "0" }],
    [{ ...row, Links: "1" }],
    [{ ...row, Links: "N/A" }],
  ])
    assert.throws(() => guardEmpty(rows, row.Name));
});
test("source and volume cleanup guards refuse identity, adoption, active or resource-bound drift", () => {
  const v = sourceVolume();
  guardFixtureVolume(v, structuredClone(v));
  guardFixtureSource(source(), id, v);
  for (const mutate of [
    (x) => (x.State.Running = true),
    (x) => (x.State.Status = "exited"),
    (x) => (x.Config.Labels["com.docker.compose.project"] = "foreign"),
    (x) => (x.HostConfig.Memory = 0),
    (x) => (x.HostConfig.Privileged = true),
    (x) => (x.HostConfig.Mounts[0].VolumeOptions.NoCopy = false),
    (x) => (x.Mounts[0].RW = true),
    (x) =>
      x.Mounts.push({
        Type: "volume",
        Name: "anonymous",
        Destination: "/other",
      }),
  ]) {
    const bad = source();
    mutate(bad);
    assert.throws(() => guardFixtureSource(bad, id, v));
  }
  for (const mutate of [
    (x) => (x.CreatedAt = "later"),
    (x) => (x.Labels.extra = "adopted"),
    (x) => (x.Mountpoint = "/foreign"),
    (x) => (x.Options.device = "/host"),
  ]) {
    const bad = structuredClone(v);
    mutate(bad);
    assert.throws(() => guardFixtureVolume(bad, v));
  }
});
test("baseline subtraction permits only acknowledged exact fixtures and preserves original inventories", () => {
  const engine = () => ({
    containers: [],
    volumes: [],
    images: [{ id: "image" }],
    networks: [{ id: "network" }],
  });
  const before = {
      configs: [{ sha256: "known" }],
      originals: { entries: ["original"] },
      engines: { owned: engine(), desktop: engine(), rancher: engine() },
    },
    current = structuredClone(before);
  current.engines.desktop.containers.push({ id });
  current.engines.desktop.volumes.push({ name: locations.sourceVolume });
  current.engines.owned.volumes.push({ name: locations.targetVolume });
  const fixture = {
    sourceContainer: id,
    sourceVolume: sourceVolume(),
    targetVolume: { Name: locations.targetVolume },
  };
  retainedBaseline(current, before, fixture);
  for (const mutate of [
    (x) => x.engines.owned.volumes.push({ name: "unacknowledged" }),
    (x) => (x.engines.rancher.images[0].id = "changed"),
    (x) => (x.configs[0].sha256 = "changed"),
    (x) => x.originals.entries.push("unexpected"),
  ]) {
    const bad = structuredClone(current);
    mutate(bad);
    assert.throws(() => retainedBaseline(bad, before, fixture));
  }
});
test("cleanup requires explicit reviewed receipt and mapping hashes, never implicit adoption", () => {
  assert.deepEqual(
    cleanupArguments([
      "--cleanup",
      "--receipt-sha=" + "a".repeat(64),
      "--mapping-sha=absent",
    ]),
    { receiptSha: "a".repeat(64), mappingSha: "absent" },
  );
  for (const args of [
    ["--cleanup"],
    ["--cleanup", "--receipt-sha=" + "a".repeat(64), "--mapping-sha=current"],
    ["--cleanup", "--receipt-sha=bad", "--mapping-sha=absent"],
    ["--cleanup", "--mapping-sha=absent", "--mapping-sha=absent"],
  ])
    assert.throws(() => cleanupArguments(args));
});

test("actual file preflight refuses extras, changed bytes, links and unreviewed UI saves", () => {
  const folder = fs.mkdtempSync(path.join(os.tmpdir(), "orbwin-ui-files-"));
  const base = path.join(folder, "compose.json"),
    mapping = path.join(folder, "mapping.json");
  const identity = (file) => {
    const s = fs.lstatSync(file, { bigint: true });
    return [s.dev, s.ino, s.mode, s.size, s.mtimeNs, s.ctimeNs]
      .map(String)
      .join(":");
  };
  try {
    fs.writeFileSync(base, baseBytes());
    const expected = {
      baseName: "compose.json",
      mappingName: "mapping.json",
      baseIdentity: identity(base),
      baseSha: sha(baseBytes()),
      mappingSha: "absent",
    };
    assert.doesNotThrow(() => guardProjectFiles(folder, expected));
    fs.writeFileSync(path.join(folder, "unexpected"), "keep");
    assert.throws(() => guardProjectFiles(folder, expected));
    fs.unlinkSync(path.join(folder, "unexpected"));
    const bytes = expectedMapping(namespace);
    fs.writeFileSync(mapping, bytes);
    assert.throws(() => guardProjectFiles(folder, expected));
    const saved = { ...expected, mappingSha: sha(bytes) };
    assert.doesNotThrow(() => guardProjectFiles(folder, saved));
    fs.writeFileSync(mapping, "changed");
    assert.throws(() => guardProjectFiles(folder, saved));
    fs.unlinkSync(mapping);
    fs.linkSync(base, mapping);
    assert.throws(() => guardProjectFiles(folder, saved));
    fs.unlinkSync(mapping);
    fs.writeFileSync(base, "changed");
    assert.throws(() => guardProjectFiles(folder, expected));
  } finally {
    assert.equal(path.dirname(path.resolve(folder)), path.resolve(os.tmpdir()));
    assert.ok(
      path.basename(folder).startsWith("orbwin-ui-files-") &&
        !fs.lstatSync(folder).isSymbolicLink(),
    );
    fs.rmSync(folder, { recursive: true });
  }
});
test("default plan dispatches no fixture/native/WSL commands", () => {
  const result = spawnSync(
    process.execPath,
    [
      fileURLToPath(
        new URL("./qualify-migration-ui-fixture.mjs", import.meta.url),
      ),
    ],
    { encoding: "utf8", windowsHide: true },
  );
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(JSON.parse(result.stdout), plan());
});
