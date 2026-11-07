import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync, realpathSync, statSync } from "node:fs";
import { resolve, isAbsolute, relative, sep } from "node:path";
import { freshOutputPath } from "./build-output.mjs";

const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const exact = (value, names) =>
  assert.deepEqual(Object.keys(value ?? {}).sort(), [...names].sort());
export const executionOrigin = Object.freeze({
  schema: 1,
  authorityRoot: "native-profile-authority-v1",
  cacheLayout: "native-profile-runtime-v1",
});
export function inputDirectory(value) {
  assert.ok(
    typeof value === "string" && isAbsolute(value),
    "Explicit absolute build input required",
  );
  const path = resolve(value),
    actual = realpathSync(path);
  assert.equal(
    process.platform === "win32" ? actual.toLowerCase() : actual,
    process.platform === "win32" ? path.toLowerCase() : path,
    "Redirected build input refused",
  );
  assert.ok(statSync(path).isDirectory());
  return path;
}
export function isolatedUpstreamInput(root, value) {
  const directory = inputDirectory(value),
    selected = resolve(root, ".upstream/rancher-desktop");
  const rel = relative(selected, directory);
  assert.ok(
    rel && (isAbsolute(rel) || rel === ".." || rel.startsWith(`..${sep}`)),
    "Selected upstream checkout is not a candidate build input",
  );
  return directory;
}
export function packageArguments(root, args) {
  const out = {},
    names = ["backend", "session", "credentials", "tools", "cli"];
  for (let i = 0; i < args.length; i += 2) {
    const key = args[i];
    assert.ok(
      key === "--output-dir" || names.some((n) => key === `--${n}-input`),
      "Unknown package option",
    );
    assert.ok(
      args[i + 1] && out[key] === undefined,
      "Missing/repeated package option",
    );
    out[key] = args[i + 1];
  }
  return {
    ...Object.fromEntries(
      names.map((n) => [n, inputDirectory(out[`--${n}-input`])]),
    ),
    output: freshOutputPath(root, out["--output-dir"]),
  };
}
export function cliBuildArguments(args) {
  const out = { sourceOnly: false },
    seen = new Set();
  const valued = [
    "--output",
    "--candidate-input",
    "--backend-input",
    "--credentials-input",
    "--tools-input",
  ];
  for (let i = 0; i < args.length; i++) {
    const key = args[i];
    assert.ok(!seen.has(key), "Repeated CLI build option");
    seen.add(key);
    if (key === "--source-only") out.sourceOnly = true;
    else {
      assert.ok(valued.includes(key), "Unknown CLI build option");
      assert.ok(
        args[i + 1] && !args[i + 1].startsWith("--"),
        "Missing CLI build value",
      );
      out[key] = args[++i];
    }
  }
  assert.ok(out["--output"], "Explicit CLI output required");
  const separate = [
    "--backend-input",
    "--credentials-input",
    "--tools-input",
  ].filter((k) => out[k] !== undefined);
  if (out.sourceOnly)
    assert.ok(
      !out["--candidate-input"] && separate.length === 0,
      "Source-only builds accept no component inputs",
    );
  else
    assert.ok(
      out["--candidate-input"] ? separate.length === 0 : separate.length === 3,
      "One complete CLI input shape required",
    );
  return out;
}

// Build DATA only. The actual replay/build recipe invokes this on the same
// source it compiles. This never constructs a native execution capability.
export function backendExecutionOrigin(root, source, lock) {
  assert.ok(
    lock.patches.length >= 18,
    "Successor execution origin requires canonical replay closure",
  );
  const files = [
    "pkg/managementconfig/private_windows.go",
    "pkg/managementconfig/reader_windows.go",
    "pkg/managementconfig/upgrade_sdk_image.go",
  ];
  const sources = Object.fromEntries(
    files.map((name) => {
      const digest = hash(
        readFileSync(resolve(source, name), "utf8").replace(/\r\n/g, "\n"),
      );
      assert.equal(
        lock.patchedFiles[`rdd/${name}`],
        digest,
        "Execution origin source differs from canonical replay",
      );
      return [name, digest];
    }),
  );
  return { executionOrigin, executionOriginSources: sources };
}

export function verifySDKBuildBinding(build, lan, publicKeyHex) {
  const pinNames = {
    upgradeSDKManagerSHA256: "orbwin-session.exe",
    upgradeSDKManifestSHA256: "lan/assets.json",
    upgradeSDKSignatureSHA256: "lan/assets.sig",
    upgradeSDKHelperSHA256: "lan/orbwin-lan-helper.exe",
    upgradeSDKWorkerSHA256: "lan/orbwin-lan-worker.exe",
  };
  assert.equal(
    build.upgradeSDK?.schema,
    1,
    "Backend has no successor SDK build input",
  );
  exact(build.upgradeSDK.pins, [
    ...Object.keys(pinNames),
    "upgradeSDKBundlePublicKeyHex",
  ]);
  for (const [pin, name] of Object.entries(pinNames)) {
    assert.match(lan.pins[name], /^[a-f0-9]{64}$/);
    assert.equal(
      build.upgradeSDK.pins[pin],
      lan.pins[name],
      "Backend and packaged SDK epochs differ",
    );
  }
  assert.match(publicKeyHex, /^[a-f0-9]{64}$/);
  assert.equal(
    build.upgradeSDK.pins.upgradeSDKBundlePublicKeyHex,
    publicKeyHex,
  );
  assert.equal(
    build.upgradeSDK.bundleBuildSha256,
    lan.pins["lan-build-manifest.json"],
    "SDK build provenance epoch differs",
  );
}

export function verifyCLIBuildBinding(
  cli,
  {
    version,
    lock,
    patchClosureSHA,
    toolLockSHA,
    build,
    credentialsSHA,
    toolPins,
  },
) {
  assert.equal(cli.schema, 2, "Only current CLI schema is supported");
  assert.equal(cli.sourceOnly, false, "Source-only CLI cannot be packaged");
  assert.equal(cli.compiledPinsPresent, true);
  assert.deepEqual(
    build.executionOrigin,
    executionOrigin,
    "Backend execution-origin DATA differs",
  );
  assert.ok(
    lock.patches.length >= 18,
    "Canonical successor patch closure required",
  );
  assert.deepEqual(
    cli.pins,
    {
      sourceCommit: lock.commit,
      version,
      patchClosureSHA,
      buildLayout: executionOrigin.cacheLayout,
      toolLockSHA,
      rddSHA: build.binarySha256,
      forwarderSHA: build.forwarder.binarySha256,
      credentialsSHA,
      dockerSHA: toolPins["bin/docker.exe"],
      composeSHA: toolPins["docker-cli-plugins/docker-compose.exe"],
      buildxSHA: toolPins["docker-cli-plugins/docker-buildx.exe"],
    },
    "CLI compiled inputs differ from packaged component epoch",
  );
  for (const [name, value] of Object.entries(cli.pins))
    if (name.endsWith("SHA")) assert.match(value, /^[a-f0-9]{64}$/);
}
