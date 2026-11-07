// Explicit candidate verification/copy only. Importing this module builds nothing.
import assert from "node:assert/strict";
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { dirname, resolve } from "node:path";
import {
  dockerCliFixed,
  childPath,
  fileHash,
  noLinks,
  verifyCatalog,
  digest,
} from "./build-docker-cli.mjs";
const sha = (value) =>
  assert(
    typeof value === "string" && /^[a-f0-9]{64}$/.test(value),
    "SHA256 pin required",
  );
export function verifyDockerCliBinary(directory, target, record) {
  assert(["windowsAmd64", "linuxAmd64"].includes(target));
  assert.equal(
    record.path,
    target === "windowsAmd64" ? "docker.exe" : "docker",
  );
  assert.equal(
    record.platform,
    target === "windowsAmd64" ? "windows/amd64" : "linux/amd64",
  );
  sha(record.binarySha256);
  const path = childPath(directory, record.path);
  assert.equal(
    fileHash(path),
    record.binarySha256,
    "Docker CLI candidate binary drifted",
  );
  return path;
}
export function verifyDockerCliLegal(directory, target, record) {
  assert.equal(record.licenseDirectory, `licenses/${target}`);
  sha(record.licenseIndexSha256);
  const indexPath = childPath(
    directory,
    `${record.licenseDirectory}/index.json`,
  );
  assert.equal(fileHash(indexPath), record.licenseIndexSha256);
  const index = JSON.parse(readFileSync(indexPath));
  assert.equal(index.binarySha256, record.binarySha256);
  assert.equal(index.platform, record.platform);
  assert.equal(index.embeddedModuleMetadataVerified, true);
  assert.equal(index.modules.length, record.compiledModules);
  const files = [
    ...index.modules.flatMap((m) => m.files),
    ...index.source,
    ...index.toolchain,
  ];
  let bytes = 0;
  const seen = new Set();
  for (const file of files) {
    sha(file.sha256);
    assert(!seen.has(file.path), "Duplicate legal entry");
    seen.add(file.path);
    assert(
      Number.isSafeInteger(file.bytes) &&
        file.bytes >= 0 &&
        file.bytes <= 8 << 20,
    );
    bytes += file.bytes;
    assert(bytes <= 64 << 20);
    const path = childPath(
        directory,
        `${record.licenseDirectory}/${file.path}`,
      ),
      data = readFileSync(path);
    assert.equal(data.length, file.bytes);
    assert.equal(digest(data), file.sha256, "Docker CLI license drifted");
  }
  return { target, index, files, indexPath };
}
export function verifyDockerCliSource(workspace, pins) {
  const source = pins.localSources?.dockerCli;
  assert(source, "Docker CLI source pin missing");
  assert.equal(source.kind, "orbwin-docker-cli-v1");
  assert(
    /^artifacts\/docker-cli\/candidate-[a-f0-9-]{36}\/docker-cli-manifest\.json$/.test(
      source.manifest,
    ),
    "Closed candidate manifest path required",
  );
  sha(source.manifestSha256);
  const path = childPath(workspace, source.manifest);
  assert.equal(
    fileHash(path),
    source.manifestSha256,
    "Docker CLI candidate manifest drifted",
  );
  const directory = dirname(path),
    manifest = JSON.parse(readFileSync(path));
  for (const key of [
    "kind",
    "repository",
    "tag",
    "tagObject",
    "commit",
    "wrapperCommit",
    "version",
    "go",
    "goArchiveSha256",
    "goExecutableSha256",
  ])
    assert.equal(
      manifest[key],
      key === "kind" ? "orbwin-docker-cli-v1" : dockerCliFixed[key],
    );
  assert.deepEqual(manifest.patches, dockerCliFixed.patches);
  assert.equal(manifest.sourceDirectory, "source");
  assert.equal(manifest.toolchainDirectory, "toolchain/go");
  assert.deepEqual(
    Object.keys(manifest.sourcePins).sort(),
    [
      "scripts/build-docker-cli.mjs",
      "scripts/docker-cli-provenance.mjs",
      "patches/docker-cli/README.md",
      "upstream.lock.json",
      ...Object.keys(dockerCliFixed.patches),
    ].sort(),
  );
  for (const [name, expected] of Object.entries(manifest.sourcePins)) {
    sha(expected);
    assert.equal(
      fileHash(childPath(workspace, name)),
      expected,
      "Docker CLI recipe input drifted",
    );
  }
  for (const name of [
    "scripts/build-docker-cli.mjs",
    "scripts/docker-cli-provenance.mjs",
    "patches/docker-cli/README.md",
    "upstream.lock.json",
    ...Object.keys(dockerCliFixed.patches),
  ])
    assert(manifest.sourcePins[name], "Missing recipe provenance input");
  for (const [name, pin] of [
    ["source-catalog.json", "sourceCatalogSha256"],
    ["toolchain-catalog.json", "toolchainCatalogSha256"],
  ]) {
    sha(manifest[pin]);
    assert.equal(fileHash(childPath(directory, name)), manifest[pin]);
  }
  verifyCatalog(
    childPath(directory, "source"),
    JSON.parse(readFileSync(childPath(directory, "source-catalog.json"))),
  );
  verifyCatalog(
    childPath(directory, "toolchain/go"),
    JSON.parse(readFileSync(childPath(directory, "toolchain-catalog.json"))),
  );
  for (const [name, expected] of Object.entries(manifest.wrapperSourceHashes)) {
    sha(expected);
    assert.equal(
      fileHash(childPath(directory, `wrapper-source/${name}`)),
      expected,
    );
  }
  const selected = pins.tools.filter(
    (t) => t.localSource?.name === "dockerCli",
  );
  assert.equal(
    selected.length,
    2,
    "Both Windows and Linux Docker CLI pins required",
  );
  assert.deepEqual(selected.map((t) => t.localSource.target).sort(), [
    "linuxAmd64",
    "windowsAmd64",
  ]);
  const legal = [];
  for (const target of ["windowsAmd64", "linuxAmd64"]) {
    const record = manifest.binaries[target];
    verifyDockerCliBinary(directory, target, record);
    for (const [name, expected] of Object.entries(record.compiledSourceHashes))
      assert.equal(
        fileHash(childPath(resolve(directory, "source"), name)),
        expected,
      );
    legal.push(verifyDockerCliLegal(directory, target, record));
  }
  for (const tool of pins.tools.filter(
    (t) => t.localSource?.name === "dockerCli",
  )) {
    const target = tool.localSource.target,
      record = manifest.binaries[target];
    assert(record);
    assert.equal(
      tool.path,
      target === "windowsAmd64" ? "bin/docker.exe" : "linux/docker",
    );
    assert.equal(tool.sha256, record.binarySha256);
    assert.equal(tool.version, manifest.version);
  }
  return { manifest, outputRoot: directory, manifestPath: path, legal };
}
export function copyDockerCliProvenance(workspace, pins, destination) {
  const proof = verifyDockerCliSource(workspace, pins);
  noLinks(destination);
  mkdirSync(destination, { recursive: true });
  const copy = (source, name) => {
    const path = childPath(destination, name);
    mkdirSync(dirname(path), { recursive: true });
    if (existsSync(path))
      assert.equal(
        fileHash(path),
        fileHash(source),
        "Existing Docker CLI notice drifted",
      );
    else copyFileSync(source, path, 1);
  };
  copy(proof.manifestPath, "docker-cli-manifest.json");
  for (const legal of proof.legal) {
    copy(legal.indexPath, `${legal.target}/index.json`);
    const record = proof.manifest.binaries[legal.target];
    for (const file of legal.files)
      copy(
        childPath(proof.outputRoot, `${record.licenseDirectory}/${file.path}`),
        `${legal.target}/${file.path}`,
      );
  }
  return proof;
}
