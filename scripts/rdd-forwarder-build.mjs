import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { dirname, resolve } from "node:path";

const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
export const forwarderSourceNames = Object.freeze([
  "src/rdd-forwarder/shim.cpp",
  "src/rdd-forwarder/README.md",
  "src/rdd-forwarder/UPSTREAM",
  "../LICENSE",
]);
export const forwarderLicenseNames = Object.freeze([
  "gcc/COPYING.RUNTIME",
  "gcc/COPYING3",
  "libgcc/COPYING.RUNTIME",
  "libgcc/COPYING3",
  "crt/COPYING",
  "crt/COPYING.MinGW-w64-runtime.txt",
  "crt/COPYING.MinGW-w64.txt",
  "headers/COPYING",
  "headers/COPYING.MinGW-w64-runtime.txt",
  "headers/COPYING.MinGW-w64.txt",
  "libwinpthread/COPYING",
]);
const sources = forwarderSourceNames,
  licenseNames = forwarderLicenseNames;
export const forwarderFlags = Object.freeze([
  "-std=c++20",
  "-O2",
  "-municode",
  "-static",
  "-fno-exceptions",
  "-fno-rtti",
  "-ffunction-sections",
  "-Wl,--gc-sections",
  "-Wl,--no-insert-timestamp",
  "-s",
]);

export function buildRddForwarder(root, source, output) {
  assert.ok(
    !existsSync(resolve(output, "rdd-forwarder.exe")),
    "Build the forwarder into a fresh candidate output",
  );
  const compiler = resolve(root, ".tools/mingw/ucrt64/bin/g++.exe");
  const inputs = Object.fromEntries(
    sources.map((name) => [name, hash(readFileSync(resolve(source, name)))]),
  );
  const target = resolve(output, "rdd-forwarder.exe");
  execFileSync(
    compiler,
    [
      ...forwarderFlags,
      resolve(source, "src/rdd-forwarder/shim.cpp"),
      "-lshell32",
      "-o",
      target,
    ],
    {
      cwd: source,
      env: { ...process.env, PATH: dirname(compiler) + ";" + process.env.PATH },
      stdio: "inherit",
      windowsHide: true,
    },
  );
  const bytes = readFileSync(target);
  assert.ok(
    bytes.length > 64 &&
      bytes.length <= 4 << 20 &&
      bytes.toString("ascii", 0, 2) === "MZ",
  );
  const pe = bytes.readUInt32LE(0x3c);
  assert.ok(
    pe >= 64 &&
      pe + 24 <= bytes.length &&
      bytes.readUInt32LE(pe) === 0x4550 &&
      bytes.readUInt16LE(pe + 4) === 0x8664,
    "Forwarder must be a bounded amd64 PE image",
  );
  for (const name of sources)
    assert.equal(
      hash(readFileSync(resolve(source, name))),
      inputs[name],
      "Forwarder source changed",
    );
  const licenses = {};
  for (const name of licenseNames) {
    const bytes = readFileSync(
      resolve(root, ".tools/mingw/ucrt64/share/licenses", name),
    );
    assert.ok(bytes.length > 0 && bytes.length <= 2 << 20);
    const target = resolve(output, "forwarder-licenses", name);
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, bytes, { flag: "wx" });
    licenses[name] = hash(bytes);
  }
  const manifest = {
    schema: 1,
    binarySha256: hash(bytes),
    sources: inputs,
    compilerSha256: hash(readFileSync(compiler)),
    flags: forwarderFlags,
    licenses,
    recipeSha256: hash(
      readFileSync(resolve(root, "scripts/rdd-forwarder-build.mjs")),
    ),
    purpose:
      "Upstream standalone Windows self-command forwarding; no cached-image hardlinks; compilation only",
  };
  writeFileSync(
    resolve(output, "forwarder-build-manifest.json"),
    JSON.stringify(manifest, null, 2),
    { flag: "wx" },
  );
  return manifest;
}

export function verifyRddForwarder(root, directory) {
  const source = resolve(
    root,
    process.env.ORBWIN_UPSTREAM_DIR ?? ".upstream/rancher-desktop",
    "rdd",
  );
  const manifest = JSON.parse(
    readFileSync(resolve(directory, "forwarder-build-manifest.json")),
  );
  assert.equal(manifest.schema, 1);
  assert.deepEqual(Object.keys(manifest.sources).sort(), [...sources].sort());
  assert.deepEqual(manifest.flags, forwarderFlags);
  assert.equal(
    manifest.recipeSha256,
    hash(readFileSync(resolve(root, "scripts/rdd-forwarder-build.mjs"))),
  );
  for (const name of sources)
    assert.equal(
      manifest.sources[name],
      hash(readFileSync(resolve(source, name))),
      "Forwarder build source differs",
    );
  assert.equal(
    manifest.binarySha256,
    hash(readFileSync(resolve(directory, "rdd-forwarder.exe"))),
  );
  assert.deepEqual(
    Object.keys(manifest.licenses).sort(),
    [...licenseNames].sort(),
  );
  for (const name of licenseNames)
    assert.equal(
      manifest.licenses[name],
      hash(readFileSync(resolve(directory, "forwarder-licenses", name))),
    );
  return manifest;
}

export function stageRddForwarderNotices(
  root,
  destination,
  directory = resolve(root, "artifacts/backend"),
) {
  const manifest = verifyRddForwarder(root, directory);
  for (const name of licenseNames) {
    const target = resolve(destination, "compiler-runtime", name);
    mkdirSync(dirname(target), { recursive: true });
    copyFileSync(resolve(directory, "forwarder-licenses", name), target);
  }
  for (const name of sources) {
    const target = resolve(destination, "source", name.split("/").at(-1));
    mkdirSync(dirname(target), { recursive: true });
    copyFileSync(
      resolve(
        root,
        process.env.ORBWIN_UPSTREAM_DIR ?? ".upstream/rancher-desktop",
        "rdd",
        name,
      ),
      target,
    );
  }
  writeFileSync(
    resolve(destination, "provenance.json"),
    JSON.stringify(manifest, null, 2),
  );
  return {
    kind: "native",
    name: "Rancher rdd Windows forwarder",
    binarySha256: manifest.binarySha256,
    compilerSha256: manifest.compilerSha256,
    sources: manifest.sources,
    runtimeLicenses: manifest.licenses,
  };
}
