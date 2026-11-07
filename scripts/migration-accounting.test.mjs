import test from "node:test";
import assert from "node:assert/strict";
import { buildSync } from "esbuild";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import {
  migrationAccounting,
  readMigrationAccountingHistories,
} from "../src/model/migration-accounting.ts";
import { parseImageExportHistory } from "../src/model/migration-image-history.ts";
import {
  parseSourceVolumeHistory,
  parseSourceVolumeHistoryEnvelope,
} from "../src/model/migration-history.ts";

const image = "sha256:" + "a".repeat(64),
  config = "sha256:" + "b".repeat(64);
const project = () => ({
  source: { kind: "docker-desktop", daemon: "source" },
  target: "owned",
  inputs: { project: "sample" },
  images: [{ id: image, services: ["web"] }],
  members: [
    { oneOff: false, volumes: ["data"] },
    { oneOff: true, volumes: ["one-off-only"] },
  ],
  unknownImages: 1,
});

test("project receipt panel initially requires an explicit read and makes no completion claim", () => {
  const { outputFiles } = buildSync({
    entryPoints: [
      fileURLToPath(
        new URL(
          "../src/components/resources/MigrationProjectAccounting.tsx",
          import.meta.url,
        ),
      ),
    ],
    bundle: true,
    write: false,
    platform: "node",
    format: "cjs",
    external: ["react"],
    logLevel: "silent",
  });
  const module = { exports: {} };
  new Function("require", "module", "exports", outputFiles[0].text)(
    createRequire(import.meta.url),
    module,
    module.exports,
  );
  const html = renderToStaticMarkup(
    createElement(module.exports.MigrationProjectAccounting, {
      project: project(),
      mapping: {},
      busy: false,
    }),
  );
  assert.match(html, /Project transfer receipts/);
  assert.match(html, /Read matching transfer receipts/);
  assert.match(html, /No latest transaction/);
  assert.doesNotMatch(html, /Historical receipts read at|migration complete/i);
});
const imageExport = (transaction = "1".repeat(32)) => ({
  transaction,
  createdAt: "2026-10-25T12:00:00Z",
  sourceKind: "docker-desktop",
  sourceDaemon: "source",
  image,
  reference: "app:shared",
  destination: "C:\\app.tar",
  uncertain: false,
  exportAcknowledged: true,
  sha256: "c".repeat(64),
  configIds: [config],
  imports: [
    { transaction: "2".repeat(32), phase: "complete", verifiedConfigs: 1 },
  ],
});
const volumeExport = () => ({
  transaction: "3".repeat(32),
  createdAt: "2026-10-25T12:00:00Z",
  sourceKind: "docker-desktop",
  sourceDaemon: "source",
  volume: "data",
  destination: "C:\\app.orbvolume",
  phase: "cleaned",
  uncertain: false,
  exportAcknowledged: true,
  manifestSha256: "d".repeat(64),
  payloadSha256: "e".repeat(64),
  restores: [
    { transaction: "4".repeat(32), volume: "mapped-data", phase: "complete" },
    {
      transaction: "5".repeat(32),
      volume: "other-destination",
      phase: "complete",
    },
    { transaction: "6".repeat(32), volume: "mapped-data", phase: "importing" },
  ],
});
const imageText = (records = [], target = "owned") =>
  JSON.stringify({ target, records, unavailableImportReceipts: 0 });
const volumeText = (records = [], target = "owned") =>
  JSON.stringify({ target, records });
const histories = (imageRows = [], volumeRows = []) => ({
  images: parseImageExportHistory(imageText(imageRows), "owned"),
  volumes: parseSourceVolumeHistoryEnvelope(volumeText(volumeRows), "owned"),
});

test("combined receipt envelopes strictly bind the reviewed target while standalone legacy parsers remain compatible", () => {
  assert.deepEqual(parseSourceVolumeHistory('{"records":[]}'), []);
  assert.deepEqual(
    parseImageExportHistory('{"records":[],"unavailableImportReceipts":0}')
      .records,
    [],
  );
  for (const target of ["foreign", "", "owned\n", false]) {
    assert.throws(() =>
      parseImageExportHistory(imageText([], target), "owned"),
    );
    assert.throws(() =>
      parseSourceVolumeHistoryEnvelope(volumeText([], target), "owned"),
    );
  }
  assert.throws(() =>
    parseImageExportHistory(
      '{"records":[],"unavailableImportReceipts":0}',
      "owned",
    ),
  );
  assert.throws(() =>
    parseSourceVolumeHistoryEnvelope('{"records":[]}', "owned"),
  );
});

test("accounting matches immutable source image and source tuple, never config ID or shared tag", () => {
  const own = imageExport(),
    second = {
      ...imageExport("0".repeat(32)),
      createdAt: "2026-10-25T20:00:00Z",
      imports: [
        { transaction: "7".repeat(32), phase: "loading", verifiedConfigs: 0 },
      ],
    };
  const sameTagOtherSource = {
    ...imageExport("8".repeat(32)),
    sourceDaemon: "foreign",
  };
  const wrongKind = {
    ...imageExport("9".repeat(32)),
    sourceKind: "rancher-desktop",
  };
  const rootConfigConflation = {
    ...imageExport("a".repeat(32)),
    image: config,
    configIds: [image],
  };
  const h = histories([
    own,
    second,
    sameTagOtherSource,
    wrongKind,
    rootConfigConflation,
  ]);
  const before = structuredClone(h);
  const view = migrationAccounting(project(), h.images, h.volumes, {});
  assert.deepEqual(
    view.images[0].exports.map((r) => r.transaction),
    [second.transaction, own.transaction],
  );
  assert.equal(view.images[0].exports[0].imports[0].phase, "loading");
  assert.equal(view.images[0].exports[1].imports[0].phase, "complete");
  assert.equal(view.unknownImages, 1);
  assert.equal(Object.hasOwn(view, "complete"), false);
  assert.deepEqual(h, before);
});

test("volume matching uses selected destination and retains multiple partial/complete owning links", () => {
  const h = histories(
    [],
    [
      volumeExport(),
      {
        ...volumeExport(),
        transaction: "b".repeat(32),
        sourceDaemon: "foreign",
      },
    ],
  );
  const view = migrationAccounting(project(), h.images, h.volumes, {
    data: "mapped-data",
  });
  assert.equal(view.volumes.length, 1);
  assert.equal(view.volumes[0].exports.length, 1);
  assert.deepEqual(
    view.volumes[0].exports[0].matchingRestores.map((r) => r.phase),
    ["complete", "importing"],
  );
  assert.deepEqual(
    migrationAccounting(project(), h.images, h.volumes, {}).volumes[0]
      .exports[0].matchingRestores,
    [],
  );
  assert.deepEqual(
    migrationAccounting(project(), h.images, h.volumes, { data: "unrelated" })
      .volumes[0].exports[0].matchingRestores,
    [],
  );
});

test("target replacement, invalid mapping and duplicate image observations refuse accounting", () => {
  const h = histories();
  assert.throws(() =>
    migrationAccounting(
      project(),
      { ...h.images, target: "foreign" },
      h.volumes,
      {},
    ),
  );
  assert.throws(() =>
    migrationAccounting(
      project(),
      h.images,
      { ...h.volumes, target: "foreign" },
      {},
    ),
  );
  assert.throws(() =>
    migrationAccounting(
      { ...project(), target: "replacement" },
      h.images,
      h.volumes,
      {},
    ),
  );
  for (const value of [false, 0, null, "../unsafe"])
    assert.throws(() =>
      migrationAccounting(project(), h.images, h.volumes, { data: value }),
    );
  assert.throws(() =>
    migrationAccounting(
      { ...project(), images: [...project().images, ...project().images] },
      h.images,
      h.volumes,
      {},
    ),
  );
});

test("uncertain receipts remain unassigned and empty history is not inferred completion", () => {
  const imageUnknown = {
    transaction: "c".repeat(32),
    createdAt: "",
    sourceKind: "",
    sourceDaemon: "",
    image: "",
    reference: "",
    destination: "",
    uncertain: true,
    exportAcknowledged: false,
  };
  const volumeUnknown = {
    transaction: "d".repeat(32),
    createdAt: "",
    sourceKind: "",
    sourceDaemon: "",
    volume: "",
    destination: "",
    phase: "",
    uncertain: true,
    exportAcknowledged: false,
  };
  const h = histories([imageUnknown], [volumeUnknown]);
  h.images.unavailableImportReceipts = 2;
  const view = migrationAccounting(project(), h.images, h.volumes, {});
  assert.equal(view.uncertainImageReceipts, 1);
  assert.equal(view.uncertainVolumeReceipts, 1);
  assert.equal(view.unavailableImportReceipts, 2);
  assert.deepEqual(view.images[0].exports, []);
  assert.deepEqual(view.volumes[0].exports, []);
});

test("sequential read publishes both matching histories or rejects with no partial result", async () => {
  const calls = [];
  const result = await readMigrationAccountingHistories(
    "owned",
    async (kind) => {
      calls.push(kind);
      return kind === "migrationImageHistory" ? imageText() : volumeText();
    },
    new AbortController().signal,
  );
  assert.deepEqual(calls, ["migrationImageHistory", "migrationVolumeHistory"]);
  assert.equal(result.images.target, result.volumes.target);
  await assert.rejects(
    readMigrationAccountingHistories(
      "owned",
      async (kind) => {
        if (kind === "migrationVolumeHistory")
          throw Error("torn volume history");
        return imageText();
      },
      new AbortController().signal,
    ),
    /torn volume/,
  );
  await assert.rejects(
    readMigrationAccountingHistories(
      "owned",
      async (kind) =>
        kind === "migrationImageHistory"
          ? imageText()
          : volumeText([], "replacement"),
      new AbortController().signal,
    ),
    /target/,
  );
});

test("cancelled or invalid first read never starts the second history request", async () => {
  for (const cancelBefore of [false, true]) {
    const abort = new AbortController();
    let calls = 0;
    if (cancelBefore) abort.abort();
    await assert.rejects(
      readMigrationAccountingHistories(
        "owned",
        async () => {
          calls++;
          abort.abort();
          return imageText();
        },
        abort.signal,
      ),
      /cancelled/,
    );
    assert.equal(calls, cancelBefore ? 0 : 1);
  }
  let calls = 0;
  await assert.rejects(
    readMigrationAccountingHistories(
      "owned",
      async () => {
        calls++;
        return imageText([], "foreign");
      },
      new AbortController().signal,
    ),
    /target/,
  );
  assert.equal(calls, 1);
});
