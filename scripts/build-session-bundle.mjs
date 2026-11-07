import {
  prepareSdkWinioInput,
  captureSdkWinio,
  settleSdkWinio,
} from "./sdk-winio-build-input.mjs";
// Local qualification bundle only. Private signing material stays in this
// process's memory; children and retained recipes receive only the public key.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash, generateKeyPairSync } from "node:crypto";
import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  sessionSourceNames,
  verifySessionSources,
} from "./session-source-catalog.mjs";
import { compileLanHelperResource } from "./lan-helper-resource.mjs";
import {
  canonicalLanAssetManifest,
  lanAssetRoles,
  signLanAssetManifest,
  verifyLanAssetBytes,
  verifyLanAssetManifest,
} from "./lan-asset-manifest.mjs";
import {
  lanBuildScripts,
  verifyLanBundleBuild,
} from "./lan-bundle-provenance.mjs";
import { buildArguments, freshOutputPath } from "./build-output.mjs";

const root = fileURLToPath(new URL("../", import.meta.url));
const { requested, flags } = buildArguments(process.argv.slice(2), [
  "--build-only",
]);
const output = freshOutputPath(root, requested);
const keys = generateKeyPairSync("ed25519");
const publicDer = keys.publicKey.export({ type: "spki", format: "der" });
assert.equal(publicDer.length, 44);
assert.equal(
  publicDer.subarray(0, 12).toString("hex"),
  "302a300506032b6570032100",
);
const publicKeyHex = publicDer.subarray(12).toString("hex");
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const source = resolve(root, "backend-tools/session");
const sourceNames = sessionSourceNames(source);
const sourceBytes = new Map(
  sourceNames.map((name) => [name, readFileSync(resolve(source, name))]),
);
const sources = Object.fromEntries(
  [...sourceBytes].map(([name, bytes]) => [name, hash(bytes)]),
);
const lockBytes = readFileSync(resolve(root, "upstream.lock.json"));
const lock = JSON.parse(lockBytes);
const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
const buildScripts = Object.fromEntries(
  lanBuildScripts.map((name) => [
    name,
    hash(readFileSync(resolve(root, "scripts", name))),
  ]),
);

// The child validates CREATE_NEW output before writing any component. It never
// receives the private KeyObject, a private key path or private key encoding.
execFileSync(
  process.execPath,
  [
    resolve(root, "scripts/build-session.mjs"),
    "--output-dir",
    requested,
    ...flags,
  ],
  {
    cwd: root,
    env: { ...process.env, ORBWIN_LAN_BUNDLE_PUBLIC_KEY: publicKeyHex },
    stdio: "inherit",
    windowsHide: true,
  },
);
const managerManifest = JSON.parse(
  readFileSync(resolve(output, "session-manifest.json")),
);
verifySessionSources(source, managerManifest, sourceNames);
assert.deepEqual(
  managerManifest.sources,
  sources,
  "Sources changed across the manager build",
);
assert.equal(managerManifest.lanBundlePublicKeyHex, publicKeyHex);

// Only immutable captured production inputs enter the helper/worker build.
// Resource generation writes to this fresh candidate, never the live source.
const frozen = resolve(output, ".lan-build-source");
mkdirSync(frozen);
for (const [name, bytes] of sourceBytes) {
  const target = resolve(frozen, name);
  mkdirSync(dirname(target), { recursive: true });
  writeFileSync(target, bytes, { flag: "wx" });
}
const resource = compileLanHelperResource(
  root,
  resolve(output, ".lan-resource"),
);
copyFileSync(
  resource.output,
  resolve(frozen, "lan-helper/lan_windows_amd64.syso"),
);
const lan = resolve(output, "lan");
mkdirSync(lan);
// Manager already staged the one SDK062 source in this same output. Helpers
// use their own frozen SDK modfile, sharing only that exact immutable module.
const helperInputOutput = resolve(output, ".lan-winio-input");
mkdirSync(helperInputOutput);
const sdkWinio = prepareSdkWinioInput({
  root,
  source: frozen,
  output: helperInputOutput,
  moduleCache:
    process.env.GOMODCACHE ??
    resolve(
      process.env.GOPATH ?? resolve(process.env.USERPROFILE, "go"),
      "pkg/mod",
    ),
  environment: process.env,
});
const env = sdkWinio.environment;
const sdkWinioTargets = { sdk: managerManifest.sdkWinio };
const ldflags =
  "-s -w -X orbwin.local/session/internal/lanhelper.bundleVerifierPublicKeyHex=" +
  publicKeyHex;
for (const [target, name, flags] of [
  ["./lan-helper", "orbwin-lan-helper.exe", [`-ldflags=${ldflags}`]],
  ["./lan-worker", "orbwin-lan-worker.exe", ["-ldflags=-s -w"]],
]) {
  const role = target === "./lan-helper" ? "helper" : "worker";
  const before = captureSdkWinio(sdkWinio, go, role);
  execFileSync(
    go,
    [
      "build",
      ...sdkWinio.flags,
      "-trimpath",
      "-buildvcs=false",
      ...flags,
      "-o",
      resolve(lan, name),
      target,
    ],
    {
      cwd: frozen,
      env,
      stdio: "inherit",
      windowsHide: true,
    },
  );
  sdkWinioTargets[role] = {
    portable: sdkWinio.portable,
    sourceFiles: sdkWinio.sourceFiles,
    capture: settleSdkWinio(
      sdkWinio,
      before,
      captureSdkWinio(sdkWinio, go, role),
    ),
  };
}
const manifest = canonicalLanAssetManifest({
  version: 1,
  assets: lanAssetRoles.map((role) => {
    const bytes = readFileSync(resolve(output, role.path));
    return {
      role: role.role,
      path: role.path,
      sha256: hash(bytes),
      size: bytes.length,
    };
  }),
});
const signature = signLanAssetManifest(manifest, keys.privateKey);
verifyLanAssetManifest(manifest, signature, publicKeyHex);
for (const role of lanAssetRoles)
  verifyLanAssetBytes(
    manifest,
    role.role,
    readFileSync(resolve(output, role.path)),
  );
verifySessionSources(source, { sources }, sessionSourceNames(source));
assert.ok(
  readFileSync(resolve(root, "upstream.lock.json")).equals(lockBytes),
  "Upstream lock changed",
);
for (const name of ["lan.manifest", "lan.rc"])
  assert.equal(resource.proof.sources[name], sources[`lan-helper/${name}`]);
for (const [name, digest] of Object.entries(buildScripts))
  assert.equal(
    hash(readFileSync(resolve(root, "scripts", name))),
    digest,
    "SDK build recipe changed during compilation",
  );
writeFileSync(resolve(lan, "assets.json"), manifest, { flag: "wx" });
writeFileSync(resolve(lan, "assets.sig"), signature, { flag: "wx" });
writeFileSync(
  resolve(output, "lan-build-manifest.json"),
  JSON.stringify(
    {
      schema: 1,
      testsNotRun: flags.has("--build-only"),
      purpose:
        "Local qualification; no publisher signing or runtime acceptance claim",
      publicKeyHex,
      sources,
      sdkWinioTargets,
      upstreamLockSha256: hash(lockBytes),
      manifestSha256: hash(manifest),
      signatureSha256: hash(Buffer.from(signature)),
      resource: resource.proof,
      binaryBuild: {
        go: lock.go.version,
        goos: "windows",
        goarch: "amd64",
        cgo: false,
        manager: "build-session.mjs",
        helperLdflags: ldflags,
        workerLdflags: "-s -w",
      },
      buildScripts,
    },
    null,
    2,
  ),
  { flag: "wx" },
);
verifyLanBundleBuild(root, output);
console.log(
  "Built a private-key-free qualification artifact; no executable was installed or started.",
);
