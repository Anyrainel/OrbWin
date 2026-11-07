import test from "node:test";
import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  rmSync,
  symlinkSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, resolve } from "node:path";
import { catalog, verifyCatalog, digest } from "./build-docker-cli.mjs";
import {
  verifyDockerCliBinary,
  verifyDockerCliLegal,
  verifyDockerCliSource,
} from "./docker-cli-provenance.mjs";
import { validateRuntimeToolPaths } from "./stage-runtime-tools.mjs";
function fixture(t) {
  const base = resolve(tmpdir()),
    dir = mkdtempSync(resolve(base, "orbwin-docker-cli-proof-"));
  t.after(() => {
    assert.equal(dirname(dir), base);
    assert(basename(dir).startsWith("orbwin-docker-cli-proof-"));
    rmSync(dir, { recursive: true, force: true });
  });
  return dir;
}
test("frozen Docker CLI inventory rejects source/vendor drift and added code", (t) => {
  const dir = fixture(t);
  mkdirSync(resolve(dir, "vendor"));
  writeFileSync(resolve(dir, "vendor/dependency.go"), "reviewed");
  const pin = catalog(dir);
  verifyCatalog(dir, pin);
  writeFileSync(resolve(dir, "vendor/dependency.go"), "altered");
  assert.throws(() => verifyCatalog(dir, pin));
  writeFileSync(resolve(dir, "vendor/dependency.go"), "reviewed");
  writeFileSync(resolve(dir, "added.go"), "extra");
  assert.throws(() => verifyCatalog(dir, pin));
});
test("source/toolchain junctions are rejected even for matching content", (t) => {
  const dir = fixture(t),
    foreign = resolve(dir, "foreign"),
    source = resolve(dir, "source");
  mkdirSync(foreign);
  mkdirSync(source);
  writeFileSync(resolve(foreign, "input.go"), "reviewed");
  symlinkSync(foreign, resolve(source, "redirect"), "junction");
  assert.throws(() => catalog(source), /Linked/);
});
test("Docker binary target path and reviewed hash cannot be substituted", (t) => {
  const dir = fixture(t),
    bytes = Buffer.from("inert executable bytes");
  writeFileSync(resolve(dir, "docker.exe"), bytes);
  const record = {
    path: "docker.exe",
    platform: "windows/amd64",
    binarySha256: digest(bytes),
  };
  verifyDockerCliBinary(dir, "windowsAmd64", record);
  assert.throws(() => verifyDockerCliBinary(dir, "linuxAmd64", record));
  assert.throws(() =>
    verifyDockerCliBinary(dir, "windowsAmd64", {
      ...record,
      path: "../docker.exe",
    }),
  );
  writeFileSync(resolve(dir, "docker.exe"), "changed");
  assert.throws(() => verifyDockerCliBinary(dir, "windowsAmd64", record));
});
test("legal inventory binds bytes, target, module count and closed paths", (t) => {
  const dir = fixture(t),
    legal = resolve(dir, "licenses/windowsAmd64");
  mkdirSync(legal, { recursive: true });
  const bytes = Buffer.from("inert legal notice");
  writeFileSync(resolve(legal, "LICENSE"), bytes);
  const index = {
    binarySha256: "a".repeat(64),
    platform: "windows/amd64",
    embeddedModuleMetadataVerified: true,
    modules: [
      {
        files: [
          { path: "LICENSE", bytes: bytes.length, sha256: digest(bytes) },
        ],
      },
    ],
    source: [],
    toolchain: [],
  };
  const indexPath = resolve(legal, "index.json");
  writeFileSync(indexPath, JSON.stringify(index));
  const record = {
    binarySha256: index.binarySha256,
    platform: index.platform,
    compiledModules: 1,
    licenseDirectory: "licenses/windowsAmd64",
    licenseIndexSha256: digest(readFileSync(indexPath)),
  };
  verifyDockerCliLegal(dir, "windowsAmd64", record);
  assert.throws(() =>
    verifyDockerCliLegal(dir, "windowsAmd64", {
      ...record,
      compiledModules: 0,
    }),
  );
  writeFileSync(resolve(legal, "LICENSE"), "changed");
  assert.throws(() => verifyDockerCliLegal(dir, "windowsAmd64", record));
  index.modules[0].files[0].path = "../LICENSE";
  writeFileSync(indexPath, JSON.stringify(index));
  assert.throws(() =>
    verifyDockerCliLegal(dir, "windowsAmd64", {
      ...record,
      licenseIndexSha256: digest(readFileSync(indexPath)),
    }),
  );
});
test("candidate source selector cannot point at selected tools or traverse", (t) => {
  const dir = fixture(t);
  for (const manifest of [
    "artifacts/runtime-tools/manifest.json",
    "artifacts/docker-cli/../runtime-tools/manifest.json",
    "C:/foreign/manifest.json",
  ])
    assert.throws(() =>
      verifyDockerCliSource(dir, {
        localSources: {
          dockerCli: {
            kind: "orbwin-docker-cli-v1",
            manifest,
            manifestSha256: "a".repeat(64),
          },
        },
        tools: [],
      }),
    );
});

test("future Docker CLI source routing is closed to the two selected destinations", (t) => {
  const dir = fixture(t);
  const tool = (path, target) => ({
    path,
    localSource: { name: "dockerCli", target },
  });
  validateRuntimeToolPaths(dir, {
    tools: [
      tool("bin/docker.exe", "windowsAmd64"),
      tool("linux/docker", "linuxAmd64"),
    ],
  });
  for (const record of [
    tool("docker.exe", "windowsAmd64"),
    tool("bin/docker.exe", "linuxAmd64"),
    tool("../docker.exe", "windowsAmd64"),
  ])
    assert.throws(() => validateRuntimeToolPaths(dir, { tools: [record] }));
});
