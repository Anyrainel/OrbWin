// Controlled SOURCE/DATA fixtures only. Never invokes Go, keys, signer or SDK.
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { resolve, dirname, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";
import {
  sdkWinioRoles,
  sdkWinioPortableData,
  composeSdkWinioModfile,
  projectSdkWinioMetadata,
} from "./sdk-winio-build-input.mjs";
const ROOT = fileURLToPath(new URL("../", import.meta.url));
const h = (b) => createHash("sha256").update(b).digest("hex");
export function sdkFixtureLock() {
  return JSON.parse(readFileSync(resolve(ROOT, "sdk-winio.lock.json")));
}
export function sdkFixtureContext(
  source,
  moduleRoot,
  role = "sdk",
  overlaySHA256 = null,
) {
  const mod = readFileSync(resolve(ROOT, "backend-tools/session/go.mod")),
    sum = readFileSync(resolve(ROOT, "backend-tools/session/go.sum")),
    composed = composeSdkWinioModfile(mod, sum, moduleRoot);
  return {
    role,
    main: "orbwin.local/session",
    goos: "windows",
    goarch: "amd64",
    goamd64: "v1",
    cgo: "0",
    tags: [],
    package: sdkWinioRoles[role].package,
    sourceRoot: source,
    moduleRoot,
    modSHA256: h(composed.mod),
    sumSHA256: h(composed.sum),
    overlaySHA256,
  };
}
export function sdkFixtureRecord(context) {
  const rows = JSON.parse(
    readFileSync(new URL("./sdk-winio-metadata-fixture.json", import.meta.url)),
  );
  const original = rows.find(
    (p) => p.ImportPath === "github.com/Microsoft/go-winio",
  ).Dir;
  const m = {
    Path: "github.com/Microsoft/go-winio",
    Version: "v0.6.2",
    Dir: context.moduleRoot,
    Replace: { Path: context.moduleRoot, Dir: context.moduleRoot },
  };
  const packages = rows.map((p) => ({
    ImportPath: p.ImportPath,
    Dir: resolve(context.moduleRoot, relative(original, p.Dir)),
    Module: m,
    ...Object.fromEntries(
      ["GoFiles", "CgoFiles", "CompiledGoFiles", "EmbedFiles"].map((k) => [
        k,
        (p[k] ?? []).map((n) =>
          k === "CompiledGoFiles"
            ? resolve(context.moduleRoot, relative(original, resolve(p.Dir, n)))
            : n,
        ),
      ]),
    ),
  }));
  const main = packages.find((p) => p.ImportPath === m.Path);
  main.GoFiles.push("file_original_close_state.go");
  main.CompiledGoFiles.push(
    resolve(context.moduleRoot, "file_original_close_state.go"),
  );
  const r = projectSdkWinioMetadata(
    Buffer.from(JSON.stringify(m)),
    Buffer.from(packages.map((p) => JSON.stringify(p)).join("\n")),
  );
  r.context = context;
  return r;
}
export function installSdkFixture(root, output) {
  const put = (n, b) => {
    const f = resolve(root, n);
    mkdirSync(dirname(f), { recursive: true });
    writeFileSync(f, b);
  };
  const lockBytes = readFileSync(resolve(ROOT, "sdk-winio.lock.json")),
    lock = JSON.parse(lockBytes);
  put("sdk-winio.lock.json", lockBytes);
  for (const n of Object.keys(lock.recipeHashes))
    put("scripts/" + n, readFileSync(resolve(ROOT, "scripts", n)));
  for (const n of ["go.mod", "go.sum"])
    put(
      "backend-tools/session/" + n,
      readFileSync(resolve(ROOT, "backend-tools/session", n)),
    );
  const offer = sdkWinioPortableData(lock, lockBytes);
  const original = resolve(ROOT, "scripts/test-data/sdk-winio-input/upstream.zip"),
    patch = resolve(
      ROOT,
      "patches/go-winio/0002-sdk062-original-file-close.patch",
    );
  put(
    relative(root, resolve(output, "sdk-winio-source/manifest.json")),
    Buffer.from(JSON.stringify(offer, null, 2) + "\n"),
  );
  for (const [name, file] of [
    ["upstream.zip", original],
    ["winio-custody.patch", patch],
    ["LICENSE", resolve(ROOT, "scripts/test-data/sdk-winio-input/original/LICENSE")],
  ])
    put(
      relative(root, resolve(output, "sdk-winio-source", name)),
      readFileSync(file),
    );
  put(
    relative(root, resolve(output, "sdk-winio-source/README.md")),
    Buffer.from(
      "Modified SDK Winio v0.6.2 close-only source; original ZIP+h1, patch, MIT/tests preserved. No native/lifecycle qualification.\n",
    ),
  );
  return { lock, offer };
}
