import test from "node:test";
import assert from "node:assert/strict";
import { parseMigrationDestination } from "../src/model/migration-destination.ts";
const expected = {
  source: "source",
  sourceVolume: "data",
  destination: "restored",
  target: "owned",
};
const valid = {
  ...expected,
  version: 1,
  restoreTransaction: "a".repeat(32),
  exportTransactions: ["b".repeat(32)],
  manifestSha256: "c".repeat(64),
  createdAt: "2026-10-25T00:00:00Z",
  observedAt: "2026-10-25T01:00:00Z",
};
test("destination observation binds current mapping and complete source evidence", () => {
  assert.equal(
    parseMigrationDestination(JSON.stringify(valid), expected)
      .restoreTransaction,
    valid.restoreTransaction,
  );
  for (const patch of [
    { source: "other" },
    { sourceVolume: "other" },
    { destination: "other" },
    { target: "other" },
    { version: 2 },
    { manifestSha256: "bad" },
    { restoreTransaction: "../file" },
    { exportTransactions: [] },
    { exportTransactions: ["b".repeat(32), "b".repeat(32)] },
    { exportTransactions: Array(257).fill("b".repeat(32)) },
    { observedAt: "invalid" },
  ])
    assert.throws(() =>
      parseMigrationDestination(
        JSON.stringify({ ...valid, ...patch }),
        expected,
      ),
    );
  assert.throws(() => parseMigrationDestination(" ".repeat(65537), expected));
});
