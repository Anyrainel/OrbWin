// Test-only owned source fixtures. Never resolves a global cache or artifact parent.
import {
  cpSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { createWinioRuntimeOfferData, winioDigestData } from "./winio-build-input.mjs";
const here = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(here, ".."),
  dataRoot = join(here, "test-data/winio-input");
const scripts = [
  "winio-build-input.mjs",
  "winio-build-input-fs.mjs",
  "lima-wsl2-build-input.mjs",
  "build-backend.mjs",
  "build-identity.mjs",
  "collect-notices.mjs",
];
function need(v, m) {
  if (!v) throw Error("Owned Winio test fixture refused: " + m);
}
function below(root, p) {
  const r = relative(resolve(root), resolve(p));
  return r && !isAbsolute(r) && r !== ".." && !r.startsWith(".." + sep);
}
function ownedRoot(root) {
  const actual = realpathSync(root),
    parent = realpathSync(tmpdir());
  need(
    dirname(actual) === parent &&
      /^orbwin-(winio-source|lima-input|build-test)-/.test(basename(actual)),
    "controlled per-test root",
  );
  return actual;
}
function name(n) {
  need(
    typeof n === "string" &&
      n &&
      !isAbsolute(n) &&
      !/[\\:\0\r\n]/.test(n) &&
      n.split("/").every((v) => v && v !== "." && v !== ".."),
    "data path",
  );
  return n;
}
export function winioFixtureInputPaths() {
  return Object.freeze({ repositoryRoot, dataRoot });
}
export function readWinioFixtureData() {
  need(below(repositoryRoot, dataRoot), "data inside repository");
  const manifest = JSON.parse(readFileSync(join(dataRoot, "manifest.json")));
  need(
    manifest.schema === 1 &&
      manifest.module === "github.com/Microsoft/go-winio" &&
      manifest.version === "v0.6.4" &&
      manifest.originalFiles === 111,
    "data identity",
  );
  const expected = new Set(["manifest.json"]);
  const bodies = new Map();
  for (const r of manifest.files) {
    const n = name(r.path),
      p = join(dataRoot, n),
      stat = lstatSync(p);
    need(stat.isFile() && !stat.isSymbolicLink(), "regular immutable data");
    const b = readFileSync(p);
    need(b.length === r.bytes && winioDigestData(b) === r.sha256, "immutable data bytes");
    need(!expected.has(n), "duplicate data");
    expected.add(n);
    bodies.set(n, b);
  }
  function walk(dir, prefix = "") {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      need(!e.isSymbolicLink(), "redirected data");
      const n = prefix + e.name;
      name(n);
      if (e.isDirectory()) walk(join(dir, e.name), n + "/");
      else need(expected.delete(n), "unknown data");
    }
  }
  walk(dataRoot);
  need(expected.size === 0, "missing data");
  const lock = JSON.parse(readFileSync(join(repositoryRoot, "winio-custody.lock.json")));
  need(
    winioDigestData(bodies.get("upstream.zip")) === lock.profile.upstreamZipSHA256,
    "current original ZIP pin",
  );
  need(winioDigestData(bodies.get("original/LICENSE")) === lock.profile.licenseSHA256, "MIT pin");
  const originals = [...bodies].filter(([n]) => n.startsWith("original/"));
  need(originals.length === 111, "full original projection");
  for (const r of lock.profile.baseline) {
    const b = bodies.get("original/" + r.path);
    need(
      b && b.length === r.bytes && winioDigestData(b) === r.sha256,
      "current original source projection",
    );
  }
  need(
    winioDigestData(bodies.get("root-go.mod")) === lock.profile.originalModSHA256 &&
      winioDigestData(bodies.get("root-go.sum")) === lock.profile.originalSumSHA256,
    "current declared main inputs",
  );
  return { manifest, bodies, lock };
}
export function attachWinioFixture(root, source, moduleCache, mod, sum) {
  const captured = ownedRoot(root);
  need(
    below(captured, source) && below(captured, moduleCache),
    "all fixture outputs inside captured root",
  );
  const { bodies, lock: baseline } = readWinioFixtureData();
  function put(n, b) {
    const p = join(captured, name(n));
    mkdirSync(dirname(p), { recursive: true });
    writeFileSync(p, b);
  }
  // Semantic code always comes from THIS current repository, never a frozen fixture repo.
  for (const n of scripts) put("scripts/" + n, readFileSync(join(here, n)));
  cpSync(join(repositoryRoot, "patches/go-winio"), join(captured, "patches/go-winio"), {
    recursive: true,
  });
  const original = join(moduleCache, "github.com/!microsoft/go-winio@v0.6.4");
  for (const [n, b] of bodies)
    if (n.startsWith("original/")) {
      const p = join(original, n.slice(9));
      mkdirSync(dirname(p), { recursive: true });
      writeFileSync(p, b);
    }
  const zip = join(moduleCache, "cache/download/github.com/!microsoft/go-winio/@v/v0.6.4.zip");
  mkdirSync(dirname(zip), { recursive: true });
  writeFileSync(zip, bodies.get("upstream.zip"));
  const lock = structuredClone(baseline);
  lock.profile.originalModSHA256 = winioDigestData(mod);
  lock.profile.originalSumSHA256 = winioDigestData(sum);
  lock.profile.recipeHashes = Object.fromEntries(
    scripts.map((n) => ["scripts/" + n, winioDigestData(readFileSync(join(here, n)))]),
  );
  const raw = Buffer.from(JSON.stringify(lock, null, 2) + "\n");
  put("winio-custody.lock.json", raw);
  const offer = createWinioRuntimeOfferData(raw, {
    upstreamZip: bodies.get("upstream.zip"),
    patch: readFileSync(
      join(captured, "patches/go-winio/0001-original-file-close-and-fresh-dial.patch"),
    ),
    license: bodies.get("original/LICENSE"),
  });
  put("winio-runtime-offer.json", Buffer.from(JSON.stringify(offer.portable, null, 2) + "\n"));
  return offer.portable;
}
export function createWinioSourceFixture(t) {
  need(t && typeof t.after === "function", "test context owns cleanup");
  // Validate repository-owned input data before creating ANY output root.
  const { bodies } = readWinioFixtureData();
  const home = realpathSync(mkdtempSync(join(realpathSync(tmpdir()), "orbwin-winio-source-")));
  t.after(() => {
    const actual = ownedRoot(home);
    need(actual === home, "original cleanup root");
    rmSync(home, { recursive: true });
  });
  const root = home,
    source = join(root, "rdd"),
    moduleCache = join(root, "module-cache"),
    output = join(root, "output");
  mkdirSync(source);
  const mod = bodies.get("root-go.mod"),
    sum = bodies.get("root-go.sum");
  writeFileSync(join(source, "go.mod"), mod);
  writeFileSync(join(source, "go.sum"), sum);
  attachWinioFixture(root, source, moduleCache, mod, sum);
  return {
    root,
    source,
    moduleCache,
    output,
    environment: {
      GOFLAGS: "",
      GOENV: "off",
      GOWORK: "off",
      GOCACHEPROG: "",
      GOTOOLCHAIN: "local",
      GOPROXY: "off",
      GOSUMDB: "off",
    },
  };
}
