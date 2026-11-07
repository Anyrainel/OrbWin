import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  assertPlannedEdit,
  assertBackupDelta,
  parameters,
  pins,
  plan,
  attestationContract,
  identityContract,
  expectedDirectory,
  productChildEnvironment,
  nativeRootsContract,
  nativeRootsPaths,
  nativeQualificationEnvironment,
  admitNativeRoots,
} from "./qualify-wsl-client-refresh-package.mjs";
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
  const lockSha = "b".repeat(64), packageSha = "c".repeat(64);
  for (const name of ["src-tauri/build.rs", "src-tauri/src/runtime_cache.rs", "src-tauri/src/client_config.rs", "src-tauri/src/wsl_tools.rs", "scripts/package-input-config.mjs", "scripts/package-contract.mjs", "scripts/build-identity.mjs", "scripts/qualify-wsl-client-refresh-package.mjs"]) sources[name] = digest;
  sources["runtime-tools.lock.json"] = lockSha;
  const value = {
    schema: 1,
    packageManifestSha256: packageSha,
    binary: native,
    binarySha256: digest,
    exactTest: pins.exactTest,
    rootsTest: pins.rootsTest,
    sources,
  };
  assert.equal(attestationContract(value, native, digest, lockSha, packageSha), value);
  for (const mutate of [
    (v) => delete v.sources["src-tauri/src/wsl_clients_security.rs"],
    (v) => (v.sources["runtime-tools.lock.json"] = digest),
    (v) => (v.sources["../outside"] = digest),
    (v) => (v.exactTest = "foreign"),
    (v) => (v.rootsTest = "foreign"),
    (v) => delete v.rootsTest,
    (v) => (v.packageManifestSha256 = digest),
    (v) => delete v.sources["src-tauri/src/runtime_cache.rs"],
  ]) {
    const bad = structuredClone(value);
    mutate(bad);
    assert.throws(() => attestationContract(bad, native, digest, lockSha, packageSha));
  }
});

test("parameters bind nonce, artifact-only binary and sibling attestation", () => {
  const value = {
    nonce: "a".repeat(32),
    packageInput: path.join(root, "artifacts/qualification/native"),
    expectedPluginDirectory: "C:/Native/.orbwin-private/runtime/linux-docker-" + "b".repeat(64),
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
    { ...value, packageInput: path.join(root, "unrelated") },
    { ...value, expectedPluginDirectory: "C:/Native/../other" },
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
      sourceCommit: "commit", binarySha256: "a".repeat(64),
      daemonId: "d9c00194-ea9d-439a-8561-6a0ec580fbd5",
      endpoint: "npipe:////./pipe/docker-rancher-desktop-orbwin",
      home,
    };
  const build = {sourceCommit:"commit", binarySha256:"a".repeat(64)};
  assert.doesNotThrow(() => identityContract(value, home, build));
  for (const bad of [
    { ...value, home: path.join(root, "foreign") },
    { ...value, identityVerified: false },
    { ...value, sourceCommit: "other" },
    { ...value, binarySha256: "b".repeat(64) },
    { ...value, daemonId: "foreign" },
    { ...value, endpoint: "npipe:////./pipe/docker_engine" },
  ])
    assert.throws(() => identityContract(bad, home, build));
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
    [path.join(root, "scripts/qualify-wsl-client-refresh-package.mjs")],
    { encoding: "utf8", windowsHide: true },
  );
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(JSON.parse(result.stdout), plan());
});

test("expected cache path tracks embedded lock and rejects public tools and traversal", () => {
  const lock = "d".repeat(64), base = "C:/Native/.orbwin-private/runtime/linux-docker-";
  assert.equal(expectedDirectory(base + lock, lock), "/mnt/c/Native/.orbwin-private/runtime/linux-docker-" + lock);
  for (const bad of [base + "e".repeat(64), "C:/tools/linux", "C:/Native/../Native/.orbwin-private/runtime/linux-docker-" + lock, "C:/Native//.orbwin-private/runtime/linux-docker-" + lock])
    assert.throws(() => expectedDirectory(bad, lock));
});

test("current product child omits every proof selector case without mutating ambient environment", () => {
  const isSelector = (key) => key.toUpperCase() === "ORBWIN_QUALIFICATION_IDENTITY";
  // Compare only digests on failure; never print unrelated environment secrets.
  const fingerprint = (env) => createHash("sha256").update(JSON.stringify(Object.entries(env).sort(([a], [b]) => a.localeCompare(b)))).digest("hex");
  const original = Object.entries(process.env).filter(([key]) => isSelector(key));
  const originalFingerprint = fingerprint(process.env);
  try {
    for (const [key] of original) delete process.env[key];
    process.env.oRbWiN_QuAlIfIcAtIoN_IdEnTiTy = "reviewed-fixed-proof";
    const ambientFingerprint = fingerprint(process.env);
    const inherited = Object.freeze({
      ...process.env,
      ORBWIN_QUALIFICATION_IDENTITY: "reviewed-fixed-proof",
      orbwin_qualification_identity: "reviewed-fixed-proof",
      ORBWIN_QUALIFY_WSL_CLIENT_REFRESH: "1",
    });
    const inputFingerprint = fingerprint(inherited);
    const env = productChildEnvironment(inherited);
    assert.equal(Object.keys(env).some(isSelector), false);
    assert.equal(env.ORBWIN_QUALIFY_WSL_CLIENT_REFRESH, "1");
    const result = spawnSync(process.execPath, ["-e", 'process.stdout.write(JSON.stringify(Object.keys(process.env).filter(key=>key.toUpperCase()==="ORBWIN_QUALIFICATION_IDENTITY")))'], {env,encoding:"utf8",windowsHide:true,timeout:10000});
    assert.equal(result.status, 0, "Inert Node environment probe failed");
    assert.deepEqual(JSON.parse(result.stdout), []);
    assert.equal(fingerprint(inherited), inputFingerprint);
    assert.equal(fingerprint(process.env), ambientFingerprint);
  } finally {
    for (const key of Object.keys(process.env)) if (isSelector(key)) delete process.env[key];
    for (const [key, value] of original) process.env[key] = value;
  }
  assert.equal(fingerprint(process.env), originalFingerprint);
});

const rootsNonce = "a".repeat(32);
const observedRoots = () => ({
  schema: 1, nonce: rootsNonce,
  localAppData: "C:\\Users\\开发 User\\AppData\\Local",
  profile: "D:\\Profiles\\开发 User",
});
const fakeRootsProbe = (record = observedRoots()) => ({
  status: 0, signal: null,
  stdout: Buffer.from("running 1 test\nWSL_PRIVATE_CLIENT_ROOTS " + JSON.stringify(record) +
    "\ntest result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 9 filtered out; finished in 0.00s\n"),
  stderr: Buffer.alloc(0),
});

test("native root DATA derives private and global baselines from independent Unicode native folders", () => {
  const roots = observedRoots();
  const selected = nativeRootsPaths(roots, rootsNonce);
  assert.equal(selected.home, roots.localAppData + "\\OrbWinData\\home");
  assert.equal(selected.identityPath, roots.localAppData + "\\OrbWinData\\runtime.json");
  assert.equal(selected.privateConfig, selected.home + "\\.rdorbwin\\wsl-docker\\config.json");
  assert.equal(selected.globalDocker, roots.profile + "\\.docker\\config.json");
  assert.equal(selected.globalKube, roots.profile + "\\.kube\\config");
  assert.equal(selected.globalWSL, roots.profile + "\\.wslconfig");
  assert.ok(Object.isFrozen(nativeRootsContract(roots, rootsNonce)));
  assert.ok(Object.isFrozen(selected));
});

test("native root record rejects malformed shape, spoofed nonce and unsafe Windows path forms", () => {
  const good = observedRoots();
  for (const bad of [
    null, [], { ...good, schema: "1" }, { ...good, schema: 2 },
    { ...good, nonce: "b".repeat(32) }, { ...good, extra: true },
    { schema: 1, nonce: rootsNonce, localAppData: good.localAppData },
    ...["C:relative", "C:/Users/Native", "\\\\server\\share", "\\\\?\\C:\\Users\\Native",
      "C:\\", "C:\\Users\\..\\Other", "C:\\Users\\.\\Native", "C:\\Users\\Bad.",
      "C:\\Users\\Bad ", "C:\\Users\\Bad:stream", "C:\\Users\\Bad\\", "C:\\Users\\\\Bad",
      "C:\\Users\\Bad\u0000", "C:\\Users\\Bad\u0085", "C:\\Users\\Bad\ud800", "C:\\" + "x".repeat(1021), 1]
      .map((localAppData) => ({ ...good, localAppData })),
    { ...good, profile: "C:\\Profiles\\..\\Other" },
  ]) assert.throws(() => nativeRootsContract(bad, rootsNonce));
});

test("rejected or unknown native roots cannot reach any private baseline or fixture effect", () => {
  const valid = fakeRootsProbe();
  const marker = (json) => Buffer.from("WSL_PRIVATE_CLIENT_ROOTS " + json +
    "\ntest result: ok. 1 passed; 0 failed; 0 ignored;\n");
  const cases = [
    () => { throw Error("fixture-secret"); },
    () => ({ ...valid, status: 1 }),
    () => ({ ...valid, status: null, signal: "SIGTERM" }),
    () => ({ ...valid, error: { code: "ETIMEDOUT" } }),
    () => ({ ...valid, stdout: valid.stdout.toString() }),
    () => ({ ...valid, stdout: Buffer.from([0xff]) }),
    () => ({ ...valid, stdout: Buffer.alloc(65537) }),
    () => ({ ...valid, stderr: Buffer.from("fixture-secret") }),
    () => ({ ...valid, stdout: Buffer.concat([valid.stdout, valid.stdout]) }),
    () => ({ ...valid, stdout: Buffer.from("unexpected-prefix " + valid.stdout.toString().replace("running 1 test\n", "")) }),
    () => ({ ...valid, stdout: marker(JSON.stringify({ ...observedRoots(), extra: "fixture-secret" })) }),
    () => ({ ...valid, stdout: marker(JSON.stringify(observedRoots()).replace('"schema":1', '"schema":1,"schema":1')) }),
    () => ({ ...valid, stdout: marker(" " + JSON.stringify(observedRoots())) }),
    () => ({ ...valid, stdout: marker(JSON.stringify(observedRoots(), null, 2)) }),
    () => ({ ...valid, stdout: Buffer.from(valid.stdout.toString().replace("1 passed", "0 passed")) }),
    () => fakeRootsProbe({ ...observedRoots(), nonce: "b".repeat(32) }),
  ];
  for (const probe of cases) {
    const effects = [];
    assert.throws(() => admitNativeRoots(rootsNonce, probe, () => {
      effects.push("private-read", "mkdir", "refresh");
    }), (error) => !error.message.includes("fixture-secret") && /refused or unknown/.test(error.message));
    assert.deepEqual(effects, []);
  }
});

test("validated roots precede baseline callback and retain original probe bytes and bindings", () => {
  const sequence = [], raw = fakeRootsProbe();
  const result = admitNativeRoots(rootsNonce, () => {
    sequence.push("exact-roots-probe");
    return raw;
  }, (observation) => {
    sequence.push("native-path-baseline");
    assert.deepEqual(observation.roots, observedRoots());
    assert.equal(observation.probe.selector, pins.rootsTest);
    assert.equal(observation.probe.stdoutSha256, createHash("sha256").update(raw.stdout).digest("hex"));
    assert.equal(observation.raw.stdout, raw.stdout);
    return observation.paths;
  });
  assert.deepEqual(sequence, ["exact-roots-probe", "native-path-baseline"]);
  assert.equal(result.home, observedRoots().localAppData + "\\OrbWinData\\home");
});

test("native qualification env ignores inherited selector variants and carries only the admitted roots contract", () => {
  const inherited = Object.freeze({
    LOCALAPPDATA: "Z:\\untrusted-local", USERPROFILE: "Z:\\untrusted-profile",
    orbwin_qualification_identity: "foreign-proof",
    orbwin_qualify_wsl_client_roots: "foreign",
    oRbWiN_QuAlIfY_WsL_ClIeNt_NoNcE: "foreign",
    orbwin_qualify_wsl_client_expected_roots: "foreign",
    orbwin_qualify_wsl_client_refresh: "foreign",
    orbwin_qualify_wsl_client_old_sha256: "foreign",
    orbwin_qualify_wsl_client_lock_sha256: "foreign",
  });
  const before = JSON.stringify(inherited), roots = observedRoots();
  const probe = nativeQualificationEnvironment(inherited, { nonce: rootsNonce });
  const actual = nativeQualificationEnvironment(inherited, { nonce: rootsNonce, roots, lockSha: "b".repeat(64) });
  assert.equal(probe.ORBWIN_QUALIFY_WSL_CLIENT_ROOTS, "1");
  assert.equal(probe.ORBWIN_QUALIFY_WSL_CLIENT_EXPECTED_ROOTS, undefined);
  assert.equal(probe.ORBWIN_QUALIFY_WSL_CLIENT_REFRESH, undefined);
  assert.equal(actual.ORBWIN_QUALIFY_WSL_CLIENT_ROOTS, undefined);
  assert.equal(actual.ORBWIN_QUALIFY_WSL_CLIENT_REFRESH, "1");
  assert.equal(actual.ORBWIN_QUALIFY_WSL_CLIENT_NONCE, rootsNonce);
  assert.equal(actual.ORBWIN_QUALIFY_WSL_CLIENT_OLD_SHA256, pins.oldSha);
  assert.deepEqual(JSON.parse(actual.ORBWIN_QUALIFY_WSL_CLIENT_EXPECTED_ROOTS), roots);
  assert.equal(nativeRootsPaths(JSON.parse(actual.ORBWIN_QUALIFY_WSL_CLIENT_EXPECTED_ROOTS), rootsNonce).globalDocker, roots.profile + "\\.docker\\config.json");
  assert.ok(Object.keys(actual).every((key) => !key.toUpperCase().startsWith("ORBWIN_") || key === key.toUpperCase()));
  assert.equal(JSON.stringify(inherited), before);
  assert.throws(() => nativeQualificationEnvironment(inherited, { nonce: rootsNonce, roots: { ...roots, nonce: "b".repeat(32) }, lockSha: "b".repeat(64) }));
});
