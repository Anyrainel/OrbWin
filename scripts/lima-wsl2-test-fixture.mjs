import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, isAbsolute, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import {
  hasLimaBuildInput,
  limaBuildEnvironment,
  prepareLimaWsl2BuildInput,
  verifyLimaBuildInput,
  verifyPortableLimaBuild,
} from "./lima-wsl2-build-input.mjs";

import { attachWinioFixture } from "./winio-test-fixtures.mjs";
const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const pinned = JSON.parse(readFileSync(join(scriptDirectory, "../lima-wsl2.lock.json")));
const hash = (body) => createHash("sha256").update(body).digest("hex");
const json = (value) => Buffer.from(JSON.stringify(value, null, 2) + "\n");
const recipeNames = [
  "lima-wsl2-build-input.mjs",
  "build-backend.mjs",
  "build-identity.mjs",
  "collect-notices.mjs",
];
const vm = "pkg/driver/wsl2/vm_windows.go";
const helper = "pkg/driver/wsl2/import_args_windows.go";
const helperTest = "pkg/driver/wsl2/import_args_windows_test.go";
const defaults = "pkg/limayaml/defaults.go";
const hostHome = "pkg/limayaml/host_home.go";
const hostHomeTest = "pkg/limayaml/host_home_test.go";
function put(path, body) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, body);
}
function removeOwned(f, path) {
  const actual = realpathSync(path);
  const suffix = relative(f.root, actual);
  assert.ok(suffix && !isAbsolute(suffix) && suffix !== ".." && !suffix.startsWith(`..${sep}`));
  rmSync(actual, { recursive: true, force: true });
}
export function fixture(t, options = {}) {
  const root = options.root ?? realpathSync(mkdtempSync(join(tmpdir(), "orbwin-lima-input-")));
  if (!options.root)
    t.after(() => {
      assert.equal(realpathSync(root), root);
      assert.match(basename(root), /^orbwin-lima-input-/);
      assert.equal(realpathSync(dirname(root)), realpathSync(tmpdir()));
      rmSync(root, { recursive: true });
    });
  const source = join(root, "source/rdd"),
    output = options.output ?? join(root, "artifacts/fresh");
  const moduleCache = join(root, "module-cache");
  const cacheModule = join(moduleCache, `${pinned.module}@${pinned.version}`);
  const cacheZip = join(
    moduleCache,
    "cache/download",
    pinned.module,
    "@v",
    `${pinned.version}.zip`,
  );
  const mod = Buffer.from(
    `module github.com/rancher-sandbox/rancher-desktop-daemon\n\ngo 1.27.0\n\nrequire ${pinned.module} ${pinned.version}\nrequire github.com/Microsoft/go-winio v0.6.4\nreplace example.invalid/kept => ./pkg/kept\n`,
  );
  const sum = Buffer.from(
    `${pinned.module} ${pinned.version} ${pinned.upstreamModuleSum}\n${pinned.module} ${pinned.version}/go.mod ${pinned.upstreamGoModSum}\n`,
  );
  put(join(source, "go.mod"), mod);
  put(join(source, "go.sum"), sum);
  mkdirSync(join(root, "artifacts"), { recursive: true });
  attachWinioFixture(root, source, moduleCache, mod, sum);
  // This trusted synthetic lock tests filesystem/hash algorithms only. Its small
  // baseline and ZIP bytes are NOT evidence of an upstream h1 verification.
  const baselineBodies = new Map([
    ["LICENSE", Buffer.from("synthetic Apache license\n")],
    ["NOTICE", Buffer.from("synthetic Lima notice\n")],
    ["go.mod", Buffer.from(`module ${pinned.module}\n\ngo 1.27.0\n`)],
    ["go.sum", Buffer.from("synthetic transitive dependency pins\n")],
    ["docs/unchanged.txt", Buffer.from("every baseline file must be checked\n")],
    [vm, Buffer.from("package wsl2\n// original producer\n")],
    [defaults, Buffer.from("package limayaml\n// original eager home\n")],
  ]);
  for (const [name, body] of baselineBodies) put(join(cacheModule, name), body);
  const zip = Buffer.from("trusted fixture ZIP bytes; not a real module archive\n");
  put(cacheZip, zip);
  const replacementBodies = new Map([
    [vm, Buffer.from("package wsl2\n// actual producer calls wslImportArgs\n")],
    [helper, Buffer.from("package wsl2\n// pure reviewed import argv\n")],
    [helperTest, Buffer.from("package wsl2\n// inert file-list regression\n")],
    [defaults, Buffer.from("package limayaml\n// Windows deferred home producer\n")],
    [hostHome, Buffer.from("package limayaml\n// std-only home DATA adapter\n")],
    [hostHomeTest, Buffer.from("package limayaml\n// inert home projection regression\n")],
  ]);
  // Exact new replacement roles from the reviewed selected-lifecycle input.
  // Bodies remain invented DATA; this fixture does not compile Lima or prove its source.
  for (const name of ["pkg/driver/wsl2/wsl_driver_windows.go", "pkg/hostagent/hostagent.go", "pkg/hostagent/requirements.go"])
    baselineBodies.set(name, Buffer.from("package fixture\n// original lifecycle DATA " + name + "\n"));
  for (const name of ["pkg/driver/wsl2/configured_lifecycle_windows_test.go", "pkg/driver/wsl2/worker_lifecycle.go", "pkg/driver/wsl2/worker_lifecycle_test.go", "pkg/driver/wsl2/wsl_driver_windows.go", "pkg/hostagent/hostagent.go", "pkg/hostagent/lifecycle_state.go", "pkg/hostagent/lifecycle_state_test.go", "pkg/hostagent/requirements.go"])
    replacementBodies.set(name, Buffer.from("package fixture\n// selected lifecycle DATA " + name + "\n"));
  for (const [name, body] of baselineBodies) put(join(cacheModule, name), body);
  const profile = structuredClone(pinned);
  profile.rddGoModSha256 = hash(mod);
  profile.rddGoSumSha256 = hash(sum);
  profile.upstreamZipSha256 = hash(zip);
  profile.upstreamZipBytes = zip.length;
  profile.baseline = [...baselineBodies].map(([path, body]) => ({
    path,
    sha256: hash(body),
    bytes: body.length,
  }));
  profile.baseline.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  profile.replacements = profile.replacements.map((entry) => {
    const body = replacementBodies.get(entry.path);
    assert.ok(body, "production replacement identity changed; review this fixture");
    put(join(root, entry.replacement), body);
    return {
      ...entry,
      before: baselineBodies.has(entry.path) ? hash(baselineBodies.get(entry.path)) : null,
      sha256: hash(body),
      bytes: body.length,
    };
  });
  const patched = new Map(baselineBodies);
  for (const [name, body] of replacementBodies) patched.set(name, body);
  const projection = [...patched].map(([path, body]) => ({
    path,
    sha256: hash(body),
    bytes: body.length,
  }));
  projection.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  profile.patchedSourceFiles = projection.length;
  profile.patchedProjectionSha256 = hash(json(projection));
  const patch = Buffer.from("trusted synthetic six-file patch offer\n");
  put(join(root, profile.patch.path), patch);
  profile.patch.sha256 = hash(patch);
  const writeLock = () => put(join(root, "lima-wsl2.lock.json"), json(profile));
  writeLock();
  const environment = Object.freeze({ PATH: "retained fixture path", GOFLAGS: "" });
  const f = {
    root,
    source,
    output,
    moduleCache,
    cacheModule,
    cacheZip,
    mod,
    sum,
    profile,
    writeLock,
    environment,
  };
  f.prepare = (overrides = {}) =>
    prepareLimaWsl2BuildInput({ root, source, output, moduleCache, environment, ...overrides });
  f.build = (prepared) => ({
    limaWsl2: prepared.manifest,
    portableLima: prepared.portable,
    portableWinio: prepared.portableWinio,
    buildScripts: Object.fromEntries(
      ["lima-wsl2-build-input.mjs", "build-backend.mjs"].map((name) => [
        name,
        hash(readFileSync(join(root, "scripts", name))),
      ]),
    ),
  });
  return f;
}
