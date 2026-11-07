import test from "node:test";
import assert from "node:assert/strict";
import {
  wslMigrationDraft,
  validWslComposeInputs,
  wslMappingReview,
  recheckWslDraft,
  wslProjectPath,
} from "../src/model/migration-wsl-draft.ts";
import { validMigrationDraft } from "../src/model/migration-draft.ts";
import { migrationProjectReview } from "../src/model/migration-projects.ts";
import {
  loadComposeInputs,
  rememberComposeInputs,
  composeInputKey,
  forgetComposeInputs,
} from "../src/model/compose-inputs.ts";
const root = "/home/user/project 例",
  registration = "11111111-1111-1111-1111-111111111111",
  image = "sha256:" + "a".repeat(64);
const connection = () => ({
  name: "ProjectUbuntu",
  registration,
  source: root,
  signature: "1:2",
});
const inventory = () => ({
  version: 1,
  source: {
    kind: "rancher-desktop",
    daemon: "source",
    engineVersion: "29.5.3",
    apiVersion: "1.54",
    product: "Rancher",
    wsl: {
      registration: "22222222-2222-2222-2222-222222222222",
      name: "SocketUbuntu",
      machine: "c".repeat(32),
    },
  },
  target: "owned",
  images: [{ id: image, tags: [], bytes: 1 }],
  volumes: [
    { name: "data", driver: "local", scope: "local", plainLocal: true },
  ],
  projectMembers: [
    {
      container: "b".repeat(64),
      image,
      project: "sample",
      service: "web",
      workingDirectory: root,
      configFilesLabel: root + "/compose.yaml",
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
const inputs = () => ({
  kind: "wsl",
  distribution: "ProjectUbuntu",
  connection: { registration, source: root, signature: "1:2" },
  project: "sample",
  file: "compose.yaml",
  overrides: [root + "/mapping.json"],
  directory: "",
  envFiles: [],
  profiles: [],
  services: [],
  migration: wslMigrationDraft(
    migrationProjectReview(inventory(), "sample"),
    connection(),
    {
      namespace: "d".repeat(16),
      selections: { data: "restored" },
      file: { path: root + "/mapping.json", sha256: "e".repeat(64) },
    },
  ),
});
const status = () => ({
  registration,
  connected: true,
  export: { source: root, signature: "1:2" },
});
function storage() {
  const map = new Map();
  globalThis.localStorage = {
    getItem: (key) => map.get(key) ?? null,
    setItem: (key, value) => map.set(key, value),
  };
  return map;
}
test("tagged WSL drafts reopen in the same input owner; project/source distro identities stay separate", () => {
  storage();
  const value = inputs();
  assert.equal(validWslComposeInputs(value), true);
  assert.equal(validMigrationDraft(value.migration), false);
  rememberComposeInputs(value);
  assert.deepEqual(loadComposeInputs(), [value]);
  assert.notEqual(
    value.connection.registration,
    value.migration.source.wsl.registration,
  );
  const other = structuredClone(value);
  other.connection.signature = "3:4";
  rememberComposeInputs(other);
  assert.equal(loadComposeInputs().length, 2);
  assert.notEqual(composeInputKey(value), composeInputKey(other));
  forgetComposeInputs(composeInputKey(value));
  assert.deepEqual(loadComposeInputs(), [other]);
});
test("untrusted transport/connection/file metadata never becomes export authority", () => {
  for (const change of [
    (v) => (v.kind = "windows"),
    (v) => (v.connection.shared_source = "/foreign"),
    (v) => (v.connection.registration = "bad"),
    (v) => (v.distribution = "docker-desktop"),
    (v) => (v.connection.signature = "bad"),
    (v) => (v.file = "/etc/passwd"),
    (v) => (v.envFiles = ["../outside"]),
    (v) => (v.migration.mapping.file.path = "C:\\mapping.json"),
    (v) => (v.migration.mapping.file.path = root + "/sub/map.json"),
    (v) => (v.state = "complete"),
  ]) {
    storage();
    const value = inputs();
    change(value);
    assert.equal(validWslComposeInputs(value), false);
    assert.throws(() => rememberComposeInputs(value));
  }
});
test("Linux path and final mapping comparison is case-sensitive and confined", () => {
  const value = inputs();
  assert.equal(
    wslProjectPath(value.connection, "compose.yaml"),
    root + "/compose.yaml",
  );
  assert.equal(wslMappingReview(value, "up").file, "mapping.json");
  for (const overrides of [
    [],
    [...value.overrides, "later.json"],
    ["mapping.json", "mapping.json"],
    [root + "/Mapping.json"],
  ])
    assert.throws(() => wslMappingReview({ ...value, overrides }, "up"));
  for (const path of [
    root + "-foreign/a",
    "../bad",
    "/mnt/wsl/orbwin/projects/internal",
    "file\\name",
    "./compose.yaml",
  ])
    assert.throws(() => wslProjectPath(value.connection, path));
  const partial = inputs();
  partial.migration.unknownImages = 1;
  assert.doesNotThrow(() => wslMappingReview(partial, "validate"));
  assert.throws(() => wslMappingReview(partial, "up"));
  assert.throws(() => wslMappingReview(value, "down"));
});
test("fresh connection then source/target checks precede typed guest mapping proof", async () => {
  const calls = [],
    value = inputs();
  const proof = await recheckWslDraft(
    value,
    "up",
    async () => {
      calls.push("connection");
      return status();
    },
    async () => {
      calls.push("source");
      return JSON.stringify(inventory());
    },
    new AbortController().signal,
  );
  assert.deepEqual(calls, ["connection", "source"]);
  assert.equal(proof.sha256, value.migration.mapping.file.sha256);
  assert.match(proof.contents, /restored/);
  for (const changed of [
    { connected: false },
    { connected: true, export: { source: root, signature: "9:9" } },
    { connected: true, export: { source: root + "-other", signature: "1:2" } },
  ])
    await assert.rejects(
      recheckWslDraft(
        value,
        "up",
        async () => ({ registration, ...changed }),
        async () => assert.fail("source must not run"),
        new AbortController().signal,
      ),
    );
  for (const change of [
    (v) => (v.target = "foreign"),
    (v) => (v.source.wsl.registration = registration),
    (v) => (v.projectMembers[0].image = "sha256:" + "f".repeat(64)),
  ]) {
    const source = inventory();
    change(source);
    await assert.rejects(
      recheckWslDraft(
        value,
        "up",
        async () => status(),
        async () => JSON.stringify(source),
        new AbortController().signal,
      ),
    );
  }
});
test("cancel and failed transport never admit Compose; no reconnection is attempted", async () => {
  for (const step of ["before", "connection", "source"]) {
    const abort = new AbortController();
    if (step === "before") abort.abort();
    await assert.rejects(
      recheckWslDraft(
        inputs(),
        "up",
        async () => {
          assert.notEqual(step, "before");
          if (step === "connection") abort.abort();
          return status();
        },
        async () => {
          assert.notEqual(step, "connection");
          if (step === "source") abort.abort();
          return JSON.stringify(inventory());
        },
        abort.signal,
      ),
      /cancelled/,
    );
  }
  await assert.rejects(
    recheckWslDraft(
      inputs(),
      "up",
      async () => {
        throw Error("registration unavailable");
      },
      async () => assert.fail(),
      new AbortController().signal,
    ),
    /registration unavailable/,
  );
});

test("prepared WSL form freezes draft hints and opens only a separate confirmation request", async () => {
  const { buildSync } = await import("esbuild"),
    { createRequire } = await import("node:module"),
    { fileURLToPath } = await import("node:url");
  const { outputFiles } = buildSync({
    entryPoints: [
      fileURLToPath(
        new URL("../src/components/Operations.tsx", import.meta.url),
      ),
    ],
    bundle: true,
    write: false,
    platform: "node",
    format: "cjs",
    external: ["react"],
    logLevel: "silent",
  });
  const module = { exports: {} };
  new Function("require", "module", "exports", outputFiles[0].text)(
    createRequire(import.meta.url),
    module,
    module.exports,
  );
  const value = inputs();
  const spec = module.exports.wslComposeSpec(
    value.distribution,
    value.connection.source,
    "validate",
    { ...value, kind: "wslCompose" },
  );
  assert.equal(spec.request.kind, "wslCompose");
  assert.equal(spec.request.action, "validate");
  assert.equal(spec.request.connection.registration, registration);
  assert.equal(spec.request.mappingReview, undefined);
  assert.deepEqual(spec.migration, value.migration);
  value.migration.target = "later";
  value.overrides.push("later.json");
  assert.equal(spec.migration.target, "owned");
  assert.deepEqual(spec.request.overrides, [root + "/mapping.json"]);
});
