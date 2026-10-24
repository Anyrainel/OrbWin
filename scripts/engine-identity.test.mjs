import test from "node:test";
import assert from "node:assert/strict";
import { endpoint, verifyIdentity } from "./engine-identity.mjs";
const qualified = {
  endpoint,
  sourceCommit: "pin",
  binarySha256: "build-hash",
  daemonId: "owned",
  coexistenceQualified: true,
};
const build = { sourceCommit: "pin", binarySha256: "build-hash" };
test("workloads reject missing, stale, foreign, and unqualified identities", () => {
  for (const record of [
    null,
    { ...qualified, sourceCommit: "old" },
    { ...qualified, endpoint: "npipe:////./pipe/docker_engine" },
    { ...qualified, coexistenceQualified: false },
    { ...qualified, binarySha256: "stale-binary" },
  ]) {
    assert.throws(() =>
      verifyIdentity(record, { ID: "owned", OSType: "linux" }, build),
    );
  }
  assert.throws(
    () => verifyIdentity(qualified, { ID: "foreign", OSType: "linux" }, build),
    /identity differs/,
  );
  assert.throws(
    () => verifyIdentity(qualified, { ID: "owned", OSType: "windows" }, build),
    /Linux engine/,
  );
});
test("workloads accept only the qualified owned Linux engine", () => {
  assert.doesNotThrow(() =>
    verifyIdentity(qualified, { ID: "owned", OSType: "linux" }, build),
  );
});
