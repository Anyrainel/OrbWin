import test from "node:test";
import assert from "node:assert/strict";
import { waitForEngine } from "./wait-engine.mjs";
test("readiness tolerates a briefly missing bridge but never retries identity/auth failures", async () => {
  let attempts = 0;
  const value = await waitForEngine(
    () => {
      if (++attempts < 3)
        throw new Error("failed to connect: cannot find the file");
      return "owned";
    },
    { delay: async () => {} },
  );
  assert.equal(value, "owned");
  assert.equal(attempts, 3);
  attempts = 0;
  await assert.rejects(
    waitForEngine(
      () => {
        attempts++;
        throw new Error("foreign daemon identity");
      },
      { delay: async () => {} },
    ),
    /foreign daemon/,
  );
  assert.equal(attempts, 1);
});
test("unreachable bridge has a bounded failure", async () => {
  await assert.rejects(
    waitForEngine(
      () => {
        throw new Error("failed to connect");
      },
      { timeoutMs: 1, intervalMs: 2 },
    ),
    /did not become reachable/,
  );
});
