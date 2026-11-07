import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { copyCredentialLicense } from "./build-credentials.mjs";
const moduleName = "golang.org/x/sys";
function fixture(run) {
  const folder = fs.mkdtempSync(
    path.join(os.tmpdir(), "orbwin-credential-license-"),
  );
  const root = path.join(folder, "build"),
    cache = path.join(folder, "cache");
  fs.mkdirSync(root);
  fs.mkdirSync(cache);
  const source = path.join(cache, "LICENSE");
  fs.writeFileSync(source, "new upstream license\n");
  const directory = path.join(root, "artifacts/backend/licenses/credentials"),
    target = path.join(directory, "golang.org_x_sys-LICENSE");
  try {
    run({ folder, root, cache, source, directory, target });
  } finally {
    assert.ok(
      path.resolve(folder).startsWith(path.resolve(os.tmpdir()) + path.sep),
    );
    fs.rmSync(folder, { recursive: true, force: true });
  }
}
test("repeated license generation replaces old readonly target without changing source attributes", () =>
  fixture(({ root, source, directory, target }) => {
    fs.mkdirSync(directory, { recursive: true });
    fs.writeFileSync(target, "old upstream license\n");
    fs.chmodSync(target, 0o444);
    fs.chmodSync(source, 0o444);
    const before = fs.statSync(source, { bigint: true });
    copyCredentialLicense(root, source, moduleName);
    assert.equal(fs.readFileSync(target, "utf8"), "new upstream license\n");
    assert.notEqual(fs.statSync(target).mode & 0o200, 0);
    fs.chmodSync(target, 0o444);
    const same = fs.statSync(target, { bigint: true });
    copyCredentialLicense(root, source, moduleName);
    const unchanged = fs.statSync(target, { bigint: true });
    for (const key of ["dev", "ino", "size", "mode", "mtimeNs", "ctimeNs"])
      assert.equal(
        unchanged[key],
        same[key],
        "identical readonly output: " + key,
      );
    const after = fs.statSync(source, { bigint: true });
    for (const key of ["dev", "ino", "size", "mode", "mtimeNs", "ctimeNs"])
      assert.equal(after[key], before[key], key);
  }));
test("unknown module and redirected parent refuse without changing outside sentinel", () =>
  fixture(({ folder, root, source }) => {
    const outside = path.join(folder, "outside");
    fs.mkdirSync(outside);
    const sentinel = path.join(outside, "sentinel");
    fs.writeFileSync(sentinel, "retain");
    assert.throws(
      () => copyCredentialLicense(root, source, "../../escape"),
      /Unexpected/,
    );
    fs.symlinkSync(outside, path.join(root, "artifacts"), "junction");
    assert.throws(
      () => copyCredentialLicense(root, source, moduleName),
      /redirected/,
    );
    assert.deepEqual(fs.readdirSync(outside), ["sentinel"]);
    assert.equal(fs.readFileSync(sentinel, "utf8"), "retain");
  }));
test("dangling target junction, directory, and hardlink are never accepted as absent output", () =>
  fixture(({ folder, root, source, directory, target }) => {
    fs.mkdirSync(directory, { recursive: true });
    fs.symlinkSync(path.join(folder, "missing"), target, "junction");
    assert.throws(
      () => copyCredentialLicense(root, source, moduleName),
      /regular/,
    );
    fs.unlinkSync(target);
    fs.mkdirSync(target);
    assert.throws(
      () => copyCredentialLicense(root, source, moduleName),
      /regular/,
    );
    fs.rmdirSync(target);
    const outside = path.join(folder, "sentinel");
    fs.writeFileSync(outside, "retain");
    fs.linkSync(outside, target);
    assert.throws(
      () => copyCredentialLicense(root, source, moduleName),
      /regular/,
    );
    assert.equal(fs.readFileSync(outside, "utf8"), "retain");
  }));
test("redirected source and oversized source refuse before destination creation", () =>
  fixture(({ folder, root, cache, source, directory }) => {
    const alias = path.join(folder, "cache-alias");
    fs.symlinkSync(cache, alias, "junction");
    assert.throws(
      () =>
        copyCredentialLicense(root, path.join(alias, "LICENSE"), moduleName),
      /redirected/,
    );
    fs.writeFileSync(source, Buffer.alloc(1024 * 1024 + 1));
    assert.throws(
      () => copyCredentialLicense(root, source, moduleName),
      /too large/,
    );
    assert.equal(fs.existsSync(directory), false);
  }));

test("fresh explicit output creates only its license tree and preserves selected output", () =>
  fixture(({ root, source, directory, target }) => {
    fs.mkdirSync(directory, { recursive: true });
    fs.writeFileSync(target, "selected output must remain\n");
    const selectedBefore = fs.statSync(target, { bigint: true });
    const fresh = path.join(root, "artifacts", "fresh-credentials");
    fs.mkdirSync(fresh);
    const output = path.join(fresh, "licenses", "credentials");
    copyCredentialLicense(root, source, moduleName, output);
    assert.equal(
      fs.readFileSync(path.join(output, "golang.org_x_sys-LICENSE"), "utf8"),
      "new upstream license\n",
    );
    assert.equal(
      fs.readFileSync(target, "utf8"),
      "selected output must remain\n",
    );
    const selectedAfter = fs.statSync(target, { bigint: true });
    for (const key of ["dev", "ino", "size", "mode", "mtimeNs", "ctimeNs"])
      assert.equal(selectedAfter[key], selectedBefore[key], key);
    assert.deepEqual(fs.readdirSync(fresh), ["licenses"]);
  }));

test("fresh explicit output does not create the selected backend directory", () =>
  fixture(({ root, source }) => {
    const output = path.join(
      root,
      "artifacts",
      "candidate",
      "licenses",
      "credentials",
    );
    copyCredentialLicense(root, source, moduleName, output);
    assert.equal(fs.existsSync(path.join(root, "artifacts", "backend")), false);
    assert.deepEqual(fs.readdirSync(output), ["golang.org_x_sys-LICENSE"]);
  }));

test("explicit output refuses escaped or redirected paths without outside writes", () =>
  fixture(({ folder, root, source }) => {
    const outside = path.join(folder, "outside");
    fs.mkdirSync(outside);
    assert.throws(
      () => copyCredentialLicense(root, source, moduleName, outside),
      /below the build root/,
    );
    assert.throws(
      () => copyCredentialLicense(root, source, moduleName, "relative-output"),
      /Absolute/,
    );
    const fresh = path.join(root, "fresh");
    fs.mkdirSync(fresh);
    fs.symlinkSync(outside, path.join(fresh, "licenses"), "junction");
    assert.throws(
      () =>
        copyCredentialLicense(
          root,
          source,
          moduleName,
          path.join(fresh, "licenses", "credentials"),
        ),
      /redirected/,
    );
    assert.deepEqual(fs.readdirSync(outside), []);
    assert.equal(fs.existsSync(path.join(root, "artifacts")), false);
  }));
