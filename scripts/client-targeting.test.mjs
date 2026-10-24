import test from "node:test";
import assert from "node:assert/strict";
import { endpoint } from "./engine-identity.mjs";
import {
  dockerEndpoint,
  nodeTestcontainersEndpoint,
} from "../src/model/client-targeting.ts";
test("CLI and WHATWG-parsed framework selectors resolve to the same owned pipe", () => {
  assert.equal(dockerEndpoint, endpoint);
  assert.equal(new URL(nodeTestcontainersEndpoint).hostname, "");
  assert.equal(
    new URL(nodeTestcontainersEndpoint).pathname.replaceAll("\\", "/"),
    "//./pipe/docker-rancher-desktop-orbwin",
  );
  assert.notEqual(
    new URL(dockerEndpoint).pathname,
    "//./pipe/docker-rancher-desktop-orbwin",
  );
});
