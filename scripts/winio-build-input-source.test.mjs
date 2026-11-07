import assert from "node:assert/strict";
import test from "node:test";
import {
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  preflightWinioBuildInput,
  prepareWinioBuildInput,
  verifyWinioBuildInput,
  verifyPortableWinioBuild,
  winioBuildEnvironment,
  winioDigestData,
  winioRowsFromBytesData,
  winioProjectionDigestData,
  reconstructWinioPatchData,
} from "./winio-build-input.mjs";
import {
  createWinioSourceFixture,
  winioFixtureInputPaths,
  readWinioFixtureData,
} from "./winio-test-fixtures.mjs";
const h = winioDigestData;
const fixture = createWinioSourceFixture;
function sourceMap(root, prefix = "") {
  const m = new Map();
  for (const e of readdirSync(join(root, prefix), { withFileTypes: true })) {
    const n = prefix + e.name;
    if (e.isDirectory()) for (const [k, b] of sourceMap(root, n + "/")) m.set(k, b);
    else m.set(n, readFileSync(join(root, n)));
  }
  return m;
}
function lima(o) {
  const mod = readFileSync(join(o.source, "go.mod")),
    dir = join(o.output, "lima-build-input/lima").replaceAll("\\", "/");
  return Buffer.concat([
    mod,
    Buffer.from(
      `\n// OrbWin: reviewed per-version WSL2 import and Windows host-home adaptation.\nreplace github.com/lima-vm/lima/v2 v2.3.0-beta.1.0.20261005214905-0ae2df64800a => ${JSON.stringify(dir)}\n`,
    ),
  ]);
}

test("actual original111 plus exact patch reconstructs every reviewed118 byte as DATA", (t) => {
  const o = fixture(t),
    lock = JSON.parse(readFileSync(join(o.root, "winio-custody.lock.json"))),
    original = sourceMap(join(o.moduleCache, "github.com/!microsoft/go-winio@v0.6.4")),
    patch = readFileSync(
      join(o.root, "patches/go-winio/0001-original-file-close-and-fresh-dial.patch"),
    );
  const final = reconstructWinioPatchData(original, patch);
  assert.equal(original.size, 111);
  assert.equal(final.size, 118);
  assert.equal(
    winioProjectionDigestData(winioRowsFromBytesData(final)),
    lock.profile.patchedProjectionSHA256,
  );
  for (const [n, b] of original)
    if (n.endsWith("_test.go") || n === "LICENSE") assert.deepEqual(final.get(n), b);
  assert.throws(
    () =>
      reconstructWinioPatchData(
        original,
        Buffer.from(patch.toString().replace("@@ -", "@@ -99999,")),
      ),
    /hunk|position/,
  );
});
test("source preflight has no CREATE; actual staged118 and notice are exact without cache mutation", (t) => {
  const o = fixture(t),
    cache = join(o.moduleCache, "github.com/!microsoft/go-winio@v0.6.4"),
    before = winioProjectionDigestData(winioRowsFromBytesData(sourceMap(cache)));
  const p = preflightWinioBuildInput(o);
  assert.equal(existsSync(o.output), false);
  assert.equal(p.create(), p);
  p.assertUnchanged();
  assert.equal(p.create(), p);
  assert.equal(sourceMap(p.manifest.winioRoot).size, 118);
  assert.equal(winioProjectionDigestData(winioRowsFromBytesData(sourceMap(cache))), before);
  assert.deepEqual(
    verifyPortableWinioBuild({ root: o.root, output: o.output, portable: p.portable }),
    p.portable,
  );
  assert.ok(Object.isFrozen(p.manifest.portable));
});
test("same private source state derives one original composite and does not publish a second modfile", (t) => {
  const o = fixture(t),
    p = prepareWinioBuildInput(o),
    a = p.deriveComposite(lima(o)),
    expected = Buffer.from(a.mod);
  assert.equal(existsSync(p.manifest.modfile), false);
  assert.ok(a.mod.toString().includes("replace github.com/Microsoft/go-winio v0.6.4 =>"));
  assert.ok(a.mod.toString().includes("replace github.com/lima-vm/lima/v2"));
  assert.ok(a.mod.toString().includes("./pkg/proxyguestprotocol"));
  a.mod.fill(0);
  assert.deepEqual(p.deriveComposite(lima(o)).mod, expected);
  assert.throws(
    () => p.deriveComposite(Buffer.concat([lima(o), Buffer.from("foreign\n")])),
    /Lima/,
  );
  assert.throws(() => {
    p.manifest.winioRoot = "C:/foreign";
  }, TypeError);
  p.assertUnchanged();
});
test("unknown/extra original cache source refuses before any output directory", (t) => {
  const o = fixture(t);
  writeFileSync(join(o.moduleCache, "github.com/!microsoft/go-winio@v0.6.4/extra.go"), "foreign");
  assert.throws(() => prepareWinioBuildInput(o), /original full byte set/);
  assert.equal(existsSync(o.output), false);
});
test("unaccepted peer or root module version cannot select a replacement", (t) => {
  const o = fixture(t),
    file = join(o.root, "winio-custody.lock.json"),
    lock = JSON.parse(readFileSync(file));
  lock.reviewPeerSHA256 = "0".repeat(64);
  writeFileSync(file, JSON.stringify(lock));
  assert.throws(() => prepareWinioBuildInput(o), /peer gate/);
  assert.equal(existsSync(o.output), false);
  const b = fixture(t);
  writeFileSync(join(b.source, "go.mod"), "module orbwin.local/session\n");
  assert.throws(() => prepareWinioBuildInput(b), /root module bytes/);
  assert.equal(existsSync(b.output), false);
});
test("preflight captures options and refuses stale byte inputs without a retry suffix", (t) => {
  const o = fixture(t),
    p = preflightWinioBuildInput(o),
    selected = p.manifest.winioRoot;
  o.output = "C:/foreign";
  p.create();
  assert.ok(existsSync(selected));
  p.assertUnchanged();
  const b = fixture(t),
    q = preflightWinioBuildInput(b);
  writeFileSync(join(b.root, "patches/go-winio/source/file.go"), "changed source");
  let first;
  try {
    q.create();
  } catch (e) {
    first = e;
  }
  assert.ok(first);
  assert.equal(existsSync(b.output), false);
  assert.throws(
    () => q.create(),
    (e) => e === first,
  );
});
test("late source offer mutation and recipe drift invalidate reusable positive state", (t) => {
  const o = fixture(t),
    p = prepareWinioBuildInput(o);
  writeFileSync(join(p.manifest.offerRoot, "README.md"), "changed");
  assert.throws(() => p.assertUnchanged(), /offer changed/);
  assert.throws(
    () => verifyPortableWinioBuild({ root: o.root, output: o.output, portable: p.portable }),
    /portable bytes/,
  );
  const b = fixture(t),
    q = preflightWinioBuildInput(b);
  writeFileSync(join(b.root, "scripts/build-backend.mjs"), "changed recipe");
  assert.throws(() => q.create(), /recipe/);
  assert.equal(existsSync(b.output), false);
});
test("ambient flags/cache program and overlapping namespaces refuse before effects", (t) => {
  for (const [k, v] of [
    ["GoFlAgS", "-modfile=foreign"],
    ["GOCACHEPROG", "foreign-helper"],
    ["GOWORK", "C:/foreign/go.work"],
    ["GOENV", "C:/foreign/env"],
  ])
    assert.throws(() => winioBuildEnvironment({ [k]: v }), /ambient/);
  const o = fixture(t);
  o.output = join(o.moduleCache, "new-output");
  assert.throws(() => prepareWinioBuildInput(o), /distinct build namespace/);
  assert.equal(existsSync(o.output), false);
});

test("stored build source record only verifies reconstructed original paths and bytes", (t) => {
  const o = fixture(t),
    p = prepareWinioBuildInput(o);
  assert.deepEqual(verifyWinioBuildInput(o, p.manifest), p.portable);
  const fake = JSON.parse(JSON.stringify(p.manifest));
  fake.winioRoot = "C:/foreign";
  assert.throws(() => verifyWinioBuildInput(o, fake), /stored source record/);
});

test("effective DATA records require the actual original on-disk composite before checking metadata", (t) => {
  const o = fixture(t),
    p = prepareWinioBuildInput(o),
    c = p.deriveComposite(lima(o));
  const flags = { cgo: "1", tags: ["sqlite_omit_load_extension", "with_distro"] };
  const names = [
    "file.go",
    "file_original_close_state.go",
    "file_original_iocp_windows.go",
    "pipe.go",
    "pipe_dial_state.go",
    "pipe_original_dial_windows.go",
  ];
  const dir = p.manifest.winioRoot.replaceAll("\\", "/"),
    module = {
      Path: "github.com/Microsoft/go-winio",
      Version: "v0.6.4",
      Dir: dir,
      Replace: { Path: dir, Dir: dir },
    };
  const record = {
    artifact: "rdd.exe",
    main: "github.com/rancher-sandbox/rancher-desktop-daemon",
    goos: "windows",
    goarch: "amd64",
    goamd64: "v1",
    ...flags,
    package: "./cmd/rdd",
    modMode: "readonly",
    modfile: p.manifest.modfile.replaceAll("\\", "/"),
    modSHA256: h(c.mod),
    sumSHA256: h(c.sum),
    sourceRoot: o.source.replaceAll("\\", "/"),
    winioRoot: dir,
    selectedModule: module,
    packages: [
      {
        ImportPath: module.Path,
        Dir: dir,
        Module: module,
        GoFiles: names,
        CgoFiles: [],
        CompiledGoFiles: names.map((n) => dir + "/" + n),
        EmbedFiles: [],
      },
    ],
  };
  assert.throws(() => p.recordEffectiveClosure(record, flags));
  mkdirSync(dirname(p.manifest.modfile), { recursive: true });
  writeFileSync(p.manifest.modfile, c.mod, { flag: "wx" });
  writeFileSync(p.manifest.modfile.replace(/\.mod$/, ".sum"), c.sum, { flag: "wx" });
  assert.equal(p.recordEffectiveClosure(record, flags).files.length, names.length);
  writeFileSync(p.manifest.modfile, "changed composite");
  assert.throws(() => p.recordEffectiveClosure(record, flags), /actual composite/);
});

test("self-consistent wrong patch digest cannot claim a reconstructable source offer", (t) => {
  const o = fixture(t),
    file = join(o.root, "winio-custody.lock.json"),
    lock = JSON.parse(readFileSync(file)),
    wrong = Buffer.from("wrong patch\n");
  writeFileSync(
    join(o.root, "patches/go-winio/0001-original-file-close-and-fresh-dial.patch"),
    wrong,
  );
  lock.profile.patchSHA256 = h(wrong);
  writeFileSync(file, JSON.stringify(lock));
  assert.throws(() => prepareWinioBuildInput(o), /patch/);
  assert.equal(existsSync(o.output), false);
});

test("fixture inputs are repository-owned and outputs use the captured test temp root", (t) => {
  const paths = winioFixtureInputPaths();
  assert.equal(paths.dataRoot, join(paths.repositoryRoot, "scripts/test-data/winio-input"));
  readWinioFixtureData();
  const o = fixture(t);
  assert.ok(!o.root.startsWith(paths.repositoryRoot));
  assert.ok(o.moduleCache.startsWith(o.root));
  assert.ok(o.output.startsWith(o.root));
  assert.equal(
    readFileSync(join(o.root, "scripts/winio-build-input.mjs")).equals(
      readFileSync(join(paths.repositoryRoot, "scripts/winio-build-input.mjs")),
    ),
    true,
  );
});
