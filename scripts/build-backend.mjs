import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  readFileSync,
  writeFileSync,
  mkdirSync,
  openSync,
  closeSync,
  renameSync,
  existsSync,
  copyFileSync,
  constants,
} from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";
import { kubernetesBuildVersion } from "./kubernetes-build-version.mjs";
import {
  buildOutput,
  buildArguments,
  freshOutputPath,
} from "./build-output.mjs";
import { buildRddForwarder } from "./rdd-forwarder-build.mjs";
import { upgradeSdkBuildInput } from "./upgrade-sdk-build-input.mjs";
import {
  stageBackendGuestInputs,
  guestLicenseNames,
  captureBackendSourceFiles,
  verifyBackendSourceFiles,
} from "./backend-guest-input.mjs";
import {
  backendExecutionOrigin,
  isolatedUpstreamInput,
} from "./package-contract.mjs";
import {
  limaBuildEnvironment,
  prepareLimaWsl2BuildInput,
} from "./lima-wsl2-build-input.mjs";
import { projectWinioGoListData, winioBuildEnvironment } from "./winio-build-input.mjs";
const childEnv = winioBuildEnvironment(limaBuildEnvironment(process.env));
const root = fileURLToPath(new URL("../", import.meta.url));
freshOutputPath(
  root,
  buildArguments(process.argv.slice(2), ["--with-distro"]).requested,
);
const winioRuntimeOfferSource = readFileSync(resolve(root,"winio-runtime-offer.json"));
const lockSource = readFileSync(resolve(root, "upstream.lock.json"));
const lock = JSON.parse(lockSource);
const upstream = isolatedUpstreamInput(root, process.env.ORBWIN_UPSTREAM_DIR);
const guestPackagesLockSource = readFileSync(
  resolve(root, "guest-packages.lock.json"),
);
const guestPackagesLock = JSON.parse(guestPackagesLockSource);
execFileSync(process.execPath, [resolve(root, "scripts/upstream.mjs")], {
  env: childEnv,
  stdio: "inherit",
});
const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
const ccDir = resolve(root, ".tools/mingw/ucrt64/bin");
const source = resolve(upstream, "rdd");
const gitlinks = new Set([
  "bats/lib/bats-assert",
  "bats/lib/bats-core",
  "bats/lib/bats-file",
  "bats/lib/bats-support",
]);
const sourceNames = () =>
  execFileSync(
    "git",
    [
      "ls-files",
      "--cached",
      "--others",
      "--exclude-standard",
      "-z",
      "--",
      "rdd",
    ],
    {
      cwd: upstream,
      encoding: "utf8",
    },
  )
    .split("\0")
    .filter(Boolean)
    .map((name) => {
      if (!name.startsWith("rdd/"))
        throw new Error("Backend source inventory escaped rdd");
      return name.slice(4);
    })
    .filter((name) => !gitlinks.has(name));
const canonicalSource = captureBackendSourceFiles(source, sourceNames());
if (canonicalSource.length !== 1210)
  throw new Error("Expected complete current 1210-file backend source");
const buildScriptNames = [
  "build-backend.mjs",
  "build-output.mjs",
  "backend-guest-input.mjs",
  "rdd-forwarder-build.mjs",
  "upgrade-sdk-build-input.mjs",
  "package-contract.mjs",
  "kubernetes-build-version.mjs",
  "lima-wsl2-build-input.mjs",
  "winio-build-input.mjs",
  "winio-build-input-fs.mjs",
];
const recipeHash = (name) =>
  createHash("sha256")
    .update(readFileSync(resolve(root, "scripts", name)))
    .digest("hex");
const buildScripts = Object.fromEntries(
  buildScriptNames.map((name) => [name, recipeHash(name)]),
);
// The SDK bundle must be produced first. Legacy/source-only builds without the
// cold SDK reader keep their existing behavior; successor builds require all
// five verified files and the signing key before compiling rdd.
const hasUpgradeSDKReader = existsSync(
  resolve(source, "pkg/managementconfig/upgrade_sdk_image.go"),
);
const requestedSDKBundle = process.env.ORBWIN_UPGRADE_SDK_BUNDLE;
if (requestedSDKBundle && !hasUpgradeSDKReader)
  throw new Error("Selected backend source has no compiled SDK upgrade reader");
const upgradeSDK = hasUpgradeSDKReader
  ? upgradeSdkBuildInput(root, requestedSDKBundle)
  : undefined;
const origin = hasUpgradeSDKReader
  ? backendExecutionOrigin(root, source, lock)
  : {};
const selectedOutput = buildOutput(root, process.argv.slice(2), [
  "--with-distro",
]);
const output = selectedOutput.directory;
const withDistro = selectedOutput.flags.has("--with-distro");
const moduleCache = execFileSync(go, ["env", "GOMODCACHE"], {
  cwd: source,
  env: childEnv,
  encoding: "utf8",
}).trim();
const lima = prepareLimaWsl2BuildInput({
  root,
  source,
  output,
  moduleCache,
  environment: childEnv,
});
const guestInputs = withDistro
  ? stageBackendGuestInputs(root, source, output, guestPackagesLockSource)
  : undefined;
const forwarder = buildRddForwarder(root, source, output);
const run = (args, env) =>
  execFileSync(go, args, {
    cwd: source,
    env: { ...lima.childEnvironment, ...env },
    stdio: "inherit",
  });
// Only the root-rdd module consumes this modfile. Nested -C src modules below
// retain their own module graph and receive no rootModuleFlags.
const runRoot = ([command, ...args], env) =>
  run([command, ...lima.rootModuleFlags, ...args], env);
runRoot(
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
// Existing WSL disks are not reimported during an upgrade. Carry the matching
// guest bridge separately so owned provisioning can update executable code
// without replacing the user's root/data disk.
writeFileSync(
  resolve(source, "pkg/embedded/rdd-guest.amd64"),
  readFileSync(resolve(guestOutput, "rdd-guest")),
);
const packageName =
  "github.com/rancher-sandbox/rancher-desktop-daemon/pkg/version";
if (withDistro) {
  const xz = process.env.ORBWIN_XZ ?? "C:/msys64/usr/bin/xz.exe";
  // Exact offline inputs were verified/staged before compiler effects. These
  // generated copies belong only to the explicitly isolated source checkout.
  for (const name of ["distro.tar.xz", "mkcert", "nerdctl-full.tar.gz"])
    copyFileSync(
      resolve(guestInputs.upstream, name),
      resolve(source, "overlay/build", name),
      constants.COPYFILE_EXCL,
    );
  const packages = guestInputs.packages;
  const licenses = resolve(output, "licenses/guest-packages");
  mkdirSync(licenses, { recursive: true });
  for (const entry of Object.values(guestPackagesLock)) {
    const asset = entry.assets[0];
    const payload = readFileSync(resolve(packages, asset.filename));
    if (
      `sha256:${createHash("sha256").update(payload).digest("hex")}` !==
      asset.checksum
    )
      throw new Error(`Guest package checksum differs: ${asset.filename}`);
    const destination = asset.filename.endsWith("-source.rpm")
      ? resolve(licenses, asset.filename)
      : resolve(source, "pkg/embedded", asset.filename);
    writeFileSync(destination, payload);
  }
  for (const [rpm, member, filename] of [
    ["socat.rpm", "./usr/share/licenses/socat/COPYING", "socat-COPYING"],
    [
      "socat.rpm",
      "./usr/share/licenses/socat/COPYING.OpenSSL",
      "socat-COPYING.OpenSSL",
    ],
    [
      "libwrap.rpm",
      "./usr/share/doc/packages/libwrap0/DISCLAIMER",
      "libwrap-DISCLAIMER",
    ],
  ])
    writeFileSync(
      resolve(licenses, filename),
      execFileSync("tar.exe", ["-xOf", resolve(packages, rpm), member]),
    );
  writeFileSync(
    resolve(licenses, "guest-packages.lock.json"),
    guestPackagesLockSource,
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
  runRoot(
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
const kubernetesComponents = ["k8s.io/component-base", "k8s.io/client-go"].map(
  (module) =>
    kubernetesBuildVersion(
      JSON.parse(
        execFileSync(
          go,
          ["list", ...lima.rootModuleFlags, "-m", "-json", module],
          {
            cwd: source,
            env: lima.childEnvironment,
            encoding: "utf8",
          },
        ),
      ),
    ),
);
const kubernetesFlags = kubernetesComponents
  .map((component) => {
    const namespace =
      component.module +
      (component.module.endsWith("client-go") ? "/pkg/version" : "/version");
    return `-X ${namespace}.gitVersion=${component.version} -X ${namespace}.gitMajor=${component.major} -X ${namespace}.gitMinor=${component.minor} -X ${namespace}.gitCommit= -X ${namespace}.gitTreeState=archive`;
  })
  .join(" ");
const ldflags = `-s -w -X ${packageName}.Version=v0.1.0-orbwin-dev -X ${packageName}.GitCommit=${lock.commit} -X github.com/rancher-sandbox/rancher-desktop-daemon/pkg/binlinks.expectedForwarderSHA256=${forwarder.binarySha256} ${kubernetesFlags} ${upgradeSDK?.linkerFlags ?? ""}`;
const nativeRootEnvironment = {
    GOOS: "windows",
    GOARCH: "amd64",
    CGO_ENABLED: "1",
    CC: resolve(ccDir, "gcc.exe"),
    PATH: `${ccDir};${process.env.PATH ?? process.env.Path}`,
    CGO_CFLAGS: "-DSQLITE_ENABLE_DBSTAT_VTAB=1 -DSQLITE_USE_ALLOCA=1",
  };
const nativeRootTags = `sqlite_omit_load_extension${withDistro ? ",with_distro" : ""}`;
const nativeRootFlags = { cgo: "1", tags: nativeRootTags.split(",") };
function captureRootWinio() {
  const env = { ...lima.childEnvironment, ...nativeRootEnvironment };
  const options = { cwd: source, env, windowsHide: true, maxBuffer: 32 << 20 };
  const moduleBytes = execFileSync(go, ["list", ...lima.rootModuleFlags, "-m", "-json", "github.com/Microsoft/go-winio"], options);
  const packageBytes = execFileSync(go, ["list", ...lima.rootModuleFlags, "-p=4", `-tags=${nativeRootTags}`, "-compiled", "-deps", "-json=ImportPath,Dir,Module,GoFiles,CgoFiles,CompiledGoFiles,EmbedFiles,Error,DepsErrors", "./cmd/rdd"], options);
  const digest = (body) => createHash("sha256").update(body).digest("hex");
  const context = {
    artifact: "rdd.exe", main: "github.com/rancher-sandbox/rancher-desktop-daemon",
    goos: env.GOOS, goarch: env.GOARCH, goamd64: env.GOAMD64, ...nativeRootFlags,
    package: "./cmd/rdd", modMode: "readonly", modfile: lima.winio.manifest.modfile,
    modSHA256: digest(readFileSync(lima.winio.manifest.modfile)),
    sumSHA256: digest(readFileSync(lima.winio.manifest.modfile.replace(/\.mod$/, ".sum"))),
    sourceRoot: source, winioRoot: lima.winio.manifest.winioRoot,
  };
  const projected = projectWinioGoListData(moduleBytes, packageBytes, context);
  lima.winio.recordEffectiveClosure(projected.record, nativeRootFlags);
  return projected;
}
const winioBefore = captureRootWinio();
runRoot([
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
  ], nativeRootEnvironment);
const winioAfter = captureRootWinio();
const winioEffective = lima.winio.verifyEffectiveClosure(winioBefore.record, winioAfter.record, nativeRootFlags);

const hash = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");
if (!readFileSync(resolve(root, "upstream.lock.json")).equals(lockSource))
  throw new Error(
    "Upstream lock changed during build. No new manifest was published; rebuild from a stable source tree.",
  );
if (
  !readFileSync(resolve(root, "guest-packages.lock.json")).equals(
    guestPackagesLockSource,
  )
)
  throw new Error(
    "Guest package lock changed during build. Rebuild before publishing a manifest.",
  );
execFileSync(process.execPath, [resolve(root, "scripts/upstream.mjs")], {
  env: childEnv,
  stdio: "inherit",
});
if (!readFileSync(resolve(root,"winio-runtime-offer.json")).equals(winioRuntimeOfferSource)) throw new Error("Runtime source offer changed during build");
upgradeSDK?.assertUnchanged();
guestInputs?.assertUnchanged();
lima.assertUnchanged();
verifyBackendSourceFiles(source, canonicalSource, sourceNames());
for (const name of buildScriptNames)
  if (recipeHash(name) !== buildScripts[name])
    throw new Error("Backend build recipe changed during build");
writeFileSync(
  resolve(output, "build-manifest.json"),
  JSON.stringify(
    {
      sourceCommit: lock.commit,
      sourceCheckout: upstream,
      canonicalSource,
      buildScripts,
      ...origin,
      go: lock.go.version,
      kubernetesComponents,
      guestPackagesLockSha256: createHash("sha256")
        .update(guestPackagesLockSource)
        .digest("hex"),
      distroEmbedded: withDistro,
      purpose: withDistro
        ? "Guest-capable development artifact; runtime coexistence remains unqualified."
        : "Management-only development build. Guest startup unsupported.",
      patches: lock.patches.map((path) => ({
        path,
        sha256: hash(resolve(root, path)),
      })),
      binarySha256: hash(resolve(output, "rdd.exe")),
      forwarder,
      limaWsl2: lima.manifest,
      portableLima: lima.portable,
      portableWinio: lima.portableWinio,
      winioRuntimeOfferSHA256: createHash("sha256").update(winioRuntimeOfferSource).digest("hex"),
      winioInput: lima.winio.manifest,
      winioEffective: { before: winioBefore, after: winioAfter, checked: winioEffective },
      ...(guestInputs
        ? {
            guestInputs: {
              ...guestInputs.manifest,
              licenses: Object.fromEntries(
                guestLicenseNames.map((name) => [
                  name,
                  hash(resolve(output, "licenses/guest-packages", name)),
                ]),
              ),
            },
          }
        : {}),
      ...(upgradeSDK ? { upgradeSDK: upgradeSDK.manifest } : {}),
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
