import assert from "node:assert/strict";
import {
  readFileSync,
  existsSync,
  mkdirSync,
  copyFileSync,
  cpSync,
  rmSync,
  writeFileSync,
  realpathSync,
  readdirSync,
  chmodSync,
} from "node:fs";
import { resolve, dirname, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { sha256, verifyBuild } from "./build-identity.mjs";
import { collectNotices } from "./collect-notices.mjs";
const root = fileURLToPath(new URL("../", import.meta.url));
const build = verifyBuild(root);
assert.equal(
  build.distroEmbedded,
  true,
  "Build the guest-capable backend first",
);
for (const [manifestFile, folder] of [
  ["session-manifest.json", "session"],
  ["credentials-manifest.json", "credentials"],
]) {
  const manifest = JSON.parse(
    readFileSync(resolve(root, "artifacts/backend", manifestFile)),
  );
  const binary =
    folder === "session"
      ? "orbwin-session.exe"
      : "docker-credential-orbwin.exe";
  assert.equal(
    sha256(readFileSync(resolve(root, "artifacts/backend", binary))),
    manifest.binarySha256,
  );
  for (const [name, hash] of Object.entries(manifest.sources))
    assert.equal(
      sha256(readFileSync(resolve(root, "backend-tools", folder, name))),
      hash,
      `Stale ${folder} helper`,
    );
}
const tools = JSON.parse(
  readFileSync(resolve(root, "runtime-tools.lock.json")),
);
for (const tool of tools.tools)
  assert.equal(
    sha256(readFileSync(resolve(root, "artifacts/runtime-tools", tool.path))),
    tool.sha256,
  );
const parent = resolve(root, "artifacts/package");
mkdirSync(parent, { recursive: true });
assert.equal(
  realpathSync(parent),
  resolve(realpathSync(root), "artifacts/package"),
  "Package staging parent must remain within the workspace",
);
const destination = resolve(parent, "runtime");
// This fixed generated directory is the only recursive replacement target.
// Resolve the parent and existing directory before removal; refuse junctions or
// an unmarked tree rather than following an unexpected path outside the stage.
if (existsSync(destination)) {
  assert.equal(
    realpathSync(destination),
    resolve(realpathSync(parent), "runtime"),
  );
  assert.equal(
    JSON.parse(readFileSync(resolve(destination, "stage.json"))).product,
    "OrbWin generated package runtime",
  );
  assert(destination.startsWith(resolve(root, "artifacts") + sep));
  rmSync(destination, { recursive: true });
}
mkdirSync(resolve(destination, "backend"), { recursive: true });
writeFileSync(
  resolve(destination, "stage.json"),
  JSON.stringify({ product: "OrbWin generated package runtime" }),
);
const hashes = {};
for (const name of [
  "rdd.exe",
  "build-manifest.json",
  "orbwin-session.exe",
  "session-manifest.json",
  "docker-credential-orbwin.exe",
  "credentials-manifest.json",
]) {
  copyFileSync(
    resolve(root, "artifacts/backend", name),
    resolve(destination, "backend", name),
  );
  hashes[`backend/${name}`] = sha256(
    readFileSync(resolve(destination, "backend", name)),
  );
}
for (const tool of tools.tools) {
  const path = resolve(destination, "tools", tool.path);
  mkdirSync(dirname(path), { recursive: true });
  copyFileSync(resolve(root, "artifacts/runtime-tools", tool.path), path);
  hashes[`tools/${tool.path}`] = tool.sha256;
}
cpSync(
  resolve(root, "artifacts/backend/licenses"),
  resolve(destination, "licenses"),
  { recursive: true },
);
collectNotices(root, resolve(destination, "licenses/dependencies"));
copyFileSync(
  resolve(root, ".upstream/rancher-desktop/rdd/dependencies.yaml"),
  resolve(destination, "licenses/upstream-guest-dependencies.yaml"),
);
copyFileSync(
  resolve(root, "upstream.lock.json"),
  resolve(destination, "licenses/upstream.lock.json"),
);
cpSync(
  resolve(root, "patches/rdd"),
  resolve(destination, "licenses/rdd-patches"),
  { recursive: true },
);
// Go module caches mark license files read-only. Normalize only these generated
// copies, otherwise Tauri cannot replace resources during a repeat build.
function writableCopies(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = resolve(directory, entry.name);
    if (entry.isSymbolicLink())
      throw new Error("A generated package resource is a symbolic link");
    if (entry.isDirectory()) writableCopies(path);
    else if (entry.isFile()) chmodSync(path, 0o644);
  }
}
writableCopies(destination);
writeFileSync(
  resolve(destination, "package-manifest.json"),
  JSON.stringify(
    {
      schema: 1,
      product: "OrbWin",
      version: JSON.parse(readFileSync(resolve(root, "package.json"))).version,
      hashes,
    },
    null,
    2,
  ),
);
console.log("Staged the checksum-verified per-user application runtime.");
