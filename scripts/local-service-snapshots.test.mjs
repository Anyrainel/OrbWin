import test from "node:test";
import assert from "node:assert/strict";
import {
  acceptLocalServiceSnapshot,
  newLocalServiceSnapshotOrder,
} from "../src/model/local-service-snapshots.ts";

test("local settings reject stale observations despite equal or backward wall time", () => {
  const order = newLocalServiceSnapshotOrder();
  const instance = "a".repeat(32);
  assert.equal(
    acceptLocalServiceSnapshot(order, {
      instance,
      sequence: 10,
      updatedAt: "2026-10-25T00:00:00Z",
    }),
    true,
  );
  assert.equal(
    acceptLocalServiceSnapshot(order, {
      instance,
      sequence: 9,
      updatedAt: "2026-10-25T00:00:00Z",
    }),
    false,
  );
  assert.equal(
    acceptLocalServiceSnapshot(order, {
      instance,
      sequence: 11,
      updatedAt: "2026-10-24T00:00:00Z",
    }),
    true,
  );
  assert.equal(order.sequence, 11);
});

test("process restart accepts a new sequence and rejects late replies from the retired process", () => {
  const order = newLocalServiceSnapshotOrder();
  const old = "a".repeat(32),
    current = "b".repeat(32);
  assert.equal(
    acceptLocalServiceSnapshot(order, { instance: old, sequence: 99 }),
    true,
  );
  assert.equal(
    acceptLocalServiceSnapshot(order, { instance: current, sequence: 1 }),
    true,
  );
  assert.equal(
    acceptLocalServiceSnapshot(order, { instance: old, sequence: 100 }),
    false,
  );
  assert.equal(order.instance, current);
  assert.equal(order.sequence, 1);
});

test("unsupported ordering metadata fails visibly without changing the accepted state", () => {
  const order = newLocalServiceSnapshotOrder();
  for (const snapshot of [
    { instance: "", sequence: 1 },
    { instance: "a".repeat(32), sequence: 0 },
    { instance: "a".repeat(32), sequence: Number.MAX_SAFE_INTEGER + 1 },
  ]) {
    assert.throws(
      () => acceptLocalServiceSnapshot(order, snapshot),
      /ordering metadata/,
    );
    assert.equal(order.instance, "");
  }
});
