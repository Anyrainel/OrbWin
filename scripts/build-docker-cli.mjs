// Fixed-source, CREATE_NEW review candidates. Never activates runtime tools.
import assert from "node:assert/strict";
import { createHash, randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const dockerCliFixed = Object.freeze({
  repository: "https://github.com/docker/cli.git",
  tag: "v29.9.2",
  tagObject: "0cc0fc64632a9ee9cab5884b99fee3b43f08a3e2",
  commit: "a82e528901260c8df6073311379eac65f92db204",
  wrapperCommit: "3ef7c31eb807e504c274f2b744f62b0a719fc97d",
  version: "v29.9.2-rd-orbwin.1",
  go: "1.27.3",
  goArchiveSha256:
    "f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff",
  goExecutableSha256:
    "83b7ba66215e696414fa49462d0db2da4f4cf15c7bebd42f0b189fa04866999d",
  patches: {
    "patches/docker-cli/0001-rancher-linux-credential-override.patch":
      "801a4c1fa40116be7e5fa3e1c8b549ab72a33fc9635a24dd6cd22720d28fece1",
    "patches/docker-cli/0002-orbwin-managed-plugin-directory.patch":
      "ed16a37e22ef6f923d7454dfea3b2f79bfeb676a1308a28c2f6e1cc067bd0ad3",
  },
});
export const digest = (bytes) =>
  createHash("sha256").update(bytes).digest("hex");
export const fileHash = (path) => digest(readFileSync(path));
export function noLinks(path) {
  for (let p = resolve(path); ; p = dirname(p)) {
    try {
      assert(!lstatSync(p).isSymbolicLink(), "Linked candidate input refused");
    } catch (e) {
      if (e.code !== "ENOENT") throw e;
    }
    if (p === dirname(p)) break;
  }
}
export function childPath(base, name) {
  assert(
    typeof name === "string" &&
      name &&
      !name.includes("\\") &&
      !name.includes(":") &&
      name.split("/").every((p) => p && p !== "." && p !== ".."),
    "Closed relative candidate name required",
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
  noLinks(path);
  return path;
}
export function catalog(directory) {
  noLinks(directory);
  const records = {};
  let bytes = 0,
    count = 0;
  function walk(prefix = "") {
    for (const name of readdirSync(resolve(directory, prefix)).sort()) {
      const key = prefix ? `${prefix}/${name}` : name,
        path = childPath(directory, key),
        info = lstatSync(path);
      if (info.isDirectory()) walk(key);
      else {
        assert(info.isFile() && info.size <= 64 << 20);
        bytes += info.size;
        assert(
          bytes <= 1 << 30 && ++count <= 40000,
          "Source inventory bound exceeded",
        );
        records[key] = fileHash(path);
      }
    }
  }
  walk();
  return records;
}
export function verifyCatalog(directory, expected) {
  assert.deepEqual(
    catalog(directory),
    expected,
    "Frozen candidate source/toolchain drifted",
  );
}
const root = fileURLToPath(new URL("../", import.meta.url));
const legalPattern =
  /^(UNLICEN[CS]E|LICEN[CS]E|COPYING|NOTICE|COPYRIGHT|AUTHORS|PATENTS)(?:$|[._-])/i;
export function legalFiles(directory, prefix = "", inside = false) {
  const out = [];
  for (const entry of readdirSync(resolve(directory, prefix), {
    withFileTypes: true,
  })) {
    const name = prefix ? `${prefix}/${entry.name}` : entry.name,
      path = childPath(directory, name),
      info = lstatSync(path);
    if (info.isFile() && (legalPattern.test(entry.name) || inside))
      out.push(name);
    else if (
      info.isDirectory() &&
      !entry.name.startsWith(".") &&
      entry.name !== "vendor"
    )
      out.push(
        ...legalFiles(
          directory,
          name,
          inside || /^(licenses?|copying|notices?)$/i.test(entry.name),
        ),
      );
  }
  return out.sort();
}
export function main() {
  assert(
    process.argv.slice(2).every((a) => a === "--test"),
    "Only --test is supported",
  );
  const checkout = resolve(root, ".upstream/docker-cli"),
    wrapper = resolve(root, ".upstream/docker-cli-wrapper");
  const run = (bin, args, cwd = checkout, env = process.env) =>
    execFileSync(bin, args, {
      cwd,
      env,
      encoding: "utf8",
      windowsHide: true,
      timeout: 600000,
      maxBuffer: 64 << 20,
    });
  const git = (args, cwd = checkout) =>
    run(
      "git",
      ["-c", "core.autocrlf=false", "-c", "core.eol=lf", ...args],
      cwd,
    );
  for (const [ref, want] of [
    ["HEAD", dockerCliFixed.commit],
    [dockerCliFixed.tag, dockerCliFixed.tagObject],
    [`${dockerCliFixed.tag}^{}`, dockerCliFixed.commit],
  ])
    assert.equal(git(["rev-parse", ref]).trim(), want);
  assert.equal(
    git(["rev-parse", "HEAD"], wrapper).trim(),
    dockerCliFixed.wrapperCommit,
  );
  const wrapperPatch =
    "patches/0001-Credentials-Linux-Allow-overriding-default-credentia.patch";
  assert.equal(
    digest(Buffer.from(git(["show", `HEAD:${wrapperPatch}`], wrapper))),
    dockerCliFixed.patches[
      "patches/docker-cli/0001-rancher-linux-credential-override.patch"
    ],
  );
  const wrapperSource = {};
  for (const name of [
    wrapperPatch,
    "LICENSE",
    "README.md",
    ".github/workflows/package.yaml",
  ])
    wrapperSource[name] = git(["show", `HEAD:${name}`], wrapper);
  const recipe = "scripts/build-docker-cli.mjs",
    provenance = "scripts/docker-cli-provenance.mjs",
    readme = "patches/docker-cli/README.md",
    lock = "upstream.lock.json";
  const pinned = {};
  for (const name of [
    recipe,
    provenance,
    readme,
    lock,
    ...Object.keys(dockerCliFixed.patches),
  ]) {
    noLinks(resolve(root, name));
    pinned[name] = fileHash(resolve(root, name));
  }
  for (const [name, sha] of Object.entries(dockerCliFixed.patches))
    assert.equal(
      pinned[name],
      sha,
      "Patch changed; explicit source review required",
    );
  const upstreamLock = JSON.parse(readFileSync(resolve(root, lock)));
  assert.equal(upstreamLock.go.version, dockerCliFixed.go);
  assert.equal(
    upstreamLock.go.windowsAmd64Sha256,
    dockerCliFixed.goArchiveSha256,
  );
  const goZip = resolve(root, ".tools/go1.27.3.zip");
  noLinks(goZip);
  assert.equal(fileHash(goZip), dockerCliFixed.goArchiveSha256);
  const base = resolve(root, "artifacts/docker-cli");
  noLinks(base);
  mkdirSync(base, { recursive: true });
  const output = resolve(base, `candidate-${randomUUID()}`);
  mkdirSync(output); // Exclusive new namespace; never reuse or replace.
  const write = (name, data) => {
    const p = childPath(output, name);
    mkdirSync(dirname(p), { recursive: true });
    writeFileSync(p, data, { flag: "wx" });
  };
  const json = (name, data) =>
    write(name, JSON.stringify(data, null, 2) + "\n");
  for (const [name, bytes] of Object.entries(wrapperSource))
    write(`wrapper-source/${name}`, bytes);
  const source = resolve(output, "source"),
    tools = resolve(output, "toolchain");
  mkdirSync(source);
  mkdirSync(tools);
  const archive = resolve(output, "upstream.tar");
  git(["archive", "--format=tar", "-o", archive, dockerCliFixed.commit]);
  const tracked = git(["ls-tree", "-r", "-z", dockerCliFixed.commit])
    .split("\0")
    .filter(Boolean)
    .map((line) => {
      const [header, name] = line.split("\t");
      const [mode, type, blob] = header.split(" ");
      assert(
        type === "blob" && ["100644", "100755"].includes(mode),
        "Source links/submodules refused",
      );
      childPath(source, name);
      return { name, mode, blob };
    });
  run("tar", ["-xf", archive, "-C", source], output);
  const pristine = catalog(source);
  assert.equal(Object.keys(pristine).length, tracked.length);
  for (const entry of tracked) {
    const bytes = readFileSync(childPath(source, entry.name));
    assert.equal(
      createHash("sha1")
        .update(`blob ${bytes.length}\0`)
        .update(bytes)
        .digest("hex"),
      entry.blob,
      "Export differs from pinned Git blob",
    );
  }
  for (const patch of Object.keys(dockerCliFixed.patches)) {
    const prefix = relative(root, source).replaceAll("\\", "/");
    run(
      "git",
      [
        "-c",
        "core.autocrlf=false",
        "apply",
        "--no-index",
        `--directory=${prefix}`,
        "--check",
        resolve(root, patch),
      ],
      root,
    );
    run(
      "git",
      [
        "-c",
        "core.autocrlf=false",
        "apply",
        "--no-index",
        `--directory=${prefix}`,
        resolve(root, patch),
      ],
      root,
    );
  }
  const changed = [
    "Dockerfile",
    "cli/config/credentials/default_store_linux.go",
    "cli-plugins/manager/manager.go",
    "cli-plugins/manager/manager_orbwin_test.go",
  ];
  const status = git(["status", "--porcelain", "--untracked-files=all"])
    .trimEnd()
    .split(/\r?\n/)
    .filter(Boolean)
    .map((s) => s.slice(3))
    .sort();
  assert.deepEqual(
    status,
    changed.slice().sort(),
    "Unreviewed checkout changes refused",
  );
  for (const name of changed)
    assert.equal(
      readFileSync(childPath(checkout, name), "utf8").replaceAll("\r\n", "\n"),
      readFileSync(childPath(source, name), "utf8"),
      "Working patch source differs from frozen reviewed patch",
    );
  assert(
    !existsSync(resolve(source, "go.mod")) &&
      !existsSync(resolve(source, "go.sum")),
    "Unexpected upstream module opt-in",
  );
  for (const [original, generated] of [
    ["vendor.mod", "go.mod"],
    ["vendor.sum", "go.sum"],
  ])
    copyFileSync(resolve(source, original), resolve(source, generated), 1);
  run("tar", ["-xf", goZip, "-C", tools], output);
  const goroot = resolve(tools, "go"),
    go = resolve(goroot, "bin/go.exe");
  assert.equal(fileHash(go), dockerCliFixed.goExecutableSha256);
  const sources = catalog(source),
    toolchain = catalog(goroot);
  json("source-catalog.json", sources);
  json("toolchain-catalog.json", toolchain);
  const verify = () => {
    verifyCatalog(source, sources);
    verifyCatalog(goroot, toolchain);
    for (const [name, sha] of Object.entries(pinned))
      assert.equal(
        fileHash(resolve(root, name)),
        sha,
        "Recipe input changed during build",
      );
  };
  const env = {
    ...process.env,
    GOROOT: goroot,
    GOCACHE: resolve(output, "gocache"),
    GOPATH: resolve(output, "gopath"),
    GOMODCACHE: resolve(output, "modcache"),
    GOENV: "off",
    GOTOOLCHAIN: "local",
    GOFLAGS: "",
    GOWORK: "off",
    GOPROXY: "off",
    GOSUMDB: "off",
    GOPRIVATE: "",
    GONOPROXY: "",
    GONOSUMDB: "",
    GOEXPERIMENT: "",
    GOARCH: "amd64",
    GOAMD64: "v1",
    CGO_ENABLED: "0",
    GOMAXPROCS: "2",
  };
  const testResults = [];
  if (process.argv.includes("--test")) {
    const text = run(
      go,
      [
        "test",
        "-mod=vendor",
        "-count=1",
        "-timeout=120s",
        "./cli-plugins/manager",
        "-run",
        "^TestOrbWin",
        "-v",
      ],
      source,
      { ...env, GOOS: "windows" },
    );
    assert.match(text, /--- PASS: TestOrbWin/);
    write("test-orbwin-windows.txt", text);
    testResults.push({
      name: "TestOrbWin",
      platform: "windows/amd64",
      sha256: digest(text),
    });
    console.log(text);
  }
  const buildTime = new Date(
    git(["show", "-s", "--format=%cI", dockerCliFixed.commit]).trim(),
  )
    .toISOString()
    .replace(".000Z", "Z");
  const binaries = {};
  for (const target of [
    { key: "windowsAmd64", os: "windows", file: "docker.exe" },
    { key: "linuxAmd64", os: "linux", file: "docker" },
  ]) {
    const platformEnv = { ...env, GOOS: target.os };
    const metadata = run(
      go,
      [
        "list",
        "-mod=vendor",
        "-tags=grpcnotrace",
        "-deps",
        "-f",
        "{{if .Module}}{{.Module.Path}}|{{.Module.Version}}{{end}}",
        "./cmd/docker",
      ],
      source,
      platformEnv,
    );
    const modules = new Map();
    for (const line of metadata.split(/\r?\n/).filter(Boolean)) {
      const [path, version] = line.split("|");
      modules.set(path, { path, version: version || dockerCliFixed.version });
    }
    const compiled = {};
    const template =
      '{{if not .Standard}}{{range .GoFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{range .EmbedFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{range .SFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{range .SysoFiles}}{{$.Dir}}/{{.}}{{"\\n"}}{{end}}{{end}}';
    for (const file of run(
      go,
      [
        "list",
        "-mod=vendor",
        "-tags=grpcnotrace",
        "-deps",
        "-f",
        template,
        "./cmd/docker",
      ],
      source,
      platformEnv,
    )
      .split(/\r?\n/)
      .filter(Boolean)) {
      const name = relative(source, file).replaceAll("\\", "/");
      assert.equal(childPath(source, name), resolve(file));
      compiled[name] = fileHash(file);
    }
    const binary = resolve(output, target.file);
    assert(!existsSync(binary));
    const flags = [
      "-s",
      "-w",
      "-buildid=",
      "-X",
      `github.com/docker/cli/cli/version.Version=${dockerCliFixed.version.slice(1)}`,
      "-X",
      `github.com/docker/cli/cli/version.GitCommit=${dockerCliFixed.commit.slice(0, 12)}-orbwin.1`,
      "-X",
      `github.com/docker/cli/cli/version.BuildTime=${buildTime}`,
    ];
    if (target.os === "linux")
      flags.push(
        "-X",
        "github.com/docker/cli/cli/config/credentials.LinuxCredentialsStoreDefaultOverride=wincred.exe",
      );
    run(
      go,
      [
        "build",
        "-mod=vendor",
        "-tags=grpcnotrace",
        "-buildvcs=false",
        "-trimpath",
        "-ldflags",
        flags.join(" "),
        "-o",
        binary,
        "./cmd/docker",
      ],
      source,
      platformEnv,
    );
    const embeddedText = run(go, ["version", "-m", binary], output, env);
    write(`metadata-${target.os}.txt`, embeddedText);
    const embedded = new Map();
    for (const line of embeddedText.split(/\r?\n/)) {
      const m = line.match(/^\s*(mod|dep)\s+(\S+)\s+(\S+)/);
      if (m) embedded.set(m[2], m[1] === "mod" ? dockerCliFixed.version : m[3]);
    }
    assert.equal(embedded.size, modules.size);
    for (const m of modules.values())
      assert.equal(
        embedded.get(m.path),
        m.version,
        "Binary embedded module differs from compiled inventory",
      );
    let legalBytes = 0;
    const copyLegal = (path, name) => {
      const bytes = readFileSync(path);
      legalBytes += bytes.length;
      assert(bytes.length <= 8 << 20 && legalBytes <= 64 << 20);
      write(`licenses/${target.key}/${name}`, bytes);
      return { path: name, bytes: bytes.length, sha256: digest(bytes) };
    };
    const legalModules = [];
    for (const m of [...modules.values()].sort((a, b) =>
      a.path.localeCompare(b.path),
    )) {
      const dir =
          m.path === "github.com/docker/cli"
            ? source
            : childPath(source, `vendor/${m.path}`),
        key = digest(`${m.path}@${m.version}`).slice(0, 20),
        files = legalFiles(dir).map((n) =>
          copyLegal(childPath(dir, n), `modules/${key}/${n}`),
        );
      legalModules.push({
        module: m.path,
        version: m.version,
        files,
        rootLicensePresent: files.some(
          (f) =>
            f.path.split("/").length === 3 &&
            /^(UNLICEN[CS]E|LICEN[CS]E|COPYING)(?:$|[._-])/i.test(
              f.path.split("/").at(-1),
            ),
        ),
      });
    }
    const sourceLegal = [...Object.keys(dockerCliFixed.patches), readme].map(
      (n) => copyLegal(resolve(root, n), `source/${n.split("/").at(-1)}`),
    );
    sourceLegal.push(
      copyLegal(
        resolve(output, "wrapper-source/LICENSE"),
        "source/RANCHER-WRAPPER-LICENSE",
      ),
    );
    const toolLegal = ["LICENSE", "PATENTS"]
      .filter((n) => existsSync(resolve(goroot, n)))
      .map((n) => copyLegal(resolve(goroot, n), `toolchain/${n}`));
    const index = {
      schema: 1,
      binarySha256: fileHash(binary),
      platform: `${target.os}/amd64`,
      embeddedModuleMetadataVerified: true,
      modules: legalModules,
      source: sourceLegal,
      toolchain: toolLegal,
      modulesWithoutRootLicense: legalModules
        .filter((m) => !m.rootLicensePresent)
        .map((m) => m.module),
      status:
        "Exact compiled vendor/source legal inventory; distribution obligations require review",
    };
    json(`licenses/${target.key}/index.json`, index);
    binaries[target.key] = {
      path: target.file,
      platform: index.platform,
      binarySha256: index.binarySha256,
      compiledSourceHashes: compiled,
      compiledModules: modules.size,
      licenseDirectory: `licenses/${target.key}`,
      licenseIndexSha256: fileHash(
        resolve(output, `licenses/${target.key}/index.json`),
      ),
    };
    verify();
  }
  verify();
  const manifest = {
    schema: 1,
    kind: "orbwin-docker-cli-v1",
    ...dockerCliFixed,
    buildTime,
    generatedModuleCopies: {
      "go.mod": sources["vendor.mod"],
      "go.sum": sources["vendor.sum"],
    },
    wrapperSourceHashes: Object.fromEntries(
      Object.entries(wrapperSource).map(([name, bytes]) => [
        name,
        digest(bytes),
      ]),
    ),
    sourceDirectory: "source",
    sourceCatalogSha256: fileHash(resolve(output, "source-catalog.json")),
    sourceArchiveSha256: fileHash(archive),
    toolchainDirectory: "toolchain/go",
    toolchainCatalogSha256: fileHash(resolve(output, "toolchain-catalog.json")),
    sourcePins: pinned,
    binaries,
    testResults,
    status:
      "Isolated review candidates only; runtime pins and selected tools unchanged",
  };
  json("docker-cli-manifest.json", manifest);
  console.log(resolve(output, "docker-cli-manifest.json"));
  return output;
}
if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
)
  main();
