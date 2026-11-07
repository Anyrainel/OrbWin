// Version-specific build input, never a runtime authority or dependency resolver.
// Original ZIP/h1 and all source bytes were independently reviewed together.
// Copy only pinned cache files; modify only a fresh per-build source namespace.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve } from "node:path";

import { preflightWinioBuildInput, verifyWinioBuildInput } from "./winio-build-input.mjs";

const MODULE = "github.com/lima-vm/lima/v2";
const VERSION = "v2.3.0-beta.1.0.20261005214905-0ae2df64800a";
const MODULE_SUM = "h1:K7eqntsK4hwmmYGDBvvxx2gurUkU1HVyYmPJVDBQo5Y=";
const MOD_SUM = "h1:RXtitG7HwKeIjjpOizu06sHXr50szp1OJAB/iafmq3c=";
const RDD = "github.com/rancher-sandbox/rancher-desktop-daemon";
const REPLACEMENTS = ["pkg/driver/wsl2/configured_lifecycle_windows_test.go", "pkg/driver/wsl2/import_args_windows.go", "pkg/driver/wsl2/import_args_windows_test.go", "pkg/driver/wsl2/vm_windows.go", "pkg/driver/wsl2/worker_lifecycle.go", "pkg/driver/wsl2/worker_lifecycle_test.go", "pkg/driver/wsl2/wsl_driver_windows.go", "pkg/hostagent/hostagent.go", "pkg/hostagent/lifecycle_state.go", "pkg/hostagent/lifecycle_state_test.go", "pkg/hostagent/requirements.go", "pkg/limayaml/defaults.go", "pkg/limayaml/host_home.go", "pkg/limayaml/host_home_test.go"];
const RECIPES = ["scripts/lima-wsl2-build-input.mjs", "scripts/build-backend.mjs", "scripts/build-identity.mjs", "scripts/collect-notices.mjs"];
const OFFER = ["LICENSE", "NOTICE", "README.md", "lima-import-version.patch", "manifest.json", "upstream.zip"];
const FIXED_ENV = { GOFLAGS: "", GOENV: "off", GOWORK: "off", GOTOOLCHAIN: "local", GOPROXY: "off", GOSUMDB: "off" };
const digest = (body) => createHash("sha256").update(body).digest("hex");
const json = (value) => Buffer.from(JSON.stringify(value, null, 2) + "\n");
const isHash = (value) => typeof value === "string" && /^[0-9a-f]{64}$/.test(value);

export function limaBuildEnvironment(environment = process.env) {
  for (const [key, value] of Object.entries(environment))
    if (key.toUpperCase() === "GOFLAGS") assert.ok(!String(value ?? "").trim(), "Nonempty ambient GOFLAGS refused before build effects");
  return { ...Object.fromEntries(Object.entries(environment).filter(([key]) => !Object.hasOwn(FIXED_ENV, key.toUpperCase()))), ...FIXED_ENV };
}

function name(value) {
  assert.ok(typeof value === "string" && value && !isAbsolute(value) && !value.includes("\\") && !value.includes(":") && !value.includes("\0"));
  assert.ok(value.split("/").every((part) => part && part !== "." && part !== ".."), "Escaped source name refused");
  return value;
}
function inside(root, value) {
  const result = resolve(root, name(value));
  assert.ok(result.startsWith(resolve(root) + "/") || result.startsWith(resolve(root) + "\\"));
  return result;
}
function read(path, maximum = 16 << 20) {
  const stat = lstatSync(path);
  assert.ok(stat.isFile() && !stat.isSymbolicLink() && stat.size <= maximum, "Non-regular or oversized build input refused");
  const body = readFileSync(path);
  assert.equal(body.length, stat.size, "Build input changed during read");
  return body;
}
function directory(path) {
  const stat = lstatSync(path);
  assert.ok(stat.isDirectory() && !stat.isSymbolicLink(), "Redirected build directory refused");
}
function sorted(files) { return [...files].sort((a, b) => a.path < b.path ? -1 : a.path > b.path ? 1 : 0); }
function files(root) {
  directory(root);
  const result = [];
  for (const entry of readdirSync(root, { withFileTypes: true }).sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0)) {
    assert.ok(!entry.isSymbolicLink(), "Source link refused");
    const path = join(root, entry.name);
    if (entry.isDirectory()) for (const file of files(path)) result.push({ ...file, path: entry.name + "/" + file.path });
    else {
      const body = read(path);
      result.push({ path: entry.name, sha256: digest(body), bytes: body.length });
    }
  }
  return sorted(result);
}
function projection(root, expected) {
  const observed = files(root);
  assert.deepEqual(observed, sorted(expected), "Lima source projection differs from reviewed bytes");
  return observed;
}
function profile(root) {
  const lockBytes = read(resolve(root, "lima-wsl2.lock.json"), 2 << 20);
  const pin = JSON.parse(lockBytes);
  assert.equal(pin.schema, 1);
  assert.equal(pin.module, MODULE);
  assert.equal(pin.version, VERSION);
  assert.equal(pin.upstreamModuleSum, MODULE_SUM);
  assert.equal(pin.upstreamGoModSum, MOD_SUM);
  for (const value of [pin.upstreamZipSha256, pin.rddGoModSha256, pin.rddGoSumSha256, pin.patchedProjectionSha256, pin.patch.sha256]) assert.ok(isHash(value));
  assert.ok(Array.isArray(pin.baseline) && pin.baseline.length > 0 && pin.baseline.length <= 4096);
  const base = new Map();
  const folded = new Set();
  for (const entry of pin.baseline) {
    name(entry.path);
    assert.ok(isHash(entry.sha256) && Number.isSafeInteger(entry.bytes) && entry.bytes >= 0 && entry.bytes <= 16 << 20);
    assert.ok(!folded.has(entry.path.toLowerCase()), "Ambiguous source name refused");
    folded.add(entry.path.toLowerCase());
    base.set(entry.path, { path: entry.path, sha256: entry.sha256, bytes: entry.bytes });
  }
  assert.deepEqual(pin.replacements.map((entry) => entry.path).sort(), REPLACEMENTS);
  const final = new Map(base);
  const replacementBytes = new Map();
  for (const entry of pin.replacements) {
    assert.equal(base.get(entry.path)?.sha256 ?? null, entry.before);
    const body = read(inside(root, entry.replacement));
    assert.equal(digest(body), entry.sha256, "Replacement source changed");
    assert.equal(body.length, entry.bytes);
    replacementBytes.set(entry.path, body);
    final.set(entry.path, { path: entry.path, sha256: entry.sha256, bytes: body.length });
  }
  const baseline = sorted([...base.values()]), patched = sorted([...final.values()]);
  assert.equal(patched.length, pin.patchedSourceFiles);
  assert.equal(digest(json(patched)), pin.patchedProjectionSha256);
  for (const license of ["LICENSE", "NOTICE"]) assert.ok(base.has(license), "Pinned Lima legal input missing");
  const patch = read(inside(root, pin.patch.path));
  assert.equal(digest(patch), pin.patch.sha256, "Lima patch changed");
  const recipeHashes = Object.fromEntries(RECIPES.map((path) => [path, digest(read(inside(root, path)))]));
  return { pin, baseline, patched, patch, replacementBytes, lockSha256: digest(lockBytes), recipeHashes };
}
function mainModule(source, pin) {
  directory(source);
  const mod = read(join(source, "go.mod")), sum = read(join(source, "go.sum"));
  assert.equal(digest(mod), pin.rddGoModSha256, "Wrong actual root-rdd go.mod");
  assert.equal(digest(sum), pin.rddGoSumSha256, "Wrong actual root-rdd go.sum");
  assert.match(mod.toString(), new RegExp("^module " + RDD.replaceAll(".", "\\.") + "\\r?$", "m"));
  assert.ok(mod.toString().includes(MODULE + " " + VERSION), "Unsupported Lima requirement");
  assert.ok(!new RegExp(MODULE.replaceAll(".", "\\.") + "(?:\\s+" + VERSION.replaceAll(".", "\\.") + ")?\\s*=>").test(mod.toString()), "Existing Lima replacement refused");
  for (const line of [MODULE + " " + VERSION + " " + MODULE_SUM, MODULE + " " + VERSION + "/go.mod " + MOD_SUM]) assert.ok(sum.toString().split(/\r?\n/).includes(line), "Unpinned Lima module sum");
  return { mod, sum };
}
function cacheInput(moduleCache, data) {
  const cacheModule = resolve(moduleCache, MODULE + "@" + VERSION);
  const cacheZip = resolve(moduleCache, "cache/download", MODULE, "@v", VERSION + ".zip");
  const zip = read(cacheZip);
  assert.equal(digest(zip), data.pin.upstreamZipSha256, "Lima module ZIP differs from verified h1/ZIP pair");
  assert.equal(zip.length, data.pin.upstreamZipBytes);
  projection(cacheModule, data.baseline);
  return { cacheModule, cacheZip, zip };
}
function derived(mod, limaRoot) {
  return Buffer.concat([mod, Buffer.from("\n// OrbWin: reviewed per-version WSL2 import and Windows host-home adaptation.\nreplace " + MODULE + " " + VERSION + " => " + JSON.stringify(limaRoot.replaceAll("\\", "/")) + "\n")]);
}
function offer(rootData, sourceFiles, inputZip, licenses) {
  const { pin, patch, lockSha256, recipeHashes } = rootData;
  const readme = Buffer.from("OrbWin local Lima adaptation\n\nModule: " + MODULE + " " + VERSION + "\nUpstream h1 (original source only): " + MODULE_SUM + "\nPatched source SHA256 projection: " + pin.patchedProjectionSha256 + "\n\nupstream.zip contains the original pinned module. Apply lima-import-version.patch to reconstruct the modified source. LICENSE and NOTICE are preserved. The patch adds --version 2 to new owned WSL imports and defers Windows host-template home lookup until evaluation after native environment projection; non-Windows eager cached home behavior is preserved. The selected lifecycle patch also tracks configured WSL workers and host-agent cleanup, preserves start/cleanup errors, and uses checked nonrecursive cleanup for owned temporary scripts. This source offer does not qualify guest authentication, native cleanup or recovery. Existing registrations and global defaults are unchanged. Generated modfiles and absolute build paths are not runtime requirements.\n");
  const payloads = { "upstream.zip": inputZip, "lima-import-version.patch": patch, LICENSE: licenses.LICENSE, NOTICE: licenses.NOTICE, "README.md": readme };
  const manifest = {
    schema: 1, module: MODULE, version: VERSION, upstreamModuleSum: MODULE_SUM, upstreamGoModSum: MOD_SUM,
    upstreamZipSha256: pin.upstreamZipSha256, baselineProjectionSha256: digest(json(rootData.baseline)),
    patchedProjectionSha256: pin.patchedProjectionSha256, sourceFiles,
    patchSha256: pin.patch.sha256, lockSha256, recipeHashes,
    files: Object.fromEntries(Object.entries(payloads).map(([path, body]) => [path, digest(body)])),
  };
  payloads["manifest.json"] = json(manifest);
  const portable = {
    schema: 1, module: MODULE, version: VERSION, upstreamModuleSum: MODULE_SUM, upstreamGoModSum: MOD_SUM,
    upstreamZipSha256: pin.upstreamZipSha256, patchedProjectionSha256: pin.patchedProjectionSha256,
    patchedSourceFiles: pin.patchedSourceFiles, lockSha256, recipeHashes,
    offer: { directory: "lima-wsl2", files: Object.fromEntries(Object.entries(payloads).map(([path, body]) => [path, digest(body)])) },
  };
  return { payloads, manifest, portable };
}
function put(path, body) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, body, { flag: "wx", mode: 0o600 });
}
function current({ root, source, output, moduleCache, environment }) {
  const childEnvironment = limaBuildEnvironment(environment);
  root = resolve(root); source = resolve(source); output = resolve(output); moduleCache = resolve(moduleCache);
  const data = profile(root), main = mainModule(source, data.pin), cache = cacheInput(moduleCache, data);
  const inputRoot = join(output, "lima-build-input"), limaRoot = join(inputRoot, "lima");
  const modfile = join(inputRoot, "rdd-lima.mod"), sumfile = join(inputRoot, "rdd-lima.sum");
  const winio = preflightWinioBuildInput({ root, source, output, moduleCache, environment: childEnvironment });
  const alternate = winio.deriveComposite(derived(main.mod, limaRoot)).mod;
  const licenses = Object.fromEntries(["LICENSE", "NOTICE"].map((name) => [name, read(join(cache.cacheModule, name))]));
  const legal = offer(data, data.patched, cache.zip, licenses);
  const manifest = {
    schema: 1, module: MODULE, version: VERSION, sourceRoot: source, outputRoot: output, inputRoot, limaRoot,
    moduleCache, cacheModule: cache.cacheModule, cacheZip: cache.cacheZip, modfile, sumfile,
    winioInput: winio.manifest, portableWinio: winio.portable,
    originalGoModSha256: digest(main.mod), originalGoSumSha256: digest(main.sum),
    alternateGoModSha256: digest(alternate), alternateGoSumSha256: digest(main.sum), portable: legal.portable,
  };
  return { root, data, main, cache, legal, alternate, manifest, childEnvironment: winio.childEnvironment, winio };
}
function result(state) {
  const { manifest, childEnvironment, legal } = state;
  return {
    rootModuleFlags: ["-mod=readonly", "-modfile=" + manifest.modfile], childEnvironment, manifest,
    limaRoot: manifest.limaRoot, portable: legal.portable,
    winio: state.winio, portableWinio: state.winio.portable,
    assertUnchanged() { verifyLimaBuildInput({ root: state.root, source: manifest.sourceRoot, manifest, environment: childEnvironment }); },
  };
}

export function prepareLimaWsl2BuildInput({ root, source, output, moduleCache, environment = process.env }) {
  // Policy and every source pin are checked before creating the output namespace.
  const state = current({ root, source, output, moduleCache, environment });
  const { manifest, data, cache, main, alternate, legal } = state;
  assert.ok(!existsSync(manifest.inputRoot), "Fresh Lima build namespace required");
  const legalDirectory = join(manifest.outputRoot, "licenses/lima-wsl2");
  assert.ok(!existsSync(legalDirectory), "Fresh Lima source offer required");
  if (existsSync(manifest.outputRoot)) directory(manifest.outputRoot);
  state.winio.create(); // both complete dependency preflights passed before any CREATE
  mkdirSync(manifest.limaRoot, { recursive: true, mode: 0o700 });
  for (const entry of data.patched) {
    // The original full cache projection already checked every before-byte.
    // Select reviewed replacement bytes before CREATE rather than rewriting
    // a copied original (whose cache attributes may also be read-only).
    const body = data.replacementBytes.get(entry.path) ?? read(inside(cache.cacheModule, entry.path));
    assert.equal(digest(body), entry.sha256);
    put(inside(manifest.limaRoot, entry.path), body);
  }
  projection(manifest.limaRoot, data.patched);
  put(manifest.modfile, alternate);
  put(manifest.sumfile, main.sum);
  for (const [path, body] of Object.entries(legal.payloads)) put(join(legalDirectory, path), body);
  const prepared = result(state);
  prepared.assertUnchanged();
  return prepared;
}

export function verifyLimaBuildInput({ root, source, manifest, environment = process.env }) {
  limaBuildEnvironment(environment); // reject before even following build metadata
  assert.equal(manifest?.schema, 1);
  assert.equal(resolve(source), manifest.sourceRoot, "Wrong actual main module for stored Lima input");
  const state = current({ root, source, output: manifest.outputRoot, moduleCache: manifest.moduleCache, environment });
  assert.deepEqual(manifest, state.manifest, "Lima build-input record changed");
  assert.ok(read(manifest.modfile).equals(state.alternate), "Alternate module changed");
  assert.ok(read(manifest.sumfile).equals(state.main.sum), "Alternate sums changed");
  projection(manifest.limaRoot, state.data.patched);
  verifyWinioBuildInput({ root, source, output: manifest.outputRoot, moduleCache: manifest.moduleCache, environment }, manifest.winioInput);
  verifyPortableLimaBuild(root, manifest.outputRoot, {
    portableLima: manifest.portable,
    buildScripts: Object.fromEntries(["scripts/build-backend.mjs", "scripts/lima-wsl2-build-input.mjs"].map((path) => [path.slice("scripts/".length), state.data.recipeHashes[path]])),
  });
  return result(state);
}

export function hasLimaBuildInput(root, build) {
  return Object.hasOwn(build, "portableLima") || Object.hasOwn(build, "limaWsl2") ||
    existsSync(resolve(root, "lima-wsl2.lock.json")) || existsSync(resolve(root, "scripts/lima-wsl2-build-input.mjs"));
}

export function verifyPortableLimaBuild(root, backend, build, { licensesDirectory = resolve(backend, "licenses") } = {}) {
  // Never inspect build.limaWsl2 paths here: source/modfile/cache may no longer exist.
  const data = profile(resolve(root));
  const location = resolve(licensesDirectory, "lima-wsl2");
  directory(location);
  assert.deepEqual(readdirSync(location).sort(), OFFER, "Incomplete or unexpected Lima source offer");
  const zip = read(join(location, "upstream.zip"));
  assert.equal(digest(zip), data.pin.upstreamZipSha256);
  const licenses = Object.fromEntries(["LICENSE", "NOTICE"].map((name) => {
    const body = read(join(location, name));
    assert.equal(digest(body), data.baseline.find((entry) => entry.path === name).sha256);
    return [name, body];
  }));
  const expected = offer(data, data.patched, zip, licenses);
  for (const [path, body] of Object.entries(expected.payloads)) assert.ok(read(join(location, path)).equals(body), "Lima portable input changed: " + path);
  assert.deepEqual(build.portableLima, expected.portable, "Lima portable provenance differs from reviewed inputs");
  for (const path of ["scripts/build-backend.mjs", "scripts/lima-wsl2-build-input.mjs"])
    assert.equal(build.buildScripts?.[path.slice("scripts/".length)], data.recipeHashes[path], "Unbound Lima build recipe");
  return { directory: location, manifest: expected.manifest, portable: expected.portable, module: MODULE, version: VERSION };
}
