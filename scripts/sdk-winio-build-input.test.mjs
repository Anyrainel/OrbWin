import assert from "node:assert/strict";
import test from "node:test";
import {
  readFileSync,
  writeFileSync,
  mkdtempSync,
  mkdirSync,
  cpSync,
  realpathSync,
  existsSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { resolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import {
  sdkFixtureLock,
  sdkFixtureRecord,
  sdkFixtureContext,
  installSdkFixture,
} from "./sdk-winio-test-fixtures.mjs";
import {
  validateSdkWinioLock,
  composeSdkWinioModfile,
  sdkWinioEnvironment,
  validateSdkWinioMetadata,
  prepareSdkWinioInput,
  verifySdkWinioProvenance,
  classifySdkWinioNotice,
  settleSdkWinio,
  sdkWinioSourceFiles,
} from "./sdk-winio-build-input.mjs";
const ROOT = fileURLToPath(new URL("../", import.meta.url));
const clone = (v) => structuredClone(v);
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "orbwin-sdk062-source-")),
    exact = realpathSync(root);
  t.after(() => {
    assert.equal(realpathSync(root), exact);
    rmSync(exact, { recursive: true });
  });
  const output = resolve(root, "output");
  mkdirSync(output);
  const installed = installSdkFixture(root, output);
  return { root, output, ...installed };
}
test("fixed062 profile preserves109/111 and rejects064 or source drift", () => {
  const l = sdkFixtureLock();
  validateSdkWinioLock(l);
  for (const change of [
    (x) => (x.version = "v0.6.4"),
    (x) => (x.main = "github.com/rancher-sandbox/rancher-desktop-daemon"),
    (x) => (x.patched[0].sha256 = "1".repeat(64)),
    (x) => (x.patchSHA256 = "2".repeat(64)),
  ]) {
    const x = clone(l);
    change(x);
    assert.throws(() => validateSdkWinioLock(x));
  }
});
test("single readonly modfile preserves SDK graph and refuses competing overrides", () => {
  const mod = readFileSync(resolve(ROOT, "backend-tools/session/go.mod")),
    sum = readFileSync(resolve(ROOT, "backend-tools/session/go.sum"));
  const r = composeSdkWinioModfile(mod, sum, "F:/source-fixture/sdk062");
  assert.ok(r.mod.subarray(0, mod.length).equals(mod));
  assert.ok(r.sum.equals(sum));
  for (const suffix of [
    "replace x => y\n",
    "exclude x v1.0.0\n",
    "toolchain go1.99\n",
  ])
    assert.throws(() =>
      composeSdkWinioModfile(
        Buffer.concat([mod, Buffer.from(suffix)]),
        sum,
        "F:/fixture",
      ),
    );
  for (const env of [
    { GOFLAGS: "-modfile=foreign.mod" },
    { GOWORK: "foreign.work" },
    { GOOS: "linux" },
    { CGO_ENABLED: "1" },
    { GOAMD64: "v3" },
    { GOFLAGS: "", goflags: "" },
  ])
    assert.throws(() => sdkWinioEnvironment(env));
  assert.equal(sdkWinioEnvironment({}).CGO_ENABLED, "0");
});
test("three role metadata cannot mix target context, version, rows or replacement", () => {
  const l = sdkFixtureLock();
  for (const role of ["sdk", "helper", "worker"]) {
    const ctx = sdkFixtureContext(
        "F:/fixture/source",
        "F:/fixture/module",
        role,
      ),
      r = sdkFixtureRecord(ctx);
    assert.ok(
      validateSdkWinioMetadata(r, l, ctx).some(
        (f) => f.path === "file_original_close_state.go",
      ),
    );
    for (const change of [
      (x) => (x.context.role = role === "sdk" ? "helper" : "sdk"),
      (x) => (x.module.Version = "v0.6.4"),
      (x) => (x.packages[0].Module.Replace.Dir = "F:/other"),
      (x) => (x.context.tags = ["with_distro"]),
      (x) => x.packages[0].GoFiles.push("../outside.go"),
      (x) =>
        (x.packages.find(
          (p) => p.ImportPath === "github.com/Microsoft/go-winio",
        ).CompiledGoFiles = []),
    ]) {
      const x = clone(r);
      change(x);
      assert.throws(() => validateSdkWinioMetadata(x, l, ctx));
    }
  }
});
test("changed before-after capture refuses without compiler or child", () => {
  const r = {
    record: sdkFixtureRecord(sdkFixtureContext("F:/source", "F:/module")),
    files: [],
  };
  let checks = 0;
  const input = {
    verify() {
      checks++;
    },
  };
  assert.deepEqual(settleSdkWinio(input, r, clone(r)).before, r);
  const rawChanged = clone(r);
  rawChanged.record.rawPackagesSHA256 = "5".repeat(64);
  settleSdkWinio(input, r, rawChanged);
  const x = clone(r);
  x.record.context.cgo = "1";
  assert.throws(() => settleSdkWinio(input, r, x));
  assert.equal(checks, 2);
});
test("portable variant and per-artifact notice allow only exact062 source", (t) => {
  const f = fixture(t),
    ctx = sdkFixtureContext(
      resolve(f.root, "backend-tools/session"),
      resolve(f.root, "module"),
    ),
    r = sdkFixtureRecord(ctx),
    p = {
      portable: f.offer,
      sourceFiles: sdkWinioSourceFiles(f.output),
      capture: {
        before: { record: r, files: validateSdkWinioMetadata(r, f.lock, ctx) },
        after: null,
      },
    };
  p.capture.after = clone(p.capture.before);
  verifySdkWinioProvenance(f.root, f.output, p, "sdk");
  assert.throws(() => verifySdkWinioProvenance(f.root, f.output, p, "helper"));
  const dep = {
    Path: "github.com/Microsoft/go-winio",
    Version: "v0.6.2",
    Replace: { Path: ctx.moduleRoot },
  };
  assert.equal(
    classifySdkWinioNotice(
      dep,
      "orbwin-session.exe",
      p,
      "orbwin.local/session",
    ),
    "orbwin-sdk-winio062-close",
  );
  assert.equal(classifySdkWinioNotice(dep, "rdd.exe", p, "other-main"), null);
  assert.throws(() =>
    classifySdkWinioNotice(
      { ...dep, Version: "v0.6.4" },
      "orbwin-session.exe",
      p,
      "orbwin.local/session",
    ),
  );
  assert.throws(() =>
    classifySdkWinioNotice(dep, "orbwin-session.exe", p, "other-main"),
  );
  assert.throws(() =>
    classifySdkWinioNotice(
      dep,
      "lan/orbwin-lan-helper.exe",
      p,
      "orbwin.local/session",
    ),
  );
  const bad = clone(p);
  bad.portable.variant = "orbwin-winio-custody";
  assert.throws(() => verifySdkWinioProvenance(f.root, f.output, bad, "sdk"));
  writeFileSync(
    resolve(f.output, "sdk-winio-source/upstream.zip"),
    "wrong source ZIP",
  );
  assert.throws(() => verifySdkWinioProvenance(f.root, f.output, p, "sdk"));
});

test("actual source adapter reconstructs only062 and fails before write on override", (t) => {
  const f = fixture(t),
    cache = resolve(f.root, "source-cache"),
    source = resolve(f.root, "backend-tools/session");
  cpSync(
    resolve(ROOT, "scripts/test-data/sdk-winio-input/original"),
    resolve(cache, "github.com/!microsoft/go-winio@v0.6.2"),
    { recursive: true },
  );
  const zip = resolve(
    cache,
    "cache/download/github.com/!microsoft/go-winio/@v/v0.6.2.zip",
  );
  mkdirSync(dirname(zip), { recursive: true });
  writeFileSync(zip, readFileSync(resolve(ROOT, "scripts/test-data/sdk-winio-input/upstream.zip")));
  const patch = resolve(
    f.root,
    "patches/go-winio/0002-sdk062-original-file-close.patch",
  );
  mkdirSync(dirname(patch), { recursive: true });
  writeFileSync(
    patch,
    readFileSync(
      resolve(ROOT, "patches/go-winio/0002-sdk062-original-file-close.patch"),
    ),
  );
  const output = resolve(f.root, "prepared");
  mkdirSync(output);
  assert.throws(() =>
    prepareSdkWinioInput({
      root: f.root,
      source,
      output,
      moduleCache: cache,
      environment: { GOFLAGS: "-tags=foreign" },
    }),
  );
  assert.equal(existsSync(resolve(output, ".sdk-winio-build")), false);
  const input = prepareSdkWinioInput({
    root: f.root,
    source,
    output,
    moduleCache: cache,
    environment: {},
  });
  assert.ok(
    input.flags[0] === "-mod=readonly" &&
      input.flags[1].startsWith("-modfile="),
  );
  input.verify();
  assert.ok(
    readFileSync(resolve(input.moduleRoot, "file_original_close_state.go"))
      .length > 0,
  );
  const mod = readFileSync(resolve(source, "go.mod"));
  assert.ok(input.mod.subarray(0, mod.length).equals(mod));
  writeFileSync(resolve(input.moduleRoot, "file.go"), "modified source");
  assert.throws(() => input.verify());
});
