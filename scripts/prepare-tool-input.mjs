// Fresh build input only; no download, selected-cache replacement or tool run.
import assert from "node:assert/strict";
import { readFileSync, writeFileSync, copyFileSync, mkdirSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";
import { buildOutput } from "./build-output.mjs";
import {
  verifyDockerCliSource,
  copyDockerCliProvenance,
} from "./docker-cli-provenance.mjs";
import {
  verifyComposeSource,
  copyComposeProvenance,
} from "./stage-runtime-tools.mjs";
import {
  verifyBuildxSource,
  copyBuildxProvenance,
} from "./buildx-provenance.mjs";
import { runtimeToolSource } from "./tool-input-sources.mjs";
const root = fileURLToPath(new URL("../", import.meta.url)),
  hash = (p) => createHash("sha256").update(readFileSync(p)).digest("hex"),
  lockPath = resolve(root, "runtime-tools.lock.json"),
  lockSHA = hash(lockPath),
  lock = JSON.parse(readFileSync(lockPath));
const docker = verifyDockerCliSource(root, lock);
const compose = verifyComposeSource(root, lock);
const buildx = verifyBuildxSource(root, lock);
assert.equal(lock.tools.length, 6, "Exact six-component tool layout required");
const sources = lock.tools.map((tool) => ({
  tool,
  path: runtimeToolSource(
    tool,
    resolve(root, "artifacts/runtime-tools"),
    docker.outputRoot,
    { compose: compose.outputRoot, buildx: buildx.outputRoot },
  ),
}));
assert.equal(
  new Set(sources.map((s) => s.tool.path)).size,
  6,
  "Repeated tool role",
);
for (const { tool, path } of sources)
  assert.equal(hash(path), tool.sha256, "Source tool bytes differ from lock");
const output = buildOutput(root, process.argv.slice(2)).directory;
for (const { tool, path } of sources) {
  const target = resolve(output, tool.path);
  mkdirSync(dirname(target), { recursive: true });
  copyFileSync(path, target);
  assert.equal(hash(target), tool.sha256, "Copied tool bytes changed");
}
copyDockerCliProvenance(root, lock, resolve(output, "docker-cli-provenance"));
copyComposeProvenance(root, lock, resolve(output, "compose-provenance"));
copyBuildxProvenance(root, lock, resolve(output, "buildx-provenance"));
assert.equal(hash(lockPath), lockSHA, "Tool lock changed during preparation");
for (const { tool, path } of sources)
  assert.equal(
    hash(path),
    tool.sha256,
    "Source tool changed during preparation",
  );
writeFileSync(
  resolve(output, "tool-input-manifest.json"),
  JSON.stringify(
    {
      schema: 1,
      purpose: "Build input; no runtime or execution qualification",
      runtimeToolLockSHA256: lockSHA,
      tools: Object.fromEntries(
        sources.map((s) => [s.tool.path, s.tool.sha256]),
      ),
    },
    null,
    2,
  ),
  { flag: "wx" },
);
