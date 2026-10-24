import { createHash } from "node:crypto";
import { readFileSync, writeFileSync, existsSync, mkdirSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
const root = fileURLToPath(new URL("../", import.meta.url));
if (process.platform !== "win32" || process.arch !== "x64")
  throw new Error("This bootstrap currently supports Windows x64 only.");
const upstream = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
const compiler = JSON.parse(
  readFileSync(resolve(root, "build-tools.lock.json")),
);
const tools = resolve(root, ".tools");
mkdirSync(tools, { recursive: true });
async function download(url, file, checksum) {
  if (!existsSync(file)) {
    const response = await fetch(url, { signal: AbortSignal.timeout(120000) });
    if (!response.ok) throw new Error(`${url}: HTTP ${response.status}`);
    writeFileSync(file, Buffer.from(await response.arrayBuffer()));
  }
  if (
    createHash("sha256").update(readFileSync(file)).digest("hex") !== checksum
  )
    throw new Error(
      `Checksum mismatch: ${file}. Preserve evidence and replace only this download.`,
    );
}
const goArchive = resolve(tools, `go${upstream.go.version}.zip`);
await download(
  `https://go.dev/dl/go${upstream.go.version}.windows-amd64.zip`,
  goArchive,
  upstream.go.windowsAmd64Sha256,
);
const goDir = resolve(tools, `go${upstream.go.version}`);
mkdirSync(goDir, { recursive: true });
if (!existsSync(resolve(goDir, "go/bin/go.exe")))
  execFileSync("tar.exe", ["-xf", goArchive, "-C", goDir], {
    stdio: "inherit",
  });
const cache = resolve(tools, "msys-cache");
const mingw = resolve(tools, "mingw");
mkdirSync(cache, { recursive: true });
mkdirSync(mingw, { recursive: true });
for (const pkg of compiler.packages) {
  if (!/^[a-zA-Z0-9_.+-]+\.pkg\.tar\.zst$/.test(pkg.filename))
    throw new Error("Invalid compiler package filename");
  const archive = resolve(cache, pkg.filename);
  await download(compiler.baseUrl + pkg.filename, archive, pkg.sha256);
  execFileSync("tar.exe", ["-xf", archive, "-C", mingw], { stdio: "inherit" });
}
console.log(
  `Verified project-local Go ${upstream.go.version} and ${compiler.compiler}. No global PATH changes.`,
);
