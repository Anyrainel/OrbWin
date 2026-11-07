// Read-only integration guards; copies legal evidence only into a fresh receipt.
import assert from "node:assert/strict";
import {
  readFileSync,
  writeFileSync,
  mkdirSync,
  symlinkSync,
  lstatSync,
} from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createHash, randomUUID } from "node:crypto";
import {
  verifyComposeSource,
  copyComposeProvenance,
  validateRuntimeToolPaths,
} from "./stage-runtime-tools.mjs";
const root = fileURLToPath(new URL("../", import.meta.url));
const hash = (p) => createHash("sha256").update(readFileSync(p)).digest("hex");
const lock = JSON.parse(readFileSync(resolve(root, "runtime-tools.lock.json")));
const output = resolve(root, `artifacts/compose/integration-${randomUUID()}`);
mkdirSync(output);
const cacheFiles = ["manifest.json", ...lock.tools.map((tool) => tool.path)];
const baseline = () =>
  Object.fromEntries(
    cacheFiles.map((name) => [
      name,
      hash(resolve(root, "artifacts/runtime-tools", name)),
    ]),
  );
const before = baseline(),
  proof = {
    schema: 1,
    checks: [],
    scope:
      "Read-only source guards and isolated legal copy; runtime/package staging not invoked",
  };
try {
  const verified = verifyComposeSource(root, lock);
  assert.equal(verified.legal.length, 2);
  proof.checks.push("valid source/compiler/binary/legal manifest");
  for (const [name, mutate] of [
    ["arbitrary command kind", (p) => (p.localSources.compose.kind = "shell")],
    ["arbitrary recipe", (p) => (p.localSources.compose.recipe = "evil.mjs")],
    [
      "stale script",
      (p) => (p.localSources.compose.buildScriptSha256 = "0".repeat(64)),
    ],
    [
      "stale patch",
      (p) => (p.localSources.compose.patchSha256 = "0".repeat(64)),
    ],
    [
      "stale compiler",
      (p) => (p.localSources.compose.goExecutableSha256 = "0".repeat(64)),
    ],
    [
      "stale upstream lock",
      (p) => (p.localSources.compose.upstreamLockSha256 = "0".repeat(64)),
    ],
    [
      "stale manifest",
      (p) => (p.localSources.compose.manifestSha256 = "0".repeat(64)),
    ],
    [
      "wrong source commit",
      (p) => (p.localSources.compose.commit = "0".repeat(40)),
    ],
    [
      "foreign artifact target",
      (p) =>
        (p.tools.find((t) => t.localSource?.name === "compose").localSource.target = "foreign"),
    ],
    [
      "wrong binary checksum",
      (p) => (p.tools.find((t) => t.localSource?.name === "compose").sha256 = "0".repeat(64)),
    ],
  ]) {
    const changed = structuredClone(lock);
    mutate(changed);
    assert.throws(() => verifyComposeSource(root, changed), undefined, name);
    proof.checks.push("refused " + name);
  }
  for (const path of [
    "",
    "../outside.exe",
    "bin/../../outside.exe",
    "/outside.exe",
    "C:/outside.exe",
    "\\\\server\\share\\outside.exe",
    "bin\\..\\outside.exe",
    "bin/file.exe:stream",
    "bin//file.exe",
  ]) {
    const changed = structuredClone(lock);
    changed.tools[0].path = path;
    assert.throws(
      () => validateRuntimeToolPaths(root, changed),
      undefined,
      JSON.stringify(path),
    );
    proof.checks.push("refused malformed tool path " + JSON.stringify(path));
  }
  const outside = resolve(output, "outside"),
    broken = resolve(output, "broken-link-copy");
  mkdirSync(outside);
  mkdirSync(broken);
  const sentinel = resolve(outside, "sentinel.txt"),
    sentinelBytes = "outside sentinel must stay intact\n";
  writeFileSync(sentinel, sentinelBytes);
  const missing = resolve(outside, "must-not-be-created.json");
  symlinkSync(missing, resolve(broken, "compose-manifest.json"), "file");
  assert(lstatSync(resolve(broken, "compose-manifest.json")).isSymbolicLink());
  assert.throws(
    () => copyComposeProvenance(root, lock, broken),
    /ordinary file/,
  );
  assert.equal(readFileSync(sentinel, "utf8"), sentinelBytes);
  assert.throws(() => lstatSync(missing), { code: "ENOENT" });
  proof.checks.push(
    "dangling file symlink refused; outside sentinel unchanged and target not created",
  );
  const existingLink = resolve(output, "existing-link-copy");
  mkdirSync(existingLink);
  symlinkSync(sentinel, resolve(existingLink, "compose-manifest.json"), "file");
  assert.throws(
    () => copyComposeProvenance(root, lock, existingLink),
    /ordinary file/,
  );
  assert.equal(readFileSync(sentinel, "utf8"), sentinelBytes);
  proof.checks.push(
    "existing file symlink refused; outside sentinel unchanged",
  );
  copyComposeProvenance(root, lock, resolve(output, "legal"));
  assert.deepEqual(baseline(), before);
  proof.checks.push("all prior staged runtime bytes unchanged");
  proof.manifestSha256 = hash(verified.manifestPath);
  proof.success = true;
} catch (error) {
  proof.success = false;
  proof.failure = String(error.stack || error);
  process.exitCode = 1;
} finally {
  writeFileSync(
    resolve(output, "proof.json"),
    JSON.stringify(proof, null, 2) + "\n",
  );
  console.log(resolve(output, "proof.json"));
}
