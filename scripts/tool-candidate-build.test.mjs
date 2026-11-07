import assert from "node:assert/strict";
import test from "node:test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {
  toolCandidateArguments,
  createToolCandidate,
  toolCandidateManifest,
  toolBuildRecipePins,
} from "./tool-candidate-build.mjs";

function fixture(t) {
  const root = fs.realpathSync(
    fs.mkdtempSync(path.join(os.tmpdir(), "orbwin-tool-build-")),
  );
  t.after(() => {
    assert.equal(fs.realpathSync(root), root);
    fs.rmSync(root, { recursive: true });
  });
  fs.mkdirSync(path.join(root, "artifacts"));
  fs.mkdirSync(path.join(root, "source"));
  fs.writeFileSync(path.join(root, "source/go.mod"), "module fixture\n");
  return root;
}
test("tool candidate preflight preserves source and selected output until exclusive creation", (t) => {
  const root = fixture(t),
    source = path.join(root, "source");
  fs.mkdirSync(path.join(root, "artifacts/runtime-tools"));
  fs.writeFileSync(
    path.join(root, "artifacts/runtime-tools/docker.exe"),
    "selected",
  );
  const selected = toolCandidateArguments(root, [
    "--source-dir",
    source,
    "--output-dir",
    "artifacts/fresh",
  ]);
  assert.equal(fs.existsSync(selected.directory), false);
  const env = createToolCandidate(selected);
  assert.equal(env.GOPROXY, "off");
  assert.equal(env.GOSUMDB, "off");
  assert.equal(env.GOCACHE, path.join(selected.directory, "gocache"));
  assert.equal(env.GOTMPDIR, path.join(selected.directory, "go-tmp"));
  assert.equal(
    fs.readFileSync(path.join(source, "go.mod"), "utf8"),
    "module fixture\n",
  );
  assert.equal(
    fs.readFileSync(
      path.join(root, "artifacts/runtime-tools/docker.exe"),
      "utf8",
    ),
    "selected",
  );
  assert.throws(() => createToolCandidate(selected), { code: "EEXIST" });
});
test("missing repeated unsafe and test selectors refuse before output effects", (t) => {
  const root = fixture(t),
    source = path.join(root, "source");
  for (const args of [
    [],
    ["--output-dir", "artifacts/fresh"],
    ["--source-dir", source],
    ["--source-dir", "relative", "--output-dir", "artifacts/fresh"],
    ["--source-dir", source, "--output-dir", "artifacts/fresh", "--test"],
    [
      "--source-dir",
      source,
      "--source-dir",
      source,
      "--output-dir",
      "artifacts/fresh",
    ],
    ["--source-dir", source, "--output-dir", "outside"],
  ])
    assert.throws(() => toolCandidateArguments(root, args));
  assert.deepEqual(fs.readdirSync(path.join(root, "artifacts")), []);
  const under = path.join(root, "artifacts/source");
  fs.mkdirSync(under);
  assert.throws(
    () =>
      toolCandidateArguments(root, [
        "--source-dir",
        under,
        "--output-dir",
        "artifacts/source/new",
      ]),
    /source tree/,
  );
  assert.equal(fs.existsSync(path.join(under, "new")), false);
});
test("redirected source and manifest refuse instead of adopting matching outside bytes", (t) => {
  const root = fixture(t),
    alias = path.join(root, "alias");
  fs.symlinkSync(path.join(root, "source"), alias, "junction");
  assert.throws(
    () =>
      toolCandidateArguments(root, [
        "--source-dir",
        alias,
        "--output-dir",
        "artifacts/fresh",
      ]),
    /Redirected/,
  );
  const actual = path.join(root, "artifacts/actual");
  fs.mkdirSync(actual);
  fs.writeFileSync(path.join(actual, "compose-manifest.json"), "{}");
  fs.symlinkSync(actual, path.join(root, "artifacts/alias"), "junction");
  assert.throws(
    () =>
      toolCandidateManifest(
        root,
        { manifest: "artifacts/alias/compose-manifest.json" },
        "compose",
      ),
    /Redirected/,
  );
});
test("explicit locked candidate never falls back to an older selected manifest", (t) => {
  const root = fixture(t);
  fs.mkdirSync(path.join(root, "artifacts/compose"));
  fs.writeFileSync(
    path.join(root, "artifacts/compose/compose-manifest.json"),
    "old",
  );
  assert.equal(
    toolCandidateManifest(root, {}, "compose"),
    path.join(root, "artifacts/compose/compose-manifest.json"),
  );
  assert.throws(
    () =>
      toolCandidateManifest(
        root,
        { manifest: "artifacts/missing/compose-manifest.json" },
        "compose",
      ),
    { code: "ENOENT" },
  );
  for (const manifest of [
    "../compose-manifest.json",
    "artifacts/../compose-manifest.json",
    "artifacts/candidate/buildx-manifest.json",
    "C:/candidate/compose-manifest.json",
  ])
    assert.throws(() => toolCandidateManifest(root, { manifest }, "compose"));
});
test("recipe closure captures both shared selector and output ownership recipes", (t) => {
  const root = fixture(t);
  fs.mkdirSync(path.join(root, "scripts"));
  for (const name of ["tool-candidate-build.mjs", "build-output.mjs"])
    fs.writeFileSync(path.join(root, "scripts", name), name);
  const before = toolBuildRecipePins(root);
  fs.writeFileSync(path.join(root, "scripts/build-output.mjs"), "changed");
  assert.notDeepEqual(toolBuildRecipePins(root), before);
});
