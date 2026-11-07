import test from "node:test";
import assert from "node:assert/strict";
import {
  imageSize,
  compareRows,
  entries,
  platformBadge,
  parseImagePlatform,
} from "../src/components/resources/model.ts";

test("size sorting keeps unknown last in both directions and distinguishes zero", () => {
  const rows = [
    { Name: "unknown" },
    { Name: "large" },
    { Name: "zero" },
    { Name: "small" },
  ];
  const sizes = new Map([
    ["large", 100],
    ["zero", 0],
    ["small", 2],
  ]);
  assert.deepEqual(
    [...rows]
      .sort(compareRows("volumes", "size-desc", sizes))
      .map((r) => r.Name),
    ["large", "small", "zero", "unknown"],
  );
  assert.deepEqual(
    [...rows]
      .sort(compareRows("volumes", "size-asc", sizes))
      .map((r) => r.Name),
    ["zero", "small", "large", "unknown"],
  );
  assert.deepEqual(
    entries("volumes", rows, "size-desc", sizes).map((e) => e.row.Name),
    ["large", "small", "zero", "unknown"],
  );
  const images = [
    { Repository: "b", Size: "2MB" },
    { Repository: "unknown", Size: "N/A" },
    { Repository: "a", Size: "2MB" },
    { Repository: "c", Size: "999kB" },
  ];
  assert.deepEqual(
    images.sort(compareRows("images", "size-desc")).map((r) => r.Repository),
    ["a", "b", "c", "unknown"],
  );
  assert.deepEqual(
    images.sort(compareRows("images", "size-asc")).map((r) => r.Repository),
    ["c", "a", "b", "unknown"],
  );
});
test("image sizes accept only bounded reported decimal counters", () => {
  assert.equal(imageSize("0B"), 0);
  assert.equal(imageSize("1.5GB"), 1_500_000_000);
  for (const value of [
    "",
    "-1B",
    "12GiB",
    "unknown",
    "InfinityB",
    1.2,
    Infinity,
    -1,
    "999999999TB",
    null,
  ])
    assert.equal(imageSize(value), undefined);
});
test("platform metadata is bound to inspected identity and never implies emulation", () => {
  const id = "sha256:" + "a".repeat(64);
  const text = JSON.stringify([{ Id: id, Os: "linux", Architecture: "arm64" }]);
  assert.deepEqual(parseImagePlatform(text, id), {
    os: "linux",
    architecture: "arm64",
  });
  assert.equal(platformBadge("linux", "amd64"), "Native linux/amd64");
  assert.equal(platformBadge("linux", "arm64"), "Unsupported linux/arm64");
  assert.equal(platformBadge(undefined, undefined), "Platform unverified");
  for (const value of [
    "[]",
    "{}",
    JSON.stringify([{ Id: "other", Os: "linux", Architecture: "amd64" }]),
    JSON.stringify([{ Id: id, Os: "linux", Architecture: "bad\nvalue" }]),
    "x".repeat(1024 * 1024 + 1),
  ])
    assert.throws(() => parseImagePlatform(value, id));
});
test("size mode preserves running and paused group semantics", () => {
  const rows = [
    { Names: "paused", State: "paused" },
    { Names: "running", State: "running" },
    { Names: "stopped", State: "exited" },
  ];
  assert.deepEqual(
    rows.sort(compareRows("containers", "status")).map((r) => r.Names),
    ["running", "paused", "stopped"],
  );
});

test("container limit reviews bind exact IDs and preserve absent versus unlimited fields", async () => {
  const { parseContainerLimits } = await import(
    "../src/components/resources/model.ts"
  );
  const id = "a".repeat(64);
  const encode = (host, selected = id) =>
    JSON.stringify([{ Id: selected, HostConfig: host }]);
  assert.deepEqual(
    parseContainerLimits(
      encode({
        NanoCpus: 500000000,
        CpuQuota: -1,
        CpuPeriod: 100000,
        Memory: 67108864,
        MemorySwap: -1,
      }),
      id,
    ),
    {
      nanoCpus: 500000000,
      cpuQuota: -1,
      cpuPeriod: 100000,
      memory: 67108864,
      memorySwap: -1,
    },
  );
  const absent = parseContainerLimits(encode({}), id);
  assert.equal(absent.memory, undefined);
  assert.equal(parseContainerLimits(encode({ Memory: 0 }), id).memory, 0);
  assert.throws(() => parseContainerLimits(encode({}, "different"), id));
  for (const host of [null, [], "invalid"])
    assert.throws(() => parseContainerLimits(encode(host), id));
  for (const host of [
    { Memory: -1 },
    { NanoCpus: 0.5 },
    { MemorySwap: -2 },
    { CpuQuota: -2 },
    { Memory: Number.MAX_SAFE_INTEGER + 1 },
  ])
    assert.throws(() => parseContainerLimits(encode(host), id));
});

test("late cancelled metadata success cannot parse or authorize a review", async () => {
  const { readResourceMetadata } = await import(
    "../src/components/resources/model.ts"
  );
  const controller = new AbortController();
  let complete,
    calls = 0,
    parsed = false;
  const pending = readResourceMetadata(
    async (output) => {
      calls++;
      output("untrusted late metadata", "stdout");
      return new Promise((resolve) => {
        complete = resolve;
      });
    },
    controller.signal,
    () => true,
    () => {
      parsed = true;
      return "review";
    },
  );
  controller.abort();
  complete({ success: true, message: "done" });
  assert.equal(await pending, undefined);
  assert.equal(calls, 1);
  assert.equal(parsed, false);
});

test("snapshot replacement discards late success and late failure without a retry", async () => {
  const { readResourceMetadata } = await import(
    "../src/components/resources/model.ts"
  );
  for (const fail of [false, true]) {
    const controller = new AbortController();
    let current = true,
      complete,
      reject,
      calls = 0;
    const pending = readResourceMetadata(
      async () => {
        calls++;
        return new Promise((resolve, fail) => {
          complete = resolve;
          reject = fail;
        });
      },
      controller.signal,
      () => current,
      () => assert.fail("A replaced snapshot must not parse"),
    );
    current = false;
    if (fail) reject(new Error("late old client error"));
    else complete({ success: true, message: "old result" });
    assert.equal(await pending, undefined);
    assert.equal(calls, 1);
  }
});

test("metadata reads stay bounded, expose current failures and never start invalidated reads", async () => {
  const { readResourceMetadata } = await import(
    "../src/components/resources/model.ts"
  );
  const signal = new AbortController().signal;
  await assert.rejects(
    readResourceMetadata(
      async (output) => {
        output("x".repeat(1024 * 1024), "stdout");
        output("overflow", "stdout");
        return { success: true, message: "done" };
      },
      signal,
      () => true,
      () => assert.fail("Oversize metadata must not parse"),
    ),
    /exceeded/,
  );
  let error;
  try {
    await readResourceMetadata(
      async (output) => {
        output("d".repeat(20000), "stderr");
        return { success: false, message: "refused" };
      },
      signal,
      () => true,
      () => assert.fail(),
    );
  } catch (value) {
    error = value;
  }
  assert.match(error.message, /refused/);
  assert.ok(error.message.length <= 8192 + "refused ".length);
  const controller = new AbortController();
  controller.abort();
  assert.equal(
    await readResourceMetadata(
      () => assert.fail("Cancelled read must not spawn"),
      controller.signal,
      () => true,
      () => assert.fail(),
    ),
    undefined,
  );
  assert.equal(
    await readResourceMetadata(
      () => assert.fail("Stale read must not spawn"),
      signal,
      () => false,
      () => assert.fail(),
    ),
    undefined,
  );
  assert.deepEqual(
    await readResourceMetadata(
      async (output) => {
        output('{"observed":true}', "stdout");
        return { success: true, message: "done" };
      },
      signal,
      () => true,
      JSON.parse,
    ),
    { observed: true },
  );
});
