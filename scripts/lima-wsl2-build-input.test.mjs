import test from "node:test";
import assert from "node:assert/strict";
import {cpSync,existsSync,mkdirSync,readFileSync,readdirSync,realpathSync,rmSync,writeFileSync}from"node:fs";
import {basename,dirname,isAbsolute,join,relative,sep}from"node:path";
import {fixture,fixture as makeFixture}from"./lima-wsl2-test-fixture.mjs";
import {hasLimaBuildInput,limaBuildEnvironment,verifyLimaBuildInput,verifyPortableLimaBuild}from"./lima-wsl2-build-input.mjs";
import{fileURLToPath}from"node:url";import{createHash}from"node:crypto";
const scriptDirectory=dirname(fileURLToPath(import.meta.url)),pinned=JSON.parse(readFileSync(join(scriptDirectory,"../lima-wsl2.lock.json"))),hash=b=>createHash("sha256").update(b).digest("hex");
const recipeNames=["lima-wsl2-build-input.mjs","build-backend.mjs","build-identity.mjs","collect-notices.mjs"],vm="pkg/driver/wsl2/vm_windows.go",helper="pkg/driver/wsl2/import_args_windows.go";
function put(p,b){mkdirSync(dirname(p),{recursive:true});writeFileSync(p,b);}
function removeOwned(f,p){const a=realpathSync(p),r=relative(f.root,a);assert.ok(r&&!isAbsolute(r)&&r!==".."&&!r.startsWith(".."+sep));rmSync(a,{recursive:true,force:true});}
test("Lima child policy rejects case variants of ambient GOFLAGS and preserves its caller", () => {
  for (const key of ["GOFLAGS", "goflags", "GoFlAgS"]) {
    const environment = Object.freeze({ [key]: "-modfile=foreign.mod", PATH: "keep" });
    assert.throws(() => limaBuildEnvironment(environment), /GOFLAGS/i);
    assert.equal(environment[key], "-modfile=foreign.mod");
  }
  const environment = Object.freeze({ goflags: "", GOENV: "prior-env-file", Gowork: "prior-workspace", PATH: "keep", KEEP: "original" });
  const child = limaBuildEnvironment(environment);
  assert.equal(child.PATH, "keep");
  assert.equal(child.KEEP, "original");
  for (const [key, value] of Object.entries({ GOFLAGS: "", GOENV: "off", GOWORK: "off", GOTOOLCHAIN: "local", GOPROXY: "off", GOSUMDB: "off" })) {
    assert.equal(child[key], value);
    assert.equal(Object.keys(child).filter((name) => name.toUpperCase() === key).length, 1);
  }
  assert.equal(environment.GOENV, "prior-env-file");
  assert.equal(environment.Gowork, "prior-workspace");
});

test("ambient flags refuse before any Lima output, even with nonexistent source inputs", (t) => {
  const f = fixture(t);
  assert.throws(() => f.prepare({ source: join(f.root, "absent"), environment: { GoFlAgS: "-overlay=foreign.json" } }), /GOFLAGS/i);
  assert.equal(existsSync(f.output), false);
});

test("verified local replacement preserves original module files, cache and existing replacements", (t) => {
  const f = fixture(t);
  const cacheBefore = readFileSync(join(f.cacheModule, vm));
  const prepared = f.prepare();
  assert.deepEqual(readFileSync(join(f.source, "go.mod")), f.mod);
  assert.deepEqual(readFileSync(join(f.source, "go.sum")), f.sum);
  assert.deepEqual(readFileSync(join(f.cacheModule, vm)), cacheBefore);
  assert.equal(prepared.manifest.originalGoModSha256, hash(f.mod));
  assert.equal(prepared.manifest.originalGoSumSha256, hash(f.sum));
  // Synthetic original10 plus nine newly added source roles = exact19 files.
  assert.equal(f.profile.replacements.length, 14);
  assert.equal(prepared.portable.patchedSourceFiles, f.profile.baseline.length + 9);
  assert.equal(prepared.portable.patchedProjectionSha256, f.profile.patchedProjectionSha256);
  assert.deepEqual(readFileSync(prepared.manifest.sumfile), f.sum);
  const derived = readFileSync(prepared.manifest.modfile, "utf8");
  assert.ok(derived.includes(f.mod.toString()));
  assert.ok(derived.includes(`replace ${pinned.module} ${pinned.version} =>`));
  assert.equal(derived.split(`replace ${pinned.module} ${pinned.version} =>`).length, 2);
  assert.ok(derived.includes(prepared.manifest.limaRoot.replaceAll("\\", "/")));
  assert.ok(prepared.rootModuleFlags.includes("-mod=readonly"));
  assert.ok(prepared.rootModuleFlags.includes(`-modfile=${prepared.manifest.modfile}`));
  assert.equal(prepared.childEnvironment.GOENV, "off");
  assert.equal(prepared.childEnvironment.PATH, f.environment.PATH);
  prepared.assertUnchanged();
  const checked = verifyLimaBuildInput({ root: f.root, source: f.source, manifest: prepared.manifest, environment: f.environment });
  assert.deepEqual(checked.rootModuleFlags, prepared.rootModuleFlags);
  assert.throws(() => f.prepare(), /exist|fresh/i);
  assert.deepEqual(readFileSync(join(f.cacheModule, vm)), cacheBefore);
});

test("nested or unrelated main-module identity cannot borrow the root-rdd recipe", (t) => {
  for (const module of ["orbwin.local/session", "github.com/rancher-sandbox/rancher-desktop-daemon/src/rdd-guest"]) {
    const f = fixture(t);
    const body = Buffer.from(`module ${module}\n\ngo 1.27.0\nrequire ${pinned.module} ${pinned.version}\n`);
    put(join(f.source, "go.mod"), body);
    // Hash agreement with the trusted fixture lock is insufficient: the API
    // must independently enforce which actual main module can use this route.
    f.profile.rddGoModSha256 = hash(body);
    f.writeLock();
    assert.throws(() => f.prepare());
    assert.equal(existsSync(f.output), false);
  }
});

test("original source, any cached baseline file, ZIP and patch inputs refuse before output on tamper", async (t) => {
  for (const fault of ["root-mod", "root-sum", "cache-unchanged", "cache-original-vm", "zip", "patch", "replacement", "recipe"]) {
    await t.test(fault, () => {
      const f = fixture(t);
      const target = {
        "root-mod": join(f.source, "go.mod"),
        "root-sum": join(f.source, "go.sum"),
        "cache-unchanged": join(f.cacheModule, "docs/unchanged.txt"),
        "cache-original-vm": join(f.cacheModule, vm),
        zip: f.cacheZip,
        patch: join(f.root, f.profile.patch.path),
        replacement: join(f.root, f.profile.replacements[0].replacement),
        recipe: join(f.root, "scripts/lima-wsl2-build-input.mjs"),
      }[fault];
      // A changed recipe is measured as the current recipe at PREPARE, so the
      // meaningful refusal for that case is its POST-PREPARE mutation below.
      if (fault === "recipe") {
        const prepared = f.prepare();
        put(target, "changed recipe after preparation\n");
        assert.throws(() => prepared.assertUnchanged());
        return;
      }
      put(target, "changed reviewed input\n");
      assert.throws(() => f.prepare());
      assert.equal(existsSync(f.output), false);
    });
  }
});

test("prepared outputs, alternate module files and original inputs remain checked after creation", async (t) => {
  for (const fault of ["copied-source", "modfile", "sumfile", "source", "cache", "patch", "lock", "identity-recipe", "collector-recipe"]) {
    await t.test(fault, () => {
      const f = fixture(t), prepared = f.prepare();
      const target = {
        "copied-source": join(prepared.manifest.limaRoot, helper),
        modfile: prepared.manifest.modfile,
        sumfile: prepared.manifest.sumfile,
        source: join(f.source, "go.mod"),
        cache: join(f.cacheModule, "docs/unchanged.txt"),
        patch: join(f.root, f.profile.patch.path),
        lock: join(f.root, "lima-wsl2.lock.json"),
        "identity-recipe": join(f.root, "scripts/build-identity.mjs"),
        "collector-recipe": join(f.root, "scripts/collect-notices.mjs"),
      }[fault];
      put(target, "post-prepare tamper\n");
      assert.throws(() => prepared.assertUnchanged());
      assert.throws(() => verifyLimaBuildInput({ root: f.root, source: f.source, manifest: prepared.manifest, environment: f.environment }));
    });
  }
});

test("portable source offer verifies after relocation without original cache/source/build inputs", (t) => {
  const f = fixture(t), prepared = f.prepare(), build = f.build(prepared);
  assert.equal(hasLimaBuildInput(f.root, build), true);
  const offer = verifyPortableLimaBuild(f.root, f.output, build);
  assert.equal(offer.module, pinned.module);
  assert.equal(offer.version, pinned.version);
  assert.deepEqual(offer.portable, prepared.portable);
  assert.equal(Object.keys(prepared.portable.offer.files).length, 6);
  const backend = join(f.root, "relocated/runtime/backend");
  const licenses = join(f.root, "relocated/runtime/licenses");
  mkdirSync(backend, { recursive: true });
  mkdirSync(licenses, { recursive: true });
  cpSync(offer.directory, join(licenses, "lima-wsl2"), { recursive: true });
  removeOwned(f, f.source);
  removeOwned(f, f.moduleCache);
  removeOwned(f, prepared.manifest.inputRoot);
  removeOwned(f, join(f.output, "licenses"));
  assert.equal(existsSync(prepared.manifest.limaRoot), false);
  assert.equal(existsSync(prepared.manifest.modfile), false);
  const relocated = verifyPortableLimaBuild(f.root, backend, build, { licensesDirectory: licenses });
  assert.equal(realpathSync(relocated.directory), realpathSync(join(licenses, "lima-wsl2")));
  assert.equal(relocated.module, pinned.module);
  assert.deepEqual(relocated.portable, prepared.portable);
  assert.throws(() => verifyLimaBuildInput({ root: f.root, source: f.source, manifest: prepared.manifest, environment: f.environment }));
});

test("each portable offer file and recorded recipe remains bound after packaging", async (t) => {
  const probe = fixture(t), initial = probe.prepare();
  const names = Object.keys(initial.portable.offer.files);
  assert.equal(names.length, 6);
  for (const name of names) {
    await t.test(name, () => {
      const f = fixture(t), prepared = f.prepare(), build = f.build(prepared);
      const offer = verifyPortableLimaBuild(f.root, f.output, build);
      assert.equal(basename(name), name);
      put(join(offer.directory, name), "changed portable offer\n");
      assert.throws(() => verifyPortableLimaBuild(f.root, f.output, build));
    });
  }
  for (const recipe of recipeNames) {
    await t.test(recipe, () => {
      const f = fixture(t), prepared = f.prepare(), build = f.build(prepared);
      put(join(f.root, "scripts", recipe), "changed current recipe\n");
      assert.throws(() => verifyPortableLimaBuild(f.root, f.output, build));
    });
  }
});

test("root pins and manifest presence cannot silently downgrade patched product verification", (t) => {
  const f = fixture(t), prepared = f.prepare(), build = f.build(prepared);
  assert.equal(hasLimaBuildInput(f.root, {}), true);
  assert.throws(() => verifyPortableLimaBuild(f.root, f.output, {}));
  for (const fault of ["module", "version", "zip", "recipe-binding"]) {
    const changed = structuredClone(build);
    if (fault === "module") changed.portableLima.module = "foreign.example/lima";
    if (fault === "version") changed.portableLima.version = "v0.0.0";
    if (fault === "zip") changed.portableLima.upstreamZipSha256 = "0".repeat(64);
    if (fault === "recipe-binding") changed.buildScripts["lima-wsl2-build-input.mjs"] = "0".repeat(64);
    assert.throws(() => verifyPortableLimaBuild(f.root, f.output, changed));
  }
  removeOwned(f, join(f.root, "lima-wsl2.lock.json"));
  assert.equal(hasLimaBuildInput(f.root, build), true);
  assert.equal(hasLimaBuildInput(f.root, {}), true);
  removeOwned(f, join(f.root, "scripts/lima-wsl2-build-input.mjs"));
  assert.equal(hasLimaBuildInput(f.root, {}), false);
  assert.throws(() => verifyPortableLimaBuild(f.root, f.output, build));
});
