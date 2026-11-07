// Build-time file staging only. Cache originals are read-only inputs; never
// invoke the upstream downloader, touch/prune its cache or replace selected
// guest-package artifacts. A missing/corrupt offline asset refuses the build.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  constants,
  copyFileSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  statSync,
  writeFileSync,
} from "node:fs";
import {
  basename,
  dirname,
  isAbsolute,
  relative,
  resolve,
  sep,
} from "node:path";

const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
// Narrow adaptation to the selected upstream manifest. Refuse dependency bumps
// until its Windows/amd64 selection and cache naming are reviewed together.
const dependencySHA256 =
  "10cc111e461319f59ad3689230a439225ac6cf1d58a8b9b7b1afe551cf647893";
const upstreamAssets = [
  [
    "distro",
    "0.2.11",
    "distro.tar.xz",
    "distro.v0.2.11.amd64.tar.xz",
    "520bc208df9fe3beaf12e80d08e3eb66f39e5dd2fa5db29bf90d14501c4422a9",
  ],
  [
    "mkcert",
    "1.4.4",
    "mkcert",
    "mkcert-v1.4.4-linux-amd64",
    "6d31c65b03972c6dc4a14ab429f2928300518b26503f58723e532d1b0a3bbb52",
  ],
  [
    "nerdctl",
    "2.1.2",
    "nerdctl-full.tar.gz",
    "nerdctl-full-2.1.2-linux-amd64.tar.gz",
    "b3ab8564c8fa6feb89d09bee881211b700b047373c767bec38256d0d68f93074",
  ],
];
function exactFile(file, allowEmpty = false) {
  const actual = realpathSync(file),
    expected = resolve(file);
  assert.equal(
    process.platform === "win32" ? actual.toLowerCase() : actual,
    process.platform === "win32" ? expected.toLowerCase() : expected,
    "Redirected guest input refused",
  );
  const info = statSync(file);
  assert.ok(
    info.isFile() &&
      (allowEmpty || info.size > 0) &&
      info.size <= 512 * 1024 ** 2,
    "Guest input kind/size refused",
  );
  return readFileSync(file);
}
function target(base, name) {
  assert.ok(
    typeof name === "string" && !name.includes("\\") && !isAbsolute(name),
  );
  const result = resolve(base, name),
    rel = relative(base, result);
  assert.ok(
    rel && rel !== ".." && !rel.startsWith(`..${sep}`) && !isAbsolute(rel),
  );
  return result;
}

// Exposed separately so filesystem regressions exercise actual copy/verification
// behavior with synthetic payloads, without requiring upstream caches or PEs.
export function stageVerifiedGuestFiles(output, assets) {
  assert.ok(assets.length > 0 && assets.length <= 16);
  const seen = new Set();
  let total = 0;
  for (const asset of assets) {
    assert.match(asset.sha256, /^[a-f0-9]{64}$/);
    const destination = target(output, asset.path).toLowerCase();
    assert.ok(!seen.has(destination), "Duplicate guest output");
    seen.add(destination);
    const bytes = exactFile(asset.source);
    assert.equal(hash(bytes), asset.sha256, "Guest cache checksum differs");
    total += bytes.length;
    assert.ok(total <= 1024 ** 3, "Guest input aggregate bound");
  }
  mkdirSync(output); // Exclusive; no retry or replacement of an earlier attempt.
  const files = [];
  for (const asset of assets) {
    const destination = target(output, asset.path);
    mkdirSync(dirname(destination), { recursive: true });
    copyFileSync(asset.source, destination, constants.COPYFILE_EXCL);
    const bytes = exactFile(destination);
    assert.equal(hash(bytes), asset.sha256, "Guest input changed during copy");
    assert.equal(
      hash(exactFile(asset.source)),
      asset.sha256,
      "Guest cache changed during copy",
    );
    files.push({
      name: asset.name,
      path: asset.path,
      source: resolve(asset.source),
      bytes: bytes.length,
      sha256: asset.sha256,
    });
  }
  return files;
}

function selectedAssets(root, packageLockBytes) {
  const packageLock = JSON.parse(packageLockBytes);
  assert.deepEqual(Object.keys(packageLock).sort(), [
    "libwrap",
    "libwrap-source",
    "socat",
    "socat-source",
  ]);
  const assets = upstreamAssets.map(
    ([name, version, filename, cacheName, sha256]) => ({
      name,
      path: `upstream/${filename}`,
      sha256,
      source: resolve(
        root,
        ".tools/guest-cache",
        name,
        `v${version}`,
        cacheName,
      ),
    }),
  );
  for (const [name, entry] of Object.entries(packageLock)) {
    assert.equal(entry.assets.length, 1);
    const asset = entry.assets[0];
    assert.equal(asset.platform, "linux");
    assert.ok(asset.arch === undefined || asset.arch === "amd64");
    assert.equal(asset.filename, `${name}.rpm`);
    assert.match(asset.checksum, /^sha256:[a-f0-9]{64}$/);
    assert.match(entry.version, /^[a-zA-Z0-9.+_-]+$/);
    const url = new URL(asset.url);
    assert.equal(url.protocol, "https:");
    const cacheName = basename(url.pathname);
    assert.ok(cacheName && cacheName !== "." && cacheName !== "..");
    assets.push({
      name,
      path: `packages/${asset.filename}`,
      sha256: asset.checksum.slice(7),
      source: resolve(
        root,
        ".tools/guest-package-cache",
        name,
        `v${entry.version.replace(/^v/, "")}`,
        cacheName,
      ),
    });
  }
  return assets;
}

export function stageBackendGuestInputs(
  root,
  source,
  output,
  packageLockBytes,
) {
  const dependencyBytes = readFileSync(resolve(source, "dependencies.yaml"));
  assert.equal(
    hash(Buffer.from(dependencyBytes.toString("utf8").replace(/\r\n/g, "\n"))),
    dependencySHA256,
    "Upstream guest dependency selection requires review",
  );
  const assets = selectedAssets(root, packageLockBytes);
  const directory = resolve(output, "guest-inputs");
  const files = stageVerifiedGuestFiles(directory, assets);
  const manifest = {
    schema: 1,
    offline: true,
    cacheOriginalsReadOnly: true,
    upstreamDependenciesSHA256: dependencySHA256,
    guestPackagesLockSHA256: hash(packageLockBytes),
    recipeSHA256: hash(
      readFileSync(resolve(root, "scripts/backend-guest-input.mjs")),
    ),
    files,
  };
  const manifestPath = "licenses/guest-packages/input-manifest.json";
  const bytes = Buffer.from(JSON.stringify(manifest, null, 2) + "\n");
  mkdirSync(dirname(resolve(output, manifestPath)), { recursive: true });
  writeFileSync(resolve(output, manifestPath), bytes, { flag: "wx" });
  return {
    upstream: resolve(directory, "upstream"),
    packages: resolve(directory, "packages"),
    manifest: {
      path: manifestPath,
      sha256: hash(bytes),
      offline: true,
      files: files.length,
    },
    assertUnchanged() {
      assert.equal(
        hash(readFileSync(resolve(output, manifestPath))),
        hash(bytes),
      );
      assert.equal(
        hash(readFileSync(resolve(source, "dependencies.yaml"))),
        hash(dependencyBytes),
      );
      for (const file of files) {
        assert.equal(
          hash(exactFile(file.source)),
          file.sha256,
          "Original guest cache changed",
        );
        assert.equal(
          hash(exactFile(target(directory, file.path))),
          file.sha256,
          "Staged guest input changed",
        );
      }
    },
  };
}

export const guestLicenseNames = Object.freeze([
  "socat-COPYING",
  "socat-COPYING.OpenSSL",
  "libwrap-DISCLAIMER",
  "guest-packages.lock.json",
  "libwrap-source.rpm",
  "socat-source.rpm",
]);

// Artifact verification reads the copied legal/provenance tree only. Original
// absolute cache paths remain build DATA and are never opened after relocation.
export function verifyBackendGuestInputs(
  root,
  directory,
  build,
  licensesDirectory = resolve(directory, "licenses"),
) {
  const input = build.guestInputs;
  assert.equal(input.path, "licenses/guest-packages/input-manifest.json");
  assert.equal(input.offline, true);
  assert.equal(input.files, 7);
  const legal = resolve(licensesDirectory, "guest-packages");
  const bytes = exactFile(resolve(legal, "input-manifest.json"));
  assert.equal(hash(bytes), input.sha256, "Guest input manifest differs");
  const manifest = JSON.parse(bytes),
    lockBytes = readFileSync(resolve(root, "guest-packages.lock.json"));
  assert.equal(manifest.schema, 1);
  assert.equal(manifest.offline, true);
  assert.equal(manifest.cacheOriginalsReadOnly, true);
  assert.equal(manifest.upstreamDependenciesSHA256, dependencySHA256);
  assert.equal(manifest.guestPackagesLockSHA256, hash(lockBytes));
  assert.equal(
    manifest.recipeSHA256,
    hash(readFileSync(resolve(root, "scripts/backend-guest-input.mjs"))),
  );
  const assets = selectedAssets(root, lockBytes);
  assert.equal(manifest.files.length, 7);
  assert.deepEqual(
    manifest.files.map((f) => f.name).sort(),
    assets.map((a) => a.name).sort(),
  );
  for (const asset of assets) {
    const file = manifest.files.find((f) => f.name === asset.name);
    assert.equal(file.path, asset.path);
    assert.equal(file.sha256, asset.sha256);
    assert.ok(
      Number.isSafeInteger(file.bytes) &&
        file.bytes > 0 &&
        file.bytes <= 512 * 1024 ** 2,
    );
    assert.ok(typeof file.source === "string" && file.source.length <= 32768);
  }
  assert.deepEqual(
    Object.keys(input.licenses).sort(),
    [...guestLicenseNames].sort(),
  );
  for (const name of guestLicenseNames) {
    const value = exactFile(resolve(legal, name));
    assert.equal(hash(value), input.licenses[name], "Guest legal file differs");
    if (name.endsWith("-source.rpm")) {
      const asset = assets.find((a) => a.path === `packages/${name}`);
      assert.equal(
        hash(value),
        asset.sha256,
        "Guest source RPM differs from lock",
      );
    }
    if (name === "guest-packages.lock.json")
      assert.equal(hash(value), hash(lockBytes));
  }
  return manifest;
}

// The caller enumerates canonical Git paths before/after the build. Generated
// outputs may be ignored only if they were absent from this original inventory.
export function captureBackendSourceFiles(source, names) {
  assert.equal(
    new Set(names).size,
    names.length,
    "Duplicate canonical source path",
  );
  return names
    .slice()
    .sort()
    .map((name) => ({
      path: name,
      sha256: hash(exactFile(target(source, name), true)),
    }));
}
export function verifyBackendSourceFiles(source, snapshot, names) {
  assert.deepEqual(
    captureBackendSourceFiles(source, names),
    snapshot,
    "Canonical backend source changed during build",
  );
}
