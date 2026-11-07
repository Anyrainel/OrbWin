import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
  existsSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import {
  stageVerifiedGuestFiles,
  captureBackendSourceFiles,
  verifyBackendSourceFiles,
} from "./backend-guest-input.mjs";
const hash = (b) => createHash("sha256").update(b).digest("hex");
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "orbwin-guest-input-"));
  t.after(() => rmSync(root, { recursive: true }));
  mkdirSync(join(root, "cache"));
  mkdirSync(join(root, "selected"));
  writeFileSync(join(root, "cache/pkg.rpm"), "verified offline RPM");
  writeFileSync(join(root, "selected/pkg.rpm"), "selected old package");
  const source = join(root, "cache/pkg.rpm"),
    output = join(root, "fresh");
  return {
    root,
    source,
    output,
    asset: {
      name: "pkg",
      path: "packages/pkg.rpm",
      source,
      sha256: hash(readFileSync(source)),
    },
  };
}
test("offline staging verifies fresh copies and leaves cache metadata and selected packages intact", (t) => {
  const f = fixture(t),
    before = statSync(f.source).mtimeMs;
  const files = stageVerifiedGuestFiles(f.output, [f.asset]);
  assert.equal(files.length, 1);
  assert.equal(
    readFileSync(join(f.output, "packages/pkg.rpm"), "utf8"),
    "verified offline RPM",
  );
  assert.equal(
    readFileSync(join(f.root, "selected/pkg.rpm"), "utf8"),
    "selected old package",
  );
  assert.equal(statSync(f.source).mtimeMs, before);
  writeFileSync(join(f.output, "packages/pkg.rpm"), "edited copy");
  assert.equal(readFileSync(f.source, "utf8"), "verified offline RPM");
  assert.throws(() => stageVerifiedGuestFiles(f.output, [f.asset]), /exist/i);
});
test("missing, corrupt, escaping and colliding inputs refuse before creating any output", (t) => {
  const f = fixture(t);
  for (const assets of [
    [{ ...f.asset, source: join(f.root, "missing") }],
    [{ ...f.asset, sha256: "0".repeat(64) }],
    [{ ...f.asset, path: "../selected/pkg.rpm" }],
    [f.asset, { ...f.asset, path: "PACKAGES/PKG.RPM" }],
  ]) {
    assert.throws(() => stageVerifiedGuestFiles(f.output, assets));
    assert.equal(existsSync(f.output), false);
  }
  assert.equal(
    readFileSync(join(f.root, "selected/pkg.rpm"), "utf8"),
    "selected old package",
  );
});
test("redirected cache ancestors refuse instead of reading outside the cache", (t) => {
  const f = fixture(t);
  symlinkSync(
    join(f.root, "cache"),
    join(f.root, "redirect"),
    process.platform === "win32" ? "junction" : "dir",
  );
  assert.throws(
    () =>
      stageVerifiedGuestFiles(f.output, [
        { ...f.asset, source: join(f.root, "redirect/pkg.rpm") },
      ]),
    /Redirected/,
  );
  assert.equal(existsSync(f.output), false);
});
test("source verification retains empty canonical placeholders and rejects changed or missing canonical paths", (t) => {
  const f = fixture(t),
    names = ["cache/pkg.rpm", "empty.go"];
  writeFileSync(join(f.root, "empty.go"), "");
  const snapshot = captureBackendSourceFiles(f.root, names);
  mkdirSync(join(f.root, "generated"));
  writeFileSync(join(f.root, "generated/output"), "derived build output");
  verifyBackendSourceFiles(f.root, snapshot, names);
  writeFileSync(
    join(f.root, "empty.go"),
    "generated overwrite of tracked placeholder",
  );
  assert.throws(
    () => verifyBackendSourceFiles(f.root, snapshot, names),
    /source changed/,
  );
  writeFileSync(join(f.root, "empty.go"), "");
  assert.throws(
    () => verifyBackendSourceFiles(f.root, snapshot, ["cache/pkg.rpm"]),
    /source changed/,
  );
  assert.throws(() => captureBackendSourceFiles(f.root, ["../outside"]));
});
