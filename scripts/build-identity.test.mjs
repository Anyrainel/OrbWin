import test from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve, dirname, basename } from "node:path";
import { verifyBuild, sha256 } from "./build-identity.mjs";
test("runtime proof rejects a changed binary or patch even at the same source commit", (t) => {
  const root = mkdtempSync(join(tmpdir(), "orbwin-build-test-"));
  t.after(() => {
    const target = resolve(root);
    assert.equal(dirname(target), resolve(tmpdir()));
    assert.match(basename(target), /^orbwin-build-test-/);
    rmSync(target, { recursive: true, force: true });
  });
  mkdirSync(join(root, "artifacts/backend"), { recursive: true });
  writeFileSync(join(root, "change.patch"), "expected patch");
  writeFileSync(
    join(root, "guest-packages.lock.json"),
    "expected package pins",
  );
  writeFileSync(
    join(root, "upstream.lock.json"),
    JSON.stringify({ commit: "pin", patches: ["change.patch"] }),
  );
  writeFileSync(join(root, "artifacts/backend/rdd.exe"), "expected binary");
  writeFileSync(
    join(root, "artifacts/backend/build-manifest.json"),
    JSON.stringify({
      sourceCommit: "pin",
      guestPackagesLockSha256: sha256("expected package pins"),
      binarySha256: sha256("expected binary"),
      patches: [{ path: "change.patch", sha256: sha256("expected patch") }],
    }),
  );
  assert.equal(verifyBuild(root).sourceCommit, "pin");
  writeFileSync(join(root, "artifacts/backend/rdd.exe"), "different binary");
  assert.throws(() => verifyBuild(root), /binary differs/);
  writeFileSync(join(root, "artifacts/backend/rdd.exe"), "expected binary");
  writeFileSync(join(root, "change.patch"), "different patch");
  assert.throws(() => verifyBuild(root), /differs from patch/);
  writeFileSync(join(root, "change.patch"), "expected patch");
  writeFileSync(join(root, "guest-packages.lock.json"), "changed package pins");
  assert.throws(() => verifyBuild(root), /guest package lock differs/);
});
