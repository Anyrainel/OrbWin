import test from "node:test";
import assert from "node:assert/strict";
import { canReviewOperationFailure } from "../src/components/resources/operation-review.ts";

test("only a reconciled local refusal can return to the same mutation review", () => {
  assert.equal(
    canReviewOperationFailure({
      dispatched: false,
      succeeded: false,
      cancelled: false,
      reconciled: true,
    }),
    true,
  );
  // A transport error, timeout, cancellation or nonzero daemon exit can follow
  // accepted creation. Even a successful inventory refresh is not non-creation proof.
  for (const succeeded of [false, true]) {
    for (const cancelled of [false, true]) {
      for (const reconciled of [false, true]) {
        assert.equal(
          canReviewOperationFailure({
            dispatched: true,
            succeeded,
            cancelled,
            reconciled,
          }),
          false,
        );
      }
    }
  }
  assert.equal(
    canReviewOperationFailure({
      dispatched: false,
      succeeded: false,
      cancelled: true,
      reconciled: true,
    }),
    false,
  );
  assert.equal(
    canReviewOperationFailure({
      dispatched: false,
      succeeded: false,
      cancelled: false,
      reconciled: false,
    }),
    false,
  );
  assert.equal(
    canReviewOperationFailure({
      dispatched: false,
      succeeded: true,
      cancelled: false,
      reconciled: true,
    }),
    false,
  );
});
