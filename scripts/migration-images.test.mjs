import test from "node:test";
import assert from "node:assert/strict";
import {
  migrationImageExport,
  migrationVolumeExport,
  parseMigrationImages,
  parseMigrationVolumeReview,
  parseMigrationWSLSources,
} from "../src/model/migration-images.ts";
const id = `sha256:${"a".repeat(64)}`;
const fixture = () => ({
  version: 1,
  source: {
    kind: "docker-desktop",
    daemon: "source",
    engineVersion: "29.8.0",
    apiVersion: "1.56",
    product: "Docker Desktop 4.91.0",
  },
  target: "owned",
  images: [{ id, tags: ["sample:test"], bytes: 10 }],
  volumes: [],
  projectMembers: [],
  warnings: [],
});
test("source image export freezes selected daemon, image and reference from review", () => {
  const review = parseMigrationImages(JSON.stringify(fixture()));
  const request = migrationImageExport(review, id, "sample:test");
  review.source.daemon = "different";
  review.images[0].tags[0] = "different:test";
  assert.deepEqual(request, {
    kind: "exportDesktopMigrationImage",
    source: "source",
    image: id,
    reference: "sample:test",
    destination: "",
  });
  assert.throws(() => migrationImageExport(review, id, "sample:test"));
  assert.throws(() =>
    migrationImageExport(review, `sha256:${"b".repeat(64)}`, "different:test"),
  );
  assert.equal(migrationImageExport(review, id, id).reference, id);
});
test("source review refuses unsupported, mixed-identity and oversized inventories", () => {
  for (const change of [
    (value) => (value.source.daemon = value.target),
    (value) => (value.source.kind = "arbitrary-host"),
    (value) => (value.source.engineVersion = "29.9.0"),
    (value) => value.images.push(value.images[0]),
    (value) => (value.images[0].id = "partial"),
    (value) => (value.images[0].bytes = -1),
    (value) => (value.images[0].tags = Array(513).fill("sample:test")),
    (value) => (value.volumes = Array(4097).fill({})),
    (value) => (value.warnings = ["bad\nmetadata"]),
  ]) {
    const value = fixture();
    change(value);
    assert.throws(() => parseMigrationImages(JSON.stringify(value)));
  }
});
test("empty source review has explicit empty image inventory", () => {
  const value = fixture();
  value.images = [];
  assert.deepEqual(parseMigrationImages(JSON.stringify(value)).images, []);
});

test("source volume review binds eligibility, daemon, target and exact metadata", () => {
  const inventory = fixture();
  inventory.volumes = [
    { name: "data", driver: "local", scope: "local", plainLocal: true },
  ];
  const value = {
    version: 1,
    source: inventory.source,
    target: inventory.target,
    volume: "data",
    fingerprint: "b".repeat(64),
    createdAt: "2026-10-25T00:00:00Z",
    quiescentObserved: true,
    message: "Coordinate external writers.",
  };
  const review = parseMigrationVolumeReview(
    JSON.stringify(value),
    inventory,
    "data",
  );
  const request = migrationVolumeExport(review);
  review.volume = "changed";
  review.fingerprint = "c".repeat(64);
  assert.deepEqual(request, {
    kind: "exportDesktopMigrationVolume",
    source: "source",
    volume: "data",
    fingerprint: "b".repeat(64),
    destination: "",
  });
  for (const change of [
    (value) => (value.source.daemon = "foreign"),
    (value) => (value.target = "foreign"),
    (value) => (value.volume = "foreign"),
    (value) => (value.quiescentObserved = false),
    (value) => (value.fingerprint = "partial"),
    (value) => (value.fingerprint = ["b".repeat(64)]),
  ]) {
    const next = structuredClone(value);
    change(next);
    assert.throws(() =>
      parseMigrationVolumeReview(JSON.stringify(next), inventory, "data"),
    );
  }
  inventory.volumes[0].plainLocal = false;
  assert.throws(() =>
    parseMigrationVolumeReview(JSON.stringify(value), inventory, "data"),
  );
});

test("source volume inventory refuses contradictory and repeated eligibility", () => {
  for (const volumes of [
    [{ name: "data", driver: "plugin", scope: "local", plainLocal: true }],
    [
      { name: "data", driver: "local", scope: "local", plainLocal: true },
      { name: "data", driver: "local", scope: "local", plainLocal: true },
    ],
  ]) {
    assert.throws(() =>
      parseMigrationImages(JSON.stringify({ ...fixture(), volumes })),
    );
  }
});

test("Rancher image export freezes the reviewed WSL registration and machine", () => {
  const value = fixture();
  value.source = {
    ...value.source,
    kind: "rancher-desktop",
    engineVersion: "29.5.3",
    wsl: {
      registration: "00000000-0000-4000-8000-000000000001",
      name: "Ubuntu",
      machine: "c".repeat(32),
    },
  };
  const review = parseMigrationImages(JSON.stringify(value));
  const request = migrationImageExport(review, id, "sample:test");
  review.source.wsl.machine = "d".repeat(32);
  assert.deepEqual(request, {
    kind: "exportRancherMigrationImage",
    source: "source",
    registration: value.source.wsl.registration,
    machine: "c".repeat(32),
    image: id,
    reference: "sample:test",
    destination: "",
  });
  for (const change of [
    (value) => delete value.source.wsl,
    (value) => (value.source.wsl.machine = "partial"),
    (value) => (value.source.wsl.registration = "Ubuntu"),
    (value) => (value.source.engineVersion = "29.8.0"),
  ]) {
    const next = structuredClone(value);
    change(next);
    assert.throws(() => parseMigrationImages(JSON.stringify(next)));
  }
});
test("WSL source choices refuse protected or duplicated registrations", () => {
  const item = { id: "00000000-0000-4000-8000-000000000001", name: "Ubuntu" };
  assert.deepEqual(
    parseMigrationWSLSources(
      JSON.stringify({ version: 1, registrations: [item] }),
    ),
    [item],
  );
  for (const registrations of [
    [item, item],
    [{ ...item, name: "rancher-desktop" }],
    [{ ...item, id: "--help" }],
    [{ ...item, name: "" }],
  ])
    assert.throws(() =>
      parseMigrationWSLSources(JSON.stringify({ version: 1, registrations })),
    );
});

test("Rancher volume review and export retain the exact source connection", () => {
  const inventory = fixture();
  inventory.source = {
    ...inventory.source,
    kind: "rancher-desktop",
    engineVersion: "29.5.3",
    wsl: {
      registration: "00000000-0000-4000-8000-000000000001",
      name: "Ubuntu",
      machine: "c".repeat(32),
    },
  };
  inventory.volumes = [
    { name: "data", driver: "local", scope: "local", plainLocal: true },
  ];
  const value = {
    version: 1,
    source: inventory.source,
    target: inventory.target,
    volume: "data",
    fingerprint: "b".repeat(64),
    createdAt: "2026-10-25T00:00:00Z",
    quiescentObserved: true,
    message: "Coordinate external writers.",
  };
  const review = parseMigrationVolumeReview(
    JSON.stringify(value),
    inventory,
    "data",
  );
  const request = migrationVolumeExport(review);
  review.source.wsl.machine = "d".repeat(32);
  assert.deepEqual(request, {
    kind: "exportRancherMigrationVolume",
    source: "source",
    registration: inventory.source.wsl.registration,
    machine: "c".repeat(32),
    volume: "data",
    fingerprint: "b".repeat(64),
    destination: "",
  });
  for (const change of [
    (value) => delete value.source.wsl,
    (value) => (value.source.kind = "docker-desktop"),
    (value) => (value.source.wsl.machine = "d".repeat(32)),
    (value) => (value.source.wsl.name = "Replacement"),
    (value) =>
      (value.source.wsl.registration = "00000000-0000-4000-8000-000000000002"),
  ]) {
    const next = structuredClone(value);
    change(next);
    assert.throws(() =>
      parseMigrationVolumeReview(JSON.stringify(next), inventory, "data"),
    );
  }
});
