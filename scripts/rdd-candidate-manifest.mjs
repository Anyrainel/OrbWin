import assert from "node:assert/strict";
import { createHash, createPublicKey, verify } from "node:crypto";

export const rddCandidateDomain = "orbwin-rdd-candidate-bundle-v1\n";
export const rddCandidateRoles = Object.freeze([
  Object.freeze({ role: "rdd", path: "rdd.exe", maxSize: 512 * 1024 * 1024 }),
  Object.freeze({
    role: "rdd-forwarder",
    path: "rdd-forwarder.exe",
    maxSize: 4 * 1024 * 1024,
  }),
]);
const exact = (v, keys) => {
  assert.ok(v && typeof v === "object" && !Array.isArray(v));
  assert.deepEqual(
    Object.keys(v).sort(),
    [...keys].sort(),
    "Candidate fields differ",
  );
};
export function candidateHex(value, digits) {
  assert.equal(typeof value, "string");
  assert.match(value, new RegExp("^[a-f0-9]{" + digits + "}$"));
  assert.notEqual(value, "0".repeat(digits));
  return value;
}
export function candidateScope(value) {
  assert.ok(
    value === "local-qualification" || value === "product-release",
    "Unsupported candidate trust scope",
  );
  return value;
}
export function canonicalRddCandidate(value) {
  exact(value, [
    "schema",
    "purpose",
    "platform",
    "protocol",
    "trustScope",
    "sourceCommit",
    "assets",
  ]);
  assert.equal(value.schema, 1, "Unsupported candidate schema");
  assert.equal(value.purpose, "rdd-modern-candidate");
  assert.equal(value.platform, "windows-amd64");
  assert.equal(value.protocol, "modern-no-writer-v1");
  candidateScope(value.trustScope);
  candidateHex(value.sourceCommit, 40);
  assert.ok(Array.isArray(value.assets) && value.assets.length === 2);
  const assets = value.assets.map((a, index) => {
    exact(a, ["role", "path", "sha256", "size"]);
    const expected = rddCandidateRoles[index];
    assert.equal(a.role, expected.role);
    assert.equal(a.path, expected.path);
    candidateHex(a.sha256, 64);
    assert.ok(
      Number.isSafeInteger(a.size) && a.size > 0 && a.size <= expected.maxSize,
    );
    return { role: a.role, path: a.path, sha256: a.sha256, size: a.size };
  });
  const bytes = Buffer.from(
    JSON.stringify({
      schema: 1,
      purpose: value.purpose,
      platform: value.platform,
      protocol: value.protocol,
      trustScope: value.trustScope,
      sourceCommit: value.sourceCommit,
      assets,
    }),
    "utf8",
  );
  assert.ok(bytes.length <= 4096);
  return bytes;
}
export function parseRddCandidate(bytes) {
  assert.ok(Buffer.isBuffer(bytes) && bytes.length > 0 && bytes.length <= 4096);
  const text = new TextDecoder("utf-8", {
    fatal: true,
    ignoreBOM: true,
  }).decode(bytes);
  const value = JSON.parse(text);
  assert.ok(
    canonicalRddCandidate(value).equals(bytes),
    "Candidate bytes are not canonical",
  );
  return value;
}
export function candidateVerifier(publicKeyHex, trustScope) {
  candidateHex(publicKeyHex, 64);
  candidateScope(trustScope);
  const publicKey = createPublicKey({
    key: Buffer.concat([
      Buffer.from("302a300506032b6570032100", "hex"),
      Buffer.from(publicKeyHex, "hex"),
    ]),
    format: "der",
    type: "spki",
  });
  return { publicKey, publicKeyHex, trustScope };
}
export function verifyRddCandidate(
  bytes,
  signature,
  publicKeyHex,
  expectedTrustScope,
) {
  const v = parseRddCandidate(bytes);
  const verifier = candidateVerifier(publicKeyHex, expectedTrustScope);
  assert.equal(
    v.trustScope,
    verifier.trustScope,
    "Candidate scope differs from independently configured verifier",
  );
  assert.equal(typeof signature, "string");
  assert.match(signature, /^[A-Za-z0-9+/]{86}==$/);
  const sig = Buffer.from(signature, "base64");
  assert.equal(sig.length, 64);
  assert.equal(sig.toString("base64"), signature);
  assert.ok(
    verify(
      null,
      Buffer.concat([Buffer.from(rddCandidateDomain, "utf8"), bytes]),
      verifier.publicKey,
      sig,
    ),
    "Candidate signature is not trusted",
  );
  return v;
}
export function verifyRddCandidateAsset(bytes, role, body) {
  const value = parseRddCandidate(bytes);
  const row = value.assets.find((a) => a.role === role);
  assert.ok(row, "Unknown candidate asset role");
  assert.ok(Buffer.isBuffer(body));
  assert.equal(body.length, row.size);
  assert.equal(
    createHash("sha256").update(body).digest("hex"),
    row.sha256,
    "Candidate actual asset bytes differ",
  );
}
