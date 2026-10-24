import test from "node:test";
import assert from "node:assert/strict";
import { conflictingRoutes } from "./network-preflight.mjs";
test("guest preflight rejects overlapping host and broad VPN routes", () => {
  assert.deepEqual(
    conflictingRoutes([
      "0.0.0.0/0",
      "192.168.0.0/16",
      "192.168.144.3/32",
      "192.168.1.0/24",
      "10.0.0.0/8",
    ]),
    ["192.168.0.0/16", "192.168.144.3/32"],
  );
});
test("guest preflight rejects invalid routes instead of skipping them", () => {
  for (const route of ["192.168.300.0/24", "1.2.3.4/33", "unknown"])
    assert.throws(() => conflictingRoutes([route]));
});
