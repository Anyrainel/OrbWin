import {
  prepareSdkWinioInput,
  captureSdkWinio,
  settleSdkWinio,
} from "./sdk-winio-build-input.mjs";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { buildOutput, buildTestArguments } from "./build-output.mjs";
import {
  prepareColdPayloadInput,
  verifyColdPayloadInput,
} from "./session-cold-payload-input.mjs";
import {
  sessionSourceNames,
  serviceSourceNames,
  verifySessionSources,
} from "./session-source-catalog.mjs";
const root = fileURLToPath(new URL("../", import.meta.url));
const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
const directory = resolve(root, "backend-tools/session");
const selectedOutput = buildOutput(root, process.argv.slice(2), [
  "--build-only",
]);
const output = selectedOutput.directory;
// Preflight fixed Windows SDK module/environment before any build child.
const sdkWinio = prepareSdkWinioInput({
  root,
  source: directory,
  output,
  moduleCache:
    process.env.GOMODCACHE ??
    resolve(
      process.env.GOPATH ?? resolve(process.env.USERPROFILE, "go"),
      "pkg/mod",
    ),
  environment: process.env,
});
const buildOnly = selectedOutput.flags.has("--build-only");
const binary = resolve(output, "orbwin-session.exe");
const bundleKey = process.env.ORBWIN_LAN_BUNDLE_PUBLIC_KEY;
if (
  bundleKey !== undefined &&
  (!/^[a-f0-9]{64}$/.test(bundleKey) || /^0+$/.test(bundleKey))
)
  throw new Error(
    "Build-time LAN verifier must be a nonzero raw Ed25519 public key",
  );
const sourceNames = sessionSourceNames(directory);
const hash = (path) =>
  createHash("sha256").update(readFileSync(path)).digest("hex");
const sourceHashes = Object.fromEntries(
  sourceNames.map((name) => [name, hash(resolve(directory, name))]),
);
const fsSources = [
  "fs-helper/main_linux.go",
  "fs-helper/main_other.go",
  "fs-helper/volume_linux.go",
  "go.mod",
  "go.sum",
];
const fsHashes = Object.fromEntries(
  fsSources.map((name) => [name, hash(resolve(directory, name))]),
);
const fsBinary = resolve(output, "orbwin-fs");
execFileSync(go, ["build", "-trimpath", "-o", fsBinary, "./fs-helper"], {
  cwd: directory,
  env: { ...process.env, CGO_ENABLED: "0", GOOS: "linux", GOARCH: "amd64" },
  stdio: "inherit",
  windowsHide: true,
});
if (fsSources.some((name) => hash(resolve(directory, name)) !== fsHashes[name]))
  throw new Error("Guest file helper sources changed during build");
writeFileSync(
  resolve(output, "fs-manifest.json"),
  JSON.stringify(
    { schema: 1, abi: 1, binarySha256: hash(fsBinary), sources: fsHashes },
    null,
    2,
  ),
);
const env = sdkWinio.environment;
const projectSources = [
  "project-client/mapping.go",
  "project-client/mapping_linux.go",
  "project-client/mapping_review_linux.go",
  "project-client/config.go",
  "project-client/main_linux.go",
  "project-client/main_other.go",
  "go.mod",
  "go.sum",
];
const projectHashes = Object.fromEntries(
  projectSources.map((name) => [name, hash(resolve(directory, name))]),
);
const projectBinary = resolve(output, "orbwin-project");
execFileSync(
  go,
  ["build", "-trimpath", "-o", projectBinary, "./project-client"],
  {
    cwd: directory,
    env: { ...process.env, CGO_ENABLED: "0", GOOS: "linux", GOARCH: "amd64" },
    stdio: "inherit",
    windowsHide: true,
  },
);
if (
  projectSources.some(
    (name) => hash(resolve(directory, name)) !== projectHashes[name],
  )
)
  throw new Error("WSL project helper sources changed during build");
writeFileSync(
  resolve(output, "project-manifest.json"),
  JSON.stringify(
    {
      schema: 1,
      abi: 1,
      binarySha256: hash(projectBinary),
      sources: projectHashes,
    },
    null,
    2,
  ),
);
const serviceSources = serviceSourceNames;
const serviceHashes = Object.fromEntries(
  serviceSources.map((name) => [name, hash(resolve(directory, name))]),
);
const serviceBinary = resolve(output, "orbwin-services");
execFileSync(
  go,
  ["build", "-trimpath", "-o", serviceBinary, "./service-tools"],
  {
    cwd: directory,
    env: { ...process.env, CGO_ENABLED: "0", GOOS: "linux", GOARCH: "amd64" },
    stdio: "inherit",
    windowsHide: true,
  },
);
if (
  serviceSources.some(
    (name) => hash(resolve(directory, name)) !== serviceHashes[name],
  )
)
  throw new Error("Local service tools sources changed during build");
writeFileSync(
  resolve(output, "services-manifest.json"),
  JSON.stringify(
    {
      schema: 1,
      abi: 5,
      binarySha256: hash(serviceBinary),
      sources: serviceHashes,
    },
    null,
    2,
  ),
);
const trustSources = [
  "trust-helper/main_linux.go",
  "trust-helper/main_other.go",
  "internal/wsltrust/protocol.go",
  "internal/wsltrust/store_linux.go",
  "internal/wsltrust/actions_linux.go",
  "internal/wsltrust/serve_linux.go",
  "internal/wsltrust/guest_issuer.go",
  "go.mod",
  "go.sum",
];
const trustHashes = Object.fromEntries(
  trustSources.map((name) => [name, hash(resolve(directory, name))]),
);
const trustBinary = resolve(output, "orbwin-trust");
execFileSync(go, ["build", "-trimpath", "-o", trustBinary, "./trust-helper"], {
  cwd: directory,
  env: { ...process.env, CGO_ENABLED: "0", GOOS: "linux", GOARCH: "amd64" },
  stdio: "inherit",
  windowsHide: true,
});
if (
  trustSources.some(
    (name) => hash(resolve(directory, name)) !== trustHashes[name],
  )
)
  throw new Error("Selected distro trust helper sources changed during build");
writeFileSync(
  resolve(output, "trust-manifest.json"),
  JSON.stringify(
    {
      schema: 1,
      abi: 1,
      binarySha256: hash(trustBinary),
      sources: trustHashes,
    },
    null,
    2,
  ),
);
const coldPayloads = prepareColdPayloadInput(root, output);
const sdkBefore = captureSdkWinio(sdkWinio, go, "sdk", coldPayloads.argument);
for (const args of [
  ...buildTestArguments(buildOnly).map((args) => [
    args[0],
    ...sdkWinio.flags,
    ...args.slice(1),
  ]),
  [
    "build",
    ...sdkWinio.flags,
    "-trimpath",
    coldPayloads.argument,
    ...(bundleKey
      ? [
          "-ldflags=-X orbwin.local/session/internal/lanhelper.bundleVerifierPublicKeyHex=" +
            bundleKey,
        ]
      : []),
    "-o",
    binary,
    ".",
  ],
])
  execFileSync(go, args, {
    cwd: directory,
    env,
    stdio: "inherit",
    windowsHide: true,
  });
const sdkCapture = settleSdkWinio(
  sdkWinio,
  sdkBefore,
  captureSdkWinio(sdkWinio, go, "sdk", coldPayloads.argument),
);
verifySessionSources(
  directory,
  { sources: sourceHashes },
  sessionSourceNames(directory),
);
verifyColdPayloadInput(root, output, coldPayloads.manifest);
writeFileSync(
  resolve(output, "session-manifest.json"),
  JSON.stringify(
    {
      sdk: "github.com/moby/moby/client@v0.6.2",
      ...(bundleKey ? { lanBundlePublicKeyHex: bundleKey } : {}),
      binarySha256: hash(binary),
      testsNotRun: buildOnly,
      coldPayloads: coldPayloads.manifest,
      sdkWinio: {
        portable: sdkWinio.portable,
        sourceFiles: sdkWinio.sourceFiles,
        capture: sdkCapture,
      },
      sources: sourceHashes,
    },
    null,
    2,
  ),
);
