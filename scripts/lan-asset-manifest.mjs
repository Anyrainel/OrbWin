import assert from "node:assert/strict";
import { createHash, createPublicKey, sign, verify } from "node:crypto";

export const lanAssetRoles = Object.freeze([
  Object.freeze({
    role: "manager",
    path: "orbwin-session.exe",
    maxSize: 256 << 20,
  }),
  Object.freeze({
    role: "helper",
    path: "lan/orbwin-lan-helper.exe",
    maxSize: 64 << 20,
  }),
  Object.freeze({
    role: "worker",
    path: "lan/orbwin-lan-worker.exe",
    maxSize: 64 << 20,
  }),
]);
const domain = Buffer.from("orbwin-lan-asset-bundle-v1\n", "utf8");
const exactKeys = (value, keys) => {
  assert.ok(value && typeof value === "object" && !Array.isArray(value));
  assert.deepEqual(
    Object.keys(value).sort(),
    [...keys].sort(),
    "LAN manifest fields differ",
  );
};

export function canonicalLanAssetManifest(value) {
  exactKeys(value, ["version", "assets"]);
  assert.equal(value.version, 1, "Unsupported LAN asset schema");
  assert.ok(
    Array.isArray(value.assets) && value.assets.length === lanAssetRoles.length,
  );
  const assets = value.assets.map((asset, index) => {
    exactKeys(asset, ["role", "path", "sha256", "size"]);
    const expected = lanAssetRoles[index];
    assert.equal(asset.role, expected.role, "LAN asset order or role differs");
    assert.equal(asset.path, expected.path, "LAN asset path differs");
    assert.equal(typeof asset.sha256, "string");
    assert.match(asset.sha256, /^[a-f0-9]{64}$/);
    assert.ok(
      Number.isSafeInteger(asset.size) &&
        asset.size > 0 &&
        asset.size <= expected.maxSize,
      "LAN asset size exceeds its role budget",
    );
    return {
      role: asset.role,
      path: asset.path,
      sha256: asset.sha256,
      size: asset.size,
    };
  });
  const bytes = Buffer.from(JSON.stringify({ version: 1, assets }), "utf8");
  assert.ok(bytes.length <= 4096, "LAN manifest exceeds byte budget");
  return bytes;
}

export function parseLanAssetManifest(bytes) {
  assert.ok(Buffer.isBuffer(bytes) && bytes.length > 0 && bytes.length <= 4096);
  const text = new TextDecoder("utf-8", {
    fatal: true,
    ignoreBOM: true,
  }).decode(bytes);
  const value = JSON.parse(text);
  assert.ok(
    canonicalLanAssetManifest(value).equals(bytes),
    "LAN manifest must be canonical exact bytes",
  );
  return value;
}

// The caller owns signing-key custody. This module never reads a key path,
// writes a private key, chooses a default key, or creates one implicitly.
export function signLanAssetManifest(bytes, privateKey) {
  parseLanAssetManifest(bytes);
  assert.equal(privateKey?.type, "private");
  assert.equal(privateKey.asymmetricKeyType, "ed25519");
  return sign(null, Buffer.concat([domain, bytes]), privateKey).toString(
    "base64",
  );
}

export function verifyLanAssetManifest(bytes, signature, publicKeyHex) {
  const value = parseLanAssetManifest(bytes);
  assert.equal(typeof publicKeyHex, "string");
  assert.match(
    publicKeyHex,
    /^[a-f0-9]{64}$/,
    "Configured Ed25519 public key required",
  );
  assert.equal(typeof signature, "string");
  assert.match(
    signature,
    /^[A-Za-z0-9+/]{86}==$/,
    "Canonical detached Ed25519 signature required",
  );
  const decoded = Buffer.from(signature, "base64");
  assert.ok(decoded.length === 64 && decoded.toString("base64") === signature);
  const publicKey = createPublicKey({
    key: Buffer.concat([
      Buffer.from("302a300506032b6570032100", "hex"),
      Buffer.from(publicKeyHex, "hex"),
    ]),
    format: "der",
    type: "spki",
  });
  assert.ok(
    verify(null, Buffer.concat([domain, bytes]), publicKey, decoded),
    "LAN bundle signature differs",
  );
  return value;
}

// Bytes come from independently selected fixed-role files, never a path read
// from this JSON. Native held-file/ACL/identity checks belong to the helper.
export function verifyLanAssetBytes(manifest, role, bytes) {
  const value = parseLanAssetManifest(manifest);
  const asset = value.assets.find((entry) => entry.role === role);
  assert.ok(asset && Buffer.isBuffer(bytes), "Known LAN asset role required");
  assert.equal(bytes.length, asset.size, "LAN asset size differs");
  assert.equal(
    createHash("sha256").update(bytes).digest("hex"),
    asset.sha256,
    "LAN asset digest differs",
  );
}
