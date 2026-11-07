import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  lstatSync,
  realpathSync,
  mkdirSync,
  writeFileSync,
  existsSync,
  openSync,
  fstatSync,
  readSync,
  closeSync,
} from "node:fs";
import { resolve, dirname, sep } from "node:path";

const sha = (bytes) => createHash("sha256").update(bytes).digest("hex");
const digest = (value) =>
  typeof value === "string" && /^[a-f0-9]{64}$/.test(value);
function bounded(path, maximum) {
  const info = lstatSync(path);
  assert(
    info.isFile() && !info.isSymbolicLink() && info.size <= maximum,
    "Notice/source input must be a bounded regular file",
  );
  const descriptor = openSync(path, "r");
  try {
    const current = fstatSync(descriptor);
    assert(
      current.isFile() && current.size === info.size,
      "Notice/source input changed while opening",
    );
    const bytes = Buffer.alloc(current.size + 1);
    let count = 0;
    while (count < bytes.length) {
      const read = readSync(
        descriptor,
        bytes,
        count,
        bytes.length - count,
        null,
      );
      if (!read) break;
      count += read;
    }
    assert(
      count === current.size,
      "Notice/source input size changed while reading",
    );
    return bytes.subarray(0, count);
  } finally {
    closeSync(descriptor);
  }
}
function relative(base, name) {
  assert(
    typeof name === "string" &&
      name &&
      !name.includes("\\") &&
      !name.startsWith("/") &&
      !name.split("/").some((part) => !part || part === "." || part === "..") &&
      !/[\x00-\x1f:]/.test(name),
    "Unsafe source/notice path",
  );
  const path = resolve(base, name);
  assert(
    path.startsWith(resolve(base) + sep),
    "Source/notice escaped its directory",
  );
  return path;
}
export function stageMaintenanceNotices(root, destination) {
  const image = JSON.parse(
    bounded(resolve(root, "maintenance-image.lock.json"), 1 << 20),
  );
  const raw = bounded(resolve(root, "maintenance-sources.lock.json"), 2 << 20);
  const pin = JSON.parse(raw);
  assert(
    pin.schema === 1 &&
      pin.imageRoot === image.root &&
      pin.imageSha256 === image.sha256,
    "Maintenance source inventory belongs to another image",
  );
  assert(
    Array.isArray(pin.packages) &&
      pin.packages.length > 0 &&
      pin.packages.length <= 256,
    "Invalid maintenance package inventory",
  );
  assert(
    Array.isArray(pin.origins) &&
      pin.origins.length > 0 &&
      pin.origins.length <= 32,
    "Invalid maintenance source origins",
  );
  assert(
    Array.isArray(pin.sourceFiles) &&
      pin.sourceFiles.length > 0 &&
      pin.sourceFiles.length <= 1024,
    "Invalid maintenance source file inventory",
  );
  const sourceFiles = new Map();
  for (const row of pin.sourceFiles) {
    relative("sources", row.file);
    assert(
      !sourceFiles.has(row.file) &&
        digest(row.sha256) &&
        Number.isSafeInteger(row.bytes) &&
        row.bytes >= 0 &&
        row.bytes <= 32 << 20,
      "Invalid or duplicate source file",
    );
    sourceFiles.set(row.file, row);
  }
  const origins = new Map(),
    used = new Set();
  for (const origin of pin.origins) {
    assert(
      typeof origin.origin === "string" &&
        /^[a-z0-9+-]+$/.test(origin.origin) &&
        !origins.has(origin.origin) &&
        /^[a-f0-9]{40}$/.test(origin.commit) &&
        typeof origin.version === "string",
      "Invalid source origin identity",
    );
    assert(
      ["installed-package-origin", "static-build-context"].includes(
        origin.role,
      ),
      "Unknown source origin role",
    );
    assert(
      Array.isArray(origin.recipeFiles) &&
        origin.recipeFiles.length > 0 &&
        Array.isArray(origin.inputs),
      "Incomplete package build inputs",
    );
    for (const row of [...origin.recipeFiles, ...origin.inputs]) {
      const file = sourceFiles.get(row.file);
      assert(
        file && file.sha256 === row.sha256 && file.bytes === row.bytes,
        "Build input absent from the frozen source archive inventory",
      );
      assert(
        row.file.startsWith(`aports/${origin.origin}/`) ||
          row.file.startsWith(`distfiles/${origin.origin}/`),
        "Build input belongs to another origin",
      );
      if (origin.inputs.includes(row))
        assert(
          typeof row.sha512 === "string" && /^[a-f0-9]{128}$/.test(row.sha512),
          "Recipe input lacks its verified checksum",
        );
      used.add(row.file);
    }
    assert(
      origin.recipeFiles.some(
        (row) => row.file === `aports/${origin.origin}/APKBUILD`,
      ),
      "Package recipe is missing",
    );
    origins.set(origin.origin, origin);
  }
  assert(
    used.size === sourceFiles.size,
    "Source inventory contains unaccounted files",
  );
  const noticeRoot = resolve(root, "third-party/maintenance-image");
  assert(
    realpathSync(noticeRoot) === noticeRoot,
    "Maintenance notices directory is redirected",
  );
  assert(
    Array.isArray(pin.notices) &&
      pin.notices.length > 0 &&
      pin.notices.length <= 256,
    "Invalid maintenance notices",
  );
  const files = new Map();
  for (const row of pin.notices) {
    assert(
      !files.has(row.file) && digest(row.sha256),
      "Invalid or duplicate notice",
    );
    const path = relative(noticeRoot, row.file);
    assert(
      realpathSync(path).startsWith(noticeRoot + sep),
      "Notice file is redirected outside its source",
    );
    const bytes = bounded(path, 1 << 20);
    assert(
      sha(bytes) === row.sha256 && bytes.length === row.bytes,
      "Maintenance notice changed after review",
    );
    files.set(row.file, bytes);
  }
  assert(files.has("NOTICE.txt"), "Source-location notice is missing");
  const packages = new Set();
  for (const item of pin.packages) {
    const origin = origins.get(item.origin);
    assert(
      typeof item.name === "string" &&
        !packages.has(item.name) &&
        origin &&
        origin.role === "installed-package-origin" &&
        origin.commit === item.commit &&
        origin.version === item.version &&
        typeof item.license === "string" &&
        item.license,
      "Package does not have matching source revision/version",
    );
    const bindings = pin.noticeBindings?.[item.name];
    assert(
      Array.isArray(bindings) &&
        bindings.length > 0 &&
        bindings.every((name) => files.has(name)),
      "Package has incomplete notice coverage",
    );
    packages.add(item.name);
  }
  assert(
    Object.keys(pin.noticeBindings).length === packages.size,
    "Unknown package notice bindings",
  );
  for (const name of ["utmps", "skalibs"]) {
    if (origins.has("busybox"))
      assert(
        origins.get(name)?.role === "static-build-context" &&
          origins.get(name).commit === origins.get("busybox")?.commit,
        "Static source context differs from the BusyBox recipe revision",
      );
  }
  assert(
    pin.sourceArchive?.file === "maintenance-sources.tar.gz" &&
      digest(pin.sourceArchive.sha256),
    "Invalid source archive pin",
  );
  const archive = bounded(
    resolve(root, "artifacts/backend/maintenance-sources.tar.gz"),
    64 << 20,
  );
  assert(
    archive.length === pin.sourceArchive.bytes &&
      sha(archive) === pin.sourceArchive.sha256,
    "Maintenance source archive differs from its pin",
  );
  files.set("SOURCES.tar.gz", archive);
  files.set("SOURCE-INVENTORY.json", raw);
  // Freeze all checked bytes before publication; a concurrent source edit must
  // never change what this package receives after its checksum was checked.
  const notices = [];
  for (const [name, bytes] of files) {
    const target = relative(destination, name);
    mkdirSync(dirname(target), { recursive: true });
    if (existsSync(target))
      assert(
        sha(bounded(target, 64 << 20)) === sha(bytes),
        "Existing staged notice differs; use a fresh generated stage",
      );
    else writeFileSync(target, bytes, { flag: "wx" });
    notices.push({ name, sha256: sha(bytes) });
  }
  return {
    notices,
    packages: pin.packages.length,
    origins: pin.origins.length,
    sourceFiles: pin.sourceFiles.length,
    sourceArchive: pin.sourceArchive,
  };
}
