// Version/checksum pins are inherited from the selected upstream tool manifest.
// Downloads are never executable until verified and atomically renamed.
import {
  readFileSync,
  existsSync,
  mkdirSync,
  writeFileSync,
  renameSync,
  unlinkSync,
  realpathSync,
  lstatSync,
  chmodSync,
} from "node:fs";
import {
  resolve,
  dirname,
  relative,
  isAbsolute,
  win32,
  posix,
} from "node:path";
import { execFileSync } from "node:child_process";
import assert from "node:assert/strict";
import { createHash, randomUUID } from "node:crypto";
import { fileURLToPath } from "node:url";
import {
  verifyBuildxSource,
  copyBuildxProvenance,
} from "./buildx-provenance.mjs";
import {
  verifyDockerCliSource,
  copyDockerCliProvenance,
} from "./docker-cli-provenance.mjs";
import {
  toolCandidateManifest,
  toolBuildRecipePins,
} from "./tool-candidate-build.mjs";
const root = fileURLToPath(new URL("../", import.meta.url));
const lock = JSON.parse(readFileSync(resolve(root, "runtime-tools.lock.json")));
const digest = (data) => createHash("sha256").update(data).digest("hex");

export function validateRuntimeToolPaths(workspace, pins) {
  const cacheRoot = resolve(workspace, "artifacts/runtime-tools"),
    seen = new Set();
  for (const tool of pins.tools) {
    const name = tool.path;
    assert(
      typeof name === "string" &&
        name.length > 0 &&
        !name.includes("\0") &&
        !name.includes(":"),
      "Runtime tool path must be a nonempty relative file path",
    );
    assert(
      !win32.isAbsolute(name) && !posix.isAbsolute(name),
      "Absolute runtime tool path refused",
    );
    assert(
      name
        .split(/[\\/]/)
        .every((part) => part && part !== "." && part !== ".."),
      "Runtime tool path traversal refused",
    );
    const path = resolve(cacheRoot, name),
      rel = relative(cacheRoot, path);
    assert(
      rel &&
        !isAbsolute(rel) &&
        rel !== ".." &&
        !rel.startsWith("../") &&
        !rel.startsWith("..\\"),
      "Runtime tool path escaped generated cache",
    );
    const normalized = rel.replaceAll("\\", "/").toLowerCase();
    assert(!seen.has(normalized), "Duplicate runtime tool destination");
    seen.add(normalized);
    if (tool.localSource) {
      const destinations = {
        dockerCli: {
          "bin/docker.exe": "windowsAmd64",
          "linux/docker": "linuxAmd64",
        },
        compose: {
          "docker-cli-plugins/docker-compose.exe": "windowsAmd64",
          "linux/docker-compose": "linuxAmd64",
        },
        buildx: {
          "docker-cli-plugins/docker-buildx.exe": "windowsAmd64",
          "linux/docker-buildx": "linuxAmd64",
        },
      };
      const expected = destinations[tool.localSource.name]?.[name];
      assert(
        expected && tool.localSource.target === expected,
        "Unknown local source or mismatched runtime destination/target",
      );
    }
  }
}

// Fixed recipe only: the lock is data, never an arbitrary executable command.
export function verifyComposeSource(workspace, pins) {
  validateRuntimeToolPaths(workspace, pins);
  const source = pins.localSources?.compose;
  assert(source, "Compose local source pin missing");
  assert.equal(source.kind, "orbwin-compose-v1");
  assert.equal(source.recipe, "scripts/build-compose.mjs");
  assert.equal(source.buildScript, source.recipe);
  assert.equal(
    source.patch,
    "patches/compose/0001-explicit-local-dockerfile-read.patch",
  );
  assert.equal(source.repository, "https://github.com/docker/compose.git");
  assert.equal(
    source.dependencyPatch,
    "patches/compose/0002-http2-dependency-floor.patch",
  );
  const fileHash = (path) => digest(readFileSync(path));
  const checked = (path, expected) =>
    assert.equal(fileHash(path), expected, `Stale Compose input: ${path}`);
  checked(resolve(workspace, source.recipe), source.buildScriptSha256);
  checked(resolve(workspace, source.patch), source.patchSha256);
  checked(
    resolve(workspace, source.dependencyPatch),
    source.dependencyPatchSha256,
  );
  checked(resolve(workspace, "upstream.lock.json"), source.upstreamLockSha256);
  const upstream = JSON.parse(
    readFileSync(resolve(workspace, "upstream.lock.json")),
  );
  assert.equal(upstream.go.windowsAmd64Sha256, source.goArchiveSha256);
  checked(
    resolve(workspace, `.tools/go${upstream.go.version}/go/bin/go.exe`),
    source.goExecutableSha256,
  );
  const sourceDirectory = source.sourceDirectory ?? ".upstream/compose";
  assert.ok(
    sourceDirectory
      .split("/")
      .every((p) => /^[A-Za-z0-9_.-]+$/.test(p) && p !== "." && p !== ".."),
  );
  const checkout = resolve(workspace, sourceDirectory);
  assert.equal(
    realpathSync(checkout),
    resolve(realpathSync(workspace), sourceDirectory),
  );
  const git = (args) =>
    execFileSync("git", ["-c", "core.autocrlf=true", ...args], {
      cwd: checkout,
      encoding: "utf8",
      windowsHide: true,
      maxBuffer: 16 << 20,
    });
  assert.equal(git(["rev-parse", "HEAD"]).trim(), source.commit);
  assert.equal(git(["rev-parse", source.tag]).trim(), source.tagObject);
  assert.equal(git(["rev-parse", `${source.tag}^{}`]).trim(), source.commit);
  // Only Git's abbreviated blob IDs differ between the no-index patch export
  // and checkout. All file modes, context and changed bytes remain exact.
  const patchText = (text) =>
    text.replace(/^index [a-f0-9]+\.\.[a-f0-9]+(.*)$/gm, "index REVIEWED$1");
  assert.equal(
    patchText(git(["diff", "--binary", "HEAD", "--"])),
    patchText(
      readFileSync(resolve(workspace, source.dependencyPatch), "utf8") +
        readFileSync(resolve(workspace, source.patch), "utf8"),
    ),
  );
  assert.equal(git(["ls-files", "--others", "--exclude-standard"]).trim(), "");
  const manifestPath = toolCandidateManifest(workspace, source, "compose"),
    output = dirname(manifestPath);
  // Verification never starts a build. Missing or divergent candidates require
  // a separately selected fresh build, preserving existing evidence.
  if (existsSync(manifestPath)) checked(manifestPath, source.manifestSha256);
  for (const [path, target] of [
    ["docker-cli-plugins/docker-compose.exe", "docker-compose.exe"],
    ["linux/docker-compose", "docker-compose"],
  ]) {
    const tool = pins.tools.find((tool) => tool.path === path);
    assert(tool?.localSource?.name === "compose");
    if (existsSync(resolve(output, target)))
      checked(resolve(output, target), tool.sha256);
  }
  const required = [
    manifestPath,
    resolve(output, "docker-compose.exe"),
    resolve(output, "docker-compose"),
  ];
  if (required.some((path) => !existsSync(path))) {
    throw new Error("Build an explicit fresh Compose candidate first");
  }
  checked(manifestPath, source.manifestSha256);
  const manifest = JSON.parse(readFileSync(manifestPath));
  if (source.manifest) {
    assert.equal(manifest.sourceDirectory, sourceDirectory);
    assert.equal(manifest.testsNotRun, true);
    assert.deepEqual(
      manifest.sourcePins.recipeDependencies,
      toolBuildRecipePins(workspace),
    );
  }
  for (const name of ["repository", "tag", "tagObject", "commit", "version"])
    assert.equal(manifest[name], source[name]);
  for (const [name, value] of Object.entries(manifest.sourcePins))
    assert.deepEqual(value, source[name]);
  const within = (base, name) => {
    const path = resolve(base, name),
      rel = relative(base, path);
    assert(
      rel &&
        !isAbsolute(rel) &&
        rel !== ".." &&
        !rel.startsWith("../") &&
        !rel.startsWith("..\\"),
      "Compose record escaped its root",
    );
    assert.equal(
      realpathSync(path),
      resolve(realpathSync(base), rel),
      "Compose source/legal file must not redirect",
    );
    assert(lstatSync(path).isFile() && !lstatSync(path).isSymbolicLink());
    return path;
  };
  for (const [name, sha] of Object.entries(manifest.sources))
    checked(within(checkout, name), sha);
  const legal = [];
  for (const tool of pins.tools.filter(
    (tool) => tool.localSource?.name === "compose",
  )) {
    assert.equal(tool.localSource.name, "compose");
    const target =
      tool.path === "docker-cli-plugins/docker-compose.exe"
        ? "windowsAmd64"
        : tool.path === "linux/docker-compose"
          ? "linuxAmd64"
          : null;
    assert(target, "Unexpected local tool destination");
    assert.equal(tool.localSource.target, target);
    const record = manifest.binaries[target];
    assert.equal(
      record.path,
      target === "windowsAmd64" ? "docker-compose.exe" : "docker-compose",
    );
    assert.equal(record.binarySha256, tool.sha256);
    checked(within(output, record.path), tool.sha256);
    for (const [name, sha] of Object.entries(record.compiledSourceHashes))
      checked(within(checkout, name), sha);
    assert.equal(record.licenseDirectory, `licenses/${tool.sha256}`);
    const directory = resolve(output, record.licenseDirectory),
      indexPath = within(directory, "index.json");
    checked(indexPath, record.licenseIndexSha256);
    const index = JSON.parse(readFileSync(indexPath));
    assert.equal(index.binarySha256, tool.sha256);
    assert.equal(index.sourceCommit, source.commit);
    assert.equal(index.patchSha256, source.patchSha256);
    assert.equal(index.dependencyPatchSha256, source.dependencyPatchSha256);
    assert.equal(index.embeddedModuleMetadataVerified, true);
    assert.deepEqual(index.modulesWithoutRootLicense, []);
    const files = [
      ...index.modules.flatMap((module) => module.files),
      ...index.toolchain,
      ...index.source,
    ];
    for (const file of files)
      checked(within(directory, file.path), file.sha256);
    // The legal source copy must remain current, too.
    for (const file of index.source)
      checked(
        resolve(
          workspace,
          "patches/compose",
          file.path.slice("source/".length),
        ),
        file.sha256,
      );
    legal.push({ target, directory, indexPath, index, files });
  }
  assert.equal(legal.length, 2);
  return { manifest, manifestPath, outputRoot: output, legal };
}

export function copyComposeProvenance(workspace, pins, destination) {
  const proof = verifyComposeSource(workspace, pins);
  mkdirSync(destination, { recursive: true });
  assert.equal(
    realpathSync(destination),
    resolve(
      realpathSync(dirname(destination)),
      destination.split(/[\\/]/).at(-1),
    ),
    "Compose provenance directory must not redirect",
  );
  const copy = (source, target) => {
    mkdirSync(dirname(target), { recursive: true });
    assert.equal(
      realpathSync(dirname(target)),
      resolve(
        realpathSync(destination),
        relative(destination, dirname(target)),
      ),
      "Compose notice parent must not redirect",
    );
    const bytes = readFileSync(source);
    let targetStat;
    try {
      targetStat = lstatSync(target);
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
    }
    if (targetStat) {
      assert(
        targetStat.isFile() && !targetStat.isSymbolicLink(),
        "Compose notice target must be an ordinary file",
      );
      if (digest(readFileSync(target)) === digest(bytes)) return;
      chmodSync(target, 0o644);
    }
    writeFileSync(target, bytes);
    assert.equal(digest(readFileSync(target)), digest(bytes));
  };
  copy(proof.manifestPath, resolve(destination, "compose-manifest.json"));
  for (const record of proof.legal) {
    const target = resolve(destination, record.target);
    mkdirSync(target, { recursive: true });
    copy(record.indexPath, resolve(target, "index.json"));
    for (const file of record.files) {
      const dest = resolve(target, file.path);
      copy(resolve(record.directory, file.path), dest);
      assert.equal(digest(readFileSync(dest)), file.sha256);
    }
  }
  return proof;
}

async function stage() {
  for (const tool of lock.tools.filter((tool) => tool.localSource))
    assert(
      ["compose", "buildx", "dockerCli"].includes(tool.localSource.name),
      "Unknown local runtime source",
    );
  const local = {
    compose: verifyComposeSource(root, lock, { buildMissing: true }),
    buildx: verifyBuildxSource(root, lock, { buildMissing: true }),
  };
  if (lock.localSources?.dockerCli)
    local.dockerCli = verifyDockerCliSource(root, lock);
  const replacementConsent = (tool) =>
    process.argv.includes("--replace-prior-generated-cache") ||
    (tool.localSource?.name === "compose" &&
      process.argv.includes("--replace-prior-compose-cache"));
  const cacheRoot = resolve(root, "artifacts/runtime-tools");
  if (existsSync(cacheRoot))
    assert.equal(
      realpathSync(cacheRoot),
      resolve(realpathSync(resolve(root, "artifacts")), "runtime-tools"),
      "Generated runtime cache must not redirect",
    );
  // Complete drift checks before any copy/download; replacement consent only covers
  // the previously pinned generated cache, never an installed executable.
  for (const tool of lock.tools) {
    const path = resolve(cacheRoot, tool.path);
    let ancestor = dirname(path);
    while (!existsSync(ancestor)) ancestor = dirname(ancestor);
    assert.equal(
      realpathSync(ancestor),
      resolve(realpathSync(root), relative(root, ancestor)),
      "Runtime cache ancestor must not redirect",
    );
    if (!existsSync(path)) continue;
    assert(lstatSync(path).isFile() && !lstatSync(path).isSymbolicLink());
    const actual = digest(readFileSync(path));
    if (actual === tool.sha256) continue;
    assert(
      tool.localSource &&
        replacementConsent(tool) &&
        actual === tool.previousGeneratedSha256,
      `Existing staged tool differs from lock: ${tool.path}`,
    );
    const previous = JSON.parse(
      readFileSync(resolve(cacheRoot, "manifest.json")),
    );
    assert.equal(
      previous.tools.find((record) => record.path === tool.path)?.sha256,
      tool.previousGeneratedSha256,
      "Prior generated-cache manifest does not authorize replacement",
    );
  }
  await Promise.all(
    lock.tools.map(async (tool) => {
      const path = resolve(root, "artifacts/runtime-tools", tool.path);
      mkdirSync(dirname(path), { recursive: true });
      if (existsSync(path)) {
        const actual = digest(readFileSync(path));
        if (
          actual !== tool.sha256 &&
          !(
            tool.localSource &&
            replacementConsent(tool) &&
            actual === tool.previousGeneratedSha256
          )
        )
          throw new Error(
            `Existing staged tool differs from lock: ${tool.path}`,
          );
        if (actual !== tool.sha256) {
          assert.equal(
            realpathSync(path),
            resolve(
              realpathSync(resolve(root, "artifacts/runtime-tools")),
              tool.path,
            ),
          );
          assert(!lstatSync(path).isSymbolicLink());
        } else {
          console.log(`Verified cached ${tool.path}`);
          return;
        }
      }
      let data;
      if (tool.localSource)
        data = readFileSync(
          resolve(
            tool.localSource.name === "dockerCli"
              ? local.dockerCli.outputRoot
              : resolve(root, `artifacts/${tool.localSource.name}`),
            local[tool.localSource.name].manifest.binaries[
              tool.localSource.target
            ].path,
          ),
        );
      else {
        const response = await fetch(tool.url, {
          signal: AbortSignal.timeout(180000),
        });
        if (!response.ok)
          throw new Error(
            `Download failed for ${tool.path}: HTTP ${response.status}`,
          );
        data = Buffer.from(await response.arrayBuffer());
      }
      if (data.length > 256 * 1024 * 1024 || digest(data) !== tool.sha256)
        throw new Error(`Checksum/size rejection: ${tool.path}`);
      const temporary = path + ".partial-" + randomUUID();
      try {
        writeFileSync(temporary, data, { flag: "wx" });
        renameSync(temporary, path);
      } finally {
        if (existsSync(temporary)) unlinkSync(temporary);
      }
      console.log(
        `${tool.localSource ? "Staged local build and verified" : "Downloaded and verified"} ${tool.path}`,
      );
    }),
  );
  copyComposeProvenance(
    root,
    lock,
    resolve(root, "artifacts/runtime-tools/compose-provenance"),
  );
  copyBuildxProvenance(
    root,
    lock,
    resolve(root, "artifacts/runtime-tools/buildx-provenance"),
  );
  if (lock.localSources?.dockerCli)
    copyDockerCliProvenance(
      root,
      lock,
      resolve(root, "artifacts/runtime-tools/docker-cli-provenance"),
    );
  writeFileSync(
    resolve(root, "artifacts/runtime-tools/manifest.json"),
    JSON.stringify(lock, null, 2) + "\n",
  );
}
if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
)
  await stage();
