import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createHash } from "node:crypto";
import {
  fixturePaths,
  extendedOriginals,
  pins,
  guestFixture,
  guestMappingIdentityGuard,
  mappingMutationIdentity,
} from "./qualify-migration-wsl-draft.mjs";
const nonce = "a".repeat(32),
  paths = fixturePaths(nonce);
const originals = () => ({
  source: pins.source,
  signature: pins.signature,
  topLevel: ["original"],
  entries: [
    {
      path: "original",
      uid: 0,
      gid: 0,
      mode: "0o644",
      device: 2112,
      inode: 7,
      size: 4,
      type: "file",
      sha256: "f".repeat(64),
    },
  ],
  fileBytes: 4,
});
const receipt = () => ({
  signature: pins.signature,
  entries: [
    {
      path: paths.project,
      uid: 1000,
      gid: 1000,
      mode: "0o700",
      device: 2112,
      inode: 10,
      size: 4096,
      type: "directory",
    },
    {
      path: paths.project + "/compose.json",
      uid: 1000,
      gid: 1000,
      mode: "0o600",
      device: 2112,
      inode: 11,
      size: 10,
      type: "file",
      sha256: "b".repeat(64),
    },
    {
      path: paths.project + ".json",
      uid: 1000,
      gid: 1000,
      mode: "0o600",
      device: 2112,
      inode: 12,
      size: 20,
      type: "file",
      sha256: "c".repeat(64),
    },
  ],
});
test("frozen native shape uses only exact nonce child/base and root mapping", () => {
  assert.equal(paths.base, paths.child + "/compose.json");
  assert.equal(paths.mapping, pins.source + "/" + paths.project + ".json");
  for (const value of [
    "../escape",
    "A".repeat(32),
    "a".repeat(31),
    "a".repeat(33),
  ])
    assert.throws(() => fixturePaths(value));
  assert.match(pins.nativeSha, /^[a-f0-9]{64}$/);
});
test("midrun baseline admits only receipted exact nonce paths and retains original metadata", () => {
  const before = originals(),
    copy = structuredClone(before),
    value = extendedOriginals(before, receipt(), nonce);
  assert.deepEqual(before, copy);
  assert.deepEqual(value.entries[0], copy.entries[0]);
  assert.equal(value.fileBytes, 34);
  assert.equal(value.entries.length, 4);
  assert.ok(value.topLevel.includes(paths.project + ".json"));
});
test("foreign names, links, owners, root signature and unbounded bytes cannot extend baseline", () => {
  for (const change of [
    (v) => (v.entries[0].path = "unknown"),
    (v) => (v.entries[1].path = paths.project + "/../original"),
    (v) => (v.entries[1].type = "symlink"),
    (v) => (v.entries[1].uid = 0),
    (v) => (v.entries[1].mode = "0o666"),
    (v) => (v.entries[1].device = 2113),
    (v) => (v.entries[1].size = 262145),
    (v) => (v.entries[1].sha256 = "wrong"),
    (v) => (v.signature = "0:0"),
  ]) {
    const value = receipt();
    change(value);
    assert.throws(() => extendedOriginals(originals(), value, nonce));
  }
  const before = originals();
  before.source = "/home/foreign";
  assert.throws(() => extendedOriginals(before, receipt(), nonce));
});
test("guest helper parses as Python source without executing or touching WSL", () => {
  const result = spawnSync(
    "python",
    ["-c", "import ast,sys; ast.parse(sys.stdin.read())"],
    {
      input: guestFixture,
      encoding: "utf8",
      windowsHide: true,
      timeout: 10000,
    },
  );
  assert.equal(result.error, undefined);
  assert.equal(result.status, 0, result.stderr);
});

test("change/restore identity derives only from the exact acknowledged mapping receipt", () => {
  const value = receipt(),
    identity = mappingMutationIdentity(value, nonce);
  assert.deepEqual(identity, { device: 2112, inode: 12 });
  for (const change of [
    (v) => v.entries.pop(),
    (v) => v.entries.push({ ...v.entries[2] }),
    (v) => (v.entries[2].inode = 0),
    (v) => (v.entries[2].uid = 0),
    (v) => (v.entries[2].path = "unknown.json"),
  ]) {
    const bad = receipt();
    change(bad);
    assert.throws(() => mappingMutationIdentity(bad, nonce));
  }
});
test("exact guest identity predicate refuses an actual same-byte replacement before writer admission", () => {
  const folder = fs.mkdtempSync(
      path.join(os.tmpdir(), "orbwin-wsl-map-identity-"),
    ),
    file = path.join(folder, "mapping.json"),
    bytes = Buffer.from("same bytes, different file");
  const identity = (stat) => ({
    device: stat.dev.toString(),
    inode: stat.ino.toString(),
  });
  const check = (expected, actual) =>
    spawnSync(
      "python",
      [
        "-c",
        guestMappingIdentityGuard +
          "\nimport json,sys,types\nv=json.load(sys.stdin)\nexpected={key:int(value) for key,value in v['expected'].items()}\nactual=types.SimpleNamespace(st_dev=int(v['actual']['device']),st_ino=int(v['actual']['inode']))\nassert_mapping_identity(actual,expected)\n",
      ],
      {
        input: JSON.stringify({ expected, actual }),
        encoding: "utf8",
        windowsHide: true,
        timeout: 10000,
      },
    );
  try {
    fs.writeFileSync(file, bytes);
    const original = identity(fs.statSync(file, { bigint: true }));
    const accepted = check(original, original);
    assert.equal(accepted.error, undefined);
    assert.equal(accepted.status, 0, accepted.stderr);
    fs.renameSync(file, file + ".original");
    fs.writeFileSync(file, bytes);
    const replacement = identity(fs.statSync(file, { bigint: true }));
    assert.notEqual(replacement.inode, original.inode);
    const hash = (value) => createHash("sha256").update(value).digest("hex");
    assert.equal(
      hash(fs.readFileSync(file)),
      hash(fs.readFileSync(file + ".original")),
    );
    const refused = check(original, replacement);
    assert.equal(refused.error, undefined);
    assert.notEqual(refused.status, 0);
    assert.match(refused.stderr, /mapping replacement retained/);
    assert.deepEqual(fs.readFileSync(file), bytes);
  } finally {
    fs.rmSync(folder, { recursive: true });
  }
});
