// Fixed SDK/LAN v0.6.2 close-only source input. Build provenance DATA only.
import assert from "node:assert/strict";
import {
  existsSync,
  lstatSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  writeFileSync,
  copyFileSync,
} from "node:fs";
import { dirname, resolve, relative, isAbsolute, join, win32 } from "node:path";
import { execFileSync } from "node:child_process";
import {
  winioDigestData as digest,
  winioRowsFromBytesData as rows,
  winioProjectionDigestData as projection,
  reconstructWinioPatchData,
} from "./winio-build-input.mjs";
const MODULE = "github.com/Microsoft/go-winio",
  VERSION = "v0.6.2",
  MAIN = "orbwin.local/session";
const SOURCE =
  "b7ff527b184b5018489b93fcfcefe803f71e875d2168caf24b7bcec1c62b2ca2";
const PEER = "fdef8aed176308008fdec4cde73ff53133f42a65ccb6843c872a125f68c2b9d4";
const PATCH = "patches/go-winio/0002-sdk062-original-file-close.patch";
export const sdkWinioRoles = Object.freeze({
  sdk: Object.freeze({ artifact: "orbwin-session.exe", package: "." }),
  helper: Object.freeze({
    artifact: "lan/orbwin-lan-helper.exe",
    package: "./lan-helper",
  }),
  worker: Object.freeze({
    artifact: "lan/orbwin-lan-worker.exe",
    package: "./lan-worker",
  }),
});
export const sdkWinioRecipes = Object.freeze([
  "sdk-winio-build-input.mjs",
  "winio-build-input.mjs",
  "winio-build-input-fs.mjs",
  "build-session.mjs",
  "build-session-bundle.mjs",
  "lan-bundle-provenance.mjs",
  "collect-notices.mjs",
]);
const json = (v) => Buffer.from(JSON.stringify(v, null, 2) + "\n");
const need = (v, m) => assert.ok(v, "SDK Winio source input refused: " + m);
const closed = (v, k) => {
  need(v && typeof v === "object" && !Array.isArray(v), "record");
  assert.deepEqual(Object.keys(v).sort(), [...k].sort());
};
function safe(p) {
  p = resolve(p);
  for (let q = p; ; q = dirname(q)) {
    if (existsSync(q)) need(!lstatSync(q).isSymbolicLink(), "redirected path");
    if (dirname(q) === q) break;
  }
  return p;
}
function read(p, max = 16 << 20) {
  p = safe(p);
  const s = lstatSync(p);
  need(s.isFile() && s.size <= max, "bounded regular file");
  const b = readFileSync(p);
  need(b.length === s.size, "read changed");
  return b;
}
function name(n) {
  need(
    typeof n === "string" &&
      n.length > 0 &&
      !/[\\:\x00-\x1f]/.test(n) &&
      n.split("/").every((x) => x && x !== "." && x !== ".."),
    "source name",
  );
  return n;
}
function tree(root) {
  const out = new Map();
  let total = 0;
  function walk(d, p = "") {
    for (const e of readdirSync(safe(d), { withFileTypes: true })) {
      const n = p + name(e.name);
      need(!e.isSymbolicLink(), "source link");
      if (e.isDirectory()) walk(join(d, e.name), n + "/");
      else {
        const b = read(join(d, e.name));
        total += b.length;
        need(total <= 256 << 20 && out.size < 4096, "tree bound");
        out.set(n, b);
      }
    }
  }
  walk(root);
  rows(out);
  return out;
}
function put(p, b) {
  safe(dirname(p));
  mkdirSync(dirname(p), { recursive: true });
  writeFileSync(p, b, { flag: "wx" });
}
function recipes(root) {
  return Object.fromEntries(
    sdkWinioRecipes.map((n) => [n, digest(read(join(root, "scripts", n)))]),
  );
}
export function validateSdkWinioLock(lock) {
  closed(lock, [
    "schema",
    "role",
    "module",
    "version",
    "main",
    "sourceReceiptSHA256",
    "reviewPeerSHA256",
    "upstreamZipSHA256",
    "upstreamH1",
    "baseline",
    "patched",
    "patchSHA256",
    "recipeHashes",
  ]);
  need(
    lock.schema === 1 &&
      lock.role === "sdk-lan-windows-amd64-close-only" &&
      lock.module === MODULE &&
      lock.version === VERSION &&
      lock.main === MAIN &&
      lock.sourceReceiptSHA256 === SOURCE &&
      lock.reviewPeerSHA256 === PEER,
    "fixed profile",
  );
  need(
    lock.baseline.length === 109 && lock.patched.length === 111,
    "reviewed109/111",
  );
  need(
    projection(lock.baseline) ===
      "5dd752f952120434525e8fe121542cf51a85ea8ff935098d154bc87dabeffda2" &&
      projection(lock.patched) ===
        "74683a7fb1ac5c6d39696392700d35cc0bba68aae77b07548cf3d9e26d216462" &&
      lock.patchSHA256 ===
        "f6cc57b0fbdad6fab76c56478327d7739234db79c22c6753cc7458072837ee67",
    "fixed reviewed source projection",
  );
  need(
    lock.upstreamZipSHA256 ===
      "cd007bdb877a578efde2780bf5dd2f57dd557b5b08c3614597b820d0369ebb0f" &&
      lock.upstreamH1 === "h1:F2VQgta7ecxGYO8k3ZZz3RS8fVIXVxONVUPlNERoyfY=",
    "original ZIP/h1",
  );
  closed(lock.recipeHashes, sdkWinioRecipes);
  return lock;
}
// Single SDK modfile, leaving its exact graph/options/overlay intact.
export function composeSdkWinioModfile(mod, sum, moduleRoot) {
  need(Buffer.isBuffer(mod) && Buffer.isBuffer(sum), "module bytes");
  const text = mod.toString("utf8");
  need(
    Buffer.from(text).equals(mod) &&
      /^module orbwin\.local\/session\r?$/m.test(text),
    "SDK main module",
  );
  need(
    !/^\s*(replace|exclude|toolchain)\b/m.test(text),
    "preexisting graph override",
  );
  need(
    /github\.com\/Microsoft\/go-winio v0\.6\.2\b/.test(text),
    "selected requirement",
  );
  const path = resolve(moduleRoot).replaceAll("\\", "/");
  need(!/[\r\n"\x00]/.test(path), "modfile path");
  return {
    mod: Buffer.concat([
      mod,
      Buffer.from("\nreplace " + MODULE + " => " + JSON.stringify(path) + "\n"),
    ]),
    sum: Buffer.from(sum),
  };
}
export function sdkWinioEnvironment(input = {}) {
  const fixed = {
    GOOS: "windows",
    GOARCH: "amd64",
    GOAMD64: "v1",
    CGO_ENABLED: "0",
    GOENV: "off",
    GOWORK: "off",
    GOFLAGS: "",
    GOCACHEPROG: "",
    GOTOOLCHAIN: "local",
    GOPROXY: "off",
    GOSUMDB: "off",
    GOVCS: "*:off",
    GOTELEMETRY: "off",
    GOEXPERIMENT: "",
  };
  const out = {},
    seen = new Set();
  for (const [k, v] of Object.entries(input)) {
    const key = k.toUpperCase();
    need(!seen.has(key), "ambiguous environment");
    seen.add(key);
    if (Object.hasOwn(fixed, key)) {
      need(v === "" || String(v) === fixed[key], "ambient override " + key);
    } else {
      need(!["GOMOD", "GO_EXTLINK_ENABLED"].includes(key), "module override");
      out[k] = v;
    }
  }
  return { ...out, ...fixed };
}
function module(m, root) {
  closed(m, ["Path", "Version", "Dir", "Replace"]);
  closed(m.Replace, ["Path", "Dir"]);
  need(m.Path === MODULE && m.Version === VERSION, "actual MVS");
  for (const p of [m.Dir, m.Replace.Path, m.Replace.Dir])
    need(
      resolve(p).toLowerCase() === resolve(root).toLowerCase(),
      "actual replacement",
    );
}
function stream(bytes) {
  need(
    Buffer.isBuffer(bytes) && bytes.length > 0 && bytes.length <= 32 << 20,
    "metadata bound",
  );
  const text = bytes.toString("utf8");
  need(Buffer.from(text).equals(bytes), "metadata UTF8");
  const out = [];
  let start = -1,
    depth = 0,
    quoted = false,
    escape = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (start < 0) {
      if (/\s/.test(c)) continue;
      need(c === "{", "metadata stream");
      start = i;
      depth = 1;
      continue;
    }
    if (quoted) {
      if (escape) escape = false;
      else if (c === "\\") escape = true;
      else if (c === '"') quoted = false;
      continue;
    }
    if (c === '"') {
      quoted = true;
      continue;
    }
    if (c === "{" || c === "[") depth++;
    if (c === "}" || c === "]") depth--;
    if (depth === 0) {
      out.push(JSON.parse(text.slice(start, i + 1)));
      start = -1;
      need(out.length <= 20000, "package bound");
    }
  }
  need(start < 0 && out.length > 0 && !quoted, "truncated metadata");
  return out;
}
export function projectSdkWinioMetadata(modBytes, packageBytes) {
  const modules = stream(modBytes);
  need(modules.length === 1, "one module");
  const selected = (m) => ({
    Path: m.Path,
    Version: m.Version,
    Dir: m.Dir,
    Replace: m.Replace ? { Path: m.Replace.Path, Dir: m.Replace.Dir } : null,
  });
  return {
    module: selected(modules[0]),
    packages: stream(packageBytes)
      .filter(
        (p) =>
          p.ImportPath === MODULE || p.ImportPath?.startsWith(MODULE + "/"),
      )
      .map((p) => ({
        ImportPath: p.ImportPath,
        Dir: p.Dir,
        Module: selected(p.Module),
        ...Object.fromEntries(
          ["GoFiles", "CgoFiles", "CompiledGoFiles", "EmbedFiles"].map((k) => [
            k,
            p[k] ?? [],
          ]),
        ),
      })),
    rawModuleSHA256: digest(modBytes),
    rawPackagesSHA256: digest(packageBytes),
  };
}
export function validateSdkWinioMetadata(record, lock, expected) {
  validateSdkWinioLock(lock);
  closed(record, [
    "module",
    "packages",
    "rawModuleSHA256",
    "rawPackagesSHA256",
    "context",
  ]);
  const ctx = record.context;
  closed(ctx, [
    "role",
    "main",
    "goos",
    "goarch",
    "goamd64",
    "cgo",
    "tags",
    "package",
    "sourceRoot",
    "moduleRoot",
    "modSHA256",
    "sumSHA256",
    "overlaySHA256",
  ]);
  assert.deepEqual(ctx, expected);
  for (const k of ["modSHA256", "sumSHA256"])
    need(/^[0-9a-f]{64}$/.test(ctx[k]) && !/^0+$/.test(ctx[k]), "module hash");
  for (const k of ["rawModuleSHA256", "rawPackagesSHA256"])
    need(
      /^[0-9a-f]{64}$/.test(record[k]) && !/^0+$/.test(record[k]),
      "raw metadata hash",
    );
  need(
    ctx.overlaySHA256 === null || /^[0-9a-f]{64}$/.test(ctx.overlaySHA256),
    "overlay hash",
  );
  need(
    Object.hasOwn(sdkWinioRoles, ctx.role) &&
      ctx.main === MAIN &&
      ctx.package === sdkWinioRoles[ctx.role].package &&
      ctx.goos === "windows" &&
      ctx.goarch === "amd64" &&
      ctx.goamd64 === "v1" &&
      ctx.cgo === "0" &&
      Array.isArray(ctx.tags) &&
      ctx.tags.length === 0,
    "Windows role",
  );
  module(record.module, ctx.moduleRoot);
  need(
    Array.isArray(record.packages) &&
      record.packages.length > 0 &&
      record.packages.length <= 128,
    "packages",
  );
  const known = new Map(lock.patched.map((r) => [r.path, r])),
    used = new Set(),
    imports = new Set();
  for (const p of record.packages) {
    closed(p, [
      "ImportPath",
      "Dir",
      "Module",
      "GoFiles",
      "CgoFiles",
      "CompiledGoFiles",
      "EmbedFiles",
    ]);
    need(
      (p.ImportPath === MODULE || p.ImportPath.startsWith(MODULE + "/")) &&
        !imports.has(p.ImportPath),
      "package import",
    );
    imports.add(p.ImportPath);
    module(p.Module, ctx.moduleRoot);
    const dir = resolve(p.Dir),
      root = resolve(ctx.moduleRoot),
      rel = relative(root, dir);
    need(
      !isAbsolute(rel) &&
        rel !== ".." &&
        !rel.startsWith("..\\") &&
        !rel.startsWith("../"),
      "package root",
    );
    const lists = {};
    for (const k of ["GoFiles", "CgoFiles", "CompiledGoFiles", "EmbedFiles"]) {
      need(Array.isArray(p[k]) && p[k].length <= 512, "file list");
      const seen = new Set();
      lists[k] = p[k].map((n) => {
        const f = isAbsolute(n) ? resolve(n) : resolve(dir, name(n));
        const nrel = relative(root, f).replaceAll("\\", "/");
        name(nrel);
        need(
          !nrel.endsWith("_test.go") &&
            known.has(nrel) &&
            !seen.has(nrel.toLowerCase()),
          "unreviewed selected file",
        );
        seen.add(nrel.toLowerCase());
        used.add(nrel);
        return nrel;
      });
    }
    need(
      lists.GoFiles.length + lists.CgoFiles.length > 0 &&
        lists.CompiledGoFiles.length > 0 &&
        [...lists.GoFiles, ...lists.CgoFiles].every((n) =>
          lists.CompiledGoFiles.includes(n),
        ),
      "compiled closure",
    );
  }
  need(
    imports.has(MODULE) &&
      ["file.go", "file_original_close_state.go", "pipe.go"].every((n) =>
        used.has(n),
      ),
    "close source selected",
  );
  return [...used].sort().map((n) => known.get(n));
}
export function prepareSdkWinioInput({
  root,
  source,
  output,
  moduleCache,
  environment = process.env,
}) {
  const env = sdkWinioEnvironment(environment);
  root = safe(root);
  source = safe(source);
  output = safe(output);
  moduleCache = safe(moduleCache);
  const outside = (a, b) => {
    const r = relative(a, b);
    return (
      r &&
      (isAbsolute(r) ||
        r === ".." ||
        r.startsWith("..\\") ||
        r.startsWith("../"))
    );
  };
  need(
    outside(source, output) &&
      outside(output, source) &&
      outside(moduleCache, output) &&
      outside(output, moduleCache),
    "distinct output",
  );
  const lockBytes = read(join(root, "sdk-winio.lock.json")),
    lock = validateSdkWinioLock(JSON.parse(lockBytes));
  assert.deepEqual(lock.recipeHashes, recipes(root));
  const original = tree(
    join(moduleCache, "github.com/!microsoft/go-winio@v0.6.2"),
  );
  assert.deepEqual(rows(original), lock.baseline);
  const zip = read(
    join(
      moduleCache,
      "cache/download/github.com/!microsoft/go-winio/@v/v0.6.2.zip",
    ),
  );
  need(digest(zip) === lock.upstreamZipSHA256, "ZIP");
  const patch = read(join(root, PATCH));
  need(digest(patch) === lock.patchSHA256, "patch");
  const modified = reconstructWinioPatchData(original, patch);
  assert.deepEqual(rows(modified), lock.patched);
  const input = join(output, ".sdk-winio-build"),
    moduleRoot = join(input, "module");
  need(!existsSync(input), "input exists");
  const composed = composeSdkWinioModfile(
    read(join(source, "go.mod")),
    read(join(source, "go.sum")),
    moduleRoot,
  );
  mkdirSync(input);
  for (const [n, b] of modified) put(join(moduleRoot, n), b);
  put(join(input, "sdk.mod"), composed.mod);
  put(join(input, "sdk.sum"), composed.sum);
  const offerDir = join(output, "sdk-winio-source");
  mkdirSync(offerDir);
  const offer = sdkWinioPortableData(lock, lockBytes);
  put(join(offerDir, "manifest.json"), json(offer));
  put(join(offerDir, "upstream.zip"), zip);
  put(join(offerDir, "winio-custody.patch"), patch);
  put(join(offerDir, "LICENSE"), modified.get("LICENSE"));
  put(
    join(offerDir, "README.md"),
    Buffer.from(
      "Modified SDK Winio v0.6.2 close-only source; original ZIP+h1, patch, MIT/tests preserved. No native/lifecycle qualification.\n",
    ),
  );
  return {
    environment: env,
    flags: ["-mod=readonly", "-modfile=" + join(input, "sdk.mod")],
    lock,
    moduleRoot,
    source,
    mod: composed.mod,
    sum: composed.sum,
    portable: offer,
    sourceFiles: sdkWinioSourceFiles(output),
    verify() {
      assert.deepEqual(rows(tree(moduleRoot)), lock.patched);
      need(
        read(join(input, "sdk.mod")).equals(composed.mod) &&
          read(join(input, "sdk.sum")).equals(composed.sum),
        "module input changed",
      );
    },
  };
}
export function captureSdkWinio(input, go, role, overlayArgument = null) {
  need(Object.hasOwn(sdkWinioRoles, role), "fixed capture role");
  const overlay = overlayArgument ? [overlayArgument] : [];
  need(
    overlay.length === 0 || /^\-overlay=/.test(overlayArgument),
    "overlay argument",
  );
  const options = {
    cwd: input.source,
    env: input.environment,
    windowsHide: true,
    maxBuffer: 32 << 20,
  };
  const m = execFileSync(
      go,
      ["list", ...input.flags, ...overlay, "-m", "-json", MODULE],
      options,
    ),
    p = execFileSync(
      go,
      [
        "list",
        ...input.flags,
        ...overlay,
        "-deps",
        "-compiled",
        "-json",
        sdkWinioRoles[role].package,
      ],
      options,
    );
  const record = projectSdkWinioMetadata(m, p);
  record.context = {
    role,
    main: MAIN,
    goos: "windows",
    goarch: "amd64",
    goamd64: "v1",
    cgo: "0",
    tags: [],
    package: sdkWinioRoles[role].package,
    sourceRoot: input.source,
    moduleRoot: input.moduleRoot,
    modSHA256: digest(input.mod),
    sumSHA256: digest(input.sum),
    overlaySHA256: overlayArgument
      ? digest(read(overlayArgument.slice("-overlay=".length)))
      : null,
  };
  const files = validateSdkWinioMetadata(record, input.lock, record.context);
  input.verify();
  return { record, files };
}
export function settleSdkWinio(input, before, after) {
  requireSdkWinioCaptureEquality(before, after);
  input.verify();
  return { before, after };
}
export function verifySdkWinioProvenance(root, directory, provenance, role) {
  const lockBytes = read(join(root, "sdk-winio.lock.json")),
    lock = validateSdkWinioLock(JSON.parse(lockBytes));
  assert.deepEqual(lock.recipeHashes, recipes(root));
  closed(provenance, ["portable", "sourceFiles", "capture"]);
  const offer = JSON.parse(
    read(join(directory, "sdk-winio-source/manifest.json")),
  );
  assert.deepEqual(provenance.portable, offer);
  assert.deepEqual(provenance.sourceFiles, sdkWinioSourceFiles(directory));
  need(
    read(join(directory, "sdk-winio-source/manifest.json")).equals(
      json(sdkWinioPortableData(lock, lockBytes)),
    ),
    "exact portable manifest bytes",
  );
  need(
    read(join(directory, "sdk-winio-source/README.md")).equals(
      Buffer.from(
        "Modified SDK Winio v0.6.2 close-only source; original ZIP+h1, patch, MIT/tests preserved. No native/lifecycle qualification.\n",
      ),
    ),
    "source README",
  );
  assert.deepEqual(
    offer,
    sdkWinioPortableData(lock, lockBytes),
    "source variant",
  );
  assert.deepEqual(offer.patched, lock.patched);
  assert.deepEqual(offer.recipeHashes, lock.recipeHashes);
  need(
    digest(read(join(directory, "sdk-winio-source/upstream.zip"))) ===
      lock.upstreamZipSHA256 &&
      digest(read(join(directory, "sdk-winio-source/winio-custody.patch"))) ===
        lock.patchSHA256,
    "portable sources",
  );
  need(
    digest(read(join(directory, "sdk-winio-source/LICENSE"))) ===
      lock.patched.find((r) => r.path === "LICENSE").sha256,
    "MIT",
  );
  requireSdkWinioCaptureEquality(
    provenance.capture.before,
    provenance.capture.after,
  );
  const c = provenance.capture.before;
  need(c.record.context.role === role, "artifact role");
  const composed = composeSdkWinioModfile(
    read(join(root, "backend-tools/session/go.mod")),
    read(join(root, "backend-tools/session/go.sum")),
    c.record.context.moduleRoot,
  );
  need(
    c.record.context.modSHA256 === digest(composed.mod) &&
      c.record.context.sumSHA256 === digest(composed.sum),
    "SDK composite bytes",
  );
  for (const captured of [provenance.capture.before, provenance.capture.after])
    assert.deepEqual(
      captured.files,
      validateSdkWinioMetadata(captured.record, lock, captured.record.context),
    );
  return { offer, lock };
}
export function classifySdkWinioNotice(dependency, artifact, provenance, main) {
  const role = Object.keys(sdkWinioRoles).find(
    (r) => sdkWinioRoles[r].artifact === artifact,
  );
  if (!role) return null;
  if (dependency.Path !== MODULE) return null;
  need(
    main === MAIN && provenance.capture.before.record.context.role === role,
    "actual SDK main/artifact role",
  );
  need(
    !dependency.Replace?.Version || dependency.Replace.Version === "(devel)",
    "SDK local source version",
  );
  if (dependency.Replace?.Dir)
    need(
      resolve(dependency.Replace.Dir).toLowerCase() ===
        resolve(
          provenance.capture.before.record.context.moduleRoot,
        ).toLowerCase(),
      "SDK source directory",
    );
  need(
    dependency.Version === VERSION && dependency.Replace,
    "SDK Winio dependency",
  );
  module(
    {
      Path: dependency.Path,
      Version: dependency.Version,
      Dir: dependency.Replace.Path,
      Replace: { Path: dependency.Replace.Path, Dir: dependency.Replace.Path },
    },
    provenance.capture.before.record.context.moduleRoot,
  );
  return "orbwin-sdk-winio062-close";
}
export function stageSdkWinioNotice(directory, destination, expected) {
  closed(expected, [
    "manifest.json",
    "upstream.zip",
    "winio-custody.patch",
    "LICENSE",
    "README.md",
  ]);
  need(!existsSync(destination), "notice destination exists");
  mkdirSync(safe(destination));
  for (const n of Object.keys(expected)) {
    const bytes = read(join(directory, "sdk-winio-source", n));
    need(digest(bytes) === expected[n], "notice source changed");
    put(join(destination, n), bytes);
    need(read(join(destination, n)).equals(bytes), "notice copy changed");
  }
}

export function sdkWinioPortableData(lock, lockBytes) {
  validateSdkWinioLock(lock);
  return {
    schema: 1,
    variant: "orbwin-sdk-winio062-close",
    module: MODULE,
    version: VERSION,
    lockSHA256: digest(lockBytes),
    sourceReceiptSHA256: SOURCE,
    reviewPeerSHA256: PEER,
    upstreamH1: lock.upstreamH1,
    upstreamZipSHA256: lock.upstreamZipSHA256,
    patchSHA256: lock.patchSHA256,
    patched: lock.patched,
    recipeHashes: lock.recipeHashes,
  };
}

export function sdkWinioSourceFiles(directory) {
  return Object.fromEntries(
    [
      "manifest.json",
      "upstream.zip",
      "winio-custody.patch",
      "LICENSE",
      "README.md",
    ].map((n) => [n, digest(read(join(directory, "sdk-winio-source", n)))]),
  );
}

// Raw Go JSON may include changing build-cache/Stale diagnostics. Preserve its
// independent hashes as evidence, but compare actual selected projection only.
function requireSdkWinioCaptureEquality(before, after) {
  const selected = (r) => {
    const { rawModuleSHA256, rawPackagesSHA256, ...projection } = r;
    return projection;
  };
  assert.deepEqual(
    selected(before.record),
    selected(after.record),
    "SDK Winio selected closure changed across build",
  );
  assert.deepEqual(before.files, after.files, "SDK selected file rows changed");
}
