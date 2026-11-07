// Pure imports; explicit verify/copy calls never launch a tool build.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  copyFileSync,
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  realpathSync,
} from "node:fs";
import { dirname, isAbsolute, relative, resolve } from "node:path";
import { verifyPinnedFile, verifySourceTree } from "./build-buildx.mjs";
import {
  toolCandidateManifest,
  toolBuildRecipePins,
} from "./tool-candidate-build.mjs";

const fixed = {
  kind: "orbwin-buildx-v1",
  recipe: "scripts/build-buildx.mjs",
  buildScript: "scripts/build-buildx.mjs",
  repository: "https://github.com/docker/buildx",
  tag: "v0.38.1",
  commit: "0dbe87f36ed472fe36b6796c3f70b4210a731fc5",
  module: "github.com/docker/buildx",
  moduleVersion: "v0.38.1",
  moduleSum: "h1:7J7nReQndOnDTUFjun6otmQqehoi9sCOvRwFjkngYlc=",
  goModSum: "h1:4lEDxmaYDJ17HtXTND9A53BaDoHCJbk5WUgcmfipN7Q=",
  sourceZipSha256:
    "4cd8755c2468a8a226cdb22bbbf18aee4a876d98f3d4f38e34f2a5f519f9b8ee",
  version: "v0.38.1-orbwin.1",
  sourceDirectory: "artifacts/buildx/source",
  patch: "patches/buildx/0001-http2-dependency-floor.patch",
  patchSha256:
    "496f3d23feb5c5e13a27883062a11b07a14c6ffbceb5242c944dde5917e52cf5",
  provenanceScript: "scripts/buildx-provenance.mjs",
  sourceBaseline: "patches/buildx/source-baseline.json",
  sourceBaselineSha256:
    "8a7819c50bbf9f36375d97f907d0c5911b535bfebbd2c388a81fd8bd2844f28c",
  goExecutableSha256:
    "83b7ba66215e696414fa49462d0db2da4f4cf15c7bebd42f0b189fa04866999d",
  goArchiveSha256:
    "f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff",
};
const digest = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");
function noRedirect(path) {
  for (let current = resolve(path); ; current = dirname(current)) {
    try {
      assert(
        !lstatSync(current).isSymbolicLink(),
        "Buildx path redirection refused",
      );
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
    }
    if (dirname(current) === current) break;
  }
}
function within(base, name) {
  assert(
    typeof name === "string" &&
      name &&
      !name.includes("\\") &&
      !name.includes(":"),
  );
  assert(
    name.split("/").every((part) => part && part !== "." && part !== ".."),
  );
  const path = resolve(base, name),
    part = relative(base, path);
  assert(
    part &&
      !isAbsolute(part) &&
      part !== ".." &&
      !part.startsWith("../") &&
      !part.startsWith("..\\"),
  );
  noRedirect(path);
  assert(lstatSync(path).isFile(), "Buildx provenance entry must be regular");
  assert.equal(realpathSync(path), resolve(realpathSync(base), part));
  return path;
}
export function verifyBuildxSourceFiles(directory, expected) {
  verifySourceTree(directory, expected);
}
export function verifyBuildxBinary(output, tool, target, record) {
  const windows = target === "windowsAmd64";
  assert(windows || target === "linuxAmd64");
  assert.equal(
    tool.path,
    windows ? "docker-cli-plugins/docker-buildx.exe" : "linux/docker-buildx",
  );
  assert.equal(tool.localSource?.name, "buildx");
  assert.equal(tool.localSource.target, target);
  assert.equal(record.path, windows ? "docker-buildx.exe" : "docker-buildx");
  assert.equal(record.platform, windows ? "windows/amd64" : "linux/amd64");
  assert.equal(record.binarySha256, tool.sha256);
  verifyPinnedFile(within(output, record.path), tool.sha256);
}
export function verifyBuildxLegalInventory(
  directory,
  index,
  binarySha256,
  platform,
) {
  assert.equal(index.schema, 1);
  assert.equal(index.binarySha256, binarySha256);
  assert.equal(index.platform, platform);
  assert.equal(index.sourceCommit, fixed.commit);
  assert.equal(index.patchSha256, fixed.patchSha256);
  assert.equal(index.embeddedModuleMetadataVerified, true);
  assert.deepEqual(index.modulesWithoutRootLicense, []);
  assert(Array.isArray(index.modules) && index.modules.length > 0);
  assert(Array.isArray(index.toolchain) && Array.isArray(index.source));
  const files = [
    ...index.modules.flatMap((m) => {
      assert.equal(m.rootLicensePresent, true);
      assert(
        m.files.some(
          (f) =>
            /\/[^/]+\/[^/]+$/.test(f.path) &&
            /^(UNLICEN[CS]E|LICEN[CS]E|COPYING)(?:$|[._-])/i.test(
              f.path.split("/").at(-1),
            ) &&
            f.path.split("/").length === 3,
        ),
      );
      return m.files;
    }),
    ...index.toolchain,
    ...index.source,
  ];
  const seen = new Set();
  for (const file of files) {
    assert(!seen.has(file.path), "Duplicate legal destination");
    seen.add(file.path);
    const path = within(directory, file.path);
    verifyPinnedFile(path, file.sha256);
    assert.equal(lstatSync(path).size, file.bytes);
  }
  return files;
}
export function verifyBuildxSource(workspace, pins) {
  const source = pins.localSources?.buildx;
  assert(source, "Buildx source pin missing");
  for (const [name, expected] of Object.entries(fixed).filter(
    ([name]) => name !== "sourceDirectory",
  ))
    assert.equal(
      source[name],
      expected,
      `Unsupported Buildx source pin: ${name}`,
    );
  for (const [path, sha] of [
    [fixed.recipe, source.buildScriptSha256],
    [fixed.patch, fixed.patchSha256],
    [fixed.provenanceScript, source.provenanceScriptSha256],
    [fixed.sourceBaseline, fixed.sourceBaselineSha256],
    ["upstream.lock.json", source.upstreamLockSha256],
    [".tools/go1.27.3/go/bin/go.exe", fixed.goExecutableSha256],
  ])
    verifyPinnedFile(within(workspace, path), sha);
  const upstream = JSON.parse(
    readFileSync(resolve(workspace, "upstream.lock.json")),
  );
  assert.equal(upstream.go.version, "1.27.3");
  assert.equal(upstream.go.windowsAmd64Sha256, fixed.goArchiveSha256);
  const tools = pins.tools.filter((t) => t.localSource?.name === "buildx");
  assert.equal(tools.length, 2);
  for (const tool of tools) {
    const target = tool.localSource.target;
    assert(["windowsAmd64", "linuxAmd64"].includes(target));
    assert.equal(
      tool.path,
      target === "windowsAmd64"
        ? "docker-cli-plugins/docker-buildx.exe"
        : "linux/docker-buildx",
    );
  }
  assert.equal(new Set(tools.map((t) => t.path)).size, 2);
  assert.ok(
    typeof source.sourceDirectory === "string" &&
      source.sourceDirectory
        .split("/")
        .every((p) => /^[A-Za-z0-9_.-]+$/.test(p) && p !== "." && p !== ".."),
  );
  const manifestPath = toolCandidateManifest(workspace, source, "buildx"),
    output = dirname(manifestPath);
  noRedirect(output);
  const paths = [
    manifestPath,
    resolve(output, "docker-buildx.exe"),
    resolve(output, "docker-buildx"),
  ];
  if (existsSync(manifestPath))
    verifyPinnedFile(manifestPath, source.manifestSha256);
  for (const tool of tools) {
    const path = resolve(
      output,
      tool.localSource.target === "windowsAmd64"
        ? "docker-buildx.exe"
        : "docker-buildx",
    );
    if (existsSync(path)) verifyPinnedFile(path, tool.sha256);
  }
  if (paths.some((path) => !existsSync(path))) {
    throw new Error("Build an explicit fresh Buildx candidate first");
  }
  verifyPinnedFile(
    within(output, "buildx-manifest.json"),
    source.manifestSha256,
  );
  const manifest = JSON.parse(readFileSync(manifestPath));
  if (source.manifest) {
    assert.equal(manifest.testsNotRun, true);
    assert.deepEqual(
      manifest.sourcePins.recipeDependencies,
      toolBuildRecipePins(workspace),
    );
  }
  for (const key of [
    "repository",
    "tag",
    "commit",
    "module",
    "moduleVersion",
    "moduleSum",
    "goModSum",
    "sourceZipSha256",
    "version",
    "sourceDirectory",
    "patchSha256",
  ])
    assert.equal(
      manifest[key],
      key === "sourceDirectory" ? source.sourceDirectory : fixed[key],
    );
  assert.equal(manifest.go, "1.27.3");
  assert.equal(manifest.schema, 1);
  for (const [key, value] of Object.entries(manifest.sourcePins))
    assert.deepEqual(source[key], value);
  for (const key of [
    "buildScript",
    "buildScriptSha256",
    "patch",
    "patchSha256",
    "goExecutableSha256",
    "goArchiveSha256",
    "upstreamLockSha256",
    "provenanceScript",
    "provenanceScriptSha256",
    "sourceBaseline",
    "sourceBaselineSha256",
  ])
    assert.equal(manifest.sourcePins[key], source[key]);
  const baseline = JSON.parse(
      readFileSync(resolve(workspace, fixed.sourceBaseline)),
    ),
    expected = {
      ...baseline,
      "go.mod":
        "2dd3cb69a0f36c2f067786bee6be625e9468437d1fcf399b9787784efb197914",
      "go.sum":
        "d7d0fb5bbb484ca6e477106338665fd57feccb4ea25ba74dd9b58f0f28383c96",
    };
  assert.deepEqual(manifest.sources, expected);
  const checkout = resolve(workspace, source.sourceDirectory);
  verifyBuildxSourceFiles(checkout, expected);
  const legal = [];
  for (const tool of tools) {
    const target = tool.localSource.target,
      record = manifest.binaries[target];
    verifyBuildxBinary(output, tool, target, record);
    assert(
      record.compiledSourceHashes["cmd/buildx/main.go"],
      "Compiled source entrypoint missing",
    );
    for (const [name, sha] of Object.entries(record.compiledSourceHashes)) {
      assert.equal(expected[name], sha);
      verifyPinnedFile(within(checkout, name), sha);
    }
    assert.equal(record.licenseDirectory, `licenses/${tool.sha256}`);
    const directory = resolve(output, record.licenseDirectory),
      indexPath = within(directory, "index.json");
    verifyPinnedFile(indexPath, record.licenseIndexSha256);
    const index = JSON.parse(readFileSync(indexPath)),
      files = verifyBuildxLegalInventory(
        directory,
        index,
        tool.sha256,
        record.platform,
      );
    assert.equal(index.modules.length, record.compiledModules);
    assert(
      index.modules.some(
        (m) => m.module === fixed.module && m.version === fixed.version,
      ),
    );
    for (const file of index.source) {
      assert(file.path.startsWith("source/"));
      verifyPinnedFile(
        within(resolve(workspace, "patches/buildx"), file.path.slice(7)),
        file.sha256,
      );
    }
    legal.push({ target, directory, indexPath, index, files });
  }
  return { manifest, manifestPath, outputRoot: output, legal };
}
export function copyBuildxProvenance(workspace, pins, destination) {
  const proof = verifyBuildxSource(workspace, pins);
  noRedirect(destination);
  mkdirSync(destination, { recursive: true });
  function copy(source, target) {
    noRedirect(target);
    mkdirSync(dirname(target), { recursive: true });
    if (existsSync(target)) {
      assert(lstatSync(target).isFile());
      assert.equal(
        digest(target),
        digest(source),
        "Divergent existing Buildx notice retained",
      );
    } else copyFileSync(source, target);
    verifyPinnedFile(target, digest(source));
  }
  copy(proof.manifestPath, resolve(destination, "buildx-manifest.json"));
  for (const record of proof.legal) {
    copy(record.indexPath, resolve(destination, record.target, "index.json"));
    for (const file of record.files)
      copy(
        within(record.directory, file.path),
        resolve(destination, record.target, file.path),
      );
  }
  return proof;
}
