import test from "node:test";
import assert from "node:assert/strict";
import { migrationDraft } from "../src/model/migration-draft.ts";
import { migrationProjectReview } from "../src/model/migration-projects.ts";
import { wslMigrationDraft } from "../src/model/migration-wsl-draft.ts";
import {
  qualifyDraftReopen,
  roundTripDraftInputs,
} from "./migration-draft-qualification.mjs";
const nonce = "a".repeat(32),
  project = "orbwin-draft-" + nonce,
  root = "/home/user/approved",
  registration = "11111111-1111-1111-1111-111111111111",
  image = "sha256:" + "b".repeat(64);
function fixture(wsl = false) {
  let mappingChanged = false,
    removed = false,
    restores = 0,
    nativeCalls = 0;
  const inventory = () => ({
    version: 1,
    source: {
      kind: "docker-desktop",
      daemon: "source",
      engineVersion: "29.8.0",
      apiVersion: "1.56",
      product: "Desktop",
    },
    target: "owned",
    images: [{ id: image, tags: [], bytes: 1 }],
    volumes: [
      { name: "data", driver: "local", scope: "local", plainLocal: true },
    ],
    projectMembers: removed
      ? []
      : [
          {
            container: "c".repeat(64),
            image,
            project,
            service: "web",
            workingDirectory: wsl ? root : "C:\\fixture",
            configFilesLabel: wsl
              ? root + "/compose.json"
              : "C:\\fixture\\compose.json",
            explicitEnvFiles: false,
            bindMounts: false,
            oneOff: false,
            volumes: ["data"],
            mounts: [
              {
                kind: "volume",
                volume: "data",
                target: "/data",
                readOnly: false,
                mappingSupported: true,
              },
            ],
          },
        ],
    warnings: [],
  });
  const connection = {
    name: "ProjectUbuntu",
    registration,
    source: root,
    signature: "1:2",
  };
  const mapping = {
    namespace: "d".repeat(16),
    selections: { data: "restored" },
    file: {
      path: wsl ? root + "/mapping.json" : "C:\\fixture\\mapping.json",
      sha256: "e".repeat(64),
    },
  };
  const review = migrationProjectReview(inventory(), project);
  const migration = wsl
    ? wslMigrationDraft(review, connection, mapping)
    : migrationDraft(review, mapping);
  const inputs = {
    project,
    file: wsl ? root + "/compose.json" : "C:\\fixture\\compose.json",
    overrides: [mapping.file.path],
    directory: wsl ? root : "C:\\fixture",
    profiles: [],
    migration,
    ...(wsl
      ? {
          kind: "wsl",
          distribution: connection.name,
          connection: { registration, source: root, signature: "1:2" },
        }
      : {}),
  };
  return {
    inputs,
    readSource: async () => JSON.stringify(inventory()),
    inspectConnection: async () => ({
      registration,
      connected: true,
      export: { source: root, signature: "1:2" },
    }),
    inspectMapping: async () => {
      if (mappingChanged) throw Error("Saved mapping bytes changed");
    },
    validate: async () => {
      nativeCalls++;
      return mappingChanged
        ? {
            success: false,
            mutation: false,
            stdout: "saved mapping bytes changed",
          }
        : { success: true, mutation: false };
    },
    changeMapping: async () => {
      mappingChanged = true;
    },
    restoreMapping: async () => {
      mappingChanged = false;
      restores++;
    },
    removeOwnedSource: async () => {
      removed = true;
    },
    verifyBaselines: async () => {},
    signal: new AbortController().signal,
    counters: () => ({ restores, nativeCalls }),
  };
}
test("Windows journey requires actual model checks before counted native dispatch and restores fixture mapping", async () => {
  const value = fixture(),
    result = await qualifyDraftReopen(value);
  assert.equal(result.validations, 3);
  assert.equal(result.uiRestartQualified, false);
  assert.equal(result.upQualified, false);
  assert.deepEqual(value.counters(), { restores: 1, nativeCalls: 3 });
  assert.equal(
    result.records.find((x) => x.step === "changed-source").refused,
    true,
  );
});
test("WSL journey separately records guest mapping refusal instead of claiming no helper was launched", async () => {
  const value = fixture(true),
    result = await qualifyDraftReopen(value);
  assert.equal(result.validations, 3);
  assert.equal(value.counters().nativeCalls, 4);
  assert.equal(
    result.records.find((x) => x.step === "changed-mapping").boundary,
    "guest-before-configuration-resolution",
  );
});
test("uncertain changed-file callback still invokes guarded restoration", async () => {
  const value = fixture();
  value.changeMapping = async () => {
    throw Error("write uncertain");
  };
  await assert.rejects(qualifyDraftReopen(value), /write uncertain/);
  assert.equal(value.counters().restores, 1);
});
test("unrelated native failure cannot qualify changed guest mapping", async () => {
  const value = fixture(true),
    validate = value.validate;
  let calls = 0;
  value.validate = async (...args) => {
    calls++;
    return calls === 3
      ? { success: false, mutation: false, stdout: "plugin unavailable" }
      : validate(...args);
  };
  await assert.rejects(
    qualifyDraftReopen(value),
    /saved mapping bytes changed/,
  );
  assert.equal(value.counters().restores, 1);
});
test("serialization session restores prior global storage and reports its narrower evidence scope", () => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "localStorage"),
    value = fixture().inputs;
  const result = roundTripDraftInputs(value);
  assert.match(result.evidenceKind, /not-WebView/);
  assert.deepEqual(
    Object.getOwnPropertyDescriptor(globalThis, "localStorage"),
    previous,
  );
});
