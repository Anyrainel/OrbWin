// Isolated patched Compose candidates. Never installs or rewrites runtime pins.
import { execFileSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  readdirSync,
  copyFileSync,
  statSync,
  realpathSync,
} from "node:fs";
import { resolve, relative, isAbsolute } from "node:path";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";
import {
  toolCandidateArguments,
  createToolCandidate,
  toolBuildRecipePins,
} from "./tool-candidate-build.mjs";
const root = fileURLToPath(new URL("../", import.meta.url)),
  scriptPath = fileURLToPath(import.meta.url);
const selected = toolCandidateArguments(root, process.argv.slice(2)),
  checkout = selected.source;
const commit = "6cbc19a3fd59e305fb814dca4d854a4fb67d2941",
  tagObject = "02f05e3787e5b2e83220923907bb014e6a36b359";
const patch = resolve(
  root,
  "patches/compose/0001-explicit-local-dockerfile-read.patch",
);
const dependencyPatch = resolve(
  root,
  "patches/compose/0002-http2-dependency-floor.patch",
);
const hash = (p) => createHash("sha256").update(readFileSync(p)).digest("hex");
const scriptSha256 = hash(scriptPath),
  patchSha256 = hash(patch),
  dependencyPatchSha256 = hash(dependencyPatch);
const run = (bin, args, cwd = checkout, extra = {}) =>
  execFileSync(bin, args, {
    cwd,
    encoding: "utf8",
    windowsHide: true,
    timeout: 600000,
    maxBuffer: 16 << 20,
    ...extra,
  });
const git = (args) => run("git", ["-c", "core.autocrlf=false", ...args]);
// Git abbreviates blob IDs differently in the isolated no-index patch source.
// Ignore only index-line blob abbreviations; retain every mode/context/change.
const patchText = (s) =>
  s.replace(/^index [a-f0-9]+\.\.[a-f0-9]+(.*)$/gm, "index REVIEWED$1");
const currentDiff = () =>
  run("git", ["-c", "core.autocrlf=true", "diff", "--binary", "HEAD", "--"]);
if (
  git(["rev-parse", "HEAD"]).trim() !== commit ||
  git(["rev-parse", "v5.6.1"]).trim() !== tagObject ||
  git(["rev-parse", "v5.6.1^{}"]).trim() !== commit
)
  throw Error("Compose tag/commit differs; checkout retained");
// The second patch changes only go.mod/go.sum, which sort before pkg/compose.
// Admit precisely the prior reviewed state or the complete reviewed state.
const expectedDiff =
  readFileSync(dependencyPatch, "utf8") + readFileSync(patch, "utf8");
const verify = () => {
  if (patchText(currentDiff()) !== patchText(expectedDiff))
    throw Error("Compose checkout differs from the exact reviewed patches");
  if (git(["ls-files", "--others", "--exclude-standard"]).trim())
    throw Error(
      "Untracked Compose source files are not part of the reviewed patch",
    );
  if (
    JSON.stringify(toolBuildRecipePins(root)) !==
      JSON.stringify(recipeDependencies) ||
    hash(lockPath) !== lockSha256 ||
    hash(go) !== goSha256 ||
    hash(scriptPath) !== scriptSha256 ||
    hash(patch) !== patchSha256 ||
    hash(dependencyPatch) !== dependencyPatchSha256
  )
    throw Error("Compose recipe or patch changed during build");
};
const lockPath = resolve(root, "upstream.lock.json"),
  lock = JSON.parse(readFileSync(lockPath));
const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`),
  lockSha256 = hash(lockPath),
  goSha256 = hash(go);
const recipeDependencies = toolBuildRecipePins(root);
if (
  lock.go.version !== "1.27.3" ||
  lock.go.windowsAmd64Sha256 !==
    "f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff" ||
  goSha256 !==
    "83b7ba66215e696414fa49462d0db2da4f4cf15c7bebd42f0b189fa04866999d"
)
  throw Error("Pinned Compose toolchain changed");
verify();
const output = selected.directory,
  isolatedEnv = createToolCandidate(selected);
const sources = Object.fromEntries(
  [
    "go.mod",
    "go.sum",
    "pkg/compose/build_bake.go",
    "pkg/compose/build_bake_orbwin_test.go",
  ].map((p) => [p, hash(resolve(checkout, p))]),
);
const baseEnv = {
  ...process.env,
  ...isolatedEnv,
  GOROOT: resolve(root, `.tools/go${lock.go.version}/go`),
  GOENV: "off",
  GOARCH: "amd64",
  GOAMD64: "v1",
  GOFLAGS: "",
  GOEXPERIMENT: "",
  GOWORK: "off",
  CGO_ENABLED: "0",
  GOTOOLCHAIN: "local",
  GOMAXPROCS: "2",
};
run(go, ["mod", "verify"], checkout, { env: baseEnv, stdio: "inherit" });
const legalName =
  /^(UNLICEN[CS]E|LICEN[CS]E|COPYING|NOTICE|COPYRIGHT|AUTHORS)(?:$|[._-])/i;
function legalFiles(dir, prefix = "", insideLegal = false) {
  const out = [];
  for (const entry of readdirSync(resolve(dir, prefix), {
    withFileTypes: true,
  })) {
    const name = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isFile() && (legalName.test(entry.name) || insideLegal))
      out.push(name);
    else if (entry.isDirectory() && !entry.name.startsWith("."))
      out.push(
        ...legalFiles(
          dir,
          name,
          insideLegal || /^(licenses?|copying|notices?)$/i.test(entry.name),
        ),
      );
  }
  return out;
}
function compiledSources(env) {
  const template =
    '{{if .Module}}{{if eq .Module.Path "github.com/docker/compose/v5"}}{{range .GoFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{range .EmbedFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{range .SFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{range .SysoFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{end}}{{end}}';
  const records = {};
  for (const line of run(
    go,
    ["list", "-mod=readonly", "-deps", "-f", template, "./cmd"],
    checkout,
    { env },
  ).split(/\r?\n/)) {
    if (!line.trim()) continue;
    const path = realpathSync(line),
      name = relative(realpathSync(checkout), path).replaceAll("\\", "/");
    if (name === ".." || name.startsWith("../") || isAbsolute(name))
      throw Error("Compiled source escaped the pinned checkout");
    records[name] = hash(path);
  }
  return Object.fromEntries(
    Object.entries(records).sort(([a], [b]) => a.localeCompare(b)),
  );
}
function notices(binary, env) {
  const binarySha256 = hash(binary),
    licenseDirectory = `licenses/${binarySha256}`,
    legalRoot = resolve(output, licenseDirectory);
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
      "./cmd",
    ],
    checkout,
    { env },
  ).split(/\r?\n/)) {
    if (!line.trim()) continue;
    const [path, version, dir] = line.split("|");
    if (!path || !dir) throw Error("Compiled module provenance unavailable");
    modules.set(path, { path, version: version || "v5.6.1-orbwin.2", dir });
  }
  const embeddedModules = new Map();
  for (const line of run(go, ["version", "-m", binary]).split(/\r?\n/)) {
    const match = line.match(/^\s*(mod|dep)\s+(\S+)\s+(\S+)/);
    if (match)
      embeddedModules.set(
        match[2],
        match[1] === "mod" ? "v5.6.1-orbwin.2" : match[3],
      );
  }
  if (
    embeddedModules.size !== modules.size ||
    [...modules.values()].some((m) => embeddedModules.get(m.path) !== m.version)
  )
    throw Error(
      "Source dependency license inventory differs from the binary embedded module metadata",
    );
  let legalBytes = 0;
  function copyLegal(source, name) {
    const bytes = statSync(source).size;
    legalBytes += bytes;
    if (bytes > 8 << 20 || legalBytes > 64 << 20)
      throw Error("License inventory exceeds bounded review size");
    const dest = resolve(legalRoot, name),
      sha256 = hash(source);
    mkdirSync(resolve(dest, ".."), { recursive: true });
    if (existsSync(dest)) {
      if (hash(dest) === sha256) return { path: name, bytes, sha256 };
      if (!name.startsWith("source/"))
        throw Error(
          "Dependency/toolchain license record changed; preserved: " + name,
        );
    }
    copyFileSync(source, dest);
    return { path: name, bytes, sha256 };
  }
  const moduleNotices = [];
  for (const module of [...modules.values()].sort((a, b) =>
    a.path.localeCompare(b.path),
  )) {
    const key = createHash("sha256")
      .update(module.path + "@" + module.version)
      .digest("hex")
      .slice(0, 20);
    const files = legalFiles(module.dir)
      .sort()
      .map((name) =>
        copyLegal(resolve(module.dir, name), `modules/${key}/${name}`),
      );
    moduleNotices.push({
      module: module.path,
      version: module.version,
      files,
      rootLicensePresent: files.some(
        (f) =>
          /^(UNLICEN[CS]E|LICEN[CS]E|COPYING)(?:$|[._-])/i.test(
            f.path.split("/").at(-1),
          ) && f.path.split("/").length === 3,
      ),
    });
  }
  const goRoot = resolve(root, `.tools/go${lock.go.version}/go`);
  const toolchainNotices = ["LICENSE", "PATENTS"]
    .filter((name) => existsSync(resolve(goRoot, name)))
    .map((name) => copyLegal(resolve(goRoot, name), `toolchain/${name}`));
  const patchNotices = [
    copyLegal(patch, "source/0001-explicit-local-dockerfile-read.patch"),
    copyLegal(dependencyPatch, "source/0002-http2-dependency-floor.patch"),
    copyLegal(resolve(root, "patches/compose/README.md"), "source/README.md"),
  ];
  const legalIndex = {
    schema: 1,
    binarySha256,
    platform: `${env.GOOS}/amd64`,
    sourceCommit: commit,
    patchSha256,
    dependencyPatchSha256,
    embeddedModuleMetadataVerified: true,
    modules: moduleNotices,
    toolchain: toolchainNotices,
    source: patchNotices,
    modulesWithoutRootLicense: moduleNotices
      .filter((m) => !m.rootLicensePresent)
      .map((m) => m.module),
    status:
      "Compiled-module source legal files collected; distribution obligations require review, not a legal completeness claim.",
  };
  writeFileSync(
    resolve(legalRoot, "index.json"),
    JSON.stringify(legalIndex, null, 2) + "\n",
  );
  return {
    binarySha256,
    licenseDirectory,
    licenseIndexSha256: hash(resolve(legalRoot, "index.json")),
    compiledModules: moduleNotices.length,
  };
}
const binaries = {};
for (const target of [
  { key: "windowsAmd64", os: "windows", file: "docker-compose.exe" },
  { key: "linuxAmd64", os: "linux", file: "docker-compose" },
]) {
  const env = { ...baseEnv, GOOS: target.os },
    binary = resolve(output, target.file),
    compiledSourceHashes = compiledSources(env);
  run(
    go,
    [
      "build",
      "-mod=readonly",
      "-buildvcs=false",
      "-trimpath",
      "-ldflags",
      "-w -X github.com/docker/compose/v5/internal.Version=v5.6.1-orbwin.2",
      "-o",
      binary,
      "./cmd",
    ],
    checkout,
    { env, stdio: "inherit" },
  );
  verify();
  for (const [name, sha] of Object.entries(compiledSourceHashes))
    if (hash(resolve(checkout, name)) !== sha)
      throw Error("Compiled source changed during build");
  binaries[target.key] = {
    path: target.file,
    platform: `${target.os}/amd64`,
    ...notices(binary, env),
    compiledSourceHashes,
  };
}
verify();
for (const [p, sha] of Object.entries(sources))
  if (hash(resolve(checkout, p)) !== sha)
    throw Error("Compose sources changed during build");
const windows = binaries.windowsAmd64;
const manifest = {
  schema: 1,
  testsNotRun: true,
  sourceDirectory: selected.sourceDirectory,
  repository: "https://github.com/docker/compose.git",
  tag: "v5.6.1",
  tagObject,
  commit,
  version: "v5.6.1-orbwin.2",
  go: lock.go.version,
  platform: "windows/amd64",
  patchSha256,
  binarySha256: windows.binarySha256,
  sources,
  licenseDirectory: windows.licenseDirectory,
  licenseIndexSha256: windows.licenseIndexSha256,
  compiledModules: windows.compiledModules,
  binaries,
  sourcePins: {
    recipeDependencies,
    buildScript: "scripts/build-compose.mjs",
    buildScriptSha256: scriptSha256,
    patch: "patches/compose/0001-explicit-local-dockerfile-read.patch",
    patchSha256,
    dependencyPatch: "patches/compose/0002-http2-dependency-floor.patch",
    dependencyPatchSha256,
    goExecutableSha256: hash(go),
    goArchiveSha256: lock.go.windowsAmd64Sha256,
    upstreamLockSha256: hash(lockPath),
  },
  status:
    "Isolated candidates; runtime pins, installed tools and acceptance unchanged",
};
writeFileSync(
  resolve(output, "compose-manifest.json"),
  JSON.stringify(manifest, null, 2) + "\n",
  { flag: "wx" },
);
console.log(resolve(output, windows.path));
console.log(resolve(output, binaries.linuxAmd64.path));
