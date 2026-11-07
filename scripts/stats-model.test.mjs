import test from "node:test";
import assert from "node:assert/strict";
import {
  bytes,
  parseStats,
  formatBytes,
} from "../src/components/resources/stats-model.ts";
const row = (id = "a".repeat(64)) => ({
  ID: id,
  CPUPerc: "120.5%",
  MemUsage: "2MiB / 8GiB",
  NetIO: "3kB / 4MB",
  BlockIO: "5B / 6GiB",
});
test("stats totals preserve multicore CPU and mixed byte units without summing memory percentages", () => {
  const result = parseStats(
    [row(), row("b".repeat(64))].map(JSON.stringify).join("\n"),
    true,
  );
  assert.deepEqual(result, {
    cpu: 241,
    memory: 4194304,
    networkIn: 6000,
    networkOut: 8000000,
    blockRead: 10,
    blockWrite: 12884901888,
    containers: 2,
  });
  assert.equal(formatBytes(0), "0.0 B");
  assert.equal(formatBytes(1024), "1.0 KiB");
});
test("empty engine succeeds while missing selected target, repeated/truncated identity and corrupt output refuse", () => {
  assert.equal(parseStats("", true).containers, 0);
  for (const output of [
    "",
    "broken",
    JSON.stringify({ ...row(), ID: "abc" }),
    [row(), row()].map(JSON.stringify).join("\n"),
    JSON.stringify({ ...row(), CPUPerc: "NaN%" }),
    JSON.stringify({ ...row(), NetIO: "bad" }),
  ]) {
    assert.throws(() => parseStats(output, false));
  }
  assert.throws(() =>
    parseStats([row(), row()].map(JSON.stringify).join("\n"), true),
  );
  assert.throws(() => parseStats("x".repeat(2097153), true));
  assert.throws(() =>
    parseStats(
      [row(), row("b".repeat(64))]
        .map((entry) =>
          JSON.stringify({ ...entry, CPUPerc: `1${"0".repeat(308)}%` }),
        )
        .join("\n"),
      true,
    ),
  );
});
test("byte parsing rejects unsupported, partial, negative and unbounded values", () => {
  for (const value of [
    "-1B",
    "1PB",
    "12B junk",
    "InfinityB",
    "999999999999999999TiB",
    null,
  ])
    assert.throws(() => bytes(value));
  assert.equal(bytes(" 1.5 KiB "), 1536);
});
test("selected stats never attach a recreated container's readings to an old identity", () => {
  assert.throws(() => parseStats(JSON.stringify(row()), false, "b".repeat(64)));
  assert.throws(() =>
    parseStats(JSON.stringify({ ...row(), PIDs: "0" }), false, "a".repeat(64)),
  );
});
