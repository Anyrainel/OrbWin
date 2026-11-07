// Test-only ephemeral KeyObjects remain in memory: no key file/export/print.
import test from "node:test";
import assert from "node:assert/strict";
import { generateKeyPairSync, createHash, sign } from "node:crypto";
import {
  canonicalRddCandidate,
  parseRddCandidate,
  verifyRddCandidate,
  verifyRddCandidateAsset,
  rddCandidateDomain,
} from "./rdd-candidate-manifest.mjs";
import {
  rddCandidateVerifierBuildInput,
  signRddCandidateBuild,
  validateRddCandidateSigner,
} from "./rdd-candidate-build-input.mjs";
import { produceRddCandidateBundle } from "./build-rdd-candidate.mjs";
const a = generateKeyPairSync("ed25519"),
  b = generateKeyPairSync("ed25519");
const publicHex = (k) =>
  k.publicKey
    .export({ format: "der", type: "spki" })
    .subarray(12)
    .toString("hex");
const pub = publicHex(a),
  other = publicHex(b),
  hash = (b) => createHash("sha256").update(b).digest("hex");
function pe(value) {
  const b = Buffer.alloc(128, value);
  b.write("MZ");
  b.writeUInt32LE(64, 0x3c);
  b.writeUInt32LE(0x4550, 64);
  b.writeUInt16LE(0x8664, 68);
  return b;
}
const rdd = pe(1),
  forwarder = pe(2);
function input() {
  const buildManifestBytes = Buffer.from(
    JSON.stringify({
      sourceCommit: "a".repeat(40),
      binarySha256: hash(rdd),
      forwarder: { binarySha256: hash(forwarder) },
      modernIncomingProfile: "modern-no-writer-v1",
      distroEmbedded: true,
    }),
  );
  return {
    buildManifestBytes,
    expectedBuildManifestSHA256: hash(buildManifestBytes),
    rddBytes: rdd,
    forwarderBytes: forwarder,
    signingKey: a.privateKey,
    verifierPublicKeyHex: pub,
    trustScope: "local-qualification",
  };
}
function produced() {
  return signRddCandidateBuild(input());
}
function editBuild(change) {
  const o = input(),
    v = JSON.parse(o.buildManifestBytes);
  change(v);
  o.buildManifestBytes = Buffer.from(JSON.stringify(v));
  o.expectedBuildManifestSHA256 = hash(o.buildManifestBytes);
  return o;
}
function signed(bytes, key = a.privateKey, domain = rddCandidateDomain) {
  return sign(
    null,
    Buffer.concat([Buffer.from(domain, "utf8"), bytes]),
    key,
  ).toString("base64");
}

test("candidate canonical signed pair roundtrip and actual bytes", () => {
  const p = produced();
  const v = verifyRddCandidate(
    p.statement,
    p.signature,
    pub,
    "local-qualification",
  );
  assert.ok(canonicalRddCandidate(v).equals(p.statement));
  verifyRddCandidateAsset(p.statement, "rdd", rdd);
  verifyRddCandidateAsset(p.statement, "rdd-forwarder", forwarder);
  assert.equal(p.signature.length, 88);
  assert.equal(typeof produceRddCandidateBundle, "function");
});
test("candidate trust input is explicit and independently bound", () => {
  for (const bad of [undefined, null, {}, a.publicKey])
    assert.throws(() =>
      validateRddCandidateSigner(bad, pub, "local-qualification"),
    );
  for (const key of [undefined, "", "0".repeat(64), "A".repeat(64)])
    assert.throws(() =>
      validateRddCandidateSigner(a.privateKey, key, "local-qualification"),
    );
  assert.throws(() =>
    validateRddCandidateSigner(a.privateKey, other, "local-qualification"),
  );
  assert.throws(() => validateRddCandidateSigner(a.privateKey, pub, undefined));
});
test("candidate cross-key and LAN purpose never authorize", () => {
  const p = produced();
  assert.throws(() =>
    verifyRddCandidate(p.statement, p.signature, other, "local-qualification"),
  );
  assert.throws(() =>
    verifyRddCandidate(
      p.statement,
      signed(p.statement, b.privateKey),
      pub,
      "local-qualification",
    ),
  );
  assert.throws(() =>
    verifyRddCandidate(
      p.statement,
      signed(p.statement, a.privateKey, "orbwin-lan-asset-bundle-v1\n"),
      pub,
      "local-qualification",
    ),
  );
});
test("candidate local scope cannot promote to product", () => {
  const p = produced();
  assert.throws(() =>
    verifyRddCandidate(p.statement, p.signature, pub, "product-release"),
  );
  const o = input();
  o.trustScope = "product-release";
  const product = signRddCandidateBuild(o);
  verifyRddCandidate(
    product.statement,
    product.signature,
    pub,
    "product-release",
  );
  assert.throws(() =>
    verifyRddCandidate(
      product.statement,
      product.signature,
      pub,
      "local-qualification",
    ),
  );
  assert.throws(() =>
    signRddCandidateBuild({
      ...editBuild((v) => (v.distroEmbedded = false)),
      trustScope: "product-release",
    }),
  );
});
test("candidate closed version role platform hash size and schema refusals", () => {
  const p = produced(),
    base = parseRddCandidate(p.statement);
  for (const change of [
    (v) => (v.schema = 2),
    (v) => (v.purpose = "lan-assets"),
    (v) => (v.platform = "windows-arm64"),
    (v) => (v.protocol = "serve"),
    (v) => (v.trustScope = "any"),
    (v) => (v.sourceCommit = "0".repeat(40)),
    (v) => (v.extra = true),
    (v) => v.assets.reverse(),
    (v) => (v.assets[0].role = "helper"),
    (v) => (v.assets[0].path = "C:/rdd.exe"),
    (v) => (v.assets[1].path = "../forwarder.exe"),
    (v) => (v.assets[0].sha256 = "0".repeat(64)),
    (v) => (v.assets[0].sha256 = "B".repeat(64)),
    (v) => (v.assets[0].size = 0),
    (v) => (v.assets[1].size = 4 * 1024 * 1024 + 1),
    (v) => (v.assets[0].size = 1.5),
    (v) => v.assets.push(v.assets[0]),
  ]) {
    const v = structuredClone(base);
    change(v);
    assert.throws(() => canonicalRddCandidate(v));
  }
});
test("candidate duplicate reordered trailing BOM and malformed UTF8 refused", () => {
  const p = produced(),
    text = p.statement.toString();
  const alias = text.replace('"schema":1', '"Schema":1');
  for (const bytes of [
    Buffer.from(text.replace('"schema":1', '"schema":1,"schema":1')),
    Buffer.from(alias),
    Buffer.from(text + " "),
    Buffer.concat([Buffer.from([0xef, 0xbb, 0xbf]), p.statement]),
    Buffer.from([0xff]),
    Buffer.alloc(4097),
    Buffer.from(
      JSON.stringify({
        assets: parseRddCandidate(p.statement).assets,
        ...parseRddCandidate(p.statement),
      }),
    ),
  ])
    assert.throws(() => parseRddCandidate(bytes));
});
test("candidate signature encoding and statement tampering refused", () => {
  const p = produced();
  for (const sig of [
    "",
    p.signature + "\n",
    p.signature.slice(1),
    "!".repeat(88),
    Buffer.alloc(64).toString("base64"),
  ])
    assert.throws(() =>
      verifyRddCandidate(p.statement, sig, pub, "local-qualification"),
    );
  const v = parseRddCandidate(p.statement);
  v.sourceCommit = "b".repeat(40);
  assert.throws(() =>
    verifyRddCandidate(
      canonicalRddCandidate(v),
      p.signature,
      pub,
      "local-qualification",
    ),
  );
});
test("candidate actual asset and reviewed build linkage refusals", () => {
  const p = produced();
  assert.throws(() =>
    verifyRddCandidateAsset(p.statement, "rdd", Buffer.alloc(128)),
  );
  assert.throws(() => verifyRddCandidateAsset(p.statement, "other", rdd));
  for (const o of [
    { ...input(), expectedBuildManifestSHA256: "b".repeat(64) },
    { ...input(), rddBytes: forwarder },
    editBuild((v) => delete v.modernIncomingProfile),
    editBuild((v) => (v.sourceCommit = "no")),
    editBuild((v) => (v.forwarder.binarySha256 = "b".repeat(64))),
  ])
    assert.throws(() => signRddCandidateBuild(o));
  const arm = pe(3);
  arm.writeUInt16LE(0xaa64, 68);
  assert.throws(() => signRddCandidateBuild({ ...input(), rddBytes: arm }));
});

test("outgoing public-only verifier configuration binds a later candidate", () => {
  const config = rddCandidateVerifierBuildInput({
    verifierPublicKeyHex: pub,
    trustScope: "local-qualification",
  });
  const p = produced();
  verifyRddCandidate(
    p.statement,
    p.signature,
    config.verifierPublicKeyHex,
    config.trustScope,
  );
  for (const bad of [
    {},
    { verifierPublicKeyHex: pub },
    { trustScope: "local-qualification" },
    { verifierPublicKeyHex: pub, trustScope: "from-manifest" },
  ])
    assert.throws(() => rddCandidateVerifierBuildInput(bad));
  const wrong = rddCandidateVerifierBuildInput({
    verifierPublicKeyHex: other,
    trustScope: "local-qualification",
  });
  assert.throws(() =>
    verifyRddCandidate(
      p.statement,
      p.signature,
      wrong.verifierPublicKeyHex,
      wrong.trustScope,
    ),
  );
});
