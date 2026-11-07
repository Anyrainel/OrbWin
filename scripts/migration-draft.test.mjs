import test from "node:test";
import assert from "node:assert/strict";
import {
  validMigrationDraft,
  migrationDraft,
  assertMigrationInputs,
  recheckDraftSource,
  recheckMigrationDraft,
  draftMappingContents,
} from "../src/model/migration-draft.ts";
import { migrationProjectReview } from "../src/model/migration-projects.ts";
import { parseMigrationImages } from "../src/model/migration-images.ts";
import {
  loadComposeInputs,
  rememberComposeInputs,
  forgetComposeInputs,
  composeInputKey,
} from "../src/model/compose-inputs.ts";
const image = "sha256:" + "c".repeat(64);
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
  images: [{ id: image, tags: ["sample:old"], bytes: 1 }],
  volumes: [
    { name: "sample_data", driver: "local", scope: "local", plainLocal: true },
  ],
  projectMembers: [
    {
      container: "a".repeat(64),
      image,
      project: "sample",
      service: "web",
      workingDirectory: "C:\\project",
      configFilesLabel: "C:\\project\\compose.yaml",
      explicitEnvFiles: false,
      bindMounts: false,
      oneOff: false,
      volumes: ["sample_data"],
      mounts: [
        {
          kind: "volume",
          volume: "sample_data",
          target: "/data",
          readOnly: false,
          mappingSupported: true,
        },
      ],
    },
  ],
  warnings: [],
});
const draft = () =>
  migrationDraft(
    migrationProjectReview(
      parseMigrationImages(JSON.stringify(inventory())),
      "sample",
    ),
    {
      namespace: "1".repeat(16),
      selections: { sample_data: "restored" },
      file: { path: "C:\\project\\mapping.json", sha256: "f".repeat(64) },
    },
  );
const inputs = (migration = draft()) => ({
  project: "sample",
  file: "C:\\project\\compose.yaml",
  overrides: ["C:\\project\\mapping.json"],
  profiles: [],
  directory: "C:\\project",
  migration,
});
function storage() {
  const records = new Map();
  globalThis.localStorage = {
    getItem: (key) => records.get(key) ?? null,
    setItem: (key, value) => {
      records.set(key, value);
    },
  };
  return records;
}
test("reopen persists provenance and exact saved hash, not transfer status; same-name identities coexist", () => {
  storage();
  const first = inputs();
  rememberComposeInputs(first);
  rememberComposeInputs({ ...first, migration: undefined });
  const second = structuredClone(first);
  second.migration.source.daemon = "other";
  rememberComposeInputs(second);
  assert.equal(loadComposeInputs().length, 3);
  assert.deepEqual(
    loadComposeInputs().find(
      (x) => composeInputKey(x) === composeInputKey(first),
    ),
    first,
  );
  forgetComposeInputs(composeInputKey(first));
  assert.equal(loadComposeInputs().length, 2);
  assert.ok(loadComposeInputs().some((x) => !x.migration));
});
test("hostile provenance and unknown secret/status fields cannot enter remembered storage", () => {
  storage();
  for (const change of [
    (d) => (d.status = "complete"),
    (d) => (d.mapping.selections = []),
    (d) => (d.source.credentials = "secret"),
    (d) => (d.mapping.file.sha256 = "wrong"),
    (d) => (d.mapping.file.path = "C:\\a\\..\\mapping.json"),
    (d) => (d.mounts[0].target = "/../escape"),
    (d) => (d.source.daemon = d.target),
    (d) => d.members.push(d.members[0]),
    (d) => (d.mapping.selections.sample_data = "${PASSWORD}"),
  ]) {
    const value = draft();
    change(value);
    assert.equal(validMigrationDraft(value), false);
    assert.throws(() => rememberComposeInputs(inputs(value)));
  }
  assert.throws(() =>
    rememberComposeInputs({ ...inputs(), environment: { PASSWORD: "secret" } }),
  );
});
test("corrupt, invalid and quota-failed stores retain exact previous bytes", () => {
  const records = storage(),
    key = "orbwin.compose.inputs.v1";
  for (const raw of [
    "{broken",
    JSON.stringify([
      { ...inputs(), migration: { ...draft(), status: "complete" } },
    ]),
  ]) {
    records.set(key, raw);
    assert.deepEqual(loadComposeInputs(), []);
    assert.throws(() => rememberComposeInputs(inputs()));
    assert.throws(() => forgetComposeInputs("sample"));
    assert.equal(records.get(key), raw);
  }
  records.clear();
  rememberComposeInputs(inputs());
  const before = records.get(key);
  globalThis.localStorage.setItem = () => {
    throw Error("quota");
  };
  assert.throws(
    () => rememberComposeInputs({ ...inputs(), project: "another" }),
    /quota/,
  );
  assert.equal(records.get(key), before);
});
test("mapping must be last, unique and Windows absolute; unknown images remain Validate-only", () => {
  const d = draft(),
    i = inputs(d);
  assert.doesNotThrow(() => assertMigrationInputs(d, i, "up"));
  for (const overrides of [
    [],
    [...i.overrides, "C:\\later.json"],
    [...i.overrides, ...i.overrides],
    ["/tmp/mapping.json"],
  ])
    assert.throws(() => assertMigrationInputs(d, { ...i, overrides }, "up"));
  const partial = structuredClone(d);
  partial.unknownImages = 1;
  assert.doesNotThrow(() => assertMigrationInputs(partial, i, "validate"));
  assert.throws(() => assertMigrationInputs(partial, i, "up"));
  delete partial.mapping;
  assert.doesNotThrow(() =>
    assertMigrationInputs(partial, { ...i, overrides: [] }, "validate"),
  );
  assert.throws(() =>
    assertMigrationInputs(partial, { ...i, overrides: [] }, "up"),
  );
});
test("source/target/member/image/mount drift rejects instead of adopting fresh identities", () => {
  assert.doesNotThrow(() =>
    recheckDraftSource(draft(), JSON.stringify(inventory())),
  );
  for (const change of [
    (v) => (v.source.daemon = "foreign"),
    (v) => (v.target = "foreign"),
    (v) => (v.projectMembers[0].container = "b".repeat(64)),
    (v) => (v.projectMembers[0].image = "sha256:" + "e".repeat(64)),
    (v) => (v.projectMembers[0].mounts[0].readOnly = true),
    (v) => (v.projectMembers = []),
  ]) {
    const changed = inventory();
    change(changed);
    assert.throws(() => recheckDraftSource(draft(), JSON.stringify(changed)));
  }
});
test("fresh source then byte/semantic inspection must both succeed; read failure never reaches mapping", async () => {
  const calls = [],
    d = draft(),
    abort = new AbortController();
  await recheckMigrationDraft(
    d,
    inputs(d),
    "up",
    async () => {
      calls.push("source");
      return JSON.stringify(inventory());
    },
    async (file, contents, target) => {
      calls.push("mapping");
      assert.equal(file.sha256, d.mapping.file.sha256);
      assert.equal(target, "owned");
      assert.equal(contents, draftMappingContents(d));
    },
    abort.signal,
  );
  assert.deepEqual(calls, ["source", "mapping"]);
  await assert.rejects(
    recheckMigrationDraft(
      d,
      inputs(d),
      "up",
      async () => {
        throw Error("offline");
      },
      async () => assert.fail("must not inspect"),
      abort.signal,
    ),
    /offline/,
  );
  await assert.rejects(
    recheckMigrationDraft(
      d,
      inputs(d),
      "up",
      async () => JSON.stringify(inventory()),
      async () => {
        throw Error("changed bytes");
      },
      abort.signal,
    ),
    /changed bytes/,
  );
});
test("cancellation before read, after read and during inspection cannot pass admission", async () => {
  for (const step of ["before", "source", "mapping"]) {
    const a = new AbortController(),
      d = draft();
    if (step === "before") a.abort();
    await assert.rejects(
      recheckMigrationDraft(
        d,
        inputs(d),
        "up",
        async () => {
          assert.notEqual(step, "before");
          if (step === "source") a.abort();
          return JSON.stringify(inventory());
        },
        async () => {
          assert.notEqual(step, "source");
          if (step === "mapping") a.abort();
        },
        a.signal,
      ),
      /cancelled/,
    );
  }
});

test("ordinary remembered inputs remain compatible and corrupt records are surfaced", async () => {
  const { composeInputsProblem } = await import(
    "../src/model/compose-inputs.ts"
  );
  const records = storage();
  const ordinary = {
    project: "legacy",
    file: "C:\\legacy\\compose.yaml",
    overrides: [],
    profiles: [],
    directory: "C:\\legacy",
  };
  rememberComposeInputs(ordinary);
  assert.deepEqual(loadComposeInputs(), [ordinary]);
  assert.equal(composeInputsProblem(), "");
  records.set("orbwin.compose.inputs.v1", "[broken");
  assert.match(composeInputsProblem(), /corrupt/);
});

test("Rancher transport changes refuse reopen even with unchanged daemon IDs", () => {
  const value = inventory();
  value.source = {
    kind: "rancher-desktop",
    daemon: "source",
    engineVersion: "29.5.3",
    apiVersion: "1.56",
    product: "Rancher",
    wsl: {
      registration: "11111111-1111-1111-1111-111111111111",
      name: "Ubuntu",
      machine: "d".repeat(32),
    },
  };
  const saved = migrationDraft(
    migrationProjectReview(
      parseMigrationImages(JSON.stringify(value)),
      "sample",
    ),
  );
  assert.doesNotThrow(() => recheckDraftSource(saved, JSON.stringify(value)));
  value.source.wsl.registration = "22222222-2222-2222-2222-222222222222";
  assert.throws(
    () => recheckDraftSource(saved, JSON.stringify(value)),
    /identity changed/,
  );
});
