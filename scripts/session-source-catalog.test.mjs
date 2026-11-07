import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import {
  sessionSourceFolders,
  sessionSourceNames,
  verifySessionSources,
} from "./session-source-catalog.mjs";

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), "orbwin-source-catalog-"));
  t.after(() => rmSync(directory, { recursive: true }));
  for (const folder of sessionSourceFolders) {
    mkdirSync(join(directory, folder), { recursive: true });
  }
  for (const name of [
    "go.mod",
    "go.sum",
    "main.go",
    "internal/lanworker/worker.go",
  ]) {
    writeFileSync(join(directory, name), `fixture ${name}`);
  }
  for (const name of ["lan-helper/lan.manifest", "lan-helper/lan.rc"])
    writeFileSync(join(directory, name), `fixed resource ${name}`);
  const names = sessionSourceNames(directory);
  const sources = Object.fromEntries(
    names.map((name) => [
      name,
      createHash("sha256")
        .update(readFileSync(join(directory, name)))
        .digest("hex"),
    ]),
  );
  return { directory, manifest: { sources }, names };
}

test("packaging refuses a manifest that omits a linked LAN package", (t) => {
  const { directory, manifest, names } = fixture(t);
  verifySessionSources(directory, manifest, names);
  delete manifest.sources["internal/lanworker/worker.go"];
  assert.throws(
    () => verifySessionSources(directory, manifest, names),
    /source set changed/,
  );
});

test("new build inputs and changed bytes invalidate an older manifest", (t) => {
  const { directory, manifest, names } = fixture(t);
  writeFileSync(join(directory, "main.go"), "changed main");
  assert.throws(
    () => verifySessionSources(directory, manifest, names),
    /Stale helper source/,
  );
  writeFileSync(
    join(directory, "internal/lanworker/transport.go"),
    "new transport",
  );
  assert.throws(
    () =>
      verifySessionSources(directory, manifest, sessionSourceNames(directory)),
    /source set changed/,
  );
});

test("unexpected manifest paths refuse before any path is read", (t) => {
  const { directory, manifest, names } = fixture(t);
  manifest.sources["../foreign-missing-secret"] = "0".repeat(64);
  assert.throws(
    () => verifySessionSources(directory, manifest, names),
    /source set changed/,
  );
});

test("Go ignored and test files do not invalidate a production catalog", (t) => {
  const { directory, manifest, names } = fixture(t);
  for (const name of ["main_test.go", "_ignored.go", ".ignored.go"]) {
    writeFileSync(join(directory, name), "not a production input");
  }
  assert.deepEqual(sessionSourceNames(directory), names);
  verifySessionSources(directory, manifest, sessionSourceNames(directory));
});
test("cold payload and both guest entry directories belong to the exact source set", (t) => {
  const { directory, manifest } = fixture(t);
  for (const folder of [
    "internal/coldassets",
    "service-tools",
    "trust-helper",
  ]) {
    writeFileSync(join(directory, folder, "new_entry.go"), "new source input");
    const names = sessionSourceNames(directory);
    assert.ok(names.includes(`${folder}/new_entry.go`));
    assert.throws(
      () => verifySessionSources(directory, manifest, names),
      /source set changed/,
    );
  }
});
