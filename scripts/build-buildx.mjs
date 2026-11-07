// Fixed-source local candidates only. No runtime activation or pin updates.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  realpathSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  toolCandidateArguments,
  createToolCandidate,
  toolBuildRecipePins,
} from "./tool-candidate-build.mjs";

const root = fileURLToPath(new URL("../", import.meta.url));
const modulePath = "github.com/docker/buildx",
  moduleVersion = "v0.38.1";
const commit = "0dbe87f36ed472fe36b6796c3f70b4210a731fc5",
  version = "v0.38.1-orbwin.1";
const moduleSum = "h1:7J7nReQndOnDTUFjun6otmQqehoi9sCOvRwFjkngYlc=",
  goModSum = "h1:4lEDxmaYDJ17HtXTND9A53BaDoHCJbk5WUgcmfipN7Q=";
const sourceZipSha256 =
  "4cd8755c2468a8a226cdb22bbbf18aee4a876d98f3d4f38e34f2a5f519f9b8ee";
const patchName = "patches/buildx/0001-http2-dependency-floor.patch",
  recipeName = "scripts/build-buildx.mjs";
const patchSha256 =
  "496f3d23feb5c5e13a27883062a11b07a14c6ffbceb5242c944dde5917e52cf5";
const goExecutableSha256 =
  "83b7ba66215e696414fa49462d0db2da4f4cf15c7bebd42f0b189fa04866999d";
const goArchiveSha256 =
  "f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff";
const hash = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");

function noLinks(path) {
  const pending = [];
  for (let current = resolve(path); ; current = dirname(current)) {
    pending.push(current);
    if (dirname(current) === current) break;
  }
  for (const current of pending.reverse()) {
    let info;
    try {
      info = lstatSync(current);
    } catch (error) {
      if (error.code === "ENOENT") continue;
      throw error;
    }
    assert(!info.isSymbolicLink(), `Linked build path refused: ${current}`);
  }
}
export function verifyPinnedFile(path, expected) {
  noLinks(path);
  assert(lstatSync(path).isFile(), "Pinned input must be regular");
  assert.equal(hash(path), expected, `Stale Buildx input: ${path}`);
}
export function sourceHashes(directory) {
  noLinks(directory);
  const records = {};
  function walk(prefix = "") {
    for (const entry of readdirSync(resolve(directory, prefix), {
      withFileTypes: true,
    })) {
      const name = prefix ? `${prefix}/${entry.name}` : entry.name,
        path = resolve(directory, name),
        info = lstatSync(path);
      assert(!info.isSymbolicLink(), `Linked source refused: ${name}`);
      if (info.isDirectory()) walk(name);
      else {
        assert(info.isFile() && info.size <= 16 << 20);
        records[name] = hash(path);
      }
    }
  }
  walk();
  assert(
    Object.keys(records).length < 20000,
    "Unexpected source inventory size",
  );
  return Object.fromEntries(
    Object.entries(records).sort(([a], [b]) => a.localeCompare(b)),
  );
}
export function verifySourceTree(directory, expected) {
  assert.deepEqual(
    sourceHashes(directory),
    expected,
    "Buildx source differs from exact patched archive",
  );
}

function main() {
  const selected = toolCandidateArguments(root, process.argv.slice(2)),
    output = selected.directory,
    checkout = selected.source,
    go = resolve(root, ".tools/go1.27.3/go/bin/go.exe");
  const patch = resolve(root, patchName),
    recipe = resolve(root, recipeName),
    readme = resolve(root, "patches/buildx/README.md"),
    lockPath = resolve(root, "upstream.lock.json");
  const sourceBaseline = resolve(root, "patches/buildx/source-baseline.json"),
    sourceBaselineSha256 =
      "8a7819c50bbf9f36375d97f907d0c5911b535bfebbd2c388a81fd8bd2844f28c",
    provenanceScript = "scripts/buildx-provenance.mjs",
    provenanceSha = hash(resolve(root, provenanceScript));
  verifyPinnedFile(sourceBaseline, sourceBaselineSha256);
  verifyPinnedFile(go, goExecutableSha256);
  verifyPinnedFile(patch, patchSha256);
  const lock = JSON.parse(readFileSync(lockPath));
  assert.equal(lock.go.version, "1.27.3");
  assert.equal(lock.go.windowsAmd64Sha256, goArchiveSha256);
  const recipeSha = hash(recipe),
    readmeSha = hash(readme),
    lockSha = hash(lockPath);
  const recipeDependencies = toolBuildRecipePins(root);
  const baseEnv = {
    ...process.env,
    GOARCH: "amd64",
    GOAMD64: "v1",
    CGO_ENABLED: "0",
    GOROOT: resolve(root, ".tools/go1.27.3/go"),
    GOENV: "off",
    GOTOOLCHAIN: "local",
    GOWORK: "off",
    GOFLAGS: "-p=2",
    GOEXPERIMENT: "",
    GOMAXPROCS: "2",
    GOPROXY: "off",
    GOSUMDB: "off",
    GOPRIVATE: "",
    GONOPROXY: "",
    GONOSUMDB: "",
  };
  const run = (bin, args, cwd = checkout, env = baseEnv) =>
    execFileSync(bin, args, {
      cwd,
      env,
      encoding: "utf8",
      windowsHide: true,
      timeout: 600000,
      maxBuffer: 32 << 20,
    });
  const baseline = JSON.parse(readFileSync(sourceBaseline)),
    expected = {
      ...baseline,
      "go.mod":
        "2dd3cb69a0f36c2f067786bee6be625e9468437d1fcf399b9787784efb197914",
      "go.sum":
        "d7d0fb5bbb484ca6e477106338665fd57feccb4ea25ba74dd9b58f0f28383c96",
    };
  const sortedExpected = Object.fromEntries(
    Object.entries(expected).sort(([a], [b]) => a.localeCompare(b)),
  );
  const verify = () => {
    verifySourceTree(checkout, sortedExpected);
    verifyPinnedFile(recipe, recipeSha);
    verifyPinnedFile(patch, patchSha256);
    verifyPinnedFile(readme, readmeSha);
    verifyPinnedFile(lockPath, lockSha);
    verifyPinnedFile(go, goExecutableSha256);
    verifyPinnedFile(sourceBaseline, sourceBaselineSha256);
    verifyPinnedFile(resolve(root, provenanceScript), provenanceSha);
    assert.deepEqual(toolBuildRecipePins(root), recipeDependencies);
  };
  verify();
  Object.assign(baseEnv, createToolCandidate(selected));
  const dependencyVerification = run(go, ["mod", "verify"]);
  assert.match(dependencyVerification, /all modules verified/);
  writeFileSync(
    resolve(output, "module-verification.txt"),
    dependencyVerification,
  );
  const manifestPath = resolve(output, "buildx-manifest.json");
  const legalName =
    /^(UNLICEN[CS]E|LICEN[CS]E|COPYING|NOTICE|COPYRIGHT|AUTHORS)(?:$|[._-])/i;
  function legalFiles(dir, prefix = "", insideLegal = false) {
    const files = [];
    for (const entry of readdirSync(resolve(dir, prefix), {
      withFileTypes: true,
    })) {
      const name = prefix ? `${prefix}/${entry.name}` : entry.name,
        info = lstatSync(resolve(dir, name));
      assert(!info.isSymbolicLink());
      if (info.isFile() && (legalName.test(entry.name) || insideLegal))
        files.push(name);
      else if (info.isDirectory() && !entry.name.startsWith("."))
        files.push(
          ...legalFiles(
            dir,
            name,
            insideLegal || /^(licenses?|copying|notices?)$/i.test(entry.name),
          ),
        );
    }
    return files.sort();
  }
  function inventory(binary, env) {
    const binarySha256 = hash(binary),
      licenseDirectory = `licenses/${binarySha256}`,
      legalRoot = resolve(output, licenseDirectory);
    noLinks(legalRoot);
    mkdirSync(legalRoot, { recursive: true });
    const modules = new Map();
    for (const line of run(
      go,
      [
        "list",
        "-mod=readonly",
        "-deps",
        "-f",
        "{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}",
        "./cmd/buildx",
      ],
      checkout,
      env,
    ).split(/\r?\n/)) {
      if (!line) continue;
      const [path, v, dir] = line.split("|");
      assert(path && dir);
      modules.set(path, { path, version: v || version, dir });
    }
    const embedded = new Map();
    for (const line of run(go, ["version", "-m", binary]).split(/\r?\n/)) {
      const match = line.match(/^\s*(mod|dep)\s+(\S+)\s+(\S+)/);
      if (match)
        embedded.set(match[2], match[1] === "mod" ? version : match[3]);
    }
    assert.equal(embedded.size, modules.size);
    assert(
      [...modules.values()].every((m) => embedded.get(m.path) === m.version),
      "Compiled module inventory differs from binary metadata",
    );
    let legalBytes = 0;
    function copyLegal(path, name) {
      const data = readFileSync(path);
      legalBytes += data.length;
      assert(data.length <= 8 << 20 && legalBytes <= 64 << 20);
      const target = resolve(legalRoot, name);
      noLinks(target);
      mkdirSync(dirname(target), { recursive: true });
      const sha256 = hash(path);
      if (existsSync(target)) verifyPinnedFile(target, sha256);
      else copyFileSync(path, target);
      return { path: name, bytes: data.length, sha256 };
    }
    const notices = [...modules.values()]
      .sort((a, b) => a.path.localeCompare(b.path))
      .map((m) => {
        const key = createHash("sha256")
            .update(`${m.path}@${m.version}`)
            .digest("hex")
            .slice(0, 20),
          files = legalFiles(m.dir).map((name) =>
            copyLegal(resolve(m.dir, name), `modules/${key}/${name}`),
          );
        return {
          module: m.path,
          version: m.version,
          files,
          rootLicensePresent: files.some(
            (f) =>
              /^(UNLICEN[CS]E|LICEN[CS]E|COPYING)(?:$|[._-])/i.test(
                f.path.split("/").at(-1),
              ) && f.path.split("/").length === 3,
          ),
        };
      });
    const toolchain = ["LICENSE", "PATENTS"].map((name) =>
      copyLegal(resolve(root, ".tools/go1.27.3/go", name), `toolchain/${name}`),
    );
    const source = [
      copyLegal(patch, "source/0001-http2-dependency-floor.patch"),
      copyLegal(readme, "source/README.md"),
      copyLegal(sourceBaseline, "source/source-baseline.json"),
    ];
    assert(
      notices.every((m) => m.rootLicensePresent),
      "Missing root module licenses; candidate retained for review",
    );
    const index = {
      schema: 1,
      binarySha256,
      platform: `${env.GOOS}/amd64`,
      sourceCommit: commit,
      patchSha256,
      embeddedModuleMetadataVerified: true,
      modules: notices,
      toolchain,
      source,
      modulesWithoutRootLicense: notices
        .filter((m) => !m.rootLicensePresent)
        .map((m) => m.module),
      status:
        "Compiled-module source legal files collected; distribution obligations require review, not a legal completeness claim.",
    };
    const indexPath = resolve(legalRoot, "index.json"),
      text = JSON.stringify(index, null, 2) + "\n";
    if (existsSync(indexPath))
      assert.equal(readFileSync(indexPath, "utf8"), text);
    else writeFileSync(indexPath, text, { flag: "wx" });
    return {
      binarySha256,
      licenseDirectory,
      licenseIndexSha256: hash(indexPath),
      compiledModules: notices.length,
    };
  }
  const binaries = {};
  for (const [target, GOOS, file] of [
    ["windowsAmd64", "windows", "docker-buildx.exe"],
    ["linuxAmd64", "linux", "docker-buildx"],
  ]) {
    const env = { ...baseEnv, GOOS },
      binary = resolve(output, file),
      compiledSourceHashes = {};
    const template =
      '{{if .Module}}{{if eq .Module.Path "github.com/docker/buildx"}}{{range .GoFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{range .EmbedFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{range .SFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{range .SysoFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{end}}{{end}}';
    for (const path of run(
      go,
      ["list", "-mod=readonly", "-deps", "-f", template, "./cmd/buildx"],
      checkout,
      env,
    )
      .split(/\r?\n/)
      .filter(Boolean)) {
      const name = relative(
        realpathSync(checkout),
        realpathSync(path),
      ).replaceAll("\\", "/");
      assert(
        name && name !== ".." && !name.startsWith("../") && !isAbsolute(name),
      );
      compiledSourceHashes[name] = hash(path);
    }
    const candidate = resolve(output, `${file}.building`);
    noLinks(candidate);
    assert(
      !existsSync(candidate),
      "Interrupted build candidate retained; inspect before retry",
    );
    run(
      go,
      [
        "build",
        "-mod=readonly",
        "-trimpath",
        "-buildvcs=false",
        "-ldflags",
        `-w -X github.com/docker/buildx/version.Version=${version} -X github.com/docker/buildx/version.Revision=${commit}`,
        "-o",
        candidate,
        "./cmd/buildx",
      ],
      checkout,
      env,
    );
    verify();
    if (existsSync(binary)) verifyPinnedFile(candidate, hash(binary));
    else copyFileSync(candidate, binary);
    binaries[target] = {
      path: file,
      platform: `${GOOS}/amd64`,
      ...inventory(binary, env),
      compiledSourceHashes: Object.fromEntries(
        Object.entries(compiledSourceHashes).sort(([a], [b]) =>
          a.localeCompare(b),
        ),
      ),
    };
    verifyPinnedFile(candidate, hash(binary));
    unlinkSync(candidate);
  }
  verify();
  const windows = binaries.windowsAmd64;
  const sourcePins = {
    recipeDependencies,
    buildScript: recipeName,
    buildScriptSha256: recipeSha,
    patch: patchName,
    patchSha256,
    goExecutableSha256,
    goArchiveSha256,
    upstreamLockSha256: lockSha,
    provenanceScript,
    provenanceScriptSha256: provenanceSha,
    sourceBaseline: "patches/buildx/source-baseline.json",
    sourceBaselineSha256,
  };
  const manifest = {
    schema: 1,
    repository: "https://github.com/docker/buildx",
    tag: moduleVersion,
    commit,
    module: modulePath,
    moduleVersion,
    moduleSum,
    goModSum,
    sourceZipSha256,
    sourceDirectory: selected.sourceDirectory,
    testsNotRun: true,
    version,
    go: "1.27.3",
    platform: "windows/amd64",
    patchSha256,
    sources: sortedExpected,
    binarySha256: windows.binarySha256,
    licenseDirectory: windows.licenseDirectory,
    licenseIndexSha256: windows.licenseIndexSha256,
    compiledModules: windows.compiledModules,
    binaries,
    sourcePins,
    sourceArchiveVerification:
      "Read-only source matches the pinned complete reviewed archive baseline plus exact module-patch hashes; no download or source repair",
    status:
      "Local candidates; runtime pins, installed tools and acceptance unchanged",
  };
  const text = JSON.stringify(manifest, null, 2) + "\n";
  if (existsSync(manifestPath))
    assert.equal(
      readFileSync(manifestPath, "utf8"),
      text,
      "Divergent existing manifest retained",
    );
  else writeFileSync(manifestPath, text, { flag: "wx" });
  console.log(manifestPath);
}
if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
)
  main();
