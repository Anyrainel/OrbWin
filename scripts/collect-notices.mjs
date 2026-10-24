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
import { resolve, dirname, join, basename } from "node:path";
import { createHash } from "node:crypto";

export function collectNotices(root, destination) {
  const inventory = [],
    missing = [];
  const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
  const safe = (text) => text.replaceAll(/[^a-zA-Z0-9_.-]/g, "_");
  function record(kind, name, version, directory, extra = {}) {
    const output = resolve(destination, kind, safe(`${name}@${version}`));
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
      name === "../ports" &&
      resolve(directory) ===
        resolve(root, ".upstream/rancher-desktop/rdd/src/ports") &&
      files.length === 0
    ) {
      const license = resolve(root, ".upstream/rancher-desktop/LICENSE");
      mkdirSync(output, { recursive: true });
      copyFileSync(license, join(output, "LICENSE"));
      files.push({
        name: "LICENSE",
        sha256: hash(readFileSync(license)),
        source: "Pinned Rancher repository root; local ports module",
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
    const item = { kind, name, version, ...extra, notices: files };
    inventory.push(item);
    if (!files.length) missing.push({ kind, name, version });
  }
  const run = (exe, args, cwd = root) =>
    execFileSync(exe, args, {
      cwd,
      encoding: "utf8",
      windowsHide: true,
      maxBuffer: 32 << 20,
    });
  const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
  const go = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
  const cache = run(go, ["env", "GOMODCACHE"]).trim();
  const escape = (value) =>
    value.replace(/[A-Z]/g, (char) => `!${char.toLowerCase()}`);
  const binaries = [
    ["artifacts/backend/rdd.exe", ".upstream/rancher-desktop/rdd"],
    ["artifacts/backend/orbwin-session.exe", "backend-tools/session"],
    [
      "artifacts/backend/docker-credential-orbwin.exe",
      "backend-tools/credentials",
    ],
    [
      ".upstream/rancher-desktop/rdd/pkg/guestagent/lima-guestagent.amd64",
      ".upstream/rancher-desktop/rdd",
    ],
    ...[
      "rancher-desktop-guest-agent",
      "network-setup",
      "vm-switch",
      "wsl-proxy",
      "rd-init",
      "rdd-guest",
    ].map((name) => [
      `.upstream/rancher-desktop/rdd/overlay/build/guest/${name}`,
      `.upstream/rancher-desktop/rdd/src/${{ "rancher-desktop-guest-agent": "wsl-guestagent", "network-setup": "networking", "vm-switch": "networking", "wsl-proxy": "networking" }[name] ?? name}`,
    ]),
    ...JSON.parse(
      readFileSync(resolve(root, "runtime-tools.lock.json")),
    ).tools.map((tool) => [
      `artifacts/runtime-tools/${tool.path}`,
      ".upstream/rancher-desktop/rdd",
    ]),
  ];
  const modules = new Map();
  const binaryInventory = [];
  for (const [binary, cwd] of binaries) {
    const info = JSON.parse(
      run(go, ["version", "-m", "-json", resolve(root, binary)]),
    );
    binaryInventory.push({
      path: binary,
      sha256: hash(readFileSync(resolve(root, binary))),
      build: info,
    });
    for (const dependency of info.Deps ?? []) {
      const module = dependency.Replace ?? dependency;
      const key = `${module.Path}@${module.Version ?? "local"}`;
      const directory =
        module.Version && module.Version !== "(devel)"
          ? join(cache, `${escape(module.Path)}@${escape(module.Version)}`)
          : resolve(root, cwd, module.Path);
      modules.set(key, { ...module, directory });
    }
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
    });
  record(
    "go",
    "Go",
    lock.go.version,
    resolve(root, `.tools/go${lock.go.version}/go`),
  );
  record(
    "upstream",
    "Rancher Desktop",
    lock.commit,
    resolve(root, ".upstream/rancher-desktop"),
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
