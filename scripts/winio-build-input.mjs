// Peer-reviewed source profile; isolated build inputs only, no product activation.
// Pure DATA validation never grants a product/native/cache execution permit.
import { createHash } from "node:crypto";
import { win32 } from "node:path";

export const winioIdentity = Object.freeze({
  module: "github.com/Microsoft/go-winio",
  version: "v0.6.4",
  mainModule: "github.com/rancher-sandbox/rancher-desktop-daemon",
});
const RECIPES = Object.freeze([
  "scripts/winio-build-input.mjs",
  "scripts/winio-build-input-fs.mjs",
  "scripts/lima-wsl2-build-input.mjs",
  "scripts/build-backend.mjs",
  "scripts/build-identity.mjs",
  "scripts/collect-notices.mjs",
]);
const REQUIRED = Object.freeze([
  "file.go",
  "file_original_close_state.go",
  "file_original_iocp_windows.go",
  "pipe.go",
  "pipe_dial_state.go",
  "pipe_original_dial_windows.go",
]);
const LIMAMODULE = "github.com/lima-vm/lima/v2";
const LIMAVERSION = "v2.3.0-beta.1.0.20261005214905-0ae2df64800a";
export const winioDigestData = (value) => createHash("sha256").update(value).digest("hex");
const bytes = (value) => {
  need(Buffer.isBuffer(value) && value.length <= 16 << 20, "bounded byte input");
  return Buffer.from(value);
};
function need(value, message) {
  if (!value) throw new Error(`Winio input refused: ${message}`);
}
function closed(value, keys) {
  need(value !== null && typeof value === "object" && !Array.isArray(value), "record");
  need([Object.prototype, null].includes(Object.getPrototypeOf(value)), "plain record");
  need(Object.getOwnPropertySymbols(value).length === 0, "symbol field");
  const descriptors = Object.getOwnPropertyDescriptors(value);
  need(
    Object.values(descriptors).every((d) => Object.hasOwn(d, "value")),
    "accessor field",
  );
  need(
    Object.keys(descriptors).sort().join("\n") === [...keys].sort().join("\n"),
    "closed record fields",
  );
}
function hash(value) {
  need(typeof value === "string" && /^[0-9a-f]{64}$/.test(value), "SHA256");
  return value;
}
function sourceName(value) {
  need(
    typeof value === "string" && value.length <= 512 && /^[\x21-\x7e]+$/.test(value),
    "source name",
  );
  need(!/[\\:*?"<>|]/.test(value) && !value.startsWith("/"), "source path syntax");
  for (const part of value.split("/")) {
    need(part && part !== "." && part !== ".." && !/[. ]$/.test(part), "source component");
    need(!/^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(part), "reserved source component");
  }
  return value;
}
function absolute(value) {
  need(
    typeof value === "string" && /^[a-z]:[\\/]/i.test(value) && !/[\0\r\n]/.test(value),
    "fixed Windows build path",
  );
  need(!value.split(/[\\/]/).includes(".."), "path traversal");
  return win32.normalize(value).replaceAll("\\", "/");
}
function under(base, value) {
  const b = absolute(base),
    v = absolute(value);
  need(
    v.toLowerCase().startsWith(b.toLowerCase().replace(/\/$/, "") + "/"),
    "selected file outside source",
  );
  return sourceName(v.slice(b.replace(/\/$/, "").length + 1));
}
function rows(value) {
  need(Array.isArray(value) && value.length > 0 && value.length <= 4096, "source projection bound");
  const folded = new Set();
  let total = 0;
  const result = value
    .map((row) => {
      closed(row, ["path", "bytes", "sha256"]);
      const path = sourceName(row.path),
        sha256 = hash(row.sha256);
      need(
        Number.isSafeInteger(row.bytes) && row.bytes >= 0 && row.bytes <= 16 << 20,
        "file bound",
      );
      total += row.bytes;
      need(total <= 256 << 20, "source total bound");
      need(!folded.has(path.toLowerCase()), "source collision");
      folded.add(path.toLowerCase());
      return { path, sha256, bytes: row.bytes };
    })
    .sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  return result;
}
const json = (value) => Buffer.from(JSON.stringify(value, null, 2) + "\n");
export function winioProjectionDigestData(value) {
  return winioDigestData(json(rows(value)));
}
export function winioRowsFromBytesData(input) {
  need(input instanceof Map && input.size > 0 && input.size <= 4096, "source Map");
  return rows(
    [...input].map(([path, value]) => {
      const body = bytes(value);
      return { path, bytes: body.length, sha256: winioDigestData(body) };
    }),
  );
}
export function validateWinioProfileData(input) {
  closed(input, [
    "schema",
    "module",
    "version",
    "mainModule",
    "originalModSHA256",
    "originalSumSHA256",
    "upstreamZipSHA256",
    "upstreamModuleSum",
    "upstreamGoModSum",
    "patchSHA256",
    "licenseSHA256",
    "baseline",
    "patched",
    "baselineProjectionSHA256",
    "patchedProjectionSHA256",
    "replacements",
    "recipeHashes",
  ]);
  need(
    input.schema === 1 &&
      input.module === winioIdentity.module &&
      input.version === winioIdentity.version &&
      input.mainModule === winioIdentity.mainModule,
    "root-rdd version profile",
  );
  for (const key of [
    "originalModSHA256",
    "originalSumSHA256",
    "upstreamZipSHA256",
    "patchSHA256",
    "licenseSHA256",
    "baselineProjectionSHA256",
    "patchedProjectionSHA256",
  ])
    hash(input[key]);
  need(
    input.upstreamModuleSum === "h1:kJF2pamtYCWXf0xAT8sAkwKC0pMl2HYBrRPHAqyUc+g=" &&
      input.upstreamGoModSum === "h1:1xl5JXG4ze/m74GH6Dc4Mwxfc3VL5siRVQjIAlHH07U=",
    "upstream sum identity",
  );
  const baseline = rows(input.baseline),
    patched = rows(input.patched);
  need(
    winioProjectionDigestData(baseline) === input.baselineProjectionSHA256 &&
      winioProjectionDigestData(patched) === input.patchedProjectionSHA256,
    "full projection digest",
  );
  const old = new Map(baseline.map((r) => [r.path, r])),
    final = new Map(patched.map((r) => [r.path, r]));
  need(
    [...old.keys()].every((n) => final.has(n)),
    "source deletion",
  );
  need(
    old.get("LICENSE")?.sha256 === input.licenseSHA256 &&
      final.get("LICENSE")?.sha256 === input.licenseSHA256,
    "MIT preservation",
  );
  for (const r of baseline)
    if (r.path.endsWith("_test.go") || r.path === "go.mod" || r.path === "go.sum")
      need(
        JSON.stringify(r) === JSON.stringify(final.get(r.path)),
        "upstream test/module preservation",
      );
  const changed = [...final]
    .filter(([n, r]) => old.get(n)?.sha256 !== r.sha256 || old.get(n)?.bytes !== r.bytes)
    .map(([n]) => n)
    .sort();
  need(
    Array.isArray(input.replacements) &&
      input.replacements.length === changed.length &&
      changed.length <= 32,
    "replacement bound",
  );
  const repl = input.replacements
    .map((r) => {
      closed(r, ["path", "before", "sha256", "bytes"]);
      const path = sourceName(r.path);
      need(r.before === (old.get(path)?.sha256 ?? null), "original replacement byte pin");
      need(
        r.sha256 === final.get(path)?.sha256 && r.bytes === final.get(path)?.bytes,
        "final replacement byte pin",
      );
      return { path, before: r.before, sha256: r.sha256, bytes: r.bytes };
    })
    .sort((a, b) => (a.path < b.path ? -1 : 1));
  need(repl.map((r) => r.path).join("\n") === changed.join("\n"), "exact replacement coverage");
  closed(input.recipeHashes, RECIPES);
  for (const d of Object.values(input.recipeHashes)) hash(d);
  return {
    ...input,
    baseline,
    patched,
    replacements: repl,
    recipeHashes: { ...input.recipeHashes },
  };
}
// DATA reconstruction only. The production filesystem gateway below is sealed.
export function reconstructWinioBytesData(profileInput, originalInput, replacementInput) {
  const profile = validateWinioProfileData(profileInput);
  const original = new Map([...originalInput].map(([n, b]) => [n, bytes(b)]));
  const replacement = new Map([...replacementInput].map(([n, b]) => [n, bytes(b)]));
  need(
    winioProjectionDigestData(winioRowsFromBytesData(original)) ===
      profile.baselineProjectionSHA256,
    "original full byte set",
  );
  need(
    [...replacement.keys()].sort().join("\n") ===
      profile.replacements.map((r) => r.path).join("\n"),
    "replacement byte set",
  );
  const result = new Map(original);
  for (const r of profile.replacements) {
    const b = replacement.get(r.path);
    need(b.length === r.bytes && winioDigestData(b) === r.sha256, "replacement body");
    result.set(r.path, b);
  }
  need(
    winioProjectionDigestData(winioRowsFromBytesData(result)) === profile.patchedProjectionSHA256,
    "reconstructed full projection",
  );
  return new Map([...result].map(([n, b]) => [n, Buffer.from(b)]));
}
// Derive ONE root modfile from exact raw original + existing fixed Lima suffix.
// Do not append to any previously emitted/sealed file or alter source mod/sum.
export function composeWinioLimaModfileData({
  profile: input,
  originalMod: m,
  originalSum: s,
  limaAlternate: a,
  limaDirectory,
  winioDirectory,
}) {
  const profile = validateWinioProfileData(input),
    mod = bytes(m),
    sum = bytes(s),
    alternate = bytes(a);
  need(
    winioDigestData(mod) === profile.originalModSHA256 &&
      winioDigestData(sum) === profile.originalSumSHA256,
    "actual main module bytes",
  );
  need(
    mod.toString().split(/\r?\n/).includes(`module ${winioIdentity.mainModule}`),
    "actual root module name",
  );
  // Root source is byte-pinned; an existing versioned or unversioned Winio
  // replacement is refused rather than overridden by a second directive.
  need(
    !mod
      .toString()
      .split(/\r?\n/)
      .some(
        (line) =>
          line.split("//")[0].includes(winioIdentity.module) && line.split("//")[0].includes("=>"),
      ),
    "existing Winio replacement",
  );
  const lima = absolute(limaDirectory),
    winio = absolute(winioDirectory);
  const suffix = Buffer.from(
    `\n// OrbWin: reviewed per-version WSL2 import and Windows host-home adaptation.\nreplace ${LIMAMODULE} ${LIMAVERSION} => ${JSON.stringify(lima)}\n`,
  );
  need(alternate.equals(Buffer.concat([mod, suffix])), "exact existing Lima alternate");
  const output = Buffer.concat([
    alternate,
    Buffer.from(
      `\n// OrbWin: reviewed original win32File Close and bounded Fresh Dial custody.\nreplace ${winioIdentity.module} ${winioIdentity.version} => ${JSON.stringify(winio)}\n`,
    ),
  ]);
  return {
    mod: output,
    sum: Buffer.from(sum),
    sourceModSHA256: winioDigestData(mod),
    alternateModSHA256: winioDigestData(output),
    alternateSumSHA256: winioDigestData(sum),
  };
}

// Exact effective build DATA, never from a root declaration or a PID/DTO permit.
export function validateEffectiveWinioClosureData(input, profileInput, expected) {
  const profile = validateWinioProfileData(profileInput);
  closed(input, [
    "artifact",
    "main",
    "goos",
    "goarch",
    "goamd64",
    "cgo",
    "tags",
    "package",
    "modMode",
    "modfile",
    "modSHA256",
    "sumSHA256",
    "sourceRoot",
    "winioRoot",
    "selectedModule",
    "packages",
  ]);
  need(
    input.artifact === "rdd.exe" &&
      input.main === winioIdentity.mainModule &&
      input.package === "./cmd/rdd",
    "actual artifact main/target",
  );
  need(
    input.goos === "windows" &&
      input.goarch === "amd64" &&
      input.goamd64 === "v1" &&
      ["0", "1"].includes(input.cgo),
    "actual platform",
  );
  need(
    Array.isArray(input.tags) &&
      input.tags.length <= 8 &&
      input.tags.every((t) => typeof t === "string" && /^[a-z0-9_]+$/.test(t)),
    "build tags",
  );
  need(
    new Set(input.tags).size === input.tags.length &&
      input.tags.includes("sqlite_omit_load_extension"),
    "closed required build tags",
  );
  need(
    input.tags.every((t) => ["sqlite_omit_load_extension", "with_distro"].includes(t)),
    "unsupported build tag",
  );
  need(input.modMode === "readonly", "root must use reviewed composite modfile, not vendor");
  const sourceRoot = absolute(input.sourceRoot),
    winioRoot = absolute(input.winioRoot),
    modfile = absolute(input.modfile);
  hash(input.modSHA256);
  hash(input.sumSHA256);
  // expected comes from the same original composite-modfile derivation and
  // fixed producer arguments, not fields copied out of the queried DTO.
  closed(expected, ["modfile", "mod", "sum", "sourceRoot", "winioRoot", "cgo", "tags"]);
  need(
    modfile === absolute(expected.modfile) &&
      sourceRoot === absolute(expected.sourceRoot) &&
      winioRoot === absolute(expected.winioRoot),
    "effective original build paths",
  );
  need(
    input.modSHA256 === winioDigestData(bytes(expected.mod)) &&
      input.sumSHA256 === winioDigestData(bytes(expected.sum)),
    "effective actual composite bytes",
  );
  need(
    input.cgo === expected.cgo &&
      JSON.stringify([...input.tags].sort()) === JSON.stringify([...expected.tags].sort()),
    "effective fixed producer flags",
  );
  closed(input.selectedModule, ["Path", "Version", "Dir", "Replace"]);
  function selected(m) {
    closed(m, ["Path", "Version", "Dir", "Replace"]);
    closed(m.Replace, ["Path", "Dir"]);
    need(
      m.Path === winioIdentity.module && m.Version === winioIdentity.version,
      "actual selected MVS version",
    );
    need(
      absolute(m.Dir).toLowerCase() === winioRoot.toLowerCase() &&
        absolute(m.Replace.Path).toLowerCase() === winioRoot.toLowerCase() &&
        absolute(m.Replace.Dir).toLowerCase() === winioRoot.toLowerCase(),
      "actual selected replacement directory",
    );
  }
  selected(input.selectedModule);
  need(
    Array.isArray(input.packages) && input.packages.length > 0 && input.packages.length <= 128,
    "affected package inventory",
  );
  const final = new Map(profile.patched.map((r) => [r.path, r]));
  const imported = new Set();
  const files = new Map();
  const packages = input.packages
    .map((p) => {
      closed(p, [
        "ImportPath",
        "Dir",
        "Module",
        "GoFiles",
        "CgoFiles",
        "CompiledGoFiles",
        "EmbedFiles",
      ]);
      selected(p.Module);
      need(
        typeof p.ImportPath === "string" &&
          (p.ImportPath === winioIdentity.module ||
            p.ImportPath.startsWith(winioIdentity.module + "/")),
        "affected package import",
      );
      need(!imported.has(p.ImportPath), "duplicate affected package");
      imported.add(p.ImportPath);
      const dir = absolute(p.Dir);
      need(
        dir.toLowerCase() === winioRoot.toLowerCase() ||
          dir.toLowerCase().startsWith(winioRoot.toLowerCase() + "/"),
        "package source directory",
      );
      const lists = {};
      for (const key of ["GoFiles", "CgoFiles", "CompiledGoFiles", "EmbedFiles"]) {
        need(Array.isArray(p[key]) && p[key].length <= 512, "selected file list");
        const seen = new Set();
        lists[key] = p[key]
          .map((n) => {
            need(typeof n === "string", "selected file name");
            const path = /^[a-z]:/i.test(n)
              ? under(winioRoot, n)
              : under(winioRoot, win32.join(dir, sourceName(n)));
            need(!seen.has(path.toLowerCase()), "duplicate selected file");
            seen.add(path.toLowerCase());
            const row = final.get(path);
            need(row && !path.endsWith("_test.go"), "selected file not reviewed production bytes");
            files.set(path, row);
            return path;
          })
          .sort();
      }
      need(
        lists.GoFiles.length + lists.CgoFiles.length > 0 && lists.CompiledGoFiles.length > 0,
        "actual compiled file closure required",
      );
      need(
        [...lists.GoFiles, ...lists.CgoFiles].every((n) => lists.CompiledGoFiles.includes(n)),
        "omitted compiled source",
      );
      return { ImportPath: p.ImportPath, Dir: dir, ...lists };
    })
    .sort((a, b) => (a.ImportPath < b.ImportPath ? -1 : 1));
  need(
    imported.has(winioIdentity.module) && REQUIRED.every((n) => files.has(n)),
    "actual native custody files absent from target",
  );
  return {
    artifact: input.artifact,
    main: input.main,
    goos: input.goos,
    goarch: input.goarch,
    goamd64: input.goamd64,
    cgo: input.cgo,
    tags: [...input.tags].sort(),
    package: input.package,
    modMode: input.modMode,
    modfile,
    modSHA256: input.modSHA256,
    sumSHA256: input.sumSHA256,
    sourceRoot,
    winioRoot,
    selectedVersion: winioIdentity.version,
    packages,
    files: rows([...files.values()]),
    patchedProjectionSHA256: profile.patchedProjectionSHA256,
  };
}
export function requireUnchangedEffectiveWinioClosureData(before, after, profile, expected) {
  const a = validateEffectiveWinioClosureData(before, profile, expected),
    b = validateEffectiveWinioClosureData(after, profile, expected);
  need(json(a).equals(json(b)), "effective module/target/file closure changed across build");
  return a;
}

// Decode actual Go -json object streams as DATA. Origin hashes remain evidence,
// not a substitute for the fixed producer's real pinned Go invocation.
export function projectWinioGoListData(moduleBytes, packageBytes, context) {
  function stream(raw) {
    need(Buffer.isBuffer(raw) && raw.length > 0 && raw.length <= 32 << 20, "Go metadata bound");
    const text = raw.toString("utf8");
    need(Buffer.from(text).equals(raw), "UTF8 Go metadata");
    const out = [];
    let start = -1,
      depth = 0,
      quoted = false,
      escape = false;
    for (let i = 0; i < text.length; i++) {
      const c = text[i];
      if (start < 0) {
        if (/\s/.test(c)) continue;
        need(c === "{", "Go metadata object stream");
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
      else if (c === "}" || c === "]") depth--;
      need(depth >= 0, "Go metadata depth");
      if (depth === 0) {
        const v = JSON.parse(text.slice(start, i + 1));
        need(v && typeof v === "object" && !Array.isArray(v), "Go metadata record");
        out.push(v);
        need(out.length <= 20000, "Go metadata record bound");
        start = -1;
      }
    }
    need(start === -1 && !quoted && out.length > 0, "truncated/empty Go metadata");
    return out;
  }
  const modules = stream(moduleBytes);
  need(modules.length === 1, "one actual module query");
  function selected(m) {
    need(m && m.Replace, "selected actual module");
    return {
      Path: m.Path,
      Version: m.Version,
      Dir: m.Dir,
      Replace: { Path: m.Replace.Path, Dir: m.Replace.Dir },
    };
  }
  const packages = stream(packageBytes)
    .filter(
      (p) =>
        typeof p.ImportPath === "string" &&
        (p.ImportPath === winioIdentity.module ||
          p.ImportPath.startsWith(winioIdentity.module + "/")),
    )
    .map((p) => ({
      ImportPath: p.ImportPath,
      Dir: p.Dir,
      Module: selected(p.Module),
      GoFiles: p.GoFiles ?? [],
      CgoFiles: p.CgoFiles ?? [],
      CompiledGoFiles: p.CompiledGoFiles ?? [],
      EmbedFiles: p.EmbedFiles ?? [],
    }));
  return {
    record: { ...context, selectedModule: selected(modules[0]), packages },
    rawModuleSHA256: winioDigestData(moduleBytes),
    rawPackagesSHA256: winioDigestData(packageBytes),
  };
}

// Reconstruct the fixed text patch as DATA; never invokes Git or a tool.
export function reconstructWinioPatchData(originalInput, patchInput) {
  const patch = bytes(patchInput);
  need(Buffer.from(patch.toString("utf8"), "utf8").equals(patch), "UTF8 source patch");
  const lines = patch.toString("utf8").match(/[^\n]*\n|[^\n]+$/g) || [];
  const result = new Map([...originalInput].map(([n, b]) => [sourceName(n), bytes(b)])),
    seen = new Set();
  let i = 0;
  while (i < lines.length) {
    need(
      lines[i].startsWith("--- ") && i + 1 < lines.length && lines[i + 1].startsWith("+++ "),
      "patch file header",
    );
    const from = lines[i++].slice(4).trimEnd(),
      to = lines[i++].slice(4).trimEnd();
    need(to.startsWith("b/"), "patch cannot delete or rename source");
    const path = sourceName(to.slice(2));
    need(!seen.has(path.toLowerCase()), "duplicate patch file");
    seen.add(path.toLowerCase());
    need(from === "/dev/null" || from === "a/" + path, "patch original name");
    need((from === "/dev/null") === !result.has(path), "patch original membership");
    const body = result.get(path) || Buffer.alloc(0);
    need(Buffer.from(body.toString("utf8"), "utf8").equals(body), "UTF8 source body");
    const old = body.toString("utf8").match(/[^\n]*\n|[^\n]+$/g) || [],
      out = [];
    let cursor = 0,
      hunks = 0;
    while (i < lines.length && lines[i].startsWith("@@ ")) {
      const m = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@[^\n]*\n$/.exec(lines[i++]);
      need(m, "patch hunk");
      const os = Number(m[1]),
        oc = m[2] === undefined ? 1 : Number(m[2]),
        ns = Number(m[3]),
        nc = m[4] === undefined ? 1 : Number(m[4]);
      need(
        [os, oc, ns, nc].every((n) => Number.isSafeInteger(n) && n >= 0 && n <= 1000000),
        "hunk bounds",
      );
      const start = oc === 0 ? os : os - 1;
      need(start >= cursor && start <= old.length, "hunk position");
      out.push(...old.slice(cursor, start));
      cursor = start;
      need(out.length === (nc === 0 ? ns : ns - 1), "new hunk position");
      let consumed = 0,
        produced = 0;
      while (i < lines.length && !lines[i].startsWith("@@ ") && !lines[i].startsWith("--- ")) {
        const line = lines[i++],
          op = line[0],
          text = line.slice(1);
        need([" ", "+", "-"].includes(op), "hunk operator");
        if (op !== "+") {
          need(old[cursor] === text, "original hunk bytes");
          cursor++;
          consumed++;
        }
        if (op !== "-") {
          out.push(text);
          produced++;
        }
      }
      need(consumed === oc && produced === nc, "hunk counts");
      hunks++;
    }
    need(hunks > 0, "empty source patch");
    out.push(...old.slice(cursor));
    result.set(path, Buffer.from(out.join("")));
  }
  need(seen.size > 0, "empty patch");
  winioRowsFromBytesData(result);
  return result;
}

export function winioPortableOfferData(profileInput, input) {
  const profile = validateWinioProfileData(profileInput);
  closed(input, ["upstreamZip", "patch", "license", "lockBytes"]);
  const lockBytes = bytes(input.lockBytes);
  need(
    JSON.stringify(validateWinioProfileData(JSON.parse(lockBytes))) === JSON.stringify(profile),
    "exact source lock profile",
  );
  const lockSHA256 = winioDigestData(lockBytes);
  const upstreamZip = bytes(input.upstreamZip),
    patch = bytes(input.patch),
    license = bytes(input.license);
  need(
    winioDigestData(upstreamZip) === profile.upstreamZipSHA256 &&
      winioDigestData(patch) === profile.patchSHA256 &&
      winioDigestData(license) === profile.licenseSHA256,
    "portable source offer bytes",
  );
  const readme = Buffer.from(
    `OrbWin go-winio source adaptation\nModule: ${winioIdentity.module} ${winioIdentity.version}\nOriginal upstream h1: ${profile.upstreamModuleSum}\nPatched projection: ${profile.patchedProjectionSHA256}\nupstream.zip is the original module. Apply winio-custody.patch to reconstruct the reviewed source. LICENSE and upstream tests are retained. h1 refers only to original source. Native qualification and actual caller custody remain separate.\n`,
  );
  const payloads = new Map([
    ["upstream.zip", upstreamZip],
    ["winio-custody.patch", patch],
    ["LICENSE", license],
    ["README.md", readme],
  ]);
  const manifest = {
    schema: 1,
    lockSHA256,
    module: winioIdentity.module,
    version: winioIdentity.version,
    upstreamModuleSum: profile.upstreamModuleSum,
    upstreamGoModSum: profile.upstreamGoModSum,
    upstreamZipSHA256: profile.upstreamZipSHA256,
    baseline: profile.baseline,
    patched: profile.patched,
    patchedProjectionSHA256: profile.patchedProjectionSHA256,
    patchSHA256: profile.patchSHA256,
    recipeHashes: profile.recipeHashes,
    files: Object.fromEntries([...payloads].map(([n, b]) => [n, winioDigestData(b)])),
  };
  payloads.set("manifest.json", json(manifest));
  const portable = {
    schema: 1,
    lockSHA256,
    module: winioIdentity.module,
    version: winioIdentity.version,
    upstreamZipSHA256: profile.upstreamZipSHA256,
    patchedProjectionSHA256: profile.patchedProjectionSHA256,
    patchSHA256: profile.patchSHA256,
    recipeHashes: { ...profile.recipeHashes },
    offerFiles: Object.fromEntries([...payloads].map(([n, b]) => [n, winioDigestData(b)])),
  };
  return {
    portable: JSON.parse(JSON.stringify(portable)),
    payloads: new Map([...payloads].map(([n, b]) => [n, Buffer.from(b)])),
  };
}

// The one static runtime fixture is generated from Node's actual source offer,
// never reserialized by Rust or placed inside its own lock/recipe digest list.
export function createWinioRuntimeOfferData(lockBytes, input) {
  const raw = bytes(lockBytes),
    lock = JSON.parse(raw);
  closed(lock, ["schema", "role", "reviewPeerSHA256", "sourceReceiptSHA256", "profile"]);
  need(
    lock.schema === 1 &&
      lock.role === "root-rdd-windows-amd64" &&
      lock.reviewPeerSHA256 ===
        "d33588322814f90603b4553cb7d322b38ed1aa09275b022674c032f8598445d8" &&
      lock.sourceReceiptSHA256 ===
        "a1c70600da2b7d2ea834935e2eb139b070a504de7aa3cc1ccc9819295ff48d19",
    "runtime offer source/peer",
  );
  closed(input, ["upstreamZip", "patch", "license"]);
  const offer = winioPortableOfferData(lock.profile, { ...input, lockBytes: json(lock.profile) }),
    lockSHA256 = winioDigestData(raw);
  const manifest = JSON.parse(offer.payloads.get("manifest.json"));
  manifest.lockSHA256 = lockSHA256;
  manifest.reviewPeerSHA256 = lock.reviewPeerSHA256;
  manifest.sourceReceiptSHA256 = lock.sourceReceiptSHA256;
  offer.payloads.set("manifest.json", json(manifest));
  offer.portable.lockSHA256 = lockSHA256;
  offer.portable.reviewPeerSHA256 = lock.reviewPeerSHA256;
  offer.portable.sourceReceiptSHA256 = lock.sourceReceiptSHA256;
  offer.portable.offerFiles = Object.fromEntries(
    [...offer.payloads].map(([n, b]) => [n, winioDigestData(b)]),
  );
  return {
    portable: JSON.parse(JSON.stringify(offer.portable)),
    payloads: new Map([...offer.payloads].map(([n, b]) => [n, Buffer.from(b)])),
  };
}
export function winioNoticeVariantSuffixData(variant) {
  need(
    variant === undefined || ["orbwin-wsl2", "orbwin-winio-custody"].includes(variant),
    "unknown local source variant",
  );
  return variant ? "#" + variant : "";
}
export function classifyWinioNoticeData(dependency, isRootConsumer, expectedSourceDir) {
  if (dependency.Path !== winioIdentity.module) return null;
  if (isRootConsumer) {
    need(dependency.Version === winioIdentity.version, "root Winio notice version");
    const r = dependency.Replace;
    need(
      r &&
        absolute(r.Path) === absolute(expectedSourceDir) &&
        (!r.Dir || absolute(r.Dir) === absolute(expectedSourceDir)) &&
        (!r.Version || r.Version === "(devel)"),
      "root Winio notice source",
    );
    return "orbwin-winio-custody";
  }
  need(
    !dependency.Replace || (dependency.Replace.Version && dependency.Replace.Version !== "(devel)"),
    "independent Winio local source needs its own review",
  );
  return null;
}

export {
  preflightWinioBuildInput,
  prepareWinioBuildInput,
  verifyWinioBuildInput,
  verifyPortableWinioBuild,
  stageWinioSourceOffer,
  winioBuildEnvironment,
} from "./winio-build-input-fs.mjs";
