// Canonicalize a Docker Save of the pinned image, then validate its full graph
// with the SDK's shared archive reader. This never contacts or mutates an engine.
import assert from "node:assert/strict";
import {
  readFileSync,
  writeFileSync,
  existsSync,
  unlinkSync,
  renameSync,
  openSync,
  fstatSync,
  readSync,
  closeSync,
} from "node:fs";
import { createHash, randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";
import { resolve, posix, isAbsolute } from "node:path";
import { fileURLToPath } from "node:url";
const root = fileURLToPath(new URL("../", import.meta.url));
const source = process.argv[2];
assert(
  source && isAbsolute(source),
  "Supply an explicit absolute Docker Save archive path",
);
const candidate = process.argv.includes("--candidate");
const sha = (bytes) => createHash("sha256").update(bytes).digest("hex");
const pinned =
  "bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30";
const budget = 8 * 1024 * 1024;
const descriptor = openSync(source, "r");
let input;
try {
  const info = fstatSync(descriptor);
  assert(
    info.isFile() && info.size <= budget,
    "Maintenance source must be a bounded regular archive",
  );
  const bytes = Buffer.alloc(budget + 1);
  let count = 0;
  while (count < bytes.length) {
    const read = readSync(descriptor, bytes, count, bytes.length - count, null);
    if (!read) break;
    count += read;
  }
  assert(count <= budget, "Maintenance source exceeds its byte budget");
  input = bytes.subarray(0, count);
} finally {
  closeSync(descriptor);
}
const entries = new Map();
let offset = 0;
while (offset + 512 <= input.length) {
  const header = input.subarray(offset, offset + 512);
  if (header.every((byte) => byte === 0)) {
    assert(
      input.subarray(offset).every((byte) => byte === 0),
      "Unexpected trailing tar data",
    );
    break;
  }
  const string = (start, size) =>
    header
      .subarray(start, start + size)
      .toString("utf8")
      .replace(/\0.*$/s, "");
  const octal = (start, size) => {
    const text = string(start, size).trim();
    assert(/^[0-7]+$/.test(text), "Unsupported tar number");
    return parseInt(text, 8);
  };
  const checksum = [...header].reduce(
    (sum, byte, index) => sum + (index >= 148 && index < 156 ? 32 : byte),
    0,
  );
  assert.equal(checksum, octal(148, 8), "Invalid source tar checksum");
  const name = string(0, 100),
    size = octal(124, 12),
    type = string(156, 1);
  assert(!string(345, 155), "Unsupported tar prefix");
  assert(
    size >= 0 && offset + 512 + size <= input.length,
    "Incomplete source entry",
  );
  if (type === "5") {
    assert.equal(size, 0);
    offset += 512;
    continue;
  }
  assert(
    type === "0" || type === "",
    "Only regular source archive files are supported",
  );
  assert(
    name &&
      posix.normalize(name) === name &&
      !name.startsWith("/") &&
      !name.startsWith("../") &&
      !entries.has(name),
    "Unexpected source path",
  );
  const data = Buffer.from(input.subarray(offset + 512, offset + 512 + size));
  if (name.startsWith("blobs/sha256/"))
    assert.equal(sha(data), name.slice(13), "Source blob digest differs");
  else
    assert(
      ["manifest.json", "index.json", "oci-layout"].includes(name),
      "Unexpected source metadata",
    );
  entries.set(name, data);
  assert(entries.size <= 256, "Maintenance source exceeds its entry budget");
  offset += 512 + Math.ceil(size / 512) * 512;
}
const rootBlob = entries.get(`blobs/sha256/${pinned}`);
assert(rootBlob, "Pinned OCI root is absent");
const manifest = JSON.parse(entries.get("manifest.json").toString());
assert.equal(manifest.length, 1);
entries.set(
  "manifest.json",
  Buffer.from(
    JSON.stringify(
      manifest.map(({ Config, Layers }) => ({ Config, RepoTags: [], Layers })),
    ),
  ),
);
entries.set(
  "index.json",
  Buffer.from(
    JSON.stringify({
      schemaVersion: 2,
      mediaType: "application/vnd.oci.image.index.v1+json",
      manifests: [
        {
          mediaType: "application/vnd.oci.image.index.v1+json",
          digest: `sha256:${pinned}`,
          size: rootBlob.length,
        },
      ],
    }),
  ),
);
entries.set("oci-layout", Buffer.from('{"imageLayoutVersion":"1.0.0"}'));
const chunks = [];
for (const [name, data] of [...entries].sort(([a], [b]) =>
  a < b ? -1 : a > b ? 1 : 0,
)) {
  const header = Buffer.alloc(512);
  const number = (offset, size, value) =>
    header.write(
      value.toString(8).padStart(size - 1, "0") + "\0",
      offset,
      size,
      "ascii",
    );
  header.write(name, 0, 100, "ascii");
  number(100, 8, 0o600);
  number(108, 8, 0);
  number(116, 8, 0);
  number(124, 12, data.length);
  number(136, 12, 0);
  header.fill(32, 148, 156);
  header.write("0", 156);
  header.write("ustar\0", 257);
  header.write("00", 263);
  const sum = [...header].reduce((a, b) => a + b, 0);
  header.write(sum.toString(8).padStart(6, "0") + "\0 ", 148, 8, "ascii");
  chunks.push(header, data, Buffer.alloc((512 - (data.length % 512)) % 512));
}
chunks.push(Buffer.alloc(1024));
const bytes = Buffer.concat(chunks);
const temporary = resolve(
  root,
  `artifacts/backend/.maintenance-${randomUUID()}.tar`,
);
writeFileSync(temporary, bytes, { flag: "wx" });
try {
  const checked = JSON.parse(
    execFileSync(
      resolve(root, "artifacts/backend/orbwin-session.exe"),
      ["maintenance-image-check", temporary],
      { encoding: "utf8", windowsHide: true },
    ),
  );
  assert.equal(checked.sha256, sha(bytes));
  if (candidate) {
    const destination = resolve(
      root,
      `artifacts/qualification/maintenance-candidate-${checked.sha256}.tar`,
    );
    if (!existsSync(destination))
      writeFileSync(destination, bytes, { flag: "wx" });
    console.log(
      JSON.stringify({ ...checked, candidate: destination }, null, 2),
    );
  } else {
    const lock = JSON.parse(
      readFileSync(resolve(root, "maintenance-image.lock.json")),
    );
    for (const key of [
      "sha256",
      "bytes",
      "root",
      "config",
      "os",
      "architecture",
    ])
      assert.deepEqual(
        checked[key],
        lock[key],
        `Maintenance ${key} differs from the reviewed pin`,
      );
    const destination = resolve(
      root,
      "artifacts/backend/orbwin-maintenance.tar",
    );
    if (existsSync(destination))
      assert.equal(
        sha(readFileSync(destination)),
        lock.sha256,
        "Existing staged maintenance archive differs; retain it for review",
      );
    else renameSync(temporary, destination);
    console.log(JSON.stringify(checked, null, 2));
  }
} finally {
  if (existsSync(temporary)) unlinkSync(temporary);
}
