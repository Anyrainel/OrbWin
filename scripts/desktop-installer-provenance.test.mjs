import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  desktopInstallerPins,
  verifyDesktopInstallerSource,
  stageDesktopInstallerNotices,
} from "./desktop-installer-provenance.mjs";
const repository = fileURLToPath(new URL("../", import.meta.url));
function fixture(run) {
  const parent = fs.mkdtempSync(
    path.join(os.tmpdir(), "orbwin-installer-notices-"),
  );
  const root = path.join(parent, "root");
  for (const file of [
    ...Object.keys(desktopInstallerPins.files).map(
      (name) => "src-tauri/" + name,
    ),
    "src-tauri/tauri.package.conf.json",
    "node_modules/@tauri-apps/cli/package.json",
  ]) {
    const target = path.join(root, file);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.copyFileSync(path.join(repository, file), target);
  }
  const destination = path.join(root, "artifacts/notices/installer-template");
  try {
    run({ parent, root, destination });
  } finally {
    assert.ok(
      path.resolve(parent).startsWith(path.resolve(os.tmpdir()) + path.sep),
    );
    fs.rmSync(parent, { recursive: true, force: true });
  }
}
test("exact pinned installer sources/licenses stage twice including readonly output without touching source attributes", () =>
  fixture(({ root, destination }) => {
    verifyDesktopInstallerSource(root);
    const source = path.join(root, "src-tauri/installer-template-LICENSE_MIT");
    fs.chmodSync(source, 0o444);
    const before = fs.statSync(source, { bigint: true });
    const first = stageDesktopInstallerNotices(root, destination);
    assert.equal(first.version, "2.12.4");
    assert.equal(first.notices.length, 2);
    assert.equal(first.sourceFiles.length, 5);
    for (const name of Object.keys(desktopInstallerPins.files))
      fs.chmodSync(path.join(destination, name), 0o444);
    assert.deepEqual(stageDesktopInstallerNotices(root, destination), first);
    const after = fs.statSync(source, { bigint: true });
    for (const field of ["dev", "ino", "mode", "size", "mtimeNs", "ctimeNs"])
      assert.equal(after[field], before[field]);
  }));
test("modified template/provenance/license or mismatched CLI fails before output creation", () => {
  for (const relative of [
    "src-tauri/installer-template.nsi",
    "src-tauri/installer-template.provenance.json",
    "src-tauri/installer-template-LICENSE_MIT",
  ])
    fixture(({ root, destination }) => {
      fs.appendFileSync(path.join(root, relative), "changed");
      assert.throws(
        () => stageDesktopInstallerNotices(root, destination),
        /Changed pinned/,
      );
      assert.equal(fs.existsSync(destination), false);
    });
  fixture(({ root, destination }) => {
    fs.writeFileSync(
      path.join(root, "node_modules/@tauri-apps/cli/package.json"),
      JSON.stringify({ version: "99.0.0" }),
    );
    assert.throws(
      () => stageDesktopInstallerNotices(root, destination),
      /actual pinned Tauri CLI/,
    );
    assert.equal(fs.existsSync(destination), false);
  });
});
test("redirected parent or dangling target refuses without following links outside", () =>
  fixture(({ parent, root, destination }) => {
    const outside = path.join(parent, "outside");
    fs.mkdirSync(outside);
    fs.writeFileSync(path.join(outside, "sentinel"), "retain");
    fs.symlinkSync(outside, path.join(root, "artifacts"), "junction");
    assert.throws(
      () => stageDesktopInstallerNotices(root, destination),
      /Redirected/,
    );
    assert.deepEqual(fs.readdirSync(outside), ["sentinel"]);
    fs.unlinkSync(path.join(root, "artifacts"));
    fs.mkdirSync(destination, { recursive: true });
    const target = path.join(destination, "installer-template.upstream.nsi");
    fs.symlinkSync(path.join(outside, "missing"), target, "junction");
    assert.throws(
      () => stageDesktopInstallerNotices(root, destination),
      /Unexpected installer attribution target/,
    );
    assert.equal(
      fs.readFileSync(path.join(outside, "sentinel"), "utf8"),
      "retain",
    );
  }));
test("escaping destination and hardlinked/different target refuse instead of adopting or overwriting", () =>
  fixture(({ parent, root, destination }) => {
    assert.throws(
      () => stageDesktopInstallerNotices(root, path.join(parent, "escape")),
      /inside generated artifacts/,
    );
    fs.mkdirSync(destination, { recursive: true });
    const target = path.join(destination, "installer-template.upstream.nsi"),
      outside = path.join(parent, "sentinel");
    fs.writeFileSync(outside, "retain");
    fs.linkSync(outside, target);
    assert.throws(
      () => stageDesktopInstallerNotices(root, destination),
      /Unexpected installer attribution target/,
    );
    assert.equal(fs.readFileSync(outside, "utf8"), "retain");
    fs.unlinkSync(target);
    fs.writeFileSync(target, "unexpected original");
    assert.throws(
      () => stageDesktopInstallerNotices(root, destination),
      /retained for review/,
    );
    assert.equal(fs.readFileSync(target, "utf8"), "unexpected original");
  }));
