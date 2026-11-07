// Attribution for the narrowly adapted, version-pinned upstream NSIS template.
// No external commands, downloads, registry calls, or installer execution.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { createHash } from "node:crypto";
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
export const desktopInstallerPins = Object.freeze({
  cliVersion: "2.12.4",
  commit: "993038c49185307dfc895ab7657d77ca0d5ed7c7",
  files: Object.freeze({
    "installer-template.upstream.nsi":
      "d00089d3b1a2c4a30d7b6e098079100cb8f25e980f84d4515aca4597b95e967d",
    "installer-template.nsi":
      "6fa293accfe30f24e42a20cc53a368dbefd8ceb81a98c99d36000a2c8aeecf9e",
    "installer-template.provenance.json":
      "dd7a742c5c9a92b58852f7c06aae6863dcfe84fb5d29d59417d6e5453f044b98",
    "installer-template-LICENSE_MIT":
      "9dd42ea92cff2ede5cd477cbfcce051b2d0115c0ac7f368ee88cb545055dff1d",
    "installer-template-LICENSE_APACHE-2.0":
      "0d542e0c8804e39aa7f37eb00da5a762149dc682d7829451287e11b938e94594",
  }),
});
function realDirectories(directory) {
  assert(
    path.isAbsolute(directory),
    "Absolute installer source/output directory required",
  );
  for (let current = directory; ; current = path.dirname(current)) {
    const stat = fs.lstatSync(current);
    assert(
      stat.isDirectory() && !stat.isSymbolicLink(),
      "Redirected installer attribution directory",
    );
    if (current === path.parse(current).root) break;
  }
}
const identity = (s) => [s.dev, s.ino, s.size, s.mtimeNs, s.ctimeNs].join(":");
function bounded(file, maximum = 1 << 20) {
  realDirectories(path.dirname(file));
  const before = fs.lstatSync(file, { bigint: true });
  assert(
    before.isFile() &&
      !before.isSymbolicLink() &&
      before.nlink === 1n &&
      before.size <= BigInt(maximum),
    "Installer source must be a bounded regular single-link file",
  );
  const fd = fs.openSync(file, "r");
  try {
    assert.equal(
      identity(fs.fstatSync(fd, { bigint: true })),
      identity(before),
      "Installer source changed before read",
    );
    const bytes = Buffer.alloc(Number(before.size) + 1);
    let size = 0;
    while (size < bytes.length) {
      const count = fs.readSync(fd, bytes, size, bytes.length - size, null);
      if (!count) break;
      size += count;
    }
    assert.equal(
      BigInt(size),
      before.size,
      "Installer source size changed during read",
    );
    assert.equal(
      identity(fs.fstatSync(fd, { bigint: true })),
      identity(before),
      "Installer source changed during read",
    );
    assert.equal(
      identity(fs.lstatSync(file, { bigint: true })),
      identity(before),
      "Installer source replaced during read",
    );
    return bytes.subarray(0, size);
  } finally {
    fs.closeSync(fd);
  }
}
export function verifyDesktopInstallerSource(root) {
  root = path.resolve(root);
  realDirectories(root);
  const config = JSON.parse(
    bounded(path.join(root, "src-tauri/tauri.package.conf.json")),
  );
  assert.equal(config.bundle.windows.nsis.template, "installer-template.nsi");
  assert.equal(
    config.bundle.windows.nsis.installerHooks,
    "installer-hooks.nsh",
  );
  assert.equal(config.bundle.windows.nsis.installMode, "currentUser");
  const cli = JSON.parse(
    bounded(path.join(root, "node_modules/@tauri-apps/cli/package.json")),
  );
  assert.equal(
    cli.version,
    desktopInstallerPins.cliVersion,
    "NSIS template must be reviewed for the actual pinned Tauri CLI",
  );
  const files = Object.entries(desktopInstallerPins.files).map(
    ([name, expected]) => {
      const bytes = bounded(path.join(root, "src-tauri", name));
      assert.equal(
        hash(bytes),
        expected,
        "Changed pinned installer source/license: " + name,
      );
      return { name, sha256: expected, bytes };
    },
  );
  const provenance = JSON.parse(
    files.find((f) => f.name === "installer-template.provenance.json").bytes,
  );
  assert.equal(provenance.commit, desktopInstallerPins.commit);
  assert.equal(provenance.cliVersion, desktopInstallerPins.cliVersion);
  return { files, provenance };
}
export function stageDesktopInstallerNotices(root, destination) {
  root = path.resolve(root);
  destination = path.resolve(destination);
  const relative = path.relative(path.join(root, "artifacts"), destination);
  assert(
    relative &&
      !path.isAbsolute(relative) &&
      !relative.split(path.sep).includes(".."),
    "Installer attribution must remain inside generated artifacts",
  );
  // Complete input preflight before creating or changing any destination.
  const verified = verifyDesktopInstallerSource(root);
  let current = root;
  for (const child of path.relative(root, destination).split(path.sep)) {
    current = path.join(current, child);
    try {
      fs.mkdirSync(current);
    } catch (e) {
      if (e.code !== "EEXIST") throw e;
    }
    realDirectories(current);
  }
  for (const file of verified.files) {
    const target = path.join(destination, file.name);
    let existing;
    try {
      existing = fs.lstatSync(target, { bigint: true });
    } catch (e) {
      if (e.code !== "ENOENT") throw e;
    }
    if (existing) {
      assert(
        existing.isFile() &&
          !existing.isSymbolicLink() &&
          existing.nlink === 1n,
        "Unexpected installer attribution target",
      );
      assert.equal(
        hash(bounded(target)),
        file.sha256,
        "Existing installer attribution differs; retained for review",
      );
    } else fs.writeFileSync(target, file.bytes, { flag: "wx", mode: 0o644 });
    assert.equal(hash(bounded(target)), file.sha256);
  }
  return {
    kind: "upstream",
    name: "Tauri NSIS installer template",
    version: desktopInstallerPins.cliVersion,
    commit: desktopInstallerPins.commit,
    source: verified.provenance.sourceUrl,
    license: "Apache-2.0 OR MIT",
    sourceManifest: "installer-template/installer-template.provenance.json",
    notices: verified.files
      .filter((f) => f.name.includes("LICENSE"))
      .map(({ name, sha256 }) => ({
        name: "installer-template/" + name,
        sha256,
      })),
    sourceFiles: verified.files.map(({ name, sha256 }) => ({
      name: "installer-template/" + name,
      sha256,
    })),
  };
}
