// Explicit build-only source slice. Importing performs no file/key/process IO.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  openSync,
  closeSync,
  fstatSync,
  readSync,
  lstatSync,
  realpathSync,
  mkdirSync,
  writeFileSync,
  fsyncSync,
} from "node:fs";
import { isAbsolute, resolve } from "node:path";
import { freshOutputPath } from "./build-output.mjs";
import {
  signRddCandidateBuild,
  validateRddCandidateSigner,
} from "./rdd-candidate-build-input.mjs";
const hash = (b) => createHash("sha256").update(b).digest("hex");
function readBounded(path, limit) {
  assert.ok(
    !lstatSync(path).isSymbolicLink() && realpathSync(path) === resolve(path),
    "Build input redirects",
  );
  const fd = openSync(path, "r");
  try {
    const before = fstatSync(fd, { bigint: true });
    assert.ok(
      before.isFile() && before.size > 0n && before.size <= BigInt(limit),
    );
    const buffer = Buffer.alloc(Number(before.size) + 1);
    let count = 0;
    while (count < buffer.length) {
      const n = readSync(fd, buffer, count, buffer.length - count, null);
      if (n === 0) break;
      count += n;
    }
    assert.equal(BigInt(count), before.size, "Build input size changed");
    const after = fstatSync(fd, { bigint: true });
    for (const k of ["dev", "ino", "size", "mtimeNs"])
      assert.equal(after[k], before[k], "Build input changed");
    return buffer.subarray(0, count);
  } finally {
    closeSync(fd);
  }
}
function writeExclusive(path, bytes) {
  const fd = openSync(path, "wx");
  try {
    writeFileSync(fd, bytes);
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
}
// Called ONLY by a future reviewed trusted build entry AFTER backend/forwarder
// completion. Private key custody is an explicit caller-owned KeyObject; this
// slice never reads a key file, generates one or exports private key bytes.
export function produceRddCandidateBundle(
  root,
  {
    backendDirectory,
    outputDirectory,
    expectedBuildManifestSHA256,
    signingKey,
    verifierPublicKeyHex,
    trustScope,
  },
) {
  validateRddCandidateSigner(signingKey, verifierPublicKeyHex, trustScope);
  assert.ok(isAbsolute(backendDirectory));
  assert.ok(
    realpathSync(backendDirectory) === resolve(backendDirectory),
    "Backend directory redirects",
  );
  const entries = [
    ["build-manifest.json", 4 * 1024 * 1024],
    ["rdd.exe", 512 * 1024 * 1024],
    ["rdd-forwarder.exe", 4 * 1024 * 1024],
  ];
  const inputs = entries.map(([name, limit]) =>
    readBounded(resolve(backendDirectory, name), limit),
  );
  const output = signRddCandidateBuild({
    buildManifestBytes: inputs[0],
    rddBytes: inputs[1],
    forwarderBytes: inputs[2],
    expectedBuildManifestSHA256,
    signingKey,
    verifierPublicKeyHex,
    trustScope,
  });
  for (let n = 0; n < entries.length; n++)
    assert.equal(
      hash(
        readBounded(resolve(backendDirectory, entries[n][0]), entries[n][1]),
      ),
      hash(inputs[n]),
      "Build source changed before publication",
    );
  const destination = freshOutputPath(root, outputDirectory);
  mkdirSync(destination); // exclusive; preserve partials on any failure
  writeExclusive(resolve(destination, "rdd-candidate.json"), output.statement);
  writeExclusive(
    resolve(destination, "rdd-candidate.sig"),
    Buffer.from(output.signature, "ascii"),
  );
  writeExclusive(
    resolve(destination, "rdd-candidate-build.json"),
    Buffer.from(
      JSON.stringify(
        {
          schema: 1,
          purpose: "explicit-operator-signed-rdd-candidate-build",
          trustScope,
          verifierPublicKeyHex,
          backendBuildManifestSHA256: expectedBuildManifestSHA256,
          statementSHA256: hash(output.statement),
          signatureSHA256: hash(Buffer.from(output.signature, "ascii")),
          sourceInputs: entries.map(([name], n) => ({
            name,
            sha256: hash(inputs[n]),
            bytes: inputs[n].length,
          })),
          nativeOrRuntimeAcceptance: false,
        },
        null,
        2,
      ) + "\n",
    ),
  );
  return {
    directory: destination,
    statementSHA256: hash(output.statement),
    signatureSHA256: hash(Buffer.from(output.signature, "ascii")),
  };
}
