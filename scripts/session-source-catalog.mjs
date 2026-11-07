import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";

// The Rust build consumes the same source-owned file, not a second folder list.
export const sessionSourceFolders = readFileSync(
  new URL("./session-source-folders.txt", import.meta.url),
  "utf8",
)
  .trim()
  .split(/\r?\n/)
  .map((name) => (name === "." ? "" : name));
assert.equal(new Set(sessionSourceFolders).size, sessionSourceFolders.length);

export function sessionSourceNames(directory) {
  return sessionSourceFolders
    .flatMap((folder) =>
      readdirSync(resolve(directory, folder))
        .filter(
          (name) =>
            name.endsWith(".go") &&
            !name.endsWith("_test.go") &&
            !name.startsWith(".") &&
            !name.startsWith("_"),
        )
        .map((name) => (folder ? `${folder}/${name}` : name)),
    )
    .concat([
      "go.mod",
      "go.sum",
      "lan-helper/lan.manifest",
      "lan-helper/lan.rc",
    ])
    .sort();
}

export const serviceSourceNames = [
  "service-tools/main_linux.go",
  "service-tools/main_other.go",
  "service-tools/lan_udp_linux.go",
  "internal/landatagram/codec.go",
  "internal/landatagram/session.go",
  "internal/gateway/tls.go",
  "internal/gateway/copy.go",
  "internal/gateway/mux.go",
  "internal/streamcontrol/protocol.go",
  "internal/streamcontrol/server.go",
  "go.mod",
  "go.sum",
];

// Compare the complete expected set BEFORE reading manifest-supplied names.
// An old manifest must not omit newly linked source and still pass packaging.
export function verifySessionSources(directory, manifest, expected) {
  assert.ok(
    manifest.sources &&
      typeof manifest.sources === "object" &&
      !Array.isArray(manifest.sources),
    "Helper source manifest is missing",
  );
  assert.deepEqual(
    Object.keys(manifest.sources).sort(),
    [...expected].sort(),
    "Helper source set changed; rebuild before packaging",
  );
  for (const name of expected) {
    const hash = createHash("sha256")
      .update(readFileSync(resolve(directory, name)))
      .digest("hex");
    assert.equal(manifest.sources[name], hash, `Stale helper source: ${name}`);
  }
}
