import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  readFileSync,
  writeFileSync,
  mkdirSync,
  openSync,
  closeSync,
  renameSync,
} from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";
const root = fileURLToPath(new URL("../", import.meta.url));
const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
execFileSync(process.execPath, [resolve(root, "scripts/upstream.mjs")], {
  stdio: "inherit",
});
const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
const ccDir = resolve(root, ".tools/mingw/ucrt64/bin");
const source = resolve(root, ".upstream/rancher-desktop/rdd");
const output = resolve(root, "artifacts/backend");
const withDistro = process.argv.includes("--with-distro");
mkdirSync(output, { recursive: true });
const run = (args, env) =>
  execFileSync(go, args, {
    cwd: source,
    env: { ...process.env, ...env },
    stdio: "inherit",
  });
run(
  [
    "build",
    "-p",
    "4",
    "-trimpath",
    "-ldflags=-s -w",
    "-o",
    "pkg/guestagent/lima-guestagent.amd64",
    "github.com/lima-vm/lima/v2/cmd/lima-guestagent",
  ],
  { GOOS: "linux", GOARCH: "amd64", CGO_ENABLED: "0" },
);
const guest = resolve(source, "pkg/guestagent/lima-guestagent.amd64");
writeFileSync(guest + ".gz", gzipSync(readFileSync(guest)));
const guestOutput = resolve(source, "overlay/build/guest");
mkdirSync(guestOutput, { recursive: true });
// Keep the upstream Makefile's module/build boundaries. All guest components
// are built from the same patched source, even in the management-only bundle.
for (const [module, target, name] of [
  ["wsl-guestagent", ".", "rancher-desktop-guest-agent"],
  ["networking", "./cmd/network", "network-setup"],
  ["networking", "./cmd/vm", "vm-switch"],
  ["networking", "./cmd/proxy", "wsl-proxy"],
  ["rd-init", ".", "rd-init"],
  ["rdd-guest", ".", "rdd-guest"],
]) {
  run(
    [
      "-C",
      resolve(source, "src", module),
      "build",
      "-p",
      "4",
      "-trimpath",
      "-ldflags=-s -w",
      "-o",
      resolve(guestOutput, name),
      target,
    ],
    { GOOS: "linux", GOARCH: "amd64", CGO_ENABLED: "0" },
  );
}
const packageName =
  "github.com/rancher-sandbox/rancher-desktop-daemon/pkg/version";
if (withDistro) {
  const xz = process.env.ORBWIN_XZ ?? "C:/msys64/usr/bin/xz.exe";
  run(
    [
      "run",
      "./cmd/download-guest-deps",
      "--os",
      "windows",
      "--arch",
      "amd64",
      "--dest",
      "overlay/build",
      "--cache",
      resolve(root, ".tools/guest-cache"),
    ],
    { GOOS: "windows", GOARCH: "amd64", CGO_ENABLED: "0" },
  );
  const nerdctl = resolve(source, "overlay/build/nerdctl");
  mkdirSync(nerdctl, { recursive: true });
  execFileSync(
    "tar.exe",
    [
      "-xzf",
      resolve(source, "overlay/build/nerdctl-full.tar.gz"),
      "-C",
      nerdctl,
      "bin/buildctl",
      "bin/buildkitd",
      "bin/nerdctl",
    ],
    { stdio: "inherit" },
  );
  function xzToFile(args, destination) {
    const temporary = destination + ".partial";
    const fd = openSync(temporary, "w");
    try {
      execFileSync(xz, args, { stdio: ["ignore", fd, "inherit"] });
    } finally {
      closeSync(fd);
    }
    renameSync(temporary, destination);
  }
  const pristine = resolve(source, "overlay/build/distro.tar.xz");
  const working = resolve(source, "overlay/build/distro.tar");
  xzToFile(["--decompress", "--stdout", pristine], working);
  const mtime = execFileSync(
    "git",
    ["show", "-s", "--format=%ct", lock.commit],
    { cwd: source, encoding: "utf8" },
  ).trim();
  run(
    [
      "run",
      "./cmd/distro-overlay",
      "--manifest",
      "overlay/manifest.yaml",
      "--source",
      "overlay",
      "--profile",
      "wsl",
      "--mtime",
      mtime,
      "--in-place",
      working,
    ],
    { GOOS: "windows", GOARCH: "amd64", CGO_ENABLED: "0" },
  );
  xzToFile(
    ["-1", "--threads=2", "--stdout", working],
    resolve(source, "pkg/embedded/distro.tar.xz"),
  );
}
const ldflags = `-s -w -X ${packageName}.Version=v0.1.0-orbwin-dev -X ${packageName}.GitCommit=${lock.commit}`;
run(
  [
    "build",
    "-p",
    "4",
    "-trimpath",
    "-buildvcs=false",
    `-tags=sqlite_omit_load_extension${withDistro ? ",with_distro" : ""}`,
    `-ldflags=${ldflags}`,
    "-o",
    resolve(output, "rdd.exe"),
    "./cmd/rdd",
  ],
  {
    GOOS: "windows",
    GOARCH: "amd64",
    CGO_ENABLED: "1",
    CC: resolve(ccDir, "gcc.exe"),
    PATH: `${ccDir};${process.env.PATH ?? process.env.Path}`,
    CGO_CFLAGS: "-DSQLITE_ENABLE_DBSTAT_VTAB=1 -DSQLITE_USE_ALLOCA=1",
  },
);
const hash = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");
writeFileSync(
  resolve(output, "build-manifest.json"),
  JSON.stringify(
    {
      sourceCommit: lock.commit,
      go: lock.go.version,
      distroEmbedded: withDistro,
      purpose: withDistro
        ? "Guest-capable development artifact; runtime coexistence remains unqualified."
        : "Management-only development build. Guest startup unsupported.",
      patches: lock.patches.map((path) => ({
        path,
        sha256: hash(resolve(root, path)),
      })),
      binarySha256: hash(resolve(output, "rdd.exe")),
      guestBinaries: [
        "rancher-desktop-guest-agent",
        "network-setup",
        "vm-switch",
        "wsl-proxy",
        "rd-init",
        "rdd-guest",
      ].map((name) => ({ name, sha256: hash(resolve(guestOutput, name)) })),
    },
    null,
    2,
  ),
);
console.log(
  withDistro
    ? "Built guest-capable rdd. Guest startup remains gated on isolation qualification."
    : "Built native rdd management binary. A guest-capable bundle remains required.",
);
