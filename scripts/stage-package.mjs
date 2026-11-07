import assert from "node:assert/strict";
import {
  readFileSync,
  existsSync,
  mkdirSync,
  copyFileSync,
  cpSync,
  writeFileSync,
  realpathSync,
  readdirSync,
  chmodSync,
  lstatSync,
} from "node:fs";
import { resolve, dirname, sep, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { sha256, verifyBuild } from "./build-identity.mjs";
import { collectNotices } from "./collect-notices.mjs";
import { verifyLanBundleBuild } from "./lan-bundle-provenance.mjs";
import { verifyDesktopInstallerSource } from "./desktop-installer-provenance.mjs";
import {
  packageArguments,
  verifyCLIBuildBinding,
  verifySDKBuildBinding,
  isolatedUpstreamInput,
} from "./package-contract.mjs";
import { verifyDockerCliSource } from "./docker-cli-provenance.mjs";
import {
  sessionSourceNames,
  serviceSourceNames,
  verifySessionSources,
} from "./session-source-catalog.mjs";
import {
  verifyComposeSource,
  copyComposeProvenance,
} from "./stage-runtime-tools.mjs";
import {
  verifyBuildxSource,
  copyBuildxProvenance,
} from "./buildx-provenance.mjs";
const root = fileURLToPath(new URL("../", import.meta.url));
const inputs = packageArguments(root, process.argv.slice(2));
const upstream = isolatedUpstreamInput(root, process.env.ORBWIN_UPSTREAM_DIR);
const component = (name) =>
  resolve(
    name === "docker-credential-orbwin.exe" ||
      name === "credentials-manifest.json"
      ? inputs.credentials
      : /^(orbwin-(session\.exe|fs|project|services|trust)|(?:session|fs|project|services|trust|lan-build)-manifest\.json|lan\/)/.test(
            name,
          )
        ? inputs.session
        : inputs.backend,
    name,
  );
const backendRoles = [
  "rdd.exe",
  "rdd-forwarder.exe",
  "forwarder-build-manifest.json",
  "build-manifest.json",
  "orbwin-session.exe",
  "session-manifest.json",
  "lan-build-manifest.json",
  "lan/assets.json",
  "lan/assets.sig",
  "lan/orbwin-lan-helper.exe",
  "lan/orbwin-lan-worker.exe",
  "orbwin-fs",
  "orbwin-maintenance.tar",
  "fs-manifest.json",
  "orbwin-project",
  "project-manifest.json",
  "orbwin-services",
  "services-manifest.json",
  "orbwin-trust",
  "trust-manifest.json",
  "docker-credential-orbwin.exe",
  "credentials-manifest.json",
];
// Refuse changed template/provenance/licenses before any package output mutation.
verifyDesktopInstallerSource(root);
const build = verifyBuild(root, inputs.backend);
assert.equal(
  build.sourceCheckout,
  upstream,
  "Use the same explicit replay checkout for build notices",
);
assert.equal(
  build.distroEmbedded,
  true,
  "Build the guest-capable backend first",
);
const lanBundle = verifyLanBundleBuild(root, inputs.session);
verifySDKBuildBinding(
  build,
  lanBundle,
  JSON.parse(readFileSync(resolve(inputs.session, "lan-build-manifest.json")))
    .publicKeyHex,
);
for (const [manifestFile, folder] of [
  ["session-manifest.json", "session"],
  ["credentials-manifest.json", "credentials"],
]) {
  const manifest = JSON.parse(readFileSync(component(manifestFile)));
  if (folder === "session") {
    const directory = resolve(root, "backend-tools/session");
    verifySessionSources(directory, manifest, sessionSourceNames(directory));
  }
  const binary =
    folder === "session"
      ? "orbwin-session.exe"
      : "docker-credential-orbwin.exe";
  assert.equal(sha256(readFileSync(component(binary))), manifest.binarySha256);
  for (const [name, hash] of Object.entries(manifest.sources))
    assert.equal(
      sha256(readFileSync(resolve(root, "backend-tools", folder, name))),
      hash,
      `Stale ${folder} helper`,
    );
}
for (const [manifestFile, binary, abi] of [
  ["fs-manifest.json", "orbwin-fs", 1],
  ["project-manifest.json", "orbwin-project", 1],
  ["services-manifest.json", "orbwin-services", 5],
  ["trust-manifest.json", "orbwin-trust", 1],
]) {
  const manifest = JSON.parse(readFileSync(component(manifestFile)));
  if (binary === "orbwin-services") {
    verifySessionSources(
      resolve(root, "backend-tools/session"),
      manifest,
      serviceSourceNames,
    );
  }
  assert.equal(manifest.abi, abi);
  assert.equal(sha256(readFileSync(component(binary))), manifest.binarySha256);
  for (const [name, hash] of Object.entries(manifest.sources))
    assert.equal(
      sha256(readFileSync(resolve(root, "backend-tools/session", name))),
      hash,
      `Stale guest helper ${binary}: ${name}`,
    );
}
const tools = JSON.parse(
  readFileSync(resolve(root, "runtime-tools.lock.json")),
);
// Packaging must reject stale local Compose candidates even if an old cache exists.
verifyComposeSource(root, tools);
verifyBuildxSource(root, tools);
verifyDockerCliSource(root, tools);
// The two entry points are the same thin client. Verify its build inputs before
// staging either alias; a runtime-only package must not silently ship stale CLI.
const cliDirectory = resolve(root, "backend-tools/cli");
const cli = JSON.parse(readFileSync(resolve(inputs.cli, "cli-manifest.json")));
const packageVersion = JSON.parse(
  readFileSync(resolve(root, "package.json")),
).version;
const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
verifyCLIBuildBinding(cli, {
  version: packageVersion,
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
  toolLockSHA: sha256(readFileSync(resolve(root, "runtime-tools.lock.json"))),
  credentialsSHA: sha256(
    readFileSync(component("docker-credential-orbwin.exe")),
  ),
  toolPins: Object.fromEntries(tools.tools.map((t) => [t.path, t.sha256])),
});
const cliSources = readdirSync(cliDirectory)
  .filter(
    (name) =>
      (name.endsWith(".go") && !name.endsWith("_test.go")) ||
      name === "go.mod" ||
      name === "go.sum",
  )
  .sort();
assert.deepEqual(
  Object.keys(cli.sources).sort(),
  cliSources,
  "CLI source inventory changed; rebuild it",
);
for (const name of cliSources)
  assert.equal(
    sha256(readFileSync(resolve(cliDirectory, name))),
    cli.sources[name],
    `Stale CLI source: ${name}`,
  );
assert.equal(cli.alias.path, "orbwin-docker.exe");
assert.equal(cli.alias.sha256, cli.binarySha256);
for (const name of ["orbwin-cli.exe", "orbwin-docker.exe"])
  assert.equal(
    sha256(readFileSync(resolve(inputs.cli, name))),
    cli.binarySha256,
    `Stale CLI binary: ${name}`,
  );
const maintenance = JSON.parse(
  readFileSync(resolve(root, "maintenance-image.lock.json")),
);
assert.equal(maintenance.schema, 1);
assert.equal(maintenance.tagFree, true);
assert.equal(
  sha256(readFileSync(component("orbwin-maintenance.tar"))),
  maintenance.sha256,
  "Stage the pinned offline maintenance image before packaging",
);
for (const tool of tools.tools)
  assert.equal(
    sha256(readFileSync(resolve(inputs.tools, tool.path))),
    tool.sha256,
  );
const parent = inputs.output;
const componentHashes = Object.fromEntries(
  backendRoles.map((name) => [name, sha256(readFileSync(component(name)))]),
);
mkdirSync(parent); // exclusive; all verification above precedes output creation
function copyPackageFile(source, target) {
  let targetStat;
  try {
    targetStat = lstatSync(target);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  if (targetStat)
    assert(
      targetStat.isFile() && !targetStat.isSymbolicLink(),
      "Package file must be an ordinary file",
    );
  copyFileSync(source, target);
  assert.equal(
    sha256(readFileSync(target)),
    sha256(readFileSync(source)),
    "Package copy changed",
  );
}
for (const name of ["orbwin-cli.exe", "orbwin-docker.exe", "cli-manifest.json"])
  copyPackageFile(resolve(inputs.cli, name), resolve(parent, name));
for (const name of ["orbwin-cli.exe", "orbwin-docker.exe"])
  assert.equal(
    sha256(readFileSync(resolve(parent, name))),
    cli.binarySha256,
    "CLI changed during staging",
  );
assert.deepEqual(
  JSON.parse(readFileSync(resolve(parent, "cli-manifest.json"))),
  cli,
  "CLI manifest changed during staging",
);
const completionDirectory = resolve(parent, "completions");
mkdirSync(completionDirectory, { recursive: true });
assert.equal(
  realpathSync(completionDirectory),
  resolve(realpathSync(parent), "completions"),
);
copyPackageFile(
  resolve(root, "completions/OrbWin.ps1"),
  resolve(completionDirectory, "OrbWin.ps1"),
);
const destination = resolve(parent, "runtime");
assert(!existsSync(destination), "Fresh package output required");
mkdirSync(resolve(destination, "backend"), { recursive: true });
writeFileSync(
  resolve(destination, "stage.json"),
  JSON.stringify({ product: "OrbWin generated package runtime" }),
);
const hashes = {};
for (const name of backendRoles) {
  mkdirSync(dirname(resolve(destination, "backend", name)), {
    recursive: true,
  });
  copyFileSync(component(name), resolve(destination, "backend", name));
  hashes[`backend/${name}`] = sha256(
    readFileSync(resolve(destination, "backend", name)),
  );
  assert.equal(
    hashes[`backend/${name}`],
    componentHashes[name],
    "Component changed during package copy",
  );
  if (Object.hasOwn(lanBundle.pins, name))
    assert.equal(
      hashes[`backend/${name}`],
      lanBundle.pins[name],
      "LAN bundle changed while staging",
    );
  if (name === "rdd-forwarder.exe")
    assert.equal(
      hashes[`backend/${name}`],
      build.forwarder.binarySha256,
      "Forwarder changed while staging",
    );
}
// The relocated backend verifier requires the same closed license tree beside
// its forwarder manifest; the human-readable notices alone are insufficient.
for (const [name, digest] of Object.entries(build.forwarder.licenses)) {
  const target = resolve(destination, "backend/forwarder-licenses", name);
  mkdirSync(dirname(target), { recursive: true });
  copyPackageFile(resolve(inputs.backend, "forwarder-licenses", name), target);
  assert.equal(sha256(readFileSync(target)), digest, "Forwarder license changed");
  hashes[`backend/forwarder-licenses/${name}`] = digest;
}
for (const tool of tools.tools) {
  const path = resolve(destination, "tools", tool.path);
  mkdirSync(dirname(path), { recursive: true });
  copyFileSync(resolve(inputs.tools, tool.path), path);
  assert.equal(
    sha256(readFileSync(path)),
    tool.sha256,
    "Tool changed during package copy",
  );
  hashes[`tools/${tool.path}`] = tool.sha256;
}
copyComposeProvenance(
  root,
  tools,
  resolve(destination, "licenses/compose-source"),
);
copyBuildxProvenance(
  root,
  tools,
  resolve(destination, "licenses/buildx-source"),
);
cpSync(resolve(inputs.backend, "licenses"), resolve(destination, "licenses"), {
  recursive: true,
});
cpSync(
  resolve(inputs.credentials, "licenses"),
  resolve(destination, "licenses"),
  { recursive: true },
);
collectNotices(root, resolve(destination, "licenses/dependencies"), inputs);
copyFileSync(
  resolve(
    root,
    process.env.ORBWIN_UPSTREAM_DIR ?? ".upstream/rancher-desktop",
    "rdd/dependencies.yaml",
  ),
  resolve(destination, "licenses/upstream-guest-dependencies.yaml"),
);
copyFileSync(
  resolve(root, "upstream.lock.json"),
  resolve(destination, "licenses/upstream.lock.json"),
);
copyFileSync(
  resolve(root, "maintenance-image.lock.json"),
  resolve(destination, "licenses/maintenance-image.lock.json"),
);
cpSync(
  resolve(root, "patches/rdd"),
  resolve(destination, "licenses/rdd-patches"),
  { recursive: true },
);
// Go module caches mark license files read-only. Normalize only these generated
// copies, otherwise Tauri cannot replace resources during a repeat build.
function writableCopies(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = resolve(directory, entry.name);
    if (entry.isSymbolicLink())
      throw new Error("A generated package resource is a symbolic link");
    if (entry.isDirectory()) writableCopies(path);
    else if (entry.isFile()) {
      chmodSync(path, 0o644);
      if (path.startsWith(resolve(destination, "licenses") + sep))
        hashes[relative(destination, path).replaceAll("\\", "/")] = sha256(
          readFileSync(path),
        );
    }
  }
}
writableCopies(destination);
assert.deepEqual(
  verifyLanBundleBuild(root, inputs.session).pins,
  lanBundle.pins,
  "LAN build changed before package publication",
);
assert.deepEqual(
  verifyBuild(root, inputs.backend),
  build,
  "Backend build changed before package publication",
);
for (const name of backendRoles)
  assert.equal(
    sha256(readFileSync(component(name))),
    componentHashes[name],
    "Component input changed before publication",
  );
writeFileSync(
  resolve(destination, "package-manifest.json"),
  JSON.stringify(
    {
      schema: 1,
      product: "OrbWin",
      version: packageVersion,
      applicationFiles: Object.fromEntries(
        [
          "orbwin-cli.exe",
          "orbwin-docker.exe",
          "cli-manifest.json",
          "completions/OrbWin.ps1",
        ].map((name) => [name, sha256(readFileSync(resolve(parent, name)))]),
      ),
      hashes,
    },
    null,
    2,
  ),
);
console.log("Staged the checksum-verified per-user application runtime.");
