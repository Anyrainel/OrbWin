import test from "node:test";
import assert from "node:assert/strict";
import { parseSourceVolumeHistory } from "../src/model/migration-history.ts";
const receipt = {
  transaction: "a".repeat(32),
  createdAt: "2026-10-25T15:00:00Z",
  sourceKind: "rancher-desktop",
  sourceDaemon: "source",
  volume: "data",
  destination: "C:\\archive.orbvolume",
  phase: "cleaned",
  exportAcknowledged: false,
  uncertain: false,
};
const parse = (records) =>
  parseSourceVolumeHistory(JSON.stringify({ records }));

test("historical restore links require complete manifest provenance", () => {
  const linked = {
    ...receipt,
    exportAcknowledged: true,
    manifestSha256: "b".repeat(64),
    payloadSha256: "c".repeat(64),
    restores: [
      { transaction: "d".repeat(32), volume: "restored", phase: "complete" },
    ],
  };
  assert.equal(parse([linked])[0].restores[0].phase, "complete");
  for (const change of [
    { manifestSha256: undefined },
    { payloadSha256: "bad" },
    { exportAcknowledged: false },
    { uncertain: true },
    { restores: [linked.restores[0], linked.restores[0]] },
    { restores: [{ ...linked.restores[0], phase: "success" }] },
    { restores: [{ ...linked.restores[0], volume: "../unsafe" }] },
    { restores: Array(257).fill(linked.restores[0]) },
  ])
    assert.throws(() => parse([{ ...linked, ...change }]));
  assert.throws(() => parse([{ ...receipt, restores: [] }]));
});
test("source export cleanup remains distinct from export acknowledgement", () => {
  const [value] = parse([receipt]);
  assert.equal(value.phase, "cleaned");
  assert.equal(value.exportAcknowledged, false);
  const [exported] = parse([{ ...receipt, exportAcknowledged: true }]);
  assert.equal(exported.exportAcknowledged, true);
});
test("uncertain receipts expose only a transaction identity", () => {
  const row = {
    transaction: "b".repeat(32),
    createdAt: "",
    sourceKind: "",
    sourceDaemon: "",
    volume: "",
    destination: "",
    phase: "",
    exportAcknowledged: false,
    uncertain: true,
  };
  assert.equal(parse([row])[0].uncertain, true);
  for (const change of [
    { phase: "cleaned" },
    { destination: "C:\\unverified" },
    { exportAcknowledged: true },
  ])
    assert.throws(() => parse([{ ...row, ...change }]));
});
test("source history rejects ambiguous, malformed and oversized observations", () => {
  for (const records of [
    [receipt, receipt],
    [{ ...receipt, phase: "success" }],
    [{ ...receipt, transaction: "../receipt" }],
    [{ ...receipt, createdAt: "invalid" }],
    [{ ...receipt, destination: "control\npath" }],
    Array(257).fill(receipt),
  ])
    assert.throws(() => parse(records));
  assert.throws(() =>
    parseSourceVolumeHistory(" ".repeat(2 * 1024 * 1024 + 1)),
  );
  assert.deepEqual(parse([]), []);
});
