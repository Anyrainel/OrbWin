import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import {
  stableBytes,
  guardVolume,
  guardSource,
  expectedIDs,
  configFingerprint,
  canonicalEngineSnapshot,
} from "./qualify-migration-draft.mjs";
const nonce = "a".repeat(32),
  id = "b".repeat(64),
  image = "sha256:" + "c".repeat(64);
function volume() {
  return {
    Name: "fixture",
    Driver: "local",
    Scope: "local",
    Options: {},
    Labels: { "dev.orbwin.qualification": nonce },
    CreatedAt: "2026-10-25T00:00:00Z",
  };
}
function source() {
  return {
    Id: id,
    Name: "/fixture",
    Image: image,
    Config: {
      Image: image,
      Entrypoint: ["/bin/true"],
      Cmd: ["true"],
      Volumes: { "/data": {} },
      Labels: { "dev.orbwin.qualification": nonce },
    },
    State: { Running: false, Paused: false },
    HostConfig: {
      NetworkMode: "none",
      ReadonlyRootfs: true,
      Privileged: false,
    },
    Mounts: [
      {
        Type: "volume",
        Name: "fixture",
        Source: "/engine/fixture",
        Destination: "/data",
        RW: false,
      },
    ],
  };
}
test("bounded stable file reader handles several chunks and refuses oversize/type changes", () => {
  const folder = fs.mkdtempSync(
    path.join(os.tmpdir(), "orbwin-draft-source-test-"),
  );
  try {
    const file = path.join(folder, "file");
    const bytes = Buffer.alloc(160000, 42);
    fs.writeFileSync(file, bytes);
    assert.deepEqual(stableBytes(file, 200000), bytes);
    assert.throws(() => stableBytes(file, 1000));
    assert.throws(() => stableBytes(folder, 200000));
    const read = fs.readSync;
    let changed = false;
    try {
      fs.readSync = (...args) => {
        const count = read(...args);
        if (!changed) {
          changed = true;
          fs.appendFileSync(file, "changed");
        }
        return count;
      };
      assert.throws(() => stableBytes(file, 200000));
    } finally {
      fs.readSync = read;
    }
  } finally {
    fs.rmSync(folder, { recursive: true });
  }
});
test("ownership checks refuse foreign/adopted volumes and altered container mounts/state", () => {
  assert.equal(Object.keys(expectedIDs).length, 3);
  const options = { name: "fixture", nonce, createdAt: volume().CreatedAt };
  assert.doesNotThrow(() => guardVolume(volume(), options));
  for (const mutate of [
    (v) => (v.CreatedAt = "other"),
    (v) => (v.Labels = {}),
    (v) => (v.Options = { device: "user-data" }),
    (v) => (v.Driver = "nfs"),
  ]) {
    const v = volume();
    mutate(v);
    assert.throws(() => guardVolume(v, options));
  }
  const guard = {
    id,
    name: "fixture",
    nonce,
    image,
    volume: "fixture",
    mountpoint: "/engine/fixture",
  };
  assert.doesNotThrow(() => guardSource(source(), guard));
  for (const mutate of [
    (v) => (v.Id = "f".repeat(64)),
    (v) => (v.State.Running = true),
    (v) => (v.Mounts[0].Type = "bind"),
    (v) => (v.Mounts[0].RW = true),
    (v) => (v.Config.Image = "latest"),
    (v) => (v.HostConfig.Privileged = true),
    (v) => (v.Config.Entrypoint = ["docker-entrypoint.sh"]),
    (v) => (v.Config.Volumes = { "/uncovered": {} }),
  ]) {
    const v = source();
    mutate(v);
    assert.throws(() => guardSource(v, guard));
  }
});

test("config snapshots distinguish missing leaf, new presence and changed bytes", () => {
  const folder = fs.mkdtempSync(
    path.join(os.tmpdir(), "orbwin-config-absence-test-"),
  );
  try {
    const file = path.join(folder, "config");
    const absent = configFingerprint(file);
    assert.deepEqual(absent, { file, exists: false });
    fs.writeFileSync(file, "first");
    const present = configFingerprint(file);
    assert.equal(present.exists, true);
    assert.throws(() => assert.deepEqual(present, absent));
    fs.writeFileSync(file, "changed");
    assert.notEqual(configFingerprint(file).sha256, present.sha256);
    assert.throws(
      () => configFingerprint(path.join(folder, "missing-parent", "config")),
      /ENOENT/,
    );
    const lstat = fs.lstatSync;
    try {
      fs.lstatSync = (candidate, ...args) => {
        if (candidate === file) {
          const error = Error("denied");
          error.code = "EACCES";
          throw error;
        }
        return lstat(candidate, ...args);
      };
      assert.throws(() => configFingerprint(file), /denied/);
    } finally {
      fs.lstatSync = lstat;
    }
  } finally {
    fs.rmSync(folder, { recursive: true });
  }
});
test("a leaf appearing between absence probes fails rather than recording false absence", () => {
  const folder = fs.mkdtempSync(
      path.join(os.tmpdir(), "orbwin-config-race-test-"),
    ),
    file = path.join(folder, "config"),
    lstat = fs.lstatSync;
  let probes = 0;
  try {
    fs.lstatSync = (candidate, ...args) => {
      if (candidate === file && ++probes === 2) fs.writeFileSync(file, "new");
      return lstat(candidate, ...args);
    };
    assert.throws(() => configFingerprint(file), /changed|appeared/);
  } finally {
    fs.lstatSync = lstat;
    fs.rmSync(folder, { recursive: true });
  }
});

test("sibling directory activity is harmless but parent object replacement refuses", () => {
  const folder = fs.mkdtempSync(
    path.join(os.tmpdir(), "orbwin-config-parent-test-"),
  );
  const file = path.join(folder, "config"),
    original = fs.lstatSync;
  let probes = 0;
  try {
    fs.lstatSync = (candidate, ...args) => {
      const stat = original(candidate, ...args);
      if (!stat.isDirectory() || !args[0]?.bigint) return stat;
      const changed = Object.create(stat);
      changed.mtimeNs = stat.mtimeNs + BigInt(++probes);
      changed.ctimeNs = stat.ctimeNs + BigInt(probes);
      changed.size = stat.size + BigInt(probes);
      return changed;
    };
    assert.deepEqual(configFingerprint(file), { file, exists: false });
    fs.writeFileSync(file, "retained bytes");
    assert.equal(configFingerprint(file).exists, true);
    probes = 0;
    fs.lstatSync = (candidate, ...args) => {
      const stat = original(candidate, ...args);
      if (candidate !== folder || !args[0]?.bigint || ++probes === 1)
        return stat;
      const changed = Object.create(stat);
      changed.ino = stat.ino + 1n;
      return changed;
    };
    assert.throws(() => configFingerprint(file), /parent changed/);
  } finally {
    fs.lstatSync = original;
    fs.rmSync(folder, { recursive: true });
  }
});

test("unordered resource and mount ordering canonicalizes without hiding real changes", () => {
  const first = {
    containers: [
      {
        id: "b",
        mounts: [
          { Destination: "/z", Source: "/2", RW: false },
          { Destination: "/a", Source: "/1", RW: true },
        ],
      },
      { id: "a", mounts: [] },
    ],
    volumes: [{ name: "b" }, { name: "a" }],
    images: [{ id: "b" }, { id: "a" }],
    networks: [{ id: "b" }, { id: "a" }],
  };
  const reordered = structuredClone(first);
  for (const name of ["containers", "volumes", "images", "networks"])
    reordered[name].reverse();
  reordered.containers.find((c) => c.id === "b").mounts.reverse();
  assert.deepEqual(
    canonicalEngineSnapshot(first),
    canonicalEngineSnapshot(reordered),
  );
  reordered.containers.find((c) => c.id === "b").mounts[0].RW = false;
  assert.notDeepEqual(
    canonicalEngineSnapshot(first),
    canonicalEngineSnapshot(reordered),
  );
  assert.equal(first.containers[0].mounts[0].Destination, "/z");
});

test("omitted image Created survives receipt JSON roundtrip without hiding defined changes", () => {
  const fresh = {
    containers: [],
    volumes: [],
    images: [
      {
        id: "sha256:61c821",
        created: undefined,
        configSha: "config",
        rootfsSha: "layers",
        repoTags: ["rdx-proxy:latest"],
      },
    ],
    networks: [{ id: "network", optionsSha: "options", containers: {} }],
  };
  const receipt = JSON.parse(JSON.stringify(fresh));
  assert.deepEqual(canonicalEngineSnapshot(fresh), receipt);
  const canonical = canonicalEngineSnapshot(fresh);
  assert.deepEqual(
    canonicalEngineSnapshot(JSON.parse(JSON.stringify(canonical))),
    canonical,
  );
  assert.equal(
    Object.hasOwn(fresh.images[0], "created"),
    true,
    "caller is not mutated",
  );
  assert.equal(Object.hasOwn(canonical.images[0], "created"), false);
  for (const mutate of [
    (s) => {
      s.images[0].created = "2026-10-25T00:00:00Z";
    },
    (s) => {
      s.images[0].created = null;
    },
    (s) => {
      s.images[0].configSha = "changed";
    },
    (s) => {
      s.images[0].rootfsSha = "changed";
    },
    (s) => {
      s.images[0].repoTags = ["changed:latest"];
    },
    (s) => {
      s.networks[0].optionsSha = "changed";
    },
  ]) {
    const changed = structuredClone(fresh);
    mutate(changed);
    assert.notDeepEqual(canonicalEngineSnapshot(changed), canonical);
  }
  const dated = structuredClone(fresh);
  dated.images[0].created = "2026-10-25T00:00:00Z";
  const changedDate = structuredClone(dated);
  changedDate.images[0].created = "2026-10-26T00:00:00Z";
  assert.notDeepEqual(
    canonicalEngineSnapshot(dated),
    canonicalEngineSnapshot(changedDate),
  );
  const invalid = structuredClone(fresh);
  invalid.images[0].repoTags = [undefined];
  assert.throws(
    () => canonicalEngineSnapshot(invalid),
    /Undefined snapshot array entry/,
  );
});
