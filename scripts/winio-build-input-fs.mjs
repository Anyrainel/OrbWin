// Fixed reviewed root-rdd source input. No compiler, package resolver or runtime API.
import {
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  winioDigestData,
  winioRowsFromBytesData,
  winioProjectionDigestData,
  validateWinioProfileData,
  reconstructWinioBytesData,
  reconstructWinioPatchData,
  composeWinioLimaModfileData,
  winioPortableOfferData,
  createWinioRuntimeOfferData,
  validateEffectiveWinioClosureData,
  requireUnchangedEffectiveWinioClosureData,
} from "./winio-build-input.mjs";
const REVIEW = "d33588322814f90603b4553cb7d322b38ed1aa09275b022674c032f8598445d8";
const SOURCE = "a1c70600da2b7d2ea834935e2eb139b070a504de7aa3cc1ccc9819295ff48d19";
const BASE = "871020d2a901e5e421131072661c09dc6f116bb9b050ea0d2d70f406809256cb";
const PATCHED = "b2eed5d8471fd48e5f6ee850ccc371a3484e3fd76303c2233880ea8048afabc7";
const ZIP = "0bb0285b68150970584752c9792137af693579c51ae5ff9c082d031f5b9cfdf1";
const PATCH = "patches/go-winio/0001-original-file-close-and-fresh-dial.patch";
const SELF = fileURLToPath(import.meta.url);
const fixed = {
  GOFLAGS: "",
  GOENV: "off",
  GOWORK: "off",
  GOCACHEPROG: "",
  GOTOOLCHAIN: "local",
  GOPROXY: "off",
  GOSUMDB: "off",
  GOVCS: "*:off",
  GOTELEMETRY: "off",
  GOAMD64: "v1",
  GOEXPERIMENT: "",
};
const json = (v) => Buffer.from(JSON.stringify(v, null, 2) + "\n"),
  copy = (v) => JSON.parse(JSON.stringify(v));
function frozen(v) {
  if (v && typeof v === "object") {
    for (const x of Object.values(v)) frozen(x);
    Object.freeze(v);
  }
  return v;
}
function need(v, m) {
  if (!v) throw Error("Winio source input refused: " + m);
}
function keys(v, k) {
  need(v && typeof v === "object" && !Array.isArray(v), "record");
  need(Object.keys(v).sort().join("\n") === [...k].sort().join("\n"), "closed record fields");
}
function safe(p) {
  p = resolve(p);
  for (let n = p; ; n = dirname(n)) {
    if (existsSync(n)) need(!lstatSync(n).isSymbolicLink(), "redirected source/output");
    if (dirname(n) === n) break;
  }
  return p;
}
function name(n) {
  need(
    typeof n === "string" &&
      n &&
      !isAbsolute(n) &&
      !/[\\:\0\r\n]/.test(n) &&
      n.split("/").every((p) => p && p !== "." && p !== ".."),
    "relative source name",
  );
  return n;
}
function inside(root, n) {
  const p = resolve(root, name(n)),
    r = relative(resolve(root), p);
  need(
    r && !isAbsolute(r) && r !== ".." && !r.startsWith("..\\") && !r.startsWith("../"),
    "source containment",
  );
  return safe(p);
}
function read(p, max = 16 << 20) {
  p = safe(p);
  const s = lstatSync(p);
  need(s.isFile() && s.size <= max, "regular bounded source file");
  const b = readFileSync(p);
  need(b.length === s.size, "source changed during read");
  return b;
}
function files(root) {
  root = safe(root);
  need(lstatSync(root).isDirectory(), "source directory");
  const result = new Map();
  let total = 0;
  function walk(dir, prefix = "") {
    for (const ent of readdirSync(dir, { withFileTypes: true }).sort((a, b) =>
      a.name < b.name ? -1 : 1,
    )) {
      const n = prefix + ent.name,
        p = inside(root, n);
      need(!ent.isSymbolicLink(), "source link");
      if (ent.isDirectory()) walk(p, n + "/");
      else {
        const b = read(p);
        total += b.length;
        need(total <= 256 << 20 && result.size < 4096, "source projection bound");
        result.set(n, b);
      }
    }
  }
  walk(root);
  winioRowsFromBytesData(result);
  return result;
}
function overlap(a, b) {
  const r = relative(resolve(a), resolve(b));
  return !r || (!isAbsolute(r) && r !== ".." && !r.startsWith("..\\") && !r.startsWith("../"));
}
function put(p, b) {
  safe(dirname(p));
  mkdirSync(dirname(p), { recursive: true });
  writeFileSync(p, b, { flag: "wx", mode: 0o600 });
}
export function winioBuildEnvironment(input = {}) {
  const folded = new Set();
  for (const [k, v] of Object.entries(input)) {
    const key = k.toUpperCase();
    need(!folded.has(key), "ambiguous environment");
    folded.add(key);
    if (Object.hasOwn(fixed, key))
      need(String(v ?? "") === fixed[key] || (v === "" && key !== "GOFLAGS"), "ambient " + key);
  }
  return {
    ...Object.fromEntries(
      Object.entries(input).filter(([k]) => !Object.hasOwn(fixed, k.toUpperCase())),
    ),
    ...fixed,
  };
}
function current(options) {
  keys(options, ["root", "source", "output", "moduleCache", "environment"]);
  const environment = winioBuildEnvironment(options.environment);
  const root = safe(options.root),
    source = safe(options.source),
    output = safe(options.output),
    moduleCache = safe(options.moduleCache);
  need(
    root !== source &&
      root !== output &&
      source !== output &&
      !overlap(source, output) &&
      !overlap(output, source) &&
      !overlap(moduleCache, output) &&
      !overlap(output, moduleCache),
    "distinct build namespace",
  );
  const lockBytes = read(join(root, "winio-custody.lock.json"), 2 << 20),
    lock = JSON.parse(lockBytes);
  keys(lock, ["schema", "role", "reviewPeerSHA256", "sourceReceiptSHA256", "profile"]);
  need(
    lock.schema === 1 &&
      lock.role === "root-rdd-windows-amd64" &&
      lock.reviewPeerSHA256 === REVIEW &&
      lock.sourceReceiptSHA256 === SOURCE,
    "accepted source/peer gate",
  );
  const profile = validateWinioProfileData(lock.profile);
  need(
    profile.baseline.length === 111 &&
      profile.patched.length === 118 &&
      profile.baselineProjectionSHA256 === BASE &&
      profile.patchedProjectionSHA256 === PATCHED &&
      profile.upstreamZipSHA256 === ZIP,
    "exact reviewed111/118 profile",
  );
  const mod = read(join(source, "go.mod")),
    sum = read(join(source, "go.sum"));
  need(
    winioDigestData(mod) === profile.originalModSHA256 &&
      winioDigestData(sum) === profile.originalSumSHA256,
    "actual root module bytes",
  );
  need(
    mod
      .toString()
      .split(/\r?\n/)
      .includes("module github.com/rancher-sandbox/rancher-desktop-daemon"),
    "actual root main module",
  );
  for (const [n, d] of Object.entries(profile.recipeHashes))
    need(winioDigestData(read(inside(root, n))) === d, "source recipe " + n);
  need(
    read(join(root, "scripts/winio-build-input-fs.mjs")).equals(read(SELF)),
    "loaded source helper differs",
  );
  const moduleRoot = join(moduleCache, "github.com/!microsoft/go-winio@v0.6.4"),
    zipPath = join(moduleCache, "cache/download/github.com/!microsoft/go-winio/@v/v0.6.4.zip");
  const original = files(moduleRoot),
    replacements = files(join(root, "patches/go-winio/source"));
  const patched = reconstructWinioBytesData(profile, original, replacements);
  const upstreamZip = read(zipPath),
    patch = read(join(root, PATCH)),
    license = original.get("LICENSE");
  need(winioDigestData(upstreamZip) === ZIP, "original module ZIP");
  need(
    winioProjectionDigestData(
      winioRowsFromBytesData(reconstructWinioPatchData(original, patch)),
    ) === profile.patchedProjectionSHA256,
    "source offer patch does not reconstruct reviewed source",
  );
  const offer = createWinioRuntimeOfferData(lockBytes, { upstreamZip, patch, license });
  const runtimeOffer = read(join(root, "winio-runtime-offer.json"), 2 << 20);
  need(
    json(JSON.parse(runtimeOffer)).equals(json(offer.portable)),
    "expected runtime source offer differs",
  );
  const lockSHA256 = winioDigestData(lockBytes);
  const inputRoot = join(output, "winio-build-input"),
    winioRoot = join(inputRoot, "winio"),
    offerRoot = join(output, "licenses/winio-custody"),
    modfile = join(output, "lima-build-input/rdd-lima.mod");
  const manifest = {
    schema: 1,
    role: lock.role,
    root,
    source,
    output,
    moduleCache,
    inputRoot,
    winioRoot,
    offerRoot,
    modfile,
    lockSHA256,
    runtimeOfferSHA256: winioDigestData(runtimeOffer),
    originalModSHA256: profile.originalModSHA256,
    originalSumSHA256: profile.originalSumSHA256,
    portable: offer.portable,
  };
  return {
    root,
    source,
    output,
    moduleCache,
    environment,
    profile,
    lockBytes,
    mod,
    sum,
    original,
    patched,
    offer,
    manifest,
  };
}
export function preflightWinioBuildInput(options) {
  // Capture fixed primitive arguments, not a caller's mutable options wrapper.
  const s = current(options),
    saved = {
      root: s.root,
      source: s.source,
      output: s.output,
      moduleCache: s.moduleCache,
      environment: { ...s.environment },
    };
  const originalManifest = json(s.manifest);
  let attempted = false,
    created = false,
    failure = Error("source CREATE incomplete"),
    composite;
  function unchanged() {
    const again = current(saved);
    need(json(again.manifest).equals(originalManifest), "input profile/path/recipe changed");
  }
  function verifyOutput() {
    need(created, "source input not created");
    need(
      winioProjectionDigestData(winioRowsFromBytesData(files(s.manifest.winioRoot))) ===
        s.profile.patchedProjectionSHA256,
      "staged source changed",
    );
    need(
      readdirSync(s.manifest.offerRoot).sort().join("\n") ===
        [...s.offer.payloads.keys()].sort().join("\n"),
      "source offer membership",
    );
    for (const [n, b] of s.offer.payloads)
      need(read(join(s.manifest.offerRoot, n)).equals(b), "source offer changed");
  }
  function derive(limaAlternate) {
    unchanged();
    const next = composeWinioLimaModfileData({
      profile: s.profile,
      originalMod: s.mod,
      originalSum: s.sum,
      limaAlternate,
      limaDirectory: join(s.output, "lima-build-input/lima"),
      winioDirectory: s.manifest.winioRoot,
    });
    if (composite)
      need(
        next.mod.equals(composite.mod) && next.sum.equals(composite.sum),
        "composite modfile rebound",
      );
    else composite = { mod: Buffer.from(next.mod), sum: Buffer.from(next.sum) };
    return { ...next, mod: Buffer.from(next.mod), sum: Buffer.from(next.sum) };
  }
  function expected(flags) {
    keys(flags, ["cgo", "tags"]);
    need(composite, "derive actual composite first");
    need(read(s.manifest.modfile).equals(composite.mod), "actual composite modfile changed");
    need(
      read(join(s.output, "lima-build-input/rdd-lima.sum")).equals(composite.sum),
      "actual composite sum changed",
    );
    return {
      modfile: s.manifest.modfile,
      mod: composite.mod,
      sum: composite.sum,
      sourceRoot: s.source,
      winioRoot: s.manifest.winioRoot,
      cgo: flags.cgo,
      tags: [...flags.tags],
    };
  }
  const api = {
    manifest: frozen(copy(s.manifest)),
    portable: frozen(copy(s.offer.portable)),
    childEnvironment: Object.freeze({ ...s.environment }),
    deriveComposite: derive,
    assertUnchanged() {
      unchanged();
      verifyOutput();
    },
    recordEffectiveClosure(record, flags) {
      unchanged();
      verifyOutput();
      return validateEffectiveWinioClosureData(record, s.profile, expected(flags));
    },
    verifyEffectiveClosure(before, after, flags) {
      unchanged();
      verifyOutput();
      return requireUnchangedEffectiveWinioClosureData(before, after, s.profile, expected(flags));
    },
    create() {
      if (attempted) {
        if (failure) throw failure;
        unchanged();
        verifyOutput();
        return api;
      }
      attempted = true;
      try {
        unchanged();
        need(
          !existsSync(s.manifest.inputRoot) && !existsSync(s.manifest.offerRoot),
          "fresh source/offer namespace required",
        );
        // Both complete source/recipe/offer inputs were checked before first CREATE.
        mkdirSync(s.output, { recursive: true, mode: 0o700 });
        mkdirSync(s.manifest.inputRoot, { recursive: false, mode: 0o700 });
        for (const [n, b] of s.patched) put(join(s.manifest.winioRoot, n), b);
        for (const [n, b] of s.offer.payloads) put(join(s.manifest.offerRoot, n), b);
        created = true;
        unchanged();
        verifyOutput();
        failure = null;
        return api;
      } catch (e) {
        failure = e;
        throw e;
      }
    },
  };
  return Object.freeze(api);
}
export function prepareWinioBuildInput(options) {
  return preflightWinioBuildInput(options).create();
}
// Read-only build-source verification. A manifest is DATA; all paths below
// are reconstructed from original fixed options, never followed from the DTO.
export function verifyWinioBuildInput(options, manifest) {
  const s = current(options);
  need(json(manifest).equals(json(s.manifest)), "stored source record differs");
  need(
    winioProjectionDigestData(winioRowsFromBytesData(files(s.manifest.winioRoot))) ===
      s.profile.patchedProjectionSHA256,
    "stored source bytes",
  );
  return verifyPortableWinioBuild({ root: s.root, output: s.output, portable: s.offer.portable });
}
export function verifyPortableWinioBuild({ root, output, portable, licensesDirectory }) {
  root = safe(root);
  output = safe(output);
  const lockBytes = read(join(root, "winio-custody.lock.json"), 2 << 20),
    lock = JSON.parse(lockBytes);
  keys(lock, ["schema", "role", "reviewPeerSHA256", "sourceReceiptSHA256", "profile"]);
  need(
    lock.schema === 1 &&
      lock.role === "root-rdd-windows-amd64" &&
      lock.reviewPeerSHA256 === REVIEW &&
      lock.sourceReceiptSHA256 === SOURCE,
    "portable peer/source gate",
  );
  const p = validateWinioProfileData(lock.profile);
  need(
    p.baseline.length === 111 &&
      p.patched.length === 118 &&
      p.patchedProjectionSHA256 === PATCHED &&
      p.baselineProjectionSHA256 === BASE &&
      p.upstreamZipSHA256 === ZIP,
    "portable exact profile",
  );
  for (const [n, d] of Object.entries(p.recipeHashes))
    need(winioDigestData(read(inside(root, n))) === d, "portable source recipe");
  const location = join(
    licensesDirectory === undefined ? join(output, "licenses") : safe(licensesDirectory),
    "winio-custody",
  );
  const offer = createWinioRuntimeOfferData(lockBytes, {
    upstreamZip: read(join(location, "upstream.zip")),
    patch: read(join(location, "winio-custody.patch")),
    license: read(join(location, "LICENSE")),
  });
  const expected = JSON.parse(read(join(root, "winio-runtime-offer.json"), 2 << 20));
  need(json(expected).equals(json(offer.portable)), "portable expected runtime source offer");
  need(
    readdirSync(location).sort().join("\n") === [...offer.payloads.keys()].sort().join("\n"),
    "portable membership",
  );
  for (const [n, b] of offer.payloads) need(read(join(location, n)).equals(b), "portable bytes");
  need(json(portable).equals(json(offer.portable)), "portable record");
  return copy(offer.portable);
}

export function stageWinioSourceOffer({ root, output, portable, destination }) {
  const expected = verifyPortableWinioBuild({ root, output, portable });
  destination = safe(destination);
  need(!existsSync(destination), "fresh Winio notice destination");
  const bodies = Object.entries(expected.offerFiles).map(([n, d]) => {
    const b = read(join(output, "licenses/winio-custody", name(n)));
    need(winioDigestData(b) === d, "source offer changed before notice");
    return [n, b];
  });
  mkdirSync(destination, { recursive: true, mode: 0o700 });
  for (const [n, b] of bodies) put(join(destination, n), b);
  for (const [n, b] of bodies)
    need(read(join(destination, n)).equals(b), "copied notice offer changed");
  return copy(expected);
}
