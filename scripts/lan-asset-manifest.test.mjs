import assert from "node:assert/strict";
import { createHash, generateKeyPairSync, sign } from "node:crypto";
import test from "node:test";
import {
  lanAssetRoles,
  canonicalLanAssetManifest,
  parseLanAssetManifest,
  signLanAssetManifest,
  verifyLanAssetManifest,
  verifyLanAssetBytes,
} from "./lan-asset-manifest.mjs";

const fixture = () => {
  const files = lanAssetRoles.map((asset) =>
    Buffer.from(`synthetic ${asset.role} binary`),
  );
  const value = {
    version: 1,
    assets: lanAssetRoles.map((asset, i) => ({
      role: asset.role,
      path: asset.path,
      sha256: createHash("sha256").update(files[i]).digest("hex"),
      size: files[i].length,
    })),
  };
  return { files, value, bytes: canonicalLanAssetManifest(value) };
};
const keyHex = (key) =>
  key.export({ format: "der", type: "spki" }).subarray(-32).toString("hex");

test("LAN asset manifest signs exact bounded fixed-role bytes and verifies every asset", () => {
  const { value, bytes, files } = fixture();
  const { privateKey, publicKey } = generateKeyPairSync("ed25519");
  const signature = signLanAssetManifest(bytes, privateKey);
  assert.equal(signature.length, 88);
  assert.deepEqual(
    verifyLanAssetManifest(bytes, signature, keyHex(publicKey)),
    value,
  );
  for (const [i, asset] of lanAssetRoles.entries())
    verifyLanAssetBytes(bytes, asset.role, files[i]);
  assert.throws(() => verifyLanAssetBytes(bytes, "helper", files[0]));
  const altered = Buffer.from(files[0]);
  altered[0] ^= 1;
  assert.throws(() => verifyLanAssetBytes(bytes, "manager", altered));
  assert.throws(() =>
    verifyLanAssetBytes(
      bytes,
      "manager",
      Buffer.concat([files[0], Buffer.from("x")]),
    ),
  );
});

test("LAN signature refuses changed content, wrong signer, wrong domain and permissive encodings", () => {
  const { bytes } = fixture();
  const first = generateKeyPairSync("ed25519"),
    second = generateKeyPairSync("ed25519");
  const signature = signLanAssetManifest(bytes, first.privateKey),
    pub = keyHex(first.publicKey);
  assert.throws(() =>
    verifyLanAssetManifest(bytes, signature, keyHex(second.publicKey)),
  );
  const changed = Buffer.from(
    bytes.toString().replace('"size":24', '"size":25'),
  );
  assert.notDeepEqual(changed, bytes);
  assert.throws(() => verifyLanAssetManifest(changed, signature, pub));
  const wrongDomain = sign(null, bytes, first.privateKey).toString("base64");
  assert.throws(() => verifyLanAssetManifest(bytes, wrongDomain, pub));
  for (const invalid of [
    signature + "\n",
    signature.slice(0, -2),
    " " + signature,
    "!".repeat(88),
  ])
    assert.throws(() => verifyLanAssetManifest(bytes, invalid, pub));
  for (const invalid of ["", pub.toUpperCase(), "0".repeat(62)])
    assert.throws(() => verifyLanAssetManifest(bytes, signature, invalid));
});

test("LAN manifest refuses aliases, duplicate/extra fields, unsafe paths, reordered roles and invalid sizes", () => {
  const { bytes, value } = fixture();
  const changes = [
    { ...value, version: 2 },
    { ...value, extra: true },
    { ...value, assets: [...value.assets].reverse() },
    ...[
      "../orbwin-session.exe",
      "C:\\foreign.exe",
      "lan/../orbwin-session.exe",
    ].map((path) => ({
      ...value,
      assets: [{ ...value.assets[0], path }, ...value.assets.slice(1)],
    })),
    ...[0, -1, 1.5, 257 << 20, "24"].map((size) => ({
      ...value,
      assets: [{ ...value.assets[0], size }, ...value.assets.slice(1)],
    })),
  ];
  for (const changed of changes)
    assert.throws(() => canonicalLanAssetManifest(changed));
  for (const raw of [
    Buffer.concat([bytes, Buffer.from("\n")]),
    Buffer.from(
      bytes.toString().replace('"version":1', '"version":1,"version":1'),
    ),
    Buffer.from(
      bytes.toString().replace('"version":1', '"version":1,"extra":true'),
    ),
    Buffer.from(" " + bytes),
    Buffer.from([0xff]),
    Buffer.alloc(4097),
  ])
    assert.throws(() => parseLanAssetManifest(raw));
});
