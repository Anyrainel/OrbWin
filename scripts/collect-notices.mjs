import { verifySdkWinioProvenance, classifySdkWinioNotice, stageSdkWinioNotice } from "./sdk-winio-build-input.mjs";
import { verifyLanBundleBuild } from "./lan-bundle-provenance.mjs";
// Preserve dependency attribution from the exact local build inputs. This is an
// inventory and notice collector, not a claim of legal review or source parity.
import { execFileSync } from "node:child_process";
import {
  existsSync,
  readdirSync,
  readFileSync,
  mkdirSync,
  copyFileSync,
  writeFileSync,
} from "node:fs";
import { resolve, dirname, join, basename, relative, isAbsolute } from "node:path";
import { createHash } from "node:crypto";
import { stageMaintenanceNotices } from "./maintenance-notices.mjs";
import { stageDesktopInstallerNotices } from "./desktop-installer-provenance.mjs";
import { copyComposeProvenance } from "./stage-runtime-tools.mjs";
import { copyBuildxProvenance } from "./buildx-provenance.mjs";
import { copyDockerCliProvenance } from "./docker-cli-provenance.mjs";
import { stageRddForwarderNotices } from "./rdd-forwarder-build.mjs";
import {
  limaBuildEnvironment,
  verifyLimaBuildInput,
  verifyPortableLimaBuild,
} from "./lima-wsl2-build-input.mjs";

import { verifyPortableWinioBuild, stageWinioSourceOffer, classifyWinioNoticeData, winioNoticeVariantSuffixData } from "./winio-build-input.mjs";

export function collectNotices(root, destination, inputs) {
  if (
    !inputs ||
    !["backend", "session", "credentials", "tools", "cli"].every(
      (k) => inputs[k],
    )
  )
    throw new Error("Explicit notice component inputs required");
  // SDK062 is a separate source variant restricted to these three artifacts.
  verifyLanBundleBuild(root, inputs.session);
  const sdkBuild = JSON.parse(readFileSync(resolve(inputs.session, "lan-build-manifest.json")));
  for (const role of ["sdk", "helper", "worker"])
    verifySdkWinioProvenance(root, inputs.session, sdkBuild.sdkWinioTargets[role], role);
  const sdkOffer = sdkBuild.sdkWinioTargets.sdk.portable;
  const environment = limaBuildEnvironment(process.env);
  const upstream = resolve(
    root,
    process.env.ORBWIN_UPSTREAM_DIR ?? ".upstream/rancher-desktop",
  );
  const rdd = resolve(upstream, "rdd");
  const build = JSON.parse(
    readFileSync(resolve(inputs.backend, "build-manifest.json")),
  );
  const limaInput = verifyLimaBuildInput({
    root,
    source: rdd,
    manifest: build.limaWsl2,
    environment,
  });
  const limaOffer = verifyPortableLimaBuild(root, inputs.backend, build, {
    licensesDirectory: resolve(inputs.backend, "licenses"),
  });
  const limaModule = "github.com/lima-vm/lima/v2";
  const winioModule = "github.com/Microsoft/go-winio";
  const winioOffer = verifyPortableWinioBuild({ root, output: inputs.backend, portable: build.portableWinio });
  const winioRoot = limaInput.manifest.winioInput.winioRoot;
  function checkWinioReplacement(replacement, requireDirectory = false) {
    if (!replacement || resolve(replacement.Path) !== winioRoot ||
        (replacement.Dir && resolve(replacement.Dir) !== winioRoot) ||
        (requireDirectory && (typeof replacement.Dir !== "string" || resolve(replacement.Dir) !== winioRoot)) ||
        (replacement.Version && replacement.Version !== "(devel)"))
      throw new Error("Winio replacement differs from reviewed root source input");
  }
  function checkLimaReplacement(replacement, requireDirectory = false) {
    if (
      !replacement ||
      typeof replacement.Path !== "string" ||
      !isAbsolute(replacement.Path) ||
      resolve(replacement.Path) !== limaInput.limaRoot ||
      (replacement.Version && replacement.Version !== "(devel)") ||
      (requireDirectory &&
        (typeof replacement.Dir !== "string" ||
          !isAbsolute(replacement.Dir) ||
          resolve(replacement.Dir) !== limaInput.limaRoot))
    )
      throw new Error("Lima local replacement differs from reviewed build input");
  }
  const inventory = [],
    missing = [];
  inventory.push(
    stageDesktopInstallerNotices(
      root,
      resolve(destination, "installer-template"),
    ),
  );
  inventory.push(
    stageRddForwarderNotices(
      root,
      resolve(destination, "rdd-forwarder"),
      inputs.backend,
    ),
  );
  const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
  const safe = (text) => text.replaceAll(/[^a-zA-Z0-9_.-]/g, "_");
  function record(kind, name, version, directory, extra = {}) {
    const variant = extra.variant === "orbwin-sdk-winio062-close" ? "#orbwin-sdk-winio062-close" : winioNoticeVariantSuffixData(extra.variant);
    const output = resolve(destination, kind, safe(`${name}@${version}${variant}`));
    const files = [];
    if (existsSync(directory)) {
      const names = readdirSync(directory, { withFileTypes: true }).filter(
        (entry) =>
          /^(licen[cs]e|copying|notice|copyright|unlicense)([._-]|$)/i.test(
            entry.name,
          ),
      );
      for (const entry of names) {
        if (entry.isFile()) {
          mkdirSync(output, { recursive: true });
          copyFileSync(join(directory, entry.name), join(output, entry.name));
          files.push({
            name: entry.name,
            sha256: hash(readFileSync(join(directory, entry.name))),
          });
        } else if (entry.isDirectory()) {
          for (const child of readdirSync(join(directory, entry.name), {
            withFileTypes: true,
          }).filter((file) => file.isFile())) {
            mkdirSync(join(output, entry.name), { recursive: true });
            copyFileSync(
              join(directory, entry.name, child.name),
              join(output, entry.name, child.name),
            );
            files.push({
              name: `${entry.name}/${child.name}`,
              sha256: hash(
                readFileSync(join(directory, entry.name, child.name)),
              ),
            });
          }
        }
      }
    }
    const supplemental = resolve(
      root,
      "third-party/notices",
      `${name}-${version}`,
    );
    if (!files.length && existsSync(supplemental)) {
      const provenance = JSON.parse(
        readFileSync(join(supplemental, "provenance.json")),
      );
      {
        if (provenance.name !== name || provenance.version !== version)
          throw new Error("Supplemental notice identity differs");
        const file = readdirSync(supplemental).find((file) =>
          /^LICENSE/.test(file),
        );
        if (
          !file ||
          hash(readFileSync(join(supplemental, file))) !== provenance.sha256
        )
          throw new Error("Supplemental notice checksum differs");
        mkdirSync(output, { recursive: true });
        copyFileSync(join(supplemental, file), join(output, file));
        copyFileSync(
          join(supplemental, "provenance.json"),
          join(output, "provenance.json"),
        );
        files.push({
          name: file,
          sha256: provenance.sha256,
          source: provenance.url,
        });
      }
    }
    if (
      kind === "go" &&
      ((name === "../ports" &&
        resolve(directory) === resolve(rdd, "src/ports")) ||
        (["./pkg/proxyguestprotocol", "../../pkg/proxyguestprotocol"].includes(
          name,
        ) &&
          resolve(directory) === resolve(rdd, "pkg/proxyguestprotocol"))) &&
      files.length === 0
    ) {
      const license = resolve(upstream, "LICENSE");
      mkdirSync(output, { recursive: true });
      copyFileSync(license, join(output, "LICENSE"));
      files.push({
        name: "LICENSE",
        sha256: hash(readFileSync(license)),
        source:
          "Pinned Rancher repository root; explicitly selected Apache-2.0 local module",
      });
    }
    if (kind === "rust" && name === "selectors") {
      const index = dirname(directory),
        registry = dirname(dirname(index));
      const archive = join(
        registry,
        "cache",
        basename(index),
        `${name}-${version}.crate`,
      );
      const block = readFileSync(resolve(root, "src-tauri/Cargo.lock"), "utf8")
        .split("[[package]]")
        .find(
          (block) =>
            block.includes(`name = "${name}"\n`) &&
            block.includes(`version = "${version}"\n`),
        );
      const expected = block?.match(/checksum = "([a-f0-9]{64})"/)?.[1];
      if (!expected || hash(readFileSync(archive)) !== expected)
        throw new Error("MPL source archive differs from Cargo.lock");
      mkdirSync(output, { recursive: true });
      copyFileSync(archive, join(output, basename(archive)));
      files.push({
        name: basename(archive),
        sha256: expected,
        source: `https://crates.io/api/v1/crates/${name}/${version}/download`,
      });
    }
    const item = {
      kind,
      name,
      version,
      ...extra,
      ...(variant
        ? { noticeDirectory: relative(destination, output).replaceAll("\\", "/") }
        : {}),
      notices: files,
    };
    inventory.push(item);
    if (!files.length) missing.push({ kind, name, version });
  }
  const run = (exe, args, cwd = root) =>
    execFileSync(exe, args, {
      cwd,
      env: limaInput.childEnvironment,
      encoding: "utf8",
      windowsHide: true,
      maxBuffer: 32 << 20,
    });
  const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
  const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
  // Only the actual rdd module query receives its reviewed alternate modfile.
  // Binary metadata and nested-module dependency work retain their own context.
  const limaListed = JSON.parse(
    run(
      go,
      ["list", ...limaInput.rootModuleFlags, "-m", "-json", limaModule],
      rdd,
    ),
  );
  if (
    limaListed.Path !== limaModule ||
    limaListed.Version !== limaOffer.version
  )
    throw new Error("Lima module identity differs from portable source offer");
  checkLimaReplacement(limaListed.Replace, true);
  const selectedWinio = JSON.parse(run(go, ["list", ...limaInput.rootModuleFlags, "-m", "-json", winioModule], rdd));
  if (selectedWinio.Path !== winioModule || selectedWinio.Version !== "v0.6.4")
    throw new Error("Wrong actual root Winio MVS version");
  checkWinioReplacement(selectedWinio.Replace, true);
  stageWinioSourceOffer({root,output:inputs.backend,portable:winioOffer,destination:resolve(destination,"winio-custody")});
  const cache = run(go, ["env", "GOMODCACHE"]).trim();
  const escape = (value) =>
    value.replace(/[A-Z]/g, (char) => `!${char.toLowerCase()}`);
  const runtimePins = JSON.parse(
    readFileSync(resolve(root, "runtime-tools.lock.json")),
  );
  const composeProof = copyComposeProvenance(
    root,
    runtimePins,
    resolve(destination, "compose-source"),
  );
  inventory.push({
    kind: "upstream",
    name: "Docker Compose",
    version: composeProof.manifest.version,
    source: composeProof.manifest.repository,
    commit: composeProof.manifest.commit,
    patchSha256: composeProof.manifest.patchSha256,
    dependencyPatchSha256:
      composeProof.manifest.sourcePins.dependencyPatchSha256,
    notices: composeProof.legal.map((record) => ({
      platform: record.target,
      index: `compose-source/${record.target}/index.json`,
      modules: record.index.modules.length,
    })),
    sourceManifest: "compose-source/compose-manifest.json",
  });
  const buildxProof = copyBuildxProvenance(
    root,
    runtimePins,
    resolve(destination, "buildx-source"),
  );
  inventory.push({
    kind: "upstream",
    name: "Docker Buildx",
    version: buildxProof.manifest.version,
    source: buildxProof.manifest.repository,
    commit: buildxProof.manifest.commit,
    patchSha256: buildxProof.manifest.sourcePins.patchSha256,
    notices: buildxProof.legal.map((record) => ({
      platform: record.target,
      index: `buildx-source/${record.target}/index.json`,
      modules: record.index.modules.length,
    })),
    sourceManifest: "buildx-source/buildx-manifest.json",
  });
  const dockerCliProof = runtimePins.localSources?.dockerCli
    ? copyDockerCliProvenance(
        root,
        runtimePins,
        resolve(destination, "docker-cli-source"),
      )
    : null;
  if (dockerCliProof)
    inventory.push({
      kind: "upstream",
      name: "Docker CLI",
      version: dockerCliProof.manifest.version,
      source: dockerCliProof.manifest.repository,
      commit: dockerCliProof.manifest.commit,
      wrapperCommit: dockerCliProof.manifest.wrapperCommit,
      patches: dockerCliProof.manifest.patches,
      notices: dockerCliProof.legal.map((record) => ({
        platform: record.target,
        index: `docker-cli-source/${record.target}/index.json`,
        modules: record.index.modules.length,
      })),
      sourceManifest: "docker-cli-source/docker-cli-manifest.json",
    });
  const binaries = [
    [resolve(inputs.cli, "orbwin-cli.exe"), "backend-tools/cli"],
    [resolve(inputs.cli, "orbwin-docker.exe"), "backend-tools/cli"],
    [resolve(inputs.backend, "rdd.exe"), rdd, true],
    ...[
      "orbwin-session.exe",
      "orbwin-fs",
      "orbwin-project",
      "orbwin-services",
      "orbwin-trust",
      "lan/orbwin-lan-helper.exe",
      "lan/orbwin-lan-worker.exe",
    ].map((n) => [resolve(inputs.session, n), "backend-tools/session"]),
    [
      resolve(inputs.credentials, "docker-credential-orbwin.exe"),
      "backend-tools/credentials",
    ],
    [resolve(rdd, "pkg/guestagent/lima-guestagent.amd64"), rdd, true],
    ...[
      "rancher-desktop-guest-agent",
      "network-setup",
      "vm-switch",
      "wsl-proxy",
      "rd-init",
      "rdd-guest",
    ].map((name) => [
      resolve(rdd, `overlay/build/guest/${name}`),
      resolve(
        rdd,
        `src/${{ "rancher-desktop-guest-agent": "wsl-guestagent", "network-setup": "networking", "vm-switch": "networking", "wsl-proxy": "networking" }[name] ?? name}`,
      ),
    ]),
    ...runtimePins.tools.map((tool) => [
      resolve(inputs.tools, tool.path),
      tool.localSource?.name === "compose"
        ? ".upstream/compose"
        : tool.localSource?.name === "buildx"
          ? "artifacts/buildx/source"
          : tool.localSource?.name === "dockerCli"
            ? relative(root, resolve(dockerCliProof.outputRoot, "source"))
            : rdd,
    ]),
  ];
  const modules = new Map();
  // Keep the reviewed root-graph source distinct from any independent binary's
  // unmodified upstream Lima, even when their original module/version match.
  modules.set(`${limaModule}@${limaOffer.version}#orbwin-wsl2`, {
    Path: limaModule,
    Version: limaOffer.version,
    directory: limaOffer.directory,
    noticeMetadata: {
      upstreamModuleSum: limaOffer.manifest.upstreamModuleSum,
      sourceManifest: "../lima-wsl2/manifest.json",
      modified: true,
      variant: "orbwin-wsl2",
    },
  });
  modules.set(`${winioModule}@v0.6.4#orbwin-winio-custody`, {
    Path: winioModule, Version: "v0.6.4", directory: winioRoot,
    noticeMetadata: { modified: true, variant: "orbwin-winio-custody", sourceManifest: "../winio-custody/manifest.json", sourceLockSha256: winioOffer.lockSHA256 },
  });
  stageSdkWinioNotice(inputs.session, resolve(destination, "sdk-winio062-custody"), sdkBuild.sdkWinioTargets.sdk.sourceFiles);
  modules.set(`${winioModule}@v0.6.2#orbwin-sdk-winio062-close`, {
    Path: winioModule, Version: "v0.6.2", directory: resolve(inputs.session, "sdk-winio-source"),
    noticeMetadata: {modified: true, variant: "orbwin-sdk-winio062-close", upstreamModuleSum: sdkOffer.upstreamH1,
      sourceManifest: "../sdk-winio062-custody/manifest.json", sourceLockSha256: sdkOffer.lockSHA256},
  });
  const sdkArtifacts = {"orbwin-session.exe": "sdk", "lan/orbwin-lan-helper.exe": "helper", "lan/orbwin-lan-worker.exe": "worker"};
  const binaryInventory = [];
  for (const [binary, cwd, patchedLimaConsumer = false] of binaries) {
    const info = JSON.parse(
      run(go, ["version", "-m", "-json", resolve(root, binary)]),
    );
    binaryInventory.push({
      path: binary,
      sha256: hash(readFileSync(resolve(root, binary))),
      build: info,
    });
    let patchedLimaSeen = false;
    let patchedWinioSeen = false;
    const sdkArtifact = Object.keys(sdkArtifacts).find(n => resolve(binary) === resolve(inputs.session, n));
    let sdkWinioSeen = false;
    // The external Lima guestagent command can identify Lima as Main instead
    // of Deps. Other main modules retain the collector's existing treatment.
    const dependencies = [
      ...(info.Main?.Path === limaModule ? [info.Main] : []),
      ...(info.Deps ?? []),
    ];
    for (const dependency of dependencies) {
      if (dependency.Path === limaModule) {
        if (patchedLimaConsumer) {
          if (dependency.Version !== limaOffer.version)
            throw new Error("Root binary Lima version differs from portable source offer");
          checkLimaReplacement(dependency.Replace);
          patchedLimaSeen = true;
          continue;
        }
        if (
          dependency.Replace &&
          (!dependency.Replace.Version ||
            dependency.Replace.Version === "(devel)")
        )
          throw new Error("Independent binary Lima local replacement lacks verified source provenance");
      }
      if (sdkArtifact && classifySdkWinioNotice(dependency, sdkArtifact, sdkBuild.sdkWinioTargets[sdkArtifacts[sdkArtifact]], info.Main?.Path)) {
        sdkWinioSeen = true; continue;
      }
      const winioVariant=classifyWinioNoticeData(dependency,patchedLimaConsumer,winioRoot);
      if(winioVariant){patchedWinioSeen=true;continue;}
      const module = dependency.Replace ?? dependency;
      const key = `${module.Path}@${module.Version ?? "local"}`;
      const directory =
        module.Version && module.Version !== "(devel)"
          ? join(cache, `${escape(module.Path)}@${escape(module.Version)}`)
          : resolve(root, cwd, module.Path);
      modules.set(key, { ...module, directory });
    }
    if (sdkArtifact && !sdkWinioSeen) throw new Error("SDK artifact lacks its reviewed062 source variant");
    if (resolve(binary) === resolve(inputs.backend, "rdd.exe") && !patchedWinioSeen)
      throw new Error("Root rdd binary lacks reviewed Winio source metadata");
    if (patchedLimaConsumer && !patchedLimaSeen)
      throw new Error("Root binary lacks reviewed Lima Main or dependency metadata");
  }
  const absent = [...modules.values()].filter(
    (module) =>
      module.Version &&
      module.Version !== "(devel)" &&
      !existsSync(module.directory),
  );
  for (let start = 0; start < absent.length; start += 20) {
    run(go, [
      "mod",
      "download",
      ...absent
        .slice(start, start + 20)
        .map((module) => `${module.Path}@${module.Version}`),
    ]);
  }
  for (const module of modules.values())
    record("go", module.Path, module.Version ?? "local", module.directory, {
      sum: module.Sum,
      ...module.noticeMetadata,
    });
  record(
    "go",
    "Go",
    lock.go.version,
    resolve(root, `.tools/go${lock.go.version}/go`),
  );
  record("upstream", "Rancher Desktop", lock.commit, upstream);
  record(
    "upstream",
    "Docker Compose source",
    composeProof.manifest.commit,
    resolve(root, ".upstream/compose"),
    {
      repository: composeProof.manifest.repository,
      patchSha256: composeProof.manifest.patchSha256,
    },
  );
  const cargo = JSON.parse(
    run("cargo", [
      "metadata",
      "--manifest-path",
      "src-tauri/Cargo.toml",
      "--locked",
      "--format-version",
      "1",
      "--filter-platform",
      "x86_64-pc-windows-msvc",
    ]),
  );
  const included = new Set(cargo.resolve.nodes.map((node) => node.id));
  for (const crate of cargo.packages.filter(
    (item) => item.source && included.has(item.id),
  ))
    record("rust", crate.name, crate.version, dirname(crate.manifest_path), {
      license: crate.license,
      source: crate.source,
    });
  const npm = JSON.parse(readFileSync(resolve(root, "package-lock.json")));
  const maintenance = JSON.parse(
    readFileSync(resolve(root, "maintenance-image.lock.json")),
  );
  let maintenanceSources;
  try {
    maintenanceSources = stageMaintenanceNotices(
      root,
      resolve(
        destination,
        "image",
        safe(`Alpine maintenance@${maintenance.root}`),
      ),
    );
  } catch (error) {
    missing.push({
      kind: "image",
      name: "Alpine maintenance",
      version: maintenance.root,
      reason: String(error),
    });
  }
  inventory.push({
    kind: "image",
    name: "Alpine maintenance",
    version: maintenance.root,
    source: maintenance.source,
    archiveSha256: maintenance.sha256,
    config: maintenance.config,
    platform: `${maintenance.os}/${maintenance.architecture}`,
    distributionScope:
      "Full pinned image with package notices, upstream sources, recipes, patches and recorded static build contexts",
    sourceAccounting: maintenanceSources ?? null,
    notices: maintenanceSources?.notices ?? [],
  });
  for (const [path, item] of Object.entries(npm.packages).filter(
    ([path, item]) => path && !item.dev,
  )) {
    const directory = resolve(root, path);
    const meta = JSON.parse(readFileSync(join(directory, "package.json")));
    record("npm", meta.name, item.version, directory, {
      license: meta.license,
      integrity: item.integrity,
    });
  }
  limaInput.assertUnchanged();
  verifyPortableLimaBuild(root, inputs.backend, build, {
    licensesDirectory: resolve(inputs.backend, "licenses"),
  });
  mkdirSync(destination, { recursive: true });
  writeFileSync(
    join(destination, "inventory.json"),
    JSON.stringify(
      { schema: 1, inventory, binaries: binaryInventory, missing },
      null,
      2,
    ),
  );
  console.log(
    `Collected notices for ${inventory.length} dependencies; ${missing.length} need source-license resolution.`,
  );
  if (missing.length)
    throw new Error(
      `Dependency notices are incomplete. Inspect ${join(destination, "inventory.json")}`,
    );
}
