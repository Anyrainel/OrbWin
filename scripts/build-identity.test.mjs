import test from "node:test";
import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  writeFileSync,
  rmSync,
  readFileSync,
  cpSync,
  unlinkSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve, dirname, basename } from "node:path";
import { verifyBuild, sha256 } from "./build-identity.mjs";
import {
  forwarderSourceNames,
  forwarderLicenseNames,
  forwarderFlags,
} from "./rdd-forwarder-build.mjs";
import { guestLicenseNames } from "./backend-guest-input.mjs";
import{fixture as dependencyFixture}from"./lima-wsl2-test-fixture.mjs";
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "orbwin-build-test-"));
  t.after(() => {
    const target = resolve(root);
    assert.equal(dirname(target), resolve(tmpdir()));
    assert.match(basename(target), /^orbwin-build-test-/);
    rmSync(target, { recursive: true, force: true });
  });
  mkdirSync(join(root, "artifacts/backend"), { recursive: true });
  writeFileSync(join(root, "change.patch"), "expected patch");
  writeFileSync(
    join(root, "guest-packages.lock.json"),
    "expected package pins",
  );
  writeFileSync(
    join(root, "upstream.lock.json"),
    JSON.stringify({ commit: "pin", patches: ["change.patch"] }),
  );
  writeFileSync(join(root, "artifacts/backend/rdd.exe"), "expected binary");
  const put = (name, bytes) => {
    const path = resolve(root, name);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, bytes);
  };
  const forwarder = {
    schema: 1,
    binarySha256: sha256("expected forwarder"),
    sources: {},
    flags: forwarderFlags,
    licenses: {},
    recipeSha256: sha256("fixture recipe"),
  };
  put("artifacts/backend/rdd-forwarder.exe", "expected forwarder");
  put("scripts/rdd-forwarder-build.mjs", "fixture recipe");
  for (const name of forwarderSourceNames) {
    put(`.upstream/rancher-desktop/rdd/${name}`, `fixture ${name}`);
    forwarder.sources[name] = sha256(`fixture ${name}`);
  }
  for (const name of forwarderLicenseNames) {
    put(`artifacts/backend/forwarder-licenses/${name}`, `license ${name}`);
    forwarder.licenses[name] = sha256(`license ${name}`);
  }
  put(
    "artifacts/backend/forwarder-build-manifest.json",
    JSON.stringify(forwarder),
  );
  const deps=dependencyFixture(t,{root,output:join(root,"artifacts/backend")}),prepared=deps.prepare();
  const build = {
    sourceCommit: "pin",
    limaWsl2:prepared.manifest,portableLima:prepared.portable,portableWinio:prepared.portableWinio,
    winioRuntimeOfferSHA256:sha256(readFileSync(join(root,"winio-runtime-offer.json"))),
    buildScripts:Object.fromEntries(["lima-wsl2-build-input.mjs","build-backend.mjs"].map(n=>[n,sha256(readFileSync(join(root,"scripts",n)))])),
    guestPackagesLockSha256: sha256("expected package pins"),
    binarySha256: sha256("expected binary"),
    forwarder,
    patches: [{ path: "change.patch", sha256: sha256("expected patch") }],
  };
  const writeBuild = () =>
    writeFileSync(
      join(root, "artifacts/backend/build-manifest.json"),
      JSON.stringify(build),
    );
  writeBuild();
  return { root, put, build, writeBuild };
}
test("runtime proof rejects a changed binary or patch even at the same source commit", (t) => {
  const { root, put } = fixture(t);
  assert.equal(verifyBuild(root).sourceCommit, "pin");
  put("artifacts/backend/rdd-forwarder.exe", "changed forwarder");
  assert.throws(() => verifyBuild(root));
  put("artifacts/backend/rdd-forwarder.exe", "expected forwarder");
  writeFileSync(join(root, "artifacts/backend/rdd.exe"), "different binary");
  assert.throws(() => verifyBuild(root), /binary differs/);
  writeFileSync(join(root, "artifacts/backend/rdd.exe"), "expected binary");
  writeFileSync(join(root, "change.patch"), "different patch");
  assert.throws(() => verifyBuild(root), /differs from patch/);
  writeFileSync(join(root, "change.patch"), "expected patch");
  writeFileSync(join(root, "guest-packages.lock.json"), "changed package pins");
  assert.throws(() => verifyBuild(root), /guest package lock differs/);
});

test("guest build proof survives minimal legal-tree relocation and refuses stale or missing evidence", (t) => {
  const { root, put, build, writeBuild } = fixture(t);
  const lock = {},
    files = [
      [
        "distro",
        "distro.tar.xz",
        "520bc208df9fe3beaf12e80d08e3eb66f39e5dd2fa5db29bf90d14501c4422a9",
      ],
      [
        "mkcert",
        "mkcert",
        "6d31c65b03972c6dc4a14ab429f2928300518b26503f58723e532d1b0a3bbb52",
      ],
      [
        "nerdctl",
        "nerdctl-full.tar.gz",
        "b3ab8564c8fa6feb89d09bee881211b700b047373c767bec38256d0d68f93074",
      ],
    ].map(([name, filename, sha256]) => ({
      name,
      path: `upstream/${filename}`,
      sha256,
      bytes: 100,
      source: "Z:/original-build-no-longer-present/input",
    }));
  for (const name of ["libwrap", "libwrap-source", "socat", "socat-source"]) {
    const bytes = `fixture ${name} RPM`;
    lock[name] = {
      version: "1",
      assets: [
        {
          platform: "linux",
          filename: `${name}.rpm`,
          checksum: `sha256:${sha256(bytes)}`,
          url: `https://example.invalid/${name}.rpm`,
        },
      ],
    };
    files.push({
      name,
      path: `packages/${name}.rpm`,
      sha256: sha256(bytes),
      bytes: bytes.length,
      source: "Z:/absent-cache/input",
    });
    if (name.endsWith("-source"))
      put(`artifacts/backend/licenses/guest-packages/${name}.rpm`, bytes);
  }
  const lockBytes = JSON.stringify(lock);
  put("guest-packages.lock.json", lockBytes);
  put("scripts/backend-guest-input.mjs", "fixture guest recipe");
  build.distroEmbedded = true;
  build.guestPackagesLockSha256 = sha256(lockBytes);
  const manifest = {
    schema: 1,
    offline: true,
    cacheOriginalsReadOnly: true,
    upstreamDependenciesSHA256:
      "10cc111e461319f59ad3689230a439225ac6cf1d58a8b9b7b1afe551cf647893",
    guestPackagesLockSHA256: sha256(lockBytes),
    recipeSHA256: sha256("fixture guest recipe"),
    files,
  };
  const manifestBytes = JSON.stringify(manifest);
  put(
    "artifacts/backend/licenses/guest-packages/input-manifest.json",
    manifestBytes,
  );
  for (const name of [
    "socat-COPYING",
    "socat-COPYING.OpenSSL",
    "libwrap-DISCLAIMER",
  ])
    put(
      `artifacts/backend/licenses/guest-packages/${name}`,
      `fixture notice ${name}`,
    );
  put(
    "artifacts/backend/licenses/guest-packages/guest-packages.lock.json",
    lockBytes,
  );
  build.guestInputs = {
    path: "licenses/guest-packages/input-manifest.json",
    sha256: sha256(manifestBytes),
    offline: true,
    files: 7,
    licenses: Object.fromEntries(
      guestLicenseNames.map((name) => [
        name,
        sha256(
          readFileSync(
            resolve(root, "artifacts/backend/licenses/guest-packages", name),
          ),
        ),
      ]),
    ),
  };
  writeBuild();
  assert.equal(verifyBuild(root).distroEmbedded, true);
  const runtime = resolve(root, "relocated/runtime"),
    backend = resolve(runtime, "backend"),
    legal = resolve(runtime, "licenses");
  mkdirSync(backend, { recursive: true });
  for (const name of [
    "rdd.exe",
    "rdd-forwarder.exe",
    "forwarder-build-manifest.json",
    "build-manifest.json",
    "forwarder-licenses",
  ])
    cpSync(resolve(root, "artifacts/backend", name), resolve(backend, name), {
      recursive: true,
    });
  cpSync(resolve(root, "artifacts/backend/licenses"), legal, {
    recursive: true,
  });
  // No guest-inputs archives or original cache exists in this fixture.
  assert.equal(
    verifyBuild(root, backend, { licensesDirectory: legal }).distroEmbedded,
    true,
  );
  assert.throws(() => verifyBuild(root, backend)); // no implicit legal-directory fallback
  put(
    "relocated/runtime/licenses/guest-packages/input-manifest.json",
    "changed",
  );
  assert.throws(
    () => verifyBuild(root, backend, { licensesDirectory: legal }),
    /manifest differs/,
  );
  put(
    "relocated/runtime/licenses/guest-packages/input-manifest.json",
    manifestBytes,
  );
  unlinkSync(resolve(legal, "guest-packages/libwrap-source.rpm"));
  assert.throws(() => verifyBuild(root, backend, { licensesDirectory: legal }));
  put(
    "relocated/runtime/licenses/guest-packages/libwrap-source.rpm",
    "foreign source RPM",
  );
  assert.throws(
    () => verifyBuild(root, backend, { licensesDirectory: legal }),
    /legal file differs/,
  );
});
