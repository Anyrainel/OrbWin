// Real provenance + in-memory pin refusals only. No source/cache tampering.
import assert from "node:assert/strict";
import { createHash, randomUUID } from "node:crypto";
import {
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  copyBuildxProvenance,
  verifyBuildxSource,
} from "./buildx-provenance.mjs";
const root = fileURLToPath(new URL("../", import.meta.url));
const folder = resolve(
  root,
  "artifacts/qualification",
  `p2-buildx-provenance-${randomUUID()}`,
);
mkdirSync(folder, { recursive: true });
const hash = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");
const lockPath = resolve(root, "runtime-tools.lock.json"),
  pins = JSON.parse(readFileSync(lockPath)),
  lockHash = hash(lockPath);
function cacheSnapshot() {
  const base = resolve(root, "artifacts/runtime-tools"),
    files = {};
  function walk(prefix = "") {
    for (const entry of readdirSync(resolve(base, prefix), {
      withFileTypes: true,
    })) {
      const name = prefix ? `${prefix}/${entry.name}` : entry.name,
        path = resolve(base, name),
        info = lstatSync(path);
      assert(!info.isSymbolicLink(), "Cache redirect refused");
      if (info.isDirectory()) walk(name);
      else {
        assert(info.isFile());
        files[name] = { bytes: info.size, sha256: hash(path) };
      }
    }
  }
  walk();
  return files;
}
const proof = {
  schema: 1,
  started: new Date().toISOString(),
  runtimeLockSha256: lockHash,
  scope:
    "Real final Buildx verify/copy and in-memory pin drift; no source tamper, engines, providers, runtime activation or buildMissing",
  refusals: [],
};
const save = () =>
  writeFileSync(
    resolve(folder, "proof.json"),
    JSON.stringify(proof, null, 2) + "\n",
  );
try {
  proof.cacheBefore = cacheSnapshot();
  save();
  const verified = verifyBuildxSource(root, pins, { buildMissing: false });
  copyBuildxProvenance(root, pins, resolve(folder, "copied-provenance"));
  proof.manifestSha256 = hash(verified.manifestPath);
  proof.version = verified.manifest.version;
  proof.legal = verified.legal.map((r) => ({
    target: r.target,
    files: r.files.length,
    indexSha256: hash(r.indexPath),
    modules: r.index.modules.length,
    missingRootLicenses: r.index.modulesWithoutRootLicense,
  }));
  const wrong = "0".repeat(64),
    source = (p) => p.localSources.buildx,
    tool = (p) => p.tools.find((t) => t.localSource?.name === "buildx");
  const cases = [
    [
      "unapproved recipe",
      (p) => {
        source(p).recipe = "scripts/unapproved.mjs";
      },
    ],
    [
      "recipe hash",
      (p) => {
        source(p).buildScriptSha256 = wrong;
      },
    ],
    [
      "helper hash",
      (p) => {
        source(p).provenanceScriptSha256 = wrong;
      },
    ],
    [
      "baseline hash",
      (p) => {
        source(p).sourceBaselineSha256 = wrong;
      },
    ],
    [
      "patch hash",
      (p) => {
        source(p).patchSha256 = wrong;
      },
    ],
    [
      "compiler hash",
      (p) => {
        source(p).goExecutableSha256 = wrong;
      },
    ],
    [
      "manifest hash",
      (p) => {
        source(p).manifestSha256 = wrong;
      },
    ],
    [
      "source commit",
      (p) => {
        source(p).commit = "0".repeat(40);
      },
    ],
    [
      "target swap",
      (p) => {
        tool(p).localSource.target = "linuxAmd64";
      },
    ],
    [
      "binary hash",
      (p) => {
        tool(p).sha256 = wrong;
      },
    ],
  ];
  for (const [name, mutate] of cases) {
    const copy = structuredClone(pins);
    mutate(copy);
    let error;
    try {
      verifyBuildxSource(root, copy, { buildMissing: false });
    } catch (e) {
      error = e;
    }
    assert(error, `Drift accepted: ${name}`);
    proof.refusals.push({
      name,
      refused: true,
      code: error.code ?? error.name,
    });
    save();
  }
  verifyBuildxSource(root, pins, { buildMissing: false });
  proof.cacheAfter = cacheSnapshot();
  assert.deepEqual(proof.cacheAfter, proof.cacheBefore);
  assert.equal(hash(lockPath), lockHash);
  proof.cacheUnchanged = true;
  proof.realVerify = true;
  proof.copy = true;
  proof.success = true;
} catch (error) {
  proof.success = false;
  proof.failure = { code: error.code ?? error.name, message: error.message };
  process.exitCode = 1;
} finally {
  if (!proof.cacheAfter && existsSync(resolve(root, "artifacts/runtime-tools")))
    proof.cacheAfter = cacheSnapshot();
  proof.finished = new Date().toISOString();
  save();
  console.log(resolve(folder, "proof.json"));
}
