import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { verifyBuild, sha256 } from "./build-identity.mjs";
import { verifyPackagedLanBundle } from "./lan-bundle-provenance.mjs";
import {
  inputDirectory,
  verifySDKBuildBinding,
  verifyCLIBuildBinding,
  isolatedUpstreamInput,
} from "./package-contract.mjs";

export function configurationForAssembly(root, input, manifest, base) {
  assert.deepEqual(
    Object.keys(manifest.applicationFiles ?? {}).sort(),
    [
      "orbwin-cli.exe",
      "orbwin-docker.exe",
      "cli-manifest.json",
      "completions/OrbWin.ps1",
    ].sort(),
    "Closed application file inventory required",
  );
  const config = structuredClone(base),
    runtime = resolve(input, "runtime");
  config.build.beforeBuildCommand = null; // wrapper owns explicit fresh frontend compilation
  config.build.frontendDist = resolve(root, "dist");
  config.bundle.resources = Object.fromEntries([
    [runtime + "/", "runtime/"],
    ...Object.keys(manifest.applicationFiles).map((n) => [
      resolve(input, n),
      n,
    ]),
  ]);
  config.bundle.icon = config.bundle.icon.map((n) =>
    resolve(root, "src-tauri", n),
  );
  const nsis = config.bundle.windows.nsis;
  for (const name of ["installerHooks", "template"])
    nsis[name] = resolve(root, "src-tauri", nsis[name]);
  nsis.customLanguageFiles = Object.fromEntries(
    Object.entries(nsis.customLanguageFiles).map(([k, v]) => [
      k,
      resolve(root, "src-tauri", v),
    ]),
  );
  return config;
}

export function packageTauriConfiguration(root, directory) {
  const input = inputDirectory(directory),
    runtime = resolve(input, "runtime"),
    backend = resolve(runtime, "backend"),
    tools = resolve(runtime, "tools");
  const manifest = JSON.parse(
    readFileSync(resolve(runtime, "package-manifest.json")),
  );
  assert.equal(manifest.schema, 1);
  assert.equal(manifest.product, "OrbWin");
  function verify(base, entries) {
    assert.ok(
      entries && typeof entries === "object" && !Array.isArray(entries),
    );
    for (const [name, digest] of Object.entries(entries)) {
      assert.ok(
        name
          .split("/")
          .every((p) => p && p !== "." && p !== ".." && !/[\\:]/.test(p)),
        "Closed package-relative path required",
      );
      assert.match(digest, /^[a-f0-9]{64}$/);
      assert.equal(
        sha256(readFileSync(resolve(base, name))),
        digest,
        "Prepared package input changed",
      );
    }
  }
  verify(runtime, manifest.hashes);
  verify(input, manifest.applicationFiles);
  const build = verifyBuild(root, backend, {
      licensesDirectory: resolve(runtime, "licenses"),
    }),
    lan = verifyPackagedLanBundle(root, backend);
  assert.equal(
    build.sourceCheckout,
    isolatedUpstreamInput(root, process.env.ORBWIN_UPSTREAM_DIR),
    "Replay checkout differs from package build source",
  );
  verifySDKBuildBinding(
    build,
    lan,
    JSON.parse(readFileSync(resolve(backend, "lan-build-manifest.json")))
      .publicKeyHex,
  );
  const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json"))),
    toolLock = readFileSync(resolve(root, "runtime-tools.lock.json"));
  const pins = JSON.parse(toolLock);
  for (const tool of pins.tools)
    assert.equal(sha256(readFileSync(resolve(tools, tool.path))), tool.sha256);
  const version = JSON.parse(
    readFileSync(resolve(root, "package.json")),
  ).version;
  assert.equal(manifest.version, version);
  verifyCLIBuildBinding(
    JSON.parse(readFileSync(resolve(input, "cli-manifest.json"))),
    {
      version,
      lock,
      build,
      patchClosureSHA: sha256(
        JSON.stringify(
          lock.patches.map((path) => ({
            path,
            sha256: sha256(readFileSync(resolve(root, path))),
          })),
        ),
      ),
      toolLockSHA: sha256(toolLock),
      credentialsSHA: sha256(
        readFileSync(resolve(backend, "docker-credential-orbwin.exe")),
      ),
      toolPins: Object.fromEntries(pins.tools.map((t) => [t.path, t.sha256])),
    },
  );
  const config = configurationForAssembly(
    root,
    input,
    manifest,
    JSON.parse(
      readFileSync(resolve(root, "src-tauri/tauri.package.conf.json")),
    ),
  );
  return { config, componentInput: backend, toolsInput: tools };
}
