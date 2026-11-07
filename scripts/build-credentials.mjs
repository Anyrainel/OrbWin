import { execFileSync } from "node:child_process";
import {
  chmodSync,
  closeSync,
  fstatSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { createHash } from "node:crypto";
import { dirname, isAbsolute, parse, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { buildOutput, buildTestArguments } from "./build-output.mjs";

const dependencyNames = [
  "github.com/docker/docker-credential-helpers",
  "github.com/danieljoos/wincred",
  "golang.org/x/sys",
];
function regular(path) {
  const stat = lstatSync(path, { bigint: true });
  if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1n)
    throw new Error(
      "Credential license must be a regular unlinked file: " + path,
    );
  return stat;
}
function realDirectories(directory) {
  if (!isAbsolute(directory))
    throw new Error("Absolute credential build directory required");
  const paths = [];
  for (let current = directory; ; current = dirname(current)) {
    paths.push(current);
    if (current === parse(current).root) break;
  }
  for (const current of paths.reverse()) {
    const stat = lstatSync(current);
    if (!stat.isDirectory() || stat.isSymbolicLink())
      throw new Error("Credential build directory is redirected: " + current);
  }
}
function identity(stat) {
  return [stat.dev, stat.ino, stat.size, stat.mtimeNs, stat.ctimeNs].join(":");
}
/** Generated attribution only, never user configuration. Serial build ownership:
 * validate real parents and regular single-link leaves; retain source attributes.
 * Read bytes rather than copyFile so newly generated copies do not inherit the
 * module cache's ReadOnly bit. A changed old ReadOnly generated copy alone is
 * made writable and unlinked; source attributes and ACLs are never changed. Interrupted replacement requires rerunning this build;
 * no new manifest is published on error. This is not hostile same-UID path CAS.
 */
export function copyCredentialLicense(root, source, name, outputDirectory) {
  if (!dependencyNames.includes(name))
    throw new Error("Unexpected credential license module");
  if (!isAbsolute(root) || !isAbsolute(source))
    throw new Error("Absolute credential license paths required");
  realDirectories(root);
  realDirectories(dirname(source));
  const before = regular(source);
  if (before.size > 1024n * 1024n)
    throw new Error("Credential license is too large");
  const fd = openSync(source, "r");
  let bytes;
  try {
    if (identity(fstatSync(fd, { bigint: true })) !== identity(before))
      throw new Error("Credential license changed before read");
    bytes = readFileSync(fd);
    if (
      identity(fstatSync(fd, { bigint: true })) !== identity(before) ||
      identity(regular(source)) !== identity(before)
    )
      throw new Error("Credential license changed during read");
  } finally {
    closeSync(fd);
  }
  const directory =
    outputDirectory ?? resolve(root, "artifacts/backend/licenses/credentials");
  if (!isAbsolute(directory))
    throw new Error("Absolute credential license output required");
  const descendants = relative(root, directory);
  if (
    !descendants ||
    isAbsolute(descendants) ||
    descendants === ".." ||
    descendants.startsWith("../") ||
    descendants.startsWith("..\\")
  )
    throw new Error("Credential license output must be below the build root");
  // Create one fixed child at a time; an existing dangling link is never absence.
  let current = root;
  for (const child of descendants.split(/[\\/]/)) {
    current = resolve(current, child);
    try {
      mkdirSync(current);
    } catch (error) {
      if (error.code !== "EEXIST") throw error;
    }
    realDirectories(current);
  }
  const target = resolve(directory, `${name.replaceAll("/", "_")}-LICENSE`);
  let previous;
  try {
    previous = regular(target);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  if (previous) {
    const previousBytes = readFileSync(target);
    if (previousBytes.equals(bytes)) return;
    if (identity(regular(target)) !== identity(previous))
      throw new Error(
        "Generated credential license changed before replacement",
      );
    if ((previous.mode & 0o200n) === 0n) {
      chmodSync(target, 0o644);
      const writable = regular(target);
      if (
        writable.dev !== previous.dev ||
        writable.ino !== previous.ino ||
        !readFileSync(target).equals(previousBytes)
      )
        throw new Error(
          "Generated credential license changed during attribute update",
        );
    }
    unlinkSync(target);
  }
  realDirectories(directory);
  writeFileSync(target, bytes, { flag: "wx", mode: 0o644 });
  if (!readFileSync(target).equals(bytes))
    throw new Error("Generated credential license verification failed");
}

function buildCredentials() {
  const root = fileURLToPath(new URL("../", import.meta.url));
  const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
  const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
  const directory = resolve(root, "backend-tools/credentials");
  const selected = buildOutput(root, process.argv.slice(2), ["--build-only"]);
  const buildOnly = selected.flags.has("--build-only");
  const binary = resolve(selected.directory, "docker-credential-orbwin.exe");
  const names = ["main.go", "vault_windows.go", "go.mod", "go.sum"];
  const hash = (path) =>
    createHash("sha256").update(readFileSync(path)).digest("hex");
  const sources = Object.fromEntries(
    names.map((name) => [name, hash(resolve(directory, name))]),
  );
  const env = {
    ...process.env,
    CGO_ENABLED: "0",
    GOOS: "windows",
    GOARCH: "amd64",
  };
  for (const args of [
    ...buildTestArguments(buildOnly),
    ["build", "-trimpath", "-o", binary, "."],
  ])
    execFileSync(go, args, {
      cwd: directory,
      env,
      stdio: "inherit",
      windowsHide: true,
    });
  const licenseDirectory = resolve(selected.directory, "licenses/credentials");
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
    if (module.Path !== name || module.Replace)
      throw new Error("Unexpected credential dependency source");
    copyCredentialLicense(
      root,
      resolve(module.Dir, "LICENSE"),
      name,
      licenseDirectory,
    );
    modules.push({
      path: module.Path,
      version: module.Version,
      sum: module.Sum,
    });
  }
  if (names.some((name) => hash(resolve(directory, name)) !== sources[name]))
    throw new Error(
      "Credential helper sources changed during build; no manifest was published.",
    );
  const manifest = {
    binarySha256: hash(binary),
    sources,
    modules,
    testsNotRun: buildOnly,
  };
  writeFileSync(
    resolve(selected.directory, "credentials-manifest.json"),
    JSON.stringify(manifest, null, 2),
  );
  writeFileSync(
    resolve(licenseDirectory, "dependency-inventory.json"),
    JSON.stringify({ modules }, null, 2),
  );
  console.log("Built the product-scoped Windows credential helper.");
}
if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
)
  buildCredentials();
