import test from "node:test";
import assert from "node:assert/strict";
import { loadLayout } from "../src/components/resources/persist.ts";

test("corrupt or old layout preferences cannot inject invalid list state", (t) => {
  const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  let stored = "";
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: { getItem: () => stored },
  });
  t.after(() => {
    if (original) Object.defineProperty(globalThis, "localStorage", original);
    else delete globalThis.localStorage;
  });
  for (const value of [
    "null",
    "[]",
    "42",
    "broken",
    '{"collapsed":12,"width":"enormous","sort":"unknown","selected":{}}',
  ]) {
    stored = value;
    assert.deepEqual(loadLayout("containers"), {});
  }
  stored = JSON.stringify({
    collapsed: ["project", 42, null],
    width: 1e20,
    sort: "name",
    selected: "container",
    secret: "discard",
  });
  assert.deepEqual(loadLayout("containers"), {
    collapsed: ["project"],
    width: 700,
    sort: "name",
    selected: "container",
  });
  stored = JSON.stringify({ collapsed: Array(2000).fill("p") });
  assert.equal(loadLayout("containers").collapsed.length, 512);
  for (const sort of ["size-desc", "size-asc"]) {
    stored = JSON.stringify({ sort });
    assert.deepEqual(loadLayout("images"), { sort });
    assert.deepEqual(loadLayout("volumes"), { sort });
    assert.deepEqual(loadLayout("containers"), {});
    assert.deepEqual(loadLayout("networks"), {});
  }
});
