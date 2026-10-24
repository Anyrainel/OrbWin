import { execFileSync } from "node:child_process";
import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
const directory = resolve(root, "backend-tools/credentials");
const binary = resolve(root, "artifacts/backend/docker-credential-orbwin.exe");
const names = ["main.go", "vault_windows.go", "go.mod", "go.sum"];
const hash = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");
const sources = Object.fromEntries(
  names.map((name) => [name, hash(resolve(directory, name))]),
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
const licenseDirectory = resolve(
  root,
  "artifacts/backend/licenses/credentials",
);
mkdirSync(licenseDirectory, { recursive: true });
const modules = [];
for (const name of [
  "github.com/docker/docker-credential-helpers",
  "github.com/danieljoos/wincred",
  "golang.org/x/sys",
]) {
  const module = JSON.parse(
    execFileSync(go, ["list", "-m", "-json", name], {
      cwd: directory,
      env,
      encoding: "utf8",
      windowsHide: true,
    }),
  );
  copyFileSync(
    resolve(module.Dir, "LICENSE"),
    resolve(licenseDirectory, `${name.replaceAll("/", "_")}-LICENSE`),
  );
  modules.push({ path: module.Path, version: module.Version, sum: module.Sum });
}
if (names.some((name) => hash(resolve(directory, name)) !== sources[name]))
  throw new Error(
    "Credential helper sources changed during build; no manifest was published.",
  );
const manifest = { binarySha256: hash(binary), sources, modules };
writeFileSync(
  resolve(root, "artifacts/backend/credentials-manifest.json"),
  JSON.stringify(manifest, null, 2),
);
writeFileSync(
  resolve(licenseDirectory, "dependency-inventory.json"),
  JSON.stringify({ modules }, null, 2),
);
console.log("Built the product-scoped Windows credential helper.");
