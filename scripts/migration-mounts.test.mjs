import test from "node:test";
import assert from "node:assert/strict";
import {
  reviewMigrationMounts,
  migrationMountDraft,
} from "../src/model/migration-mounts.ts";
const member = () => ({
  service: "web",
  oneOff: false,
  mounts: [
    {
      kind: "volume",
      volume: "source_data",
      target: "/data/${literal}",
      readOnly: true,
      mappingSupported: true,
    },
  ],
  volumes: ["source_data"],
});
const namespace = "0123456789abcdef";
test("migration mapping deduplicates replicas, excludes one-offs and preserves literal paths and read-only intent", () => {
  const first = member(),
    second = member();
  const mounts = reviewMigrationMounts([
    first,
    second,
    { ...member(), service: "task", oneOff: true },
  ]);
  assert.equal(mounts.length, 1);
  const draft = JSON.parse(
    migrationMountDraft(mounts, { source_data: "restored" }, namespace, [
      "restored",
    ]),
  );
  assert.deepEqual(Object.keys(draft), ["services", "volumes"]);
  assert.equal(draft.services.web.volumes[0].target, "/data/$${literal}");
  assert.equal(draft.services.web.volumes[0].read_only, true);
  assert.equal(draft.services.web.volumes[0].volume.nocopy, true);
  assert.deepEqual(Object.values(draft.volumes), [
    { external: true, name: "restored" },
  ]);
});
test("missing metadata, unsupported roots and differing replica mounts cannot silently become a mapping", () => {
  const first = member();
  assert.throws(() => reviewMigrationMounts([{ ...member(), service: "" }]));
  for (const other of [
    { ...member(), mounts: undefined },
    { ...member(), mounts: [] },
    { ...member(), mounts: [{ ...member().mounts[0], readOnly: false }] },
    {
      ...member(),
      mounts: [{ kind: "bind", target: "/data/${literal}", readOnly: true }],
    },
  ])
    assert.throws(() => reviewMigrationMounts([first, other]));
  assert.throws(() =>
    reviewMigrationMounts([
      {
        ...member(),
        mounts: [{ ...member().mounts[0], mappingSupported: false }],
      },
    ]),
  );
  assert.throws(() =>
    reviewMigrationMounts([
      {
        ...member(),
        mounts: [{ ...member().mounts[0], target: "/../escape" }],
      },
    ]),
  );
});
test("mapping refuses unknown/merged destinations, duplicate targets and invalid namespaces", () => {
  const mounts = reviewMigrationMounts([member()]);
  assert.throws(() =>
    migrationMountDraft(mounts, { source_data: "missing" }, namespace, [
      "restored",
    ]),
  );
  assert.throws(() =>
    migrationMountDraft(mounts, { source_data: "restored" }, "invalid", [
      "restored",
    ]),
  );
  assert.throws(() =>
    migrationMountDraft(
      [...mounts, ...mounts],
      { source_data: "restored" },
      namespace,
      ["restored"],
    ),
  );
  assert.throws(() =>
    migrationMountDraft(
      [...mounts, { ...mounts[0], target: "/other", sourceVolume: "second" }],
      { source_data: "restored", second: "restored" },
      namespace,
      ["restored"],
    ),
  );
});
