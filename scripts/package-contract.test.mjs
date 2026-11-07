import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createHash } from "node:crypto";
import {
  backendExecutionOrigin,
  executionOrigin,
  packageArguments,
  verifyCLIBuildBinding,
  isolatedUpstreamInput,
  cliBuildArguments,
} from "./package-contract.mjs";
test("CLI build selector rejects partial/mixed/repeated input shapes before build effects", () => {
  const base = ["--output", "absolute-later"];
  assert.equal(cliBuildArguments([...base, "--source-only"]).sourceOnly, true);
  assert.equal(
    cliBuildArguments([...base, "--candidate-input", "candidate"])[
      "--candidate-input"
    ],
    "candidate",
  );
  for (const tail of [
    ["--backend-input", "backend"],
    ["--candidate-input", "c", "--tools-input", "t"],
    ["--source-only", "--candidate-input", "c"],
    ["--source-only", "--source-only"],
    ["--candidate-input"],
  ])
    assert.throws(() => cliBuildArguments([...base, ...tail]));
});
const sha = (b) => createHash("sha256").update(b).digest("hex");
test("schema2 CLI requires exact compiled component and canonical closure epoch", () => {
  const lock = { commit: "a".repeat(40), patches: Array(18).fill("patch") };
  const context = {
    version: "0.1.0",
    lock,
    patchClosureSHA: sha("patches"),
    toolLockSHA: sha("tools"),
    build: {
      binarySha256: sha("rdd"),
      forwarder: { binarySha256: sha("forwarder") },
      executionOrigin,
    },
    credentialsSHA: sha("credentials"),
    toolPins: {
      "bin/docker.exe": sha("docker"),
      "docker-cli-plugins/docker-compose.exe": sha("compose"),
      "docker-cli-plugins/docker-buildx.exe": sha("buildx"),
    },
  };
  const cli = {
    schema: 2,
    sourceOnly: false,
    compiledPinsPresent: true,
    pins: {
      sourceCommit: lock.commit,
      version: context.version,
      patchClosureSHA: context.patchClosureSHA,
      buildLayout: executionOrigin.cacheLayout,
      toolLockSHA: context.toolLockSHA,
      rddSHA: context.build.binarySha256,
      forwarderSHA: context.build.forwarder.binarySha256,
      credentialsSHA: context.credentialsSHA,
      dockerSHA: context.toolPins["bin/docker.exe"],
      composeSHA: context.toolPins["docker-cli-plugins/docker-compose.exe"],
      buildxSHA: context.toolPins["docker-cli-plugins/docker-buildx.exe"],
    },
  };
  verifyCLIBuildBinding(cli, context);
  for (const key of Object.keys(cli.pins)) {
    const wrong = structuredClone(cli);
    wrong.pins[key] = "foreign";
    assert.throws(() => verifyCLIBuildBinding(wrong, context));
  }
  for (const change of [
    { schema: 1 },
    { sourceOnly: true },
    { compiledPinsPresent: false },
  ])
    assert.throws(() => verifyCLIBuildBinding({ ...cli, ...change }, context));
  assert.throws(() =>
    verifyCLIBuildBinding(cli, {
      ...context,
      build: { ...context.build, executionOrigin: undefined },
    }),
  );
  assert.throws(() =>
    verifyCLIBuildBinding(cli, { ...context, lock: { ...lock, patches: [] } }),
  );
});
test("fresh explicit package inputs refuse redirects/existing output before mutation", (t) => {
  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "orbwin-package-contract-"),
  );
  t.after(() => fs.rmSync(root, { recursive: true }));
  fs.mkdirSync(path.join(root, "artifacts"));
  const args = [];
  for (const name of ["backend", "session", "credentials", "tools", "cli"]) {
    const dir = path.join(root, name);
    fs.mkdirSync(dir);
    args.push(`--${name}-input`, dir);
  }
  args.push("--output-dir", "artifacts/fresh");
  const out = packageArguments(root, args);
  assert.equal(fs.existsSync(out.output), false);
  assert.throws(() => packageArguments(root, []));
  assert.throws(() =>
    packageArguments(root, [...args, "--cli-input", out.cli]),
  );
  fs.mkdirSync(out.output);
  fs.writeFileSync(path.join(out.output, "sentinel"), "keep");
  assert.throws(() => packageArguments(root, args), /already exists/);
  assert.equal(
    fs.readFileSync(path.join(out.output, "sentinel"), "utf8"),
    "keep",
  );
  fs.mkdirSync(path.join(root, ".upstream/rancher-desktop"), {
    recursive: true,
  });
  assert.throws(() => isolatedUpstreamInput(root, undefined), /absolute/);
  assert.throws(
    () =>
      isolatedUpstreamInput(root, path.join(root, ".upstream/rancher-desktop")),
    /Selected upstream/,
  );
  assert.equal(isolatedUpstreamInput(root, out.backend), out.backend);
});
test("execution origin DATA comes only from exact locked actual source files", (t) => {
  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "orbwin-origin-contract-"),
  );
  t.after(() => fs.rmSync(root, { recursive: true }));
  const source = path.join(root, "source"),
    lock = { patches: Array(18).fill("p"), patchedFiles: {} };
  for (const name of [
    "private_windows.go",
    "reader_windows.go",
    "upgrade_sdk_image.go",
  ]) {
    const file = path.join(source, "pkg/managementconfig", name);
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, "source\r\n");
    lock.patchedFiles[`rdd/pkg/managementconfig/${name}`] = sha("source\n");
  }
  assert.deepEqual(
    backendExecutionOrigin(root, source, lock).executionOrigin,
    executionOrigin,
  );
  fs.writeFileSync(
    path.join(source, "pkg/managementconfig/private_windows.go"),
    "drift",
  );
  assert.throws(
    () => backendExecutionOrigin(root, source, lock),
    /canonical replay/,
  );
});
