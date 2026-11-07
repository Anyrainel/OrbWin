import {
  installSdkFixture,
  sdkFixtureContext,
  sdkFixtureRecord,
} from "./sdk-winio-test-fixtures.mjs";
import {
  validateSdkWinioMetadata,
  sdkWinioSourceFiles,
} from "./sdk-winio-build-input.mjs";
import assert from "node:assert/strict";
import { createHash, generateKeyPairSync } from "node:crypto";
import {
  cpSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import {
  sessionSourceFolders,
  sessionSourceNames,
} from "./session-source-catalog.mjs";
import {
  canonicalLanAssetManifest,
  lanAssetRoles,
  signLanAssetManifest,
} from "./lan-asset-manifest.mjs";
import {
  lanBuildScripts,
  verifyLanBundleBuild,
  verifyPackagedLanBundle,
} from "./lan-bundle-provenance.mjs";
import { upgradeSdkBuildInput } from "./upgrade-sdk-build-input.mjs";
import { prepareColdPayloadInput } from "./session-cold-payload-input.mjs";
import { verifySDKBuildBinding } from "./package-contract.mjs";

const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "orbwin-lan-build-proof-"));
  const exact = realpathSync(root);
  t.after(() => {
    assert.equal(realpathSync(root), exact);
    rmSync(exact, { recursive: true });
  });
  const source = join(root, "backend-tools/session"),
    output = join(root, "artifacts/candidate");
  const put = (name, bytes) => {
    const path = join(root, name);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, bytes);
  };
  for (const folder of sessionSourceFolders)
    mkdirSync(join(source, folder), { recursive: true });
  for (const name of [
    "go.mod",
    "go.sum",
    "main.go",
    "internal/coldassets/compiled.go",
    "lan-helper/main_windows.go",
    "lan-worker/main_windows.go",
    "lan-helper/lan.rc",
    "lan-helper/lan.manifest",
  ])
    put(`backend-tools/session/${name}`, `synthetic source ${name}`);
  const sdkFixture = installSdkFixture(root, output);
  const sources = Object.fromEntries(
    sessionSourceNames(source).map((name) => [
      name,
      hash(readFileSync(join(source, name))),
    ]),
  );
  put("upstream.lock.json", "{}");
  const scripts = Object.fromEntries(
    lanBuildScripts.map((name) => {
      if (!Object.hasOwn(sdkFixture.lock.recipeHashes, name))
        put(`scripts/${name}`, `synthetic recipe ${name}`);
      return [name, hash(readFileSync(join(root, "scripts", name)))];
    }),
  );
  const keys = generateKeyPairSync("ed25519"),
    publicKeyHex = keys.publicKey
      .export({ type: "spki", format: "der" })
      .subarray(12)
      .toString("hex");
  const embedded = Buffer.alloc(64);
  Buffer.from([0x7f, 69, 76, 70, 2, 1, 1]).copy(embedded);
  embedded.writeUInt16LE(62, 18);
  const assets = [
    Buffer.from(
      "synthetic manager " + embedded.toString("base64") + hash(embedded),
    ),
    Buffer.from("synthetic helper synthetic source lan-helper/lan.manifest"),
    Buffer.from("synthetic worker"),
  ];
  for (const [index, role] of lanAssetRoles.entries())
    put(`artifacts/candidate/${role.path}`, assets[index]);
  const manifest = canonicalLanAssetManifest({
    version: 1,
    assets: lanAssetRoles.map((role, index) => ({
      role: role.role,
      path: role.path,
      sha256: hash(assets[index]),
      size: assets[index].length,
    })),
  });
  const signature = signLanAssetManifest(manifest, keys.privateKey);
  put("artifacts/candidate/lan/assets.json", manifest);
  put("artifacts/candidate/lan/assets.sig", signature);
  const elf = Buffer.alloc(64);
  Buffer.from([0x7f, 69, 76, 70, 2, 1, 1]).copy(elf);
  elf.writeUInt16LE(62, 18);
  put("artifacts/candidate/orbwin-services", elf);
  put("artifacts/candidate/orbwin-trust", elf);
  const coldPayloads = prepareColdPayloadInput(root, output).manifest;
  const sdkWinioTargets = Object.fromEntries(
    ["sdk", "helper", "worker"].map((role) => {
      const context = sdkFixtureContext(
        source,
        join(root, "sdk-fixture-module"),
        role,
      );
      const record = sdkFixtureRecord(context),
        capture = {
          record,
          files: validateSdkWinioMetadata(record, sdkFixture.lock, context),
        };
      return [
        role,
        {
          portable: sdkFixture.offer,
          sourceFiles: sdkWinioSourceFiles(output),
          capture: { before: capture, after: structuredClone(capture) },
        },
      ];
    }),
  );
  put(
    "artifacts/candidate/session-manifest.json",
    JSON.stringify({
      sources,
      lanBundlePublicKeyHex: publicKeyHex,
      binarySha256: hash(assets[0]),
      coldPayloads,
      sdkWinio: sdkWinioTargets.sdk,
      testsNotRun: true,
    }),
  );
  put(
    "artifacts/candidate/lan-build-manifest.json",
    JSON.stringify({
      schema: 1,
      testsNotRun: true,
      sdkWinioTargets,
      sources,
      publicKeyHex,
      buildScripts: scripts,
      upstreamLockSha256: hash(Buffer.from("{}")),
      manifestSha256: hash(manifest),
      signatureSha256: hash(Buffer.from(signature)),
      resource: {
        success: true,
        machine: "amd64",
        sources: {
          "lan.manifest": sources["lan-helper/lan.manifest"],
          "lan.rc": sources["lan-helper/lan.rc"],
        },
      },
    }),
  );
  return { root, output, put };
}

test("bundle packaging binds signed executable bytes, full source closure and helper resources", (t) => {
  const { root, output } = fixture(t);
  const result = verifyLanBundleBuild(root, output);
  assert.equal(Object.keys(result.pins).length, 6);
  assert.equal(result.assets.length, 3);
});

test("relocated package verifies its copied payloads without build-only overlay files", (t) => {
  const { root, output } = fixture(t);
  const packaged = join(root, "assembly/runtime/backend");
  cpSync(output, packaged, {
    recursive: true,
    filter: (path) => !path.includes(".cold-build-input"),
  });
  assert.deepEqual(
    verifyPackagedLanBundle(root, packaged).pins,
    verifyLanBundleBuild(root, output).pins,
  );
  assert.throws(() => verifyLanBundleBuild(root, packaged), { code: "ENOENT" });
});

test("relocated packages retain source, recipe, signature and copied-byte checks", (t) => {
  for (const name of [
    "assembly/runtime/backend/orbwin-services",
    "assembly/runtime/backend/orbwin-trust",
    "assembly/runtime/backend/orbwin-session.exe",
    "assembly/runtime/backend/lan/orbwin-lan-helper.exe",
    "assembly/runtime/backend/lan/orbwin-lan-worker.exe",
    "assembly/runtime/backend/lan/assets.json",
    "assembly/runtime/backend/lan/assets.sig",
    "assembly/runtime/backend/session-manifest.json",
    "assembly/runtime/backend/lan-build-manifest.json",
    "backend-tools/session/lan-helper/main_windows.go",
    "scripts/build-session-bundle.mjs",
  ]) {
    const { root, output, put } = fixture(t);
    const packaged = join(root, "assembly/runtime/backend");
    cpSync(output, packaged, {
      recursive: true,
      filter: (path) => !path.includes(".cold-build-input"),
    });
    put(name, "changed");
    assert.throws(() => verifyPackagedLanBundle(root, packaged), name);
  }
});
test("changed helper entry or dialog resource invalidates an otherwise signed build", (t) => {
  for (const name of [
    "lan-helper/main_windows.go",
    "lan-helper/lan.manifest",
    "lan-worker/main_windows.go",
  ]) {
    const { root, output, put } = fixture(t);
    put(`backend-tools/session/${name}`, "changed source");
    assert.throws(
      () => verifyLanBundleBuild(root, output),
      /Stale helper source/,
    );
  }
});
test("worker replacement and changed build recipes refuse before package copying", (t) => {
  const worker = fixture(t);
  worker.put(
    "artifacts/candidate/lan/orbwin-lan-worker.exe",
    "replaced worker",
  );
  assert.throws(
    () => verifyLanBundleBuild(worker.root, worker.output),
    /size differs|digest differs/,
  );
  const recipe = fixture(t);
  recipe.put("scripts/build-session-bundle.mjs", "changed recipe");
  assert.throws(
    () => verifyLanBundleBuild(recipe.root, recipe.output),
    /recipe changed/,
  );
});

test("backend SDK pins require a signed source-matched bundle and reject later drift", (t) => {
  const { root, output, put } = fixture(t);
  assert.throws(
    () => upgradeSdkBuildInput(root, "relative-input"),
    /absolute build input/,
  );
  const input = upgradeSdkBuildInput(root, output);
  const verified = verifyLanBundleBuild(root, output);
  assert.equal(
    input.manifest.pins.upgradeSDKManagerSHA256,
    verified.pins["orbwin-session.exe"],
  );
  assert.equal(
    input.manifest.pins.upgradeSDKManifestSHA256,
    verified.pins["lan/assets.json"],
  );
  assert.equal(
    input.manifest.pins.upgradeSDKSignatureSHA256,
    verified.pins["lan/assets.sig"],
  );
  assert.equal(
    input.manifest.pins.upgradeSDKHelperSHA256,
    verified.pins["lan/orbwin-lan-helper.exe"],
  );
  assert.equal(
    input.manifest.pins.upgradeSDKWorkerSHA256,
    verified.pins["lan/orbwin-lan-worker.exe"],
  );
  input.assertUnchanged();
  put(
    "artifacts/candidate/lan/orbwin-lan-worker.exe",
    "changed after admission",
  );
  assert.throws(() => input.assertUnchanged(), /size differs|digest differs/);
  assert.throws(
    () => upgradeSdkBuildInput(root, output),
    /size differs|digest differs/,
  );
});
test("backend and independently valid signed SDK bundles cannot mix epochs", (t) => {
  const first = fixture(t),
    second = fixture(t);
  const input = upgradeSdkBuildInput(first.root, first.output);
  const verified = verifyLanBundleBuild(first.root, first.output);
  const key = JSON.parse(
    readFileSync(join(first.output, "lan-build-manifest.json")),
  ).publicKeyHex;
  const build = { upgradeSDK: input.manifest };
  verifySDKBuildBinding(build, verified, key);
  const other = verifyLanBundleBuild(second.root, second.output);
  const otherKey = JSON.parse(
    readFileSync(join(second.output, "lan-build-manifest.json")),
  ).publicKeyHex;
  assert.throws(() => verifySDKBuildBinding(build, other, otherKey));
  const manager = JSON.parse(
    readFileSync(join(first.output, "session-manifest.json")),
  );
  delete manager.testsNotRun;
  first.put(
    "artifacts/candidate/session-manifest.json",
    JSON.stringify(manager),
  );
  assert.throws(
    () => verifyLanBundleBuild(first.root, first.output),
    /test execution provenance/,
  );
});
