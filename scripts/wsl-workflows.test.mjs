import test from "node:test";
import assert from "node:assert/strict";
import { wslComposeRequest } from "../src/model/wsl-workflows.ts";
import { wslComposeInputs } from "../src/components/resources/model.ts";
const source = "/home/user/project 例";

test("reopened WSL inputs retain the reviewed root, identity and independent file/service lists", () => {
  const hints = {
    signature: "1:2",
    file: "base.yaml",
    overrides: ["override.yaml"],
    envFiles: ["first.env", "last 例.env"],
    profiles: ["worker"],
    services: ["web"],
    project: "sample",
  };
  const request = wslComposeRequest("Ubuntu", source, "up", hints);
  hints.signature = "3:4";
  hints.overrides.push("later.yaml");
  hints.envFiles.push("later.env");
  hints.profiles.push("later");
  hints.services.push("later");
  assert.deepEqual(request.connection, { source, signature: "1:2" });
  assert.deepEqual(request.overrides, ["override.yaml"]);
  assert.deepEqual(request.envFiles, ["first.env", "last 例.env"]);
  assert.deepEqual(request.services, ["web"]);
  assert.deepEqual(request.profiles, ["worker"]);
  for (const action of ["validate", "down"])
    assert.deepEqual(
      wslComposeRequest("Ubuntu", source, action, hints).services,
      [],
    );
});
test("missing or contradictory WSL review cannot become a workflow request", () => {
  assert.throws(() => wslComposeRequest("Ubuntu", source, "up"), /Inspect/);
  for (const signature of [
    "",
    ":",
    "1:",
    "1:2:3",
    "bad",
    "1".repeat(65) + ":2",
  ])
    assert.throws(() =>
      wslComposeRequest("Ubuntu", source, "up", { signature }),
    );
  assert.throws(
    () =>
      wslComposeRequest("Ubuntu", source, "up", {
        connection: { source: source + "-other", signature: "1:2" },
      }),
    /differ/,
  );
});
test("WSL group labels retain source identity while older labels remain identifiable for manual review", () => {
  const inputs = {
    distribution: "Ubuntu",
    source,
    signature: "1:2",
    file: "compose.yaml",
    overrides: [],
    directory: "",
    project: "sample",
    profiles: [],
  };
  const row = (value) => ({
    Labels: {
      "com.docker.compose.project": "sample",
      "dev.orbwin.wsl.compose-inputs": JSON.stringify(value),
    },
  });
  const parsed = wslComposeInputs("sample", [row(inputs)]);
  assert.equal(parsed.signature, "1:2");
  assert.deepEqual(
    wslComposeRequest("Ubuntu", parsed.source, "validate", parsed).connection,
    { source, signature: "1:2" },
  );
  const old = { ...inputs };
  delete old.signature;
  const legacy = wslComposeInputs("sample", [row(old)]);
  assert.ok(legacy);
  assert.throws(
    () => wslComposeRequest("Ubuntu", legacy.source, "validate", legacy),
    /Inspect/,
  );
  assert.equal(
    wslComposeInputs("sample", [row({ ...inputs, signature: "bad" })]),
    undefined,
  );
  assert.equal(
    wslComposeInputs("sample", [
      row(inputs),
      row({ ...inputs, signature: "3:4" }),
    ]),
    undefined,
  );
});
