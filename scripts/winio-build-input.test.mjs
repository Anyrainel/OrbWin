import assert from "node:assert/strict";
import test from "node:test";
import {
  winioIdentity,
  winioDigestData,
  winioRowsFromBytesData,
  winioProjectionDigestData,
  validateWinioProfileData,
  reconstructWinioBytesData,
  composeWinioLimaModfileData,
  validateEffectiveWinioClosureData,
  requireUnchangedEffectiveWinioClosureData,
  winioPortableOfferData,
  prepareWinioBuildInput,
  projectWinioGoListData,
  reconstructWinioPatchData,
} from "./winio-build-input.mjs";

// All byte buffers and Go-list records below are invented DATA in memory.
// No actual module/source cache, ZIP parser, compiler, process or filesystem.
const recipeNames = [
  "scripts/winio-build-input.mjs",
  "scripts/winio-build-input-fs.mjs",
  "scripts/lima-wsl2-build-input.mjs",
  "scripts/build-backend.mjs",
  "scripts/build-identity.mjs",
  "scripts/collect-notices.mjs",
];
const h = winioDigestData,
  clone = (v) => JSON.parse(JSON.stringify(v));
function fixture() {
  const original = new Map(
    Object.entries({
      LICENSE: "synthetic MIT notice\n",
      "go.mod": `module ${winioIdentity.module}\n`,
      "go.sum": "synthetic pinned sums\n",
      "file.go": "original file DATA\n",
      "pipe.go": "unchanged pipe DATA\n",
      "original_test.go": "unchanged upstream test DATA\n",
    }).map(([n, b]) => [n, Buffer.from(b)]),
  );
  const replacement = new Map(
    [
      "file.go",
      "file_original_close_state.go",
      "file_original_iocp_windows.go",
      "pipe_dial_state.go",
      "pipe_original_dial_windows.go",
    ].map((n) => [n, Buffer.from(`reviewed synthetic DATA ${n}\n`)]),
  );
  const final = new Map([...original, ...replacement]);
  const mod = Buffer.from(
      `module ${winioIdentity.mainModule}\n\ngo 1.27.0\n\nrequire ${winioIdentity.module} ${winioIdentity.version}\nreplace example.invalid/kept => ./pkg/kept\n`,
    ),
    sum = Buffer.from("synthetic main sums\n"),
    zip = Buffer.from("synthetic ZIP fixture; not valid real h1 evidence\n"),
    patch = Buffer.from("synthetic reviewed patch\n");
  const baseline = winioRowsFromBytesData(original),
    patched = winioRowsFromBytesData(final);
  const profile = {
    schema: 1,
    ...winioIdentity,
    originalModSHA256: h(mod),
    originalSumSHA256: h(sum),
    upstreamZipSHA256: h(zip),
    upstreamModuleSum: "h1:kJF2pamtYCWXf0xAT8sAkwKC0pMl2HYBrRPHAqyUc+g=",
    upstreamGoModSum: "h1:1xl5JXG4ze/m74GH6Dc4Mwxfc3VL5siRVQjIAlHH07U=",
    patchSHA256: h(patch),
    licenseSHA256: h(original.get("LICENSE")),
    baseline,
    patched,
    baselineProjectionSHA256: winioProjectionDigestData(baseline),
    patchedProjectionSHA256: winioProjectionDigestData(patched),
    replacements: [...replacement].map(([path, b]) => ({
      path,
      before: original.has(path) ? h(original.get(path)) : null,
      sha256: h(b),
      bytes: b.length,
    })),
    recipeHashes: Object.fromEntries(recipeNames.map((n) => [n, h(Buffer.from(n))])),
  };
  const limaDirectory = "C:/fixture/build/lima",
    winioDirectory = "C:/fixture/build/winio";
  const alternate = Buffer.concat([
    mod,
    Buffer.from(
      `\n// OrbWin: reviewed per-version WSL2 import and Windows host-home adaptation.\nreplace github.com/lima-vm/lima/v2 v2.3.0-beta.1.0.20261005214905-0ae2df64800a => ${JSON.stringify(limaDirectory)}\n`,
    ),
  ]);
  const goFiles = [...final.keys()].filter((n) => n.endsWith(".go") && !n.endsWith("_test.go"));
  const selected = {
    Path: winioIdentity.module,
    Version: winioIdentity.version,
    Dir: winioDirectory,
    Replace: { Path: winioDirectory, Dir: winioDirectory },
  };
  const composite = composeWinioLimaModfileData({
    profile,
    originalMod: mod,
    originalSum: sum,
    limaAlternate: alternate,
    limaDirectory,
    winioDirectory,
  });
  const effective = {
    artifact: "rdd.exe",
    main: winioIdentity.mainModule,
    goos: "windows",
    goarch: "amd64",
    goamd64: "v1",
    cgo: "1",
    tags: ["sqlite_omit_load_extension", "with_distro"],
    package: "./cmd/rdd",
    modMode: "readonly",
    modfile: "C:/fixture/build/lima-build-input/rdd-lima.mod",
    modSHA256: h(composite.mod),
    sumSHA256: h(sum),
    sourceRoot: "C:/fixture/source/rdd",
    winioRoot: winioDirectory,
    selectedModule: clone(selected),
    packages: [
      {
        ImportPath: winioIdentity.module,
        Dir: winioDirectory,
        Module: clone(selected),
        GoFiles: goFiles,
        CgoFiles: [],
        CompiledGoFiles: goFiles.map((n) => winioDirectory + "/" + n),
        EmbedFiles: [],
      },
    ],
  };
  const expected = {
    modfile: effective.modfile,
    mod: composite.mod,
    sum,
    sourceRoot: effective.sourceRoot,
    winioRoot: effective.winioRoot,
    cgo: effective.cgo,
    tags: [...effective.tags],
  };
  return {
    original,
    replacement,
    final,
    mod,
    sum,
    zip,
    patch,
    profile,
    alternate,
    limaDirectory,
    winioDirectory,
    effective,
    expected,
  };
}
function compose(f, overrides = {}) {
  return composeWinioLimaModfileData({
    profile: f.profile,
    originalMod: f.mod,
    originalSum: f.sum,
    limaAlternate: f.alternate,
    limaDirectory: f.limaDirectory,
    winioDirectory: f.winioDirectory,
    ...overrides,
  });
}

test("production gateway rejects unsupported arguments before filesystem work", () => {
  let reads = 0;
  const options = new Proxy(
    {},
    {
      get() {
        reads++;
        throw Error("read");
      },
    },
  );
  assert.throws(() => prepareWinioBuildInput(options), /closed record/);
  assert.equal(reads, 0);
});
test("complete source reconstruction preserves original MIT/tests/module and input backing", () => {
  const f = fixture(),
    before = h(f.original.get("file.go")),
    r = reconstructWinioBytesData(f.profile, f.original, f.replacement);
  assert.equal(
    winioProjectionDigestData(winioRowsFromBytesData(r)),
    f.profile.patchedProjectionSHA256,
  );
  for (const n of ["LICENSE", "original_test.go", "go.mod", "go.sum"])
    assert.deepEqual(r.get(n), f.original.get(n));
  r.get("file.go").fill(0);
  assert.equal(h(f.original.get("file.go")), before);
  assert.notEqual(h(f.replacement.get("file.go")), h(r.get("file.go")));
});
test("unknown source bytes, extras, omissions and mismatched before pins refuse", () => {
  for (const mutation of [
    (f) => f.original.set("stray.go", Buffer.from("stray")),
    (f) => f.original.delete("pipe.go"),
    (f) => f.original.get("pipe.go").fill(0),
    (f) => f.replacement.set("stray.go", Buffer.from("stray")),
    (f) => f.replacement.delete("file.go"),
    (f) => (f.profile.replacements[0].before = "0".repeat(64)),
  ]) {
    const f = fixture();
    mutation(f);
    assert.throws(() => reconstructWinioBytesData(f.profile, f.original, f.replacement));
  }
});
test("projection case aliases, traversal, reserved names and undeclared fields refuse", () => {
  for (const path of ["../x", "a/../../x", "C:/x", "a\\x", "a:ads", "CON.go", "a/", "a/./x"]) {
    assert.throws(() => winioRowsFromBytesData(new Map([[path, Buffer.from("x")]])));
  }
  assert.throws(
    () =>
      winioRowsFromBytesData(
        new Map([
          ["File.go", Buffer.from("x")],
          ["file.go", Buffer.from("y")],
        ]),
      ),
    /collision/,
  );
  const f = fixture();
  f.profile.extra = true;
  assert.throws(() => validateWinioProfileData(f.profile), /closed/);
});
test("changed upstream test or license cannot be laundered through final projection", () => {
  for (const name of ["LICENSE", "original_test.go", "go.mod"]) {
    const f = fixture(),
      body = Buffer.from("changed");
    f.profile.patched = f.profile.patched.map((r) =>
      r.path === name ? { path: name, sha256: h(body), bytes: body.length } : r,
    );
    f.profile.patchedProjectionSHA256 = winioProjectionDigestData(f.profile.patched);
    assert.throws(() => validateWinioProfileData(f.profile), /preservation/);
  }
});
test("single composed modfile retains exact Lima and original relative replacement", () => {
  const f = fixture(),
    r = compose(f);
  assert.equal(r.mod.subarray(0, f.alternate.length).toString(), f.alternate.toString());
  assert.match(r.mod.toString(), /example\.invalid\/kept => \.\/pkg\/kept/);
  assert.equal(
    (r.mod.toString().match(/replace github.com\/Microsoft\/go-winio/g) || []).length,
    1,
  );
  assert.deepEqual(r.sum, f.sum);
  const original = Buffer.from(f.mod);
  r.mod.fill(0);
  assert.deepEqual(f.mod, original);
});
test("wrong module, prior Winio replacement, foreign Lima suffix and traversal refuse", () => {
  const f = fixture();
  assert.throws(
    () =>
      compose(f, {
        limaAlternate: Buffer.concat([f.alternate, Buffer.from("\nreplace foreign => C:/bad\n")]),
      }),
    /Lima/,
  );
  assert.throws(() => compose(f, { winioDirectory: "C:/fixture/../other" }), /traversal/);
  const wrong = fixture();
  wrong.mod = Buffer.from("module orbwin.local/session\n");
  wrong.profile.originalModSHA256 = h(wrong.mod);
  assert.throws(() => compose(wrong), /root module/);
  const conflict = fixture();
  conflict.mod = Buffer.concat([
    conflict.mod,
    Buffer.from(`replace ${winioIdentity.module} => C:/unreviewed\n`),
  ]);
  conflict.profile.originalModSHA256 = h(conflict.mod);
  assert.throws(() => compose(conflict), /existing Winio/);
});
test("effective target binds actual MVS replacement and all selected native file bytes", () => {
  const f = fixture(),
    r = validateEffectiveWinioClosureData(f.effective, f.profile, f.expected);
  assert.equal(r.selectedVersion, "v0.6.4");
  assert.equal(r.files.length, f.effective.packages[0].GoFiles.length);
  assert.equal(r.patchedProjectionSHA256, f.profile.patchedProjectionSHA256);
  assert.deepEqual(
    requireUnchangedEffectiveWinioClosureData(
      f.effective,
      clone(f.effective),
      f.profile,
      f.expected,
    ),
    r,
  );
});
test("declared version or root module-only evidence never substitutes for actual closure", () => {
  for (const change of [
    (r) => (r.selectedModule.Version = "v0.6.2"),
    (r) => (r.packages[0].Module.Version = "v0.6.2"),
    (r) => (r.selectedModule.Replace.Dir = "C:/unmodified/cache"),
    (r) => (r.packages = []),
    (r) => delete r.packages[0].Module,
    (r) => (r.packages[0].CompiledGoFiles = []),
  ]) {
    const f = fixture();
    change(f.effective);
    assert.throws(() => validateEffectiveWinioClosureData(f.effective, f.profile, f.expected));
  }
});
test("different main/vendor/Linux/tag targets do not inherit root Windows qualification", () => {
  for (const change of [
    (r) => (r.main = "orbwin.local/session"),
    (r) => (r.modMode = "vendor"),
    (r) => (r.goos = "linux"),
    (r) => (r.goarch = "arm64"),
    (r) => (r.package = "./src/networking/cmd/network"),
    (r) => r.tags.push("unreviewed"),
  ]) {
    const f = fixture();
    change(f.effective);
    assert.throws(() => validateEffectiveWinioClosureData(f.effective, f.profile, f.expected));
  }
});
test("omitted, escaping, test-only and duplicate compiled files refuse", () => {
  for (const change of [
    (p) => p.CompiledGoFiles.pop(),
    (p) => p.CompiledGoFiles.push("C:/foreign/file.go"),
    (p) => p.GoFiles.push("original_test.go"),
    (p) => p.CompiledGoFiles.push(p.CompiledGoFiles[0]),
    (p) => {
      p.GoFiles = p.GoFiles.filter((n) => n !== "pipe_original_dial_windows.go");
      p.CompiledGoFiles = p.CompiledGoFiles.filter(
        (n) => !n.endsWith("/pipe_original_dial_windows.go"),
      );
    },
  ]) {
    const f = fixture();
    change(f.effective.packages[0]);
    assert.throws(() => validateEffectiveWinioClosureData(f.effective, f.profile, f.expected));
  }
});
test("late effective modfile/package context changes invalidate before-after record", () => {
  const f = fixture(),
    after = clone(f.effective);
  after.modSHA256 = "0".repeat(64);
  assert.throws(
    () => requireUnchangedEffectiveWinioClosureData(f.effective, after, f.profile, f.expected),
    /effective/,
  );
  const b = clone(f.effective);
  b.cgo = "0";
  assert.throws(
    () => requireUnchangedEffectiveWinioClosureData(f.effective, b, f.profile, f.expected),
    /effective/,
  );
});
test("portable offer binds source variant/license/patch and carries no private build path", () => {
  const f = fixture(),
    o = winioPortableOfferData(f.profile, {
      upstreamZip: f.zip,
      patch: f.patch,
      license: f.original.get("LICENSE"),
      lockBytes: Buffer.from(JSON.stringify(f.profile)),
    });
  assert.equal(o.payloads.size, 5);
  assert.equal(o.portable.patchedProjectionSHA256, f.profile.patchedProjectionSHA256);
  assert.ok(!JSON.stringify(o.portable).includes("C:/"));
  assert.match(o.payloads.get("README.md").toString(), /h1 refers only to original source/);
  o.payloads.get("upstream.zip").fill(0);
  assert.equal(h(f.zip), f.profile.upstreamZipSHA256);
  assert.throws(
    () =>
      winioPortableOfferData(f.profile, {
        upstreamZip: Buffer.from("wrong"),
        patch: f.patch,
        license: f.original.get("LICENSE"),
        lockBytes: Buffer.from(JSON.stringify(f.profile)),
      }),
    /offer bytes/,
  );
});
test("DATA profile cannot silently change actual version or recipe inventory", () => {
  for (const change of [
    (p) => (p.version = "v0.6.2"),
    (p) => delete p.recipeHashes[recipeNames[0]],
    (p) => (p.recipeHashes.foreign = "0".repeat(64)),
    (p) => (p.upstreamModuleSum = "h1:wrong"),
  ]) {
    const f = fixture();
    change(f.profile);
    assert.throws(() => validateWinioProfileData(f.profile));
  }
});

test("self-consistent DTO from a different modfile is rejected by original composite bytes", () => {
  const f = fixture(),
    fake = clone(f.effective);
  fake.modSHA256 = h(f.alternate);
  assert.throws(
    () => requireUnchangedEffectiveWinioClosureData(fake, clone(fake), f.profile, f.expected),
    /actual composite bytes/,
  );
  const changed = clone(f.profile);
  changed.patchSHA256 = "0".repeat(64);
  assert.throws(
    () =>
      winioPortableOfferData(f.profile, {
        upstreamZip: f.zip,
        patch: f.patch,
        license: f.original.get("LICENSE"),
        lockBytes: Buffer.from(JSON.stringify(changed)),
      }),
    /lock profile/,
  );
});

test("patch DATA rejects deletion, rename, changed context and trailing unframed bytes", () => {
  const original = new Map([["file.go", Buffer.from("old\n")]]);
  const good = "--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n";
  assert.equal(
    reconstructWinioPatchData(original, Buffer.from(good)).get("file.go").toString(),
    "new\n",
  );
  for (const value of [
    good.replace("b/file.go", "/dev/null"),
    good.replace("b/file.go", "b/renamed.go"),
    good.replace("-old", "-other"),
    good + "unframed\n",
  ])
    assert.throws(() => reconstructWinioPatchData(original, Buffer.from(value)));
});

test("actual Go JSON object-stream projection retains source hashes and selects only Winio packages", () => {
  const f = fixture(),
    { selectedModule, packages, ...context } = f.effective;
  const mod = Buffer.from(JSON.stringify({ ...selectedModule, GoMod: "evidence-only" })),
    raw = Buffer.from(
      JSON.stringify({ ImportPath: "fmt", Doc: 'escaped "{ }"' }) +
        "\n" +
        JSON.stringify({ ...packages[0], IgnoredGoFiles: ["other.go"] }) +
        "\n",
    );
  const projected = projectWinioGoListData(mod, raw, context);
  assert.equal(projected.rawPackagesSHA256, h(raw));
  assert.equal(projected.record.packages.length, 1);
  assert.deepEqual(
    validateEffectiveWinioClosureData(projected.record, f.profile, f.expected),
    validateEffectiveWinioClosureData(f.effective, f.profile, f.expected),
  );
});
test("truncated, mixed and non-object Go output cannot become selected metadata", () => {
  const f = fixture(),
    { selectedModule, packages, ...context } = f.effective,
    mod = Buffer.from(JSON.stringify(selectedModule));
  for (const raw of ["{", "{} trailing", "[]", '{"x":"unterminated', "{}\n{"]) {
    assert.throws(() => projectWinioGoListData(mod, Buffer.from(raw), context));
  }
  assert.throws(
    () =>
      projectWinioGoListData(
        Buffer.concat([mod, mod]),
        Buffer.from(JSON.stringify(packages[0])),
        context,
      ),
    /one actual module/,
  );
});
