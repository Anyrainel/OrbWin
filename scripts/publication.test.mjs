import test from "node:test";
import assert from "node:assert/strict";
import {
  publishedPorts,
  publicationWarning,
  containerActionUnavailable,
  containerRemovalUnavailable,
} from "../src/components/resources/model.ts";

test("a running Docker container cannot authorize an unverified Windows link", () => {
  const row = { State: "running", Ports: "127.0.0.1:54321->80/tcp" };
  assert.equal(publishedPorts(row)[0].url, undefined);
  assert.match(publicationWarning(row), /unavailable/);
  const verified = {
    ...row,
    OrbWinHostPorts: [{ port: 54321, protocol: "tcp", published: true }],
  };
  assert.equal(publishedPorts(verified)[0].url, "http://localhost:54321/");
  assert.equal(publicationWarning(verified), "");
  assert.equal(
    publishedPorts({
      ...verified,
      OrbWinHostPorts: [{ port: 54321, protocol: "tcp", published: "true" }],
    })[0].url,
    undefined,
  );
  assert.match(
    publicationWarning({ ...row, OrbWinPortError: "status read failed" }),
    /status read failed/,
  );
});

test("paused and unknown container states never offer unsafe lifecycle guesses", () => {
  assert.equal(
    containerActionUnavailable("unpause", { State: "paused" }),
    undefined,
  );
  assert.match(
    containerActionUnavailable("start", { State: "paused" }),
    /Resume/,
  );
  assert.equal(
    containerActionUnavailable("stop", { State: "paused" }),
    undefined,
  );
  assert.match(containerRemovalUnavailable({ State: "paused" }), /Stop/);
  assert.match(containerActionUnavailable("start", {}), /unknown/);
  assert.match(containerRemovalUnavailable({}), /Refresh/);
  assert.equal(
    containerActionUnavailable("start", { State: "exited" }),
    undefined,
  );
  assert.equal(containerRemovalUnavailable({ State: "exited" }), undefined);
});
