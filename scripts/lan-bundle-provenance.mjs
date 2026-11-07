import { verifySdkWinioProvenance } from "./sdk-winio-build-input.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { resolve } from "node:path";
import {
  sessionSourceNames,
  verifySessionSources,
} from "./session-source-catalog.mjs";
import {
  lanAssetRoles,
  verifyLanAssetManifest,
  verifyLanAssetBytes,
} from "./lan-asset-manifest.mjs";
import {
  verifyColdPayloadArtifact,
  verifyColdPayloadInput,
} from "./session-cold-payload-input.mjs";

export const lanBuildScripts = Object.freeze([
  "sdk-winio-build-input.mjs",
  "winio-build-input.mjs",
  "winio-build-input-fs.mjs",
  "build-session-bundle.mjs",
  "build-session.mjs",
  "build-output.mjs",
  "session-source-catalog.mjs",
  "session-source-folders.txt",
  "session-cold-payload-input.mjs",
  "lan-helper-resource.mjs",
  "lan-asset-manifest.mjs",
  "lan-bundle-provenance.mjs",
]);
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");

function verifyBundle(root, directory, verifyPayload) {
  const read = (name) => readFileSync(resolve(directory, name));
  const build = JSON.parse(read("lan-build-manifest.json"));
  assert.equal(build.schema, 1, "LAN build schema differs");
  verifySessionSources(
    resolve(root, "backend-tools/session"),
    build,
    sessionSourceNames(resolve(root, "backend-tools/session")),
  );
  assert.equal(
    build.upstreamLockSha256,
    hash(readFileSync(resolve(root, "upstream.lock.json"))),
  );
  assert.deepEqual(
    Object.keys(build.buildScripts).sort(),
    [...lanBuildScripts].sort(),
  );
  for (const name of lanBuildScripts)
    assert.equal(
      build.buildScripts[name],
      hash(readFileSync(resolve(root, "scripts", name))),
      "LAN build recipe changed",
    );
  const manager = JSON.parse(read("session-manifest.json"));
  assert.deepEqual(
    manager.sources,
    build.sources,
    "LAN/SDK source generations differ",
  );
  assert.equal(manager.lanBundlePublicKeyHex, build.publicKeyHex);
  assert.deepEqual(Object.keys(build.sdkWinioTargets).sort(), [
    "helper",
    "sdk",
    "worker",
  ]);
  assert.deepEqual(build.sdkWinioTargets.sdk, manager.sdkWinio);
  for (const role of ["sdk", "helper", "worker"])
    verifySdkWinioProvenance(
      root,
      directory,
      build.sdkWinioTargets[role],
      role,
    );
  assert.equal(
    typeof manager.testsNotRun,
    "boolean",
    "Missing SDK test execution provenance",
  );
  assert.equal(
    build.testsNotRun,
    manager.testsNotRun,
    "SDK test execution provenance differs",
  );
  verifyPayload(root, directory, manager.coldPayloads);
  const manifest = read("lan/assets.json"),
    signature = read("lan/assets.sig");
  assert.ok(
    Buffer.from(signature.toString("ascii"), "ascii").equals(signature),
    "LAN signature is not exact ASCII",
  );
  assert.equal(hash(manifest), build.manifestSha256);
  assert.equal(hash(signature), build.signatureSha256);
  const parsed = verifyLanAssetManifest(
    manifest,
    signature.toString("ascii"),
    build.publicKeyHex,
  );
  const pins = {
    "lan/assets.json": hash(manifest),
    "lan/assets.sig": hash(signature),
    "lan-build-manifest.json": hash(read("lan-build-manifest.json")),
  };
  for (const role of lanAssetRoles) {
    const bytes = read(role.path);
    verifyLanAssetBytes(manifest, role.role, bytes);
    pins[role.path] = hash(bytes);
  }
  assert.equal(manager.binarySha256, pins["orbwin-session.exe"]);
  assert.equal(build.resource?.success, true);
  assert.equal(build.resource?.machine, "amd64");
  const helper = read("lan/orbwin-lan-helper.exe");
  for (const name of ["lan.manifest", "lan.rc"])
    assert.equal(
      build.resource.sources[name],
      build.sources[`lan-helper/${name}`],
    );
  assert.ok(
    helper.includes(
      readFileSync(
        resolve(root, "backend-tools/session/lan-helper/lan.manifest"),
      ),
    ),
    "Built helper lacks the exact dialog manifest resource",
  );
  return { pins, assets: parsed.assets };
}

// Build inputs retain the original Go overlay and absolute source mapping.
export function verifyLanBundleBuild(root, directory) {
  return verifyBundle(root, directory, verifyColdPayloadInput);
}

// Assembly copies retain signed assets and payload provenance, while the
// build-only overlay stays with its original SDK input directory.
export function verifyPackagedLanBundle(root, directory) {
  return verifyBundle(root, directory, verifyColdPayloadArtifact);
}
