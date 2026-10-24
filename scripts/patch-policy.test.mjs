import test from "node:test";
import assert from "node:assert/strict";
import { assertOwnedChanges } from "./patch-policy.mjs";

test("preparation rejects unrelated changes instead of resetting developer work", () => {
  assert.throws(
    () =>
      assertOwnedChanges(
        ["rdd/go.mod", "rdd/pkg/instance/instance.go"],
        ["rdd/pkg/instance/instance.go"],
      ),
    /rdd\/go.mod/,
  );
});
test("preparation accepts the recorded patch surface and a clean checkout", () => {
  assert.doesNotThrow(() =>
    assertOwnedChanges([], ["rdd/pkg/instance/instance.go"]),
  );
  assert.doesNotThrow(() =>
    assertOwnedChanges(
      ["rdd/pkg/instance/instance.go"],
      ["rdd/pkg/instance/instance.go"],
    ),
  );
});
