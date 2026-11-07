import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
  existsSync,
  readdirSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test from "node:test";
import {
  buildOutput,
  buildArguments,
  buildTestArguments,
  freshOutputPath,
} from "./build-output.mjs";

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "orbwin-build-output-"));
  const exactRoot = realpathSync(root);
  t.after(() => {
    assert.equal(realpathSync(root), exactRoot);
    rmSync(exactRoot, { recursive: true });
  });
  mkdirSync(join(root, "artifacts/backend"), { recursive: true });
  writeFileSync(
    join(root, "artifacts/backend/rdd.exe"),
    "retained active image",
  );
  return root;
}

test("candidate builds use a fresh output without replacing active artifacts", (t) => {
  const root = fixture(t);
  const selected = buildOutput(
    root,
    ["--with-distro", "--output-dir", "artifacts/candidate"],
    ["--with-distro"],
  );
  assert.equal(selected.directory, resolve(root, "artifacts/candidate"));
  assert.ok(selected.flags.has("--with-distro"));
  writeFileSync(join(selected.directory, "rdd.exe"), "new candidate");
  assert.equal(
    readFileSync(join(root, "artifacts/backend/rdd.exe"), "utf8"),
    "retained active image",
  );
  assert.throws(
    () => buildOutput(root, ["--output-dir", "artifacts/candidate"]),
    /already exists/,
  );
  assert.throws(
    () => buildOutput(root, ["--output-dir", "artifacts/backend"]),
    /already exists/,
  );
});

test("output parent junction into a retained source tree refuses before creation", (t) => {
  const root = fixture(t),
    source = join(root, "artifacts/buildx/source");
  mkdirSync(source, { recursive: true });
  const sentinel = join(source, "go.mod");
  writeFileSync(sentinel, "retained source\n");
  symlinkSync(
    source,
    join(root, "artifacts/source-alias"),
    process.platform === "win32" ? "junction" : "dir",
  );
  assert.throws(
    () => freshOutputPath(root, "artifacts/source-alias/candidate"),
    /redirect/,
  );
  assert.equal(existsSync(join(source, "candidate")), false);
  assert.deepEqual(readdirSync(source), ["go.mod"]);
  assert.equal(readFileSync(sentinel, "utf8"), "retained source\n");
  const normal = buildOutput(root, [
    "--output-dir",
    "artifacts/ordinary-candidate",
  ]);
  assert.equal(
    realpathSync(normal.directory),
    resolve(root, "artifacts/ordinary-candidate"),
  );
});

test("invalid build selectors refuse rather than falling back to active output", (t) => {
  const root = fixture(t);
  for (const args of [
    [],
    ["--output-dir"],
    ["--output-dir", "--with-distro"],
    ["--unknown"],
    ["--with-distro", "--with-distro"],
    ["--output-dir", "artifacts/x", "--output-dir", "artifacts/y"],
    ["--output-dir", "artifacts/../outside"],
  ]) {
    assert.throws(() => buildOutput(root, args, ["--with-distro"]));
  }
  assert.equal(
    readFileSync(join(root, "artifacts/backend/rdd.exe"), "utf8"),
    "retained active image",
  );
  const outside = join(root, "outside-artifacts");
  mkdirSync(outside);
  symlinkSync(
    outside,
    join(root, "artifacts/redirect"),
    process.platform === "win32" ? "junction" : "dir",
  );
  assert.throws(
    () => buildOutput(root, ["--output-dir", "artifacts/redirect/candidate"]),
    /inside this repository/,
  );
});
test("explicit build-only omits required tests without changing default test policy", () => {
  const args = buildArguments(
    ["--output-dir", "artifacts/new", "--build-only"],
    ["--build-only"],
  );
  assert.equal(args.flags.has("--build-only"), true);
  assert.deepEqual(buildTestArguments(true), []);
  assert.deepEqual(buildTestArguments(false), [["test", "./..."]]);
  assert.throws(
    () => buildArguments(["--build-only"], ["--build-only"]),
    /fresh/,
  );
});
