import test from "node:test";
import assert from "node:assert/strict";
import { parseVolumeSize } from "../src/components/resources/volume-size-model.ts";
const sample = {
  name: "data",
  createdAt: "2026-10-25T00:00:00Z",
  bytes: 0,
  references: 2,
  measuredAt: "2026-10-25T01:00:00Z",
};
test("volume accounting distinguishes a measured empty volume from missing or invalid measurements", () => {
  assert.equal(
    parseVolumeSize(JSON.stringify(sample), sample.name, sample.createdAt)
      .bytes,
    0,
  );
  for (const patch of [
    { bytes: -1 },
    { bytes: 1.2 },
    { bytes: Number.MAX_SAFE_INTEGER + 1 },
    { references: -1 },
    { measuredAt: "unknown" },
    { name: "replaced" },
    { createdAt: "older" },
  ])
    assert.throws(() =>
      parseVolumeSize(
        JSON.stringify({ ...sample, ...patch }),
        sample.name,
        sample.createdAt,
      ),
    );
  assert.throws(() => parseVolumeSize("", sample.name, sample.createdAt));
  assert.throws(() =>
    parseVolumeSize(" ".repeat(8193), sample.name, sample.createdAt),
  );
});
