import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import {
  verifyBuildxBinary,
  verifyBuildxLegalInventory,
  verifyBuildxSourceFiles,
} from "./buildx-provenance.mjs";
const hash = (data) => createHash("sha256").update(data).digest("hex");
function fixture(t) {
  const dir = mkdtempSync(join(tmpdir(), "orbwin-buildx-provenance-"));
  t.after(() => {
    assert.equal(dirname(dir), resolve(tmpdir()));
    assert(basename(dir).startsWith("orbwin-buildx-provenance-"));
    rmSync(dir, { recursive: true, force: true });
  });
  return dir;
}
test("restored source refuses changed bytes and untracked extra code", (t) => {
  const dir = fixture(t),
    expected = { "main.go": hash("reviewed source") };
  writeFileSync(join(dir, "main.go"), "reviewed source");
  verifyBuildxSourceFiles(dir, expected);
  writeFileSync(join(dir, "main.go"), "altered source");
  assert.throws(() => verifyBuildxSourceFiles(dir, expected));
  writeFileSync(join(dir, "main.go"), "reviewed source");
  writeFileSync(join(dir, "extra.go"), "unreviewed source");
  assert.throws(() => verifyBuildxSourceFiles(dir, expected));
});
test("source redirects fail even when their contents would match", (t) => {
  const dir = fixture(t),
    source = join(dir, "source"),
    foreign = join(dir, "foreign");
  mkdirSync(source);
  mkdirSync(foreign);
  writeFileSync(join(foreign, "same.go"), "same source");
  try {
    symlinkSync(
      foreign,
      join(source, "nested"),
      process.platform === "win32" ? "junction" : "dir",
    );
  } catch (error) {
    if (["EPERM", "EACCES"].includes(error.code))
      return t.skip("Host cannot create a test link without elevation");
    throw error;
  }
  assert.throws(() =>
    verifyBuildxSourceFiles(source, { "nested/same.go": hash("same source") }),
  );
});
test("target swaps and changed candidate binaries fail before acceptance", (t) => {
  const dir = fixture(t),
    bytes = "reviewed Windows candidate",
    sha = hash(bytes);
  writeFileSync(join(dir, "docker-buildx.exe"), bytes);
  const tool = {
      path: "docker-cli-plugins/docker-buildx.exe",
      sha256: sha,
      localSource: { name: "buildx", target: "windowsAmd64" },
    },
    record = {
      path: "docker-buildx.exe",
      platform: "windows/amd64",
      binarySha256: sha,
    };
  verifyBuildxBinary(dir, tool, "windowsAmd64", record);
  assert.throws(() =>
    verifyBuildxBinary(
      dir,
      { ...tool, localSource: { name: "buildx", target: "linuxAmd64" } },
      "windowsAmd64",
      record,
    ),
  );
  assert.throws(() =>
    verifyBuildxBinary(dir, tool, "windowsAmd64", {
      ...record,
      path: "../docker-buildx.exe",
    }),
  );
  writeFileSync(join(dir, "docker-buildx.exe"), "foreign replacement");
  assert.throws(() => verifyBuildxBinary(dir, tool, "windowsAmd64", record));
});
test("legal metadata cannot excuse missing, changed or redirected license bytes", (t) => {
  const dir = fixture(t),
    name = "modules/fixture/LICENSE",
    path = join(dir, name);
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, "license bytes");
  const file = { path: name, bytes: 13, sha256: hash(readFileSync(path)) },
    sha = "a".repeat(64),
    index = {
      schema: 1,
      binarySha256: sha,
      platform: "windows/amd64",
      sourceCommit: "0dbe87f36ed472fe36b6796c3f70b4210a731fc5",
      patchSha256:
        "496f3d23feb5c5e13a27883062a11b07a14c6ffbceb5242c944dde5917e52cf5",
      embeddedModuleMetadataVerified: true,
      modulesWithoutRootLicense: [],
      modules: [{ rootLicensePresent: true, files: [file] }],
      toolchain: [],
      source: [],
    };
  verifyBuildxLegalInventory(dir, index, sha, "windows/amd64");
  assert.throws(() =>
    verifyBuildxLegalInventory(
      dir,
      { ...index, modulesWithoutRootLicense: ["missing"] },
      sha,
      "windows/amd64",
    ),
  );
  assert.throws(() =>
    verifyBuildxLegalInventory(
      dir,
      { ...index, sourceCommit: "foreign" },
      sha,
      "windows/amd64",
    ),
  );
  assert.throws(() =>
    verifyBuildxLegalInventory(
      dir,
      { ...index, source: [{ ...file, path: "../user-file" }] },
      sha,
      "windows/amd64",
    ),
  );
  writeFileSync(path, "altered license");
  assert.throws(() =>
    verifyBuildxLegalInventory(dir, index, sha, "windows/amd64"),
  );
});
