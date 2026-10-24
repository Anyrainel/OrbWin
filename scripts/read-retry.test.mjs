import test from "node:test";
import assert from "node:assert/strict";
import { retryBusyRead } from "../src/model/read-retry.ts";
const busy =
  "Another operation is active. Wait for it to finish before retrying.";
test("read contention retries are bounded and never retry identity failures", async () => {
  let calls = 0;
  await assert.rejects(
    retryBusyRead(
      async () => {
        calls++;
        throw busy;
      },
      new AbortController().signal,
      [0, 0],
    ),
    (error) => error === busy,
  );
  assert.equal(calls, 3);
  calls = 0;
  await assert.rejects(
    retryBusyRead(
      async () => {
        calls++;
        throw Error("Engine identity changed");
      },
      new AbortController().signal,
      [0, 0],
    ),
    /identity/,
  );
  assert.equal(calls, 1);
  calls = 0;
  assert.equal(
    await retryBusyRead(
      async () => {
        if (++calls === 1) throw busy;
        return "details";
      },
      new AbortController().signal,
      [0],
    ),
    "details",
  );
});
test("cancelling a pending read retry prevents another dispatch", async () => {
  const abort = new AbortController();
  let calls = 0;
  const pending = retryBusyRead(
    async () => {
      calls++;
      throw busy;
    },
    abort.signal,
    [1000],
  );
  await Promise.resolve();
  abort.abort(Error("view closed"));
  await assert.rejects(pending, /view closed/);
  assert.equal(calls, 1);
  await assert.rejects(
    retryBusyRead(async () => {
      calls++;
    }, abort.signal),
    /view closed/,
  );
  assert.equal(calls, 1);
});
