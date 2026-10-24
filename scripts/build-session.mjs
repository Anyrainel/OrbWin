import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
const root = fileURLToPath(new URL("../", import.meta.url));
const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
const directory = resolve(root, "backend-tools/session");
const binary = resolve(root, "artifacts/backend/orbwin-session.exe");
const sourceNames = [
  "main.go",
  "kube.go",
  "publication.go",
  "go.mod",
  "go.sum",
];
const hash = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");
const sourceHashes = Object.fromEntries(
  sourceNames.map((name) => [name, hash(resolve(directory, name))]),
);
const env = { ...process.env, CGO_ENABLED: "0" };
for (const args of [
  ["test", "./..."],
  ["build", "-trimpath", "-o", binary, "."],
])
  execFileSync(go, args, {
    cwd: directory,
    env,
    stdio: "inherit",
    windowsHide: true,
  });
if (
  sourceNames.some(
    (name) => hash(resolve(directory, name)) !== sourceHashes[name],
  )
)
  throw new Error(
    "Session-helper sources changed during build; no new manifest was published.",
  );
writeFileSync(
  resolve(root, "artifacts/backend/session-manifest.json"),
  JSON.stringify(
    {
      sdk: "github.com/moby/moby/client@v0.6.2",
      binarySha256: hash(binary),
      sources: sourceHashes,
    },
    null,
    2,
  ),
);
