import assert from "node:assert/strict";
import { KeyObject, createPublicKey, createHash, sign } from "node:crypto";
import {
  canonicalRddCandidate,
  candidateVerifier,
  candidateHex,
  parseRddCandidate,
  rddCandidateDomain,
  rddCandidateRoles,
  verifyRddCandidate,
  verifyRddCandidateAsset,
} from "./rdd-candidate-manifest.mjs";
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
export function validateRddCandidateSigner(
  signingKey,
  verifierPublicKeyHex,
  trustScope,
) {
  const configured = candidateVerifier(verifierPublicKeyHex, trustScope);
  assert.ok(
    signingKey instanceof KeyObject,
    "Explicit operator-owned private KeyObject required",
  );
  assert.equal(signingKey.type, "private");
  assert.equal(signingKey.asymmetricKeyType, "ed25519");
  const publicDer = createPublicKey(signingKey).export({
    format: "der",
    type: "spki",
  });
  assert.equal(publicDer.length, 44);
  assert.equal(
    publicDer.subarray(0, 12).toString("hex"),
    "302a300506032b6570032100",
  );
  assert.equal(
    publicDer.subarray(12).toString("hex"),
    configured.publicKeyHex,
    "Independent verifier does not match supplied signer",
  );
  return configured;
}
function boundedAmd64PE(body, limit) {
  assert.ok(
    Buffer.isBuffer(body) && body.length >= 64 && body.length <= limit,
    "Bounded candidate PE required",
  );
  assert.equal(body.toString("ascii", 0, 2), "MZ");
  const offset = body.readUInt32LE(0x3c);
  assert.ok(offset >= 64 && offset + 24 <= body.length);
  assert.equal(body.readUInt32LE(offset), 0x4550);
  assert.equal(
    body.readUInt16LE(offset + 4),
    0x8664,
    "Candidate PE must be AMD64",
  );
}
// Pure build DATA producer. No key path, environment default, automatic keygen,
// staging, compiler, native issuer or implicit product/release qualification.
export function signRddCandidateBuild({
  buildManifestBytes,
  expectedBuildManifestSHA256,
  rddBytes,
  forwarderBytes,
  signingKey,
  verifierPublicKeyHex,
  trustScope,
}) {
  validateRddCandidateSigner(signingKey, verifierPublicKeyHex, trustScope);
  candidateHex(expectedBuildManifestSHA256, 64);
  assert.ok(
    Buffer.isBuffer(buildManifestBytes) &&
      buildManifestBytes.length > 0 &&
      buildManifestBytes.length <= 4 * 1024 * 1024,
  );
  assert.equal(
    hash(buildManifestBytes),
    expectedBuildManifestSHA256,
    "Backend build manifest differs from independent build input",
  );
  const build = JSON.parse(
    new TextDecoder("utf-8", { fatal: true }).decode(buildManifestBytes),
  );
  candidateHex(build.sourceCommit, 40);
  assert.equal(
    build.modernIncomingProfile,
    "modern-no-writer-v1",
    "Backend has no explicit modern receiver profile",
  );
  if (trustScope === "product-release")
    assert.equal(
      build.distroEmbedded,
      true,
      "Product candidate requires the existing guest-capable build profile",
    );
  const bodies = [rddBytes, forwarderBytes];
  for (let n = 0; n < 2; n++)
    boundedAmd64PE(bodies[n], rddCandidateRoles[n].maxSize);
  assert.equal(
    build.binarySha256,
    hash(rddBytes),
    "rdd differs from reviewed backend build",
  );
  assert.equal(
    build.forwarder?.binarySha256,
    hash(forwarderBytes),
    "Forwarder differs from reviewed backend build",
  );
  const statement = canonicalRddCandidate({
    schema: 1,
    purpose: "rdd-modern-candidate",
    platform: "windows-amd64",
    protocol: "modern-no-writer-v1",
    trustScope,
    sourceCommit: build.sourceCommit,
    assets: rddCandidateRoles.map((role, n) => ({
      role: role.role,
      path: role.path,
      sha256: hash(bodies[n]),
      size: bodies[n].length,
    })),
  });
  parseRddCandidate(statement);
  const signature = sign(
    null,
    Buffer.concat([Buffer.from(rddCandidateDomain, "utf8"), statement]),
    signingKey,
  ).toString("base64");
  verifyRddCandidate(statement, signature, verifierPublicKeyHex, trustScope);
  for (let n = 0; n < 2; n++)
    verifyRddCandidateAsset(statement, rddCandidateRoles[n].role, bodies[n]);
  return {
    statement,
    signature,
    buildManifestSHA256: expectedBuildManifestSHA256,
    verifierPublicKeyHex,
    trustScope,
  };
}

// Public-only input for the outgoing verifier build: independently selected,
// no future manifest or signer/private key is required or consulted here.
export function rddCandidateVerifierBuildInput({
  verifierPublicKeyHex,
  trustScope,
}) {
  candidateVerifier(verifierPublicKeyHex, trustScope);
  return Object.freeze({ schema: 1, verifierPublicKeyHex, trustScope });
}
