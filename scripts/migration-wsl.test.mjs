import test from "node:test";
import assert from "node:assert/strict";
import {
  migrationWslReview,
  parseProjectConnection,
  appendWslMapping,
  parseWslMappingSave,
} from "../src/model/migration-wsl.ts";
import { migrationProjectReview } from "../src/model/migration-projects.ts";
const choice = {
  id: "00000000-0000-4000-8000-000000000001",
  name: "ProjectUbuntu",
};
const source = "/home/user/project 例";
const signature = "1:2";
const connected = () =>
  parseProjectConnection(
    { connected: true, export: { source, signature } },
    choice,
  );
test("saved Linux mappings remain bound to the current root and append after reviewed overrides", () => {
  const connection = connected(),
    prepared = migrationWslReview(review(), connection),
    file = source + "/mapping 例.json";
  const saved = parseWslMappingSave(
    JSON.stringify({
      version: 1,
      path: file,
      signature,
      sha256: "a".repeat(64),
    }),
    connection,
    "mapping 例.json",
  );
  const next = appendWslMapping(prepared, connection, saved.path);
  assert.deepEqual(next.inputs.overrides, [source + "/override.yaml", file]);
  assert.deepEqual(prepared.inputs.overrides, [source + "/override.yaml"]);
  for (const bad of [
    source + "-other/file.json",
    "/mnt/wsl/orbwin/projects/internal.json",
    source + "/../escape.json",
  ])
    assert.throws(() => appendWslMapping(prepared, connection, bad));
  assert.throws(() =>
    appendWslMapping(prepared, { ...connection, signature: "9:9" }, file),
  );
  assert.throws(() =>
    parseWslMappingSave(
      JSON.stringify({
        version: 1,
        path: file,
        signature: "9:9",
        sha256: "a".repeat(64),
      }),
      connection,
      "mapping 例.json",
    ),
  );
  assert.throws(() =>
    appendWslMapping(
      {
        ...prepared,
        inputs: {
          ...prepared.inputs,
          overrides: Array(15).fill(source + "/override.yaml"),
        },
      },
      connection,
      file,
    ),
  );
});
function review() {
  return migrationProjectReview(
    {
      version: 1,
      source: {
        kind: "rancher-desktop",
        daemon: "source",
        engineVersion: "29.5.3",
        apiVersion: "1.54",
        product: "",
        wsl: {
          registration: "00000000-0000-4000-8000-000000000002",
          name: "SocketUbuntu",
          machine: "a".repeat(32),
        },
      },
      target: "owned",
      images: [],
      volumes: [],
      warnings: [],
      projectMembers: [
        {
          project: "sample",
          container: "b".repeat(64),
          service: "web",
          workingDirectory: source,
          configFilesLabel: source + "/base.yaml," + source + "/override.yaml",
          explicitEnvFiles: false,
          bindMounts: true,
          oneOff: false,
          volumes: ["data"],
        },
      ],
    },
    "sample",
  );
}
test("WSL migration handoff uses the explicitly chosen project connection and freezes its root", () => {
  const selected = connected();
  const value = migrationWslReview(review(), selected);
  selected.source = "/home/user/replacement";
  selected.registration = "changed";
  selected.signature = "9:9";
  assert.equal(value.suggestedFiles, true);
  assert.deepEqual(value.inputs, {
    kind: "wslCompose",
    action: "validate",
    distribution: "ProjectUbuntu",
    connection: { registration: choice.id, source, signature },
    project: "sample",
    file: source + "/base.yaml",
    overrides: [source + "/override.yaml"],
    directory: source,
    profiles: [],
    services: [],
  });
});
test("disconnected, protected, malformed and missing project observations cannot authorize handoff", () => {
  const comma = parseProjectConnection(
    { connected: true, export: { source: source + ",comma", signature } },
    choice,
  );
  const commaReview = review();
  commaReview.members[0].workingDirectory = comma.source;
  commaReview.members[0].configFilesLabel = comma.source + "/compose.yaml";
  const manual = migrationWslReview(commaReview, comma);
  assert.equal(manual.suggestedFiles, false);
  assert.equal(manual.inputs.connection.source, comma.source);
  assert.equal(manual.inputs.file, "");
  for (const value of [
    null,
    {},
    { connected: false, export: { source } },
    { connected: true, export: null },
    { connected: true, export: { source } },
    { connected: true, export: { source, signature: "1:" } },
    { connected: true, export: { source: "/etc", signature } },
    { connected: true, export: { source: source + "/../other", signature } },
  ])
    assert.throws(() => parseProjectConnection(value, choice));
  assert.throws(() =>
    parseProjectConnection(
      { connected: true, export: { source, signature } },
      { ...choice, name: "docker-desktop" },
    ),
  );
});
test("WSL file suggestions refuse scope escapes, conflicting labels, Windows paths and explicit env files", () => {
  for (const change of [
    (value) =>
      (value.members[0].configFilesLabel =
        "/home/user/project 例-other/compose.yaml"),
    (value) =>
      (value.members[0].configFilesLabel = source + "/../outside.yaml"),
    (value) =>
      (value.members[0].configFilesLabel = "C:\\project\\compose.yaml"),
    (value) => (value.members[0].explicitEnvFiles = true),
    (value) =>
      value.members.push({
        ...value.members[0],
        configFilesLabel: source + "/different.yaml",
      }),
    (value) => (value.members[0].workingDirectory = "/home/user/outside"),
  ]) {
    const value = review();
    change(value);
    const next = migrationWslReview(value, connected());
    assert.equal(next.suggestedFiles, false);
    assert.equal(next.inputs.file, "");
    assert.deepEqual(next.inputs.overrides, []);
    assert.equal(next.inputs.connection.source, source);
  }
});
