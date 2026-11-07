import test from "node:test";
import assert from "node:assert/strict";
import { migrationProjectReview } from "../src/model/migration-projects.ts";
import { parseMigrationImages } from "../src/model/migration-images.ts";

const member = (container = "a".repeat(64)) => ({
  container,
  project: "sample",
  service: "web",
  workingDirectory: "C:\\project 例",
  configFilesLabel:
    "C:\\project 例\\compose.yaml,C:\\project 例\\override.yaml",
  explicitEnvFiles: false,
  bindMounts: false,
  oneOff: false,
  volumes: ["sample_data"],
});
const inventory = () => ({
  version: 1,
  source: {
    kind: "docker-desktop",
    daemon: "source",
    engineVersion: "29.8.0",
    apiVersion: "1.56",
    product: "Docker Desktop",
  },
  target: "owned",
  images: [],
  volumes: [],
  projectMembers: [member()],
  warnings: [],
});

test("project image association uses immutable member identities and keeps ambiguity visible", () => {
  const value = inventory(),
    first = "sha256:" + "c".repeat(64),
    second = "sha256:" + "d".repeat(64);
  value.images = [{ id: first, tags: ["sample:old"], bytes: 1 }];
  value.projectMembers[0].image = first;
  value.projectMembers.push({
    ...member("b".repeat(64)),
    image: first,
    service: "worker",
  });
  value.projectMembers.push({
    ...member("e".repeat(64)),
    image: second,
    service: "web",
  });
  value.projectMembers.push({
    ...member("f".repeat(64)),
    image: "sha256:" + "a".repeat(64),
    oneOff: true,
  });
  const review = migrationProjectReview(
    parseMigrationImages(JSON.stringify(value)),
    "sample",
  );
  assert.equal(review.unknownImages, 0);
  assert.deepEqual(review.images, [
    {
      id: first,
      services: ["web", "worker"],
      tags: ["sample:old"],
      available: true,
    },
    { id: second, services: ["web"], tags: [], available: false },
  ]);
  value.images[0].tags.push("later");
  assert.deepEqual(review.images[0].tags, ["sample:old"]);
  assert.equal(
    migrationProjectReview(
      parseMigrationImages(JSON.stringify(inventory())),
      "sample",
    ).unknownImages,
    1,
  );
  for (const image of [
    "sample:latest",
    "sha256:" + "g".repeat(64),
    null,
    123,
  ]) {
    const changed = inventory();
    changed.projectMembers[0].image = image;
    assert.throws(() => parseMigrationImages(JSON.stringify(changed)));
  }
});

test("migration project review preserves coherent ordered files and freezes source hints", () => {
  const value = inventory();
  value.projectMembers.push({
    ...member("b".repeat(64)),
    service: "oneoff",
    oneOff: true,
    volumes: ["sample_data", "other"],
  });
  const parsed = parseMigrationImages(JSON.stringify(value));
  const review = migrationProjectReview(parsed, "sample");
  assert.equal(review.suggestedFiles, true);
  assert.deepEqual(review.inputs, {
    project: "sample",
    file: "C:\\project 例\\compose.yaml",
    overrides: ["C:\\project 例\\override.yaml"],
    directory: "C:\\project 例",
    profiles: [],
  });
  assert.deepEqual(review.services, ["web"]);
  assert.deepEqual(review.volumes, ["other", "sample_data"]);
  parsed.source.daemon = "foreign";
  parsed.projectMembers[0].volumes.push("later");
  assert.equal(review.source.daemon, "source");
  assert.deepEqual(review.members[0].volumes, ["sample_data"]);
  assert.throws(() => migrationProjectReview(parsed, "absent"));
});
test("migration project review never guesses unsupported file sets or env files", () => {
  for (const change of [
    (value) => (value.projectMembers[0].explicitEnvFiles = true),
    (value) =>
      (value.projectMembers[0].configFilesLabel = "/home/user/compose.yaml"),
    (value) =>
      (value.projectMembers[0].configFilesLabel =
        "C:\\project\\base.yaml,relative.yaml"),
    (value) =>
      value.projectMembers.push({
        ...member("b".repeat(64)),
        workingDirectory: "C:\\different",
      }),
    (value) => (value.projectMembers[0].workingDirectory = "\\\\server\\share"),
  ]) {
    const value = inventory();
    change(value);
    const review = migrationProjectReview(
      parseMigrationImages(JSON.stringify(value)),
      "sample",
    );
    assert.equal(review.suggestedFiles, false);
    assert.deepEqual(review.inputs, {
      project: "sample",
      file: "",
      overrides: [],
      profiles: [],
      directory: "",
    });
  }
});
test("project inventory refuses malformed, duplicated or unbounded source members", () => {
  for (const change of [
    (value) => (value.projectMembers[0].container = "g".repeat(64)),
    (value) => value.projectMembers.push(member()),
    (value) => (value.projectMembers[0].project = ""),
    (value) => (value.projectMembers[0].service = "bad\nservice"),
    (value) => (value.projectMembers[0].volumes = Array(513).fill("volume")),
    (value) => (value.projectMembers[0].bindMounts = "false"),
    (value) => delete value.projectMembers[0].explicitEnvFiles,
  ]) {
    const value = inventory();
    change(value);
    assert.throws(() => parseMigrationImages(JSON.stringify(value)));
  }
});

test("Rancher connection identity does not imply Linux project ownership or path conversion", () => {
  const value = inventory();
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
  value.projectMembers[0].workingDirectory = "/home/other/project";
  value.projectMembers[0].configFilesLabel = "/home/other/project/compose.yaml";
  const review = migrationProjectReview(
    parseMigrationImages(JSON.stringify(value)),
    "sample",
  );
  assert.equal(review.suggestedFiles, false);
  assert.equal(review.inputs.file, "");
  assert.equal(review.inputs.directory, "");
  value.source.wsl.machine = "d".repeat(32);
  assert.equal(review.source.wsl.machine, "c".repeat(32));
});
