import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { isAbsolute, resolve } from "node:path";
import { verifyLanBundleBuild } from "./lan-bundle-provenance.mjs";

const namespace =
  "github.com/rancher-sandbox/rancher-desktop-daemon/pkg/managementconfig";
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");

// Build-time input only. Runtime SDK code is selected by these compiled pins,
// never by this build path or an environment variable in the running product.
export function upgradeSdkBuildInput(root, directory) {
  assert.ok(
    directory && isAbsolute(directory),
    "Upgrade SDK bundle requires an absolute build input",
  );
  const verified = verifyLanBundleBuild(root, directory);
  const buildBytes = readFileSync(
    resolve(directory, "lan-build-manifest.json"),
  );
  assert.equal(
    hash(buildBytes),
    verified.pins["lan-build-manifest.json"],
    "SDK build input changed during verification",
  );
  const publicKeyHex = JSON.parse(buildBytes).publicKeyHex;
  assert.match(publicKeyHex, /^[a-f0-9]{64}$/);
  const values = {
    upgradeSDKManagerSHA256: verified.pins["orbwin-session.exe"],
    upgradeSDKManifestSHA256: verified.pins["lan/assets.json"],
    upgradeSDKSignatureSHA256: verified.pins["lan/assets.sig"],
    upgradeSDKHelperSHA256: verified.pins["lan/orbwin-lan-helper.exe"],
    upgradeSDKWorkerSHA256: verified.pins["lan/orbwin-lan-worker.exe"],
    upgradeSDKBundlePublicKeyHex: publicKeyHex,
  };
  for (const value of Object.values(values)) {
    assert.match(value, /^[a-f0-9]{64}$/);
    assert.notEqual(value, "0".repeat(64));
  }
  return {
    linkerFlags: Object.entries(values)
      .map(([name, value]) => `-X ${namespace}.${name}=${value}`)
      .join(" "),
    manifest: {
      schema: 1,
      pins: values,
      bundleBuildSha256: verified.pins["lan-build-manifest.json"],
    },
    assertUnchanged() {
      assert.deepEqual(
        verifyLanBundleBuild(root, directory).pins,
        verified.pins,
        "Upgrade SDK bundle changed during backend build",
      );
    },
  };
}
