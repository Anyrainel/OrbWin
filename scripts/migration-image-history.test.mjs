import test from "node:test";
import assert from "node:assert/strict";
import { parseImageExportHistory } from "../src/model/migration-image-history.ts";
const row = {
  transaction: "a".repeat(32),
  createdAt: "2026-10-25T20:00:00Z",
  sourceKind: "docker-desktop",
  sourceDaemon: "source",
  image: "sha256:" + "b".repeat(64),
  reference: "app:reviewed",
  destination: "C:\\image.tar",
  uncertain: false,
  exportAcknowledged: true,
  sha256: "c".repeat(64),
  configIds: ["sha256:" + "d".repeat(64)],
  imports: [
    { transaction: "e".repeat(32), phase: "complete", verifiedConfigs: 1 },
  ],
};
const parse = (records, unavailableImportReceipts = 0) =>
  parseImageExportHistory(
    JSON.stringify({ records, unavailableImportReceipts }),
  );
test("image history distinguishes complete and unfinished imports from export", () => {
  assert.equal(parse([row]).records[0].imports[0].phase, "complete");
  assert.equal(
    parse(
      [
        {
          ...row,
          imports: [{ ...row.imports[0], phase: "intent", verifiedConfigs: 0 }],
        },
      ],
      1,
    ).unavailableImportReceipts,
    1,
  );
  const intent = { ...row, exportAcknowledged: false };
  delete intent.sha256;
  delete intent.configIds;
  delete intent.imports;
  assert.equal(parse([intent]).records[0].exportAcknowledged, false);
  assert.throws(() => parse([{ ...intent, imports: row.imports }]));
});
test("image history refuses fabricated provenance and invalid completion counts", () => {
  for (const change of [
    { transaction: "bad" },
    { sourceKind: "foreign" },
    { image: "bad" },
    { sha256: "bad" },
    { configIds: [] },
    { configIds: [row.configIds[0], row.configIds[0]] },
    { uncertain: true },
    { imports: [{ ...row.imports[0], verifiedConfigs: 0 }] },
    { imports: [{ ...row.imports[0], phase: "success" }] },
    { imports: [row.imports[0], row.imports[0]] },
    { imports: Array(257).fill(row.imports[0]) },
  ])
    assert.throws(() => parse([{ ...row, ...change }]));
  assert.throws(() => parse([row, row]));
  assert.throws(() => parse(Array(257).fill(row)));
  assert.throws(() => parse([], 257));
});
test("unverified receipts expose only their filename identity", () => {
  const uncertain = {
    transaction: row.transaction,
    createdAt: "",
    sourceKind: "",
    sourceDaemon: "",
    image: "",
    reference: "",
    destination: "",
    uncertain: true,
    exportAcknowledged: false,
  };
  assert.equal(parse([uncertain]).records[0].uncertain, true);
  for (const change of [
    { sourceDaemon: "unverified" },
    { sha256: row.sha256 },
    { configIds: row.configIds },
    { imports: [] },
  ])
    assert.throws(() => parse([{ ...uncertain, ...change }]));
});
