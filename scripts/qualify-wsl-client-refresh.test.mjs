import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import {
  assertPlannedEdit,
  assertBackupDelta,
  parameters,
  pins,
  plan,
  attestationContract,
  identityContract,
} from "./qualify-wsl-client-refresh.mjs";
const root = fileURLToPath(new URL("../", import.meta.url));

test("raw edit preserves unknown precision, nested credentials, strings and Unicode", () => {
  const old = Buffer.from(
    '{"auths":{"private":{"auth":"fixture-secret","nested":"[}\\\""}},"large":123456789012345678901234567890,"decimal":0.12345678901234567890,"unicode":"é","cliPluginsExtraDirs":["/old"]}\n',
  );
  const next = Buffer.from(old.toString().replace('["/old"]', '["/new"]'));
  assert.doesNotThrow(() => assertPlannedEdit(old, next, "/new"));
  for (const changed of [
    next.toString().replace("fixture-secret", "changed"),
    next
      .toString()
      .replace(
        "123456789012345678901234567890",
        "123456789012345678901234567891",
      ),
    next.toString().replace('"/new"', '"/foreign"'),
  ]) {
    assert.throws(
      () => assertPlannedEdit(old, Buffer.from(changed), "/new"),
      (error) => !error.message.includes("fixture-secret"),
    );
  }
  assert.throws(() => assertPlannedEdit(old, old, "/new"));
});

test("backup delta refuses extra backups, partials, missing existing files and changed hashes", () => {
  const before = [
    { name: "config.json", kind: "file", sha256: pins.oldSha },
    { name: "buildx", kind: "directory" },
  ];
  const backup = {
    name: ".orbwin-client-refresh-1-2-3.original.json",
    kind: "file",
    sha256: pins.oldSha,
  };
  const after = [
    { name: "config.json", kind: "file", sha256: "a".repeat(64) },
    before[1],
    backup,
  ];
  assert.equal(assertBackupDelta(before, after, pins.oldSha), backup);
  for (const bad of [
    [
      ...after,
      { ...backup, name: ".orbwin-client-refresh-1-2-4.original.json" },
    ],
    [...after, { name: "bad.partial", kind: "file" }],
    after.filter((item) => item.name !== "buildx"),
    after.map((item) =>
      item === backup ? { ...item, sha256: "f".repeat(64) } : item,
    ),
  ])
    assert.throws(() => assertBackupDelta(before, bad, pins.oldSha));
});

test("native source contract requires reviewed lock and all production security seams", () => {
  const native = path.join(
      root,
      "artifacts/qualification/native/orbwin-native-test.exe",
    ),
    digest = "a".repeat(64);
  const sources = Object.fromEntries(
    [
      "runtime-tools.lock.json",
      "src-tauri/Cargo.toml",
      "src-tauri/Cargo.lock",
      "src-tauri/src/main.rs",
      "src-tauri/src/engine.rs",
      "src-tauri/src/runtime.rs",
      "src-tauri/src/registry.rs",
      "src-tauri/src/wsl_integration.rs",
      "src-tauri/src/wsl_clients.rs",
      "src-tauri/src/wsl_clients_security.rs",
      "src-tauri/src/wsl_clients_qualification.rs",
    ].map((key) => [key, digest]),
  );
  sources["runtime-tools.lock.json"] = pins.lockSha;
  const value = {
    schema: 1,
    binary: native,
    binarySha256: digest,
    exactTest: pins.exactTest,
    sources,
  };
  assert.equal(attestationContract(value, native, digest), value);
  for (const mutate of [
    (v) => delete v.sources["src-tauri/src/wsl_clients_security.rs"],
    (v) => (v.sources["runtime-tools.lock.json"] = digest),
    (v) => (v.sources["../outside"] = digest),
    (v) => (v.exactTest = "foreign"),
  ]) {
    const bad = structuredClone(value);
    mutate(bad);
    assert.throws(() => attestationContract(bad, native, digest));
  }
});

test("parameters bind nonce, artifact-only binary and sibling attestation", () => {
  const value = {
    nonce: "a".repeat(32),
    native: path.join(root, "artifacts/qualification/native/a.exe"),
    nativeSha: "a".repeat(64),
    attestation: path.join(root, "artifacts/qualification/native/a.json"),
    attestationSha: "b".repeat(64),
  };
  assert.equal(parameters(value), value);
  for (const bad of [
    { ...value, nonce: "../bad" },
    { ...value, native: path.join(root, "a.exe") },
    { ...value, nativeSha: "bad" },
    {
      ...value,
      attestation: path.join(root, "artifacts/qualification/other/a.json"),
    },
  ])
    assert.throws(() => parameters(bad));
});

test("owned home identity cannot silently redirect planned private-config update", () => {
  const home = path.join(root, "owned-home"),
    value = {
      identityVerified: true,
      daemonId: "d9c00194-ea9d-439a-8561-6a0ec580fbd5",
      endpoint: "npipe:////./pipe/docker-rancher-desktop-orbwin",
      home,
    };
  assert.doesNotThrow(() => identityContract(value, home));
  for (const bad of [
    { ...value, home: path.join(root, "foreign") },
    { ...value, identityVerified: false },
    { ...value, daemonId: "foreign" },
    { ...value, endpoint: "npipe:////./pipe/docker_engine" },
  ])
    assert.throws(() => identityContract(bad, home));
});

test("default plan requires distinct runtime opt-in and does not dispatch native/WSL", () => {
  assert.equal(plan().pins.oldSha, pins.oldSha);
  assert.deepEqual(plan().budgets, {
    scenarioSeconds: 240,
    stepSeconds: 45,
    postcheckSeconds: 60,
  });
  const result = spawnSync(
    process.execPath,
    [path.join(root, "scripts/qualify-wsl-client-refresh.mjs")],
    { encoding: "utf8", windowsHide: true },
  );
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(JSON.parse(result.stdout), plan());
});
