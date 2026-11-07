import test from "node:test";
import assert from "node:assert/strict";
import { buildSync } from "esbuild";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
const { outputFiles } = buildSync({
  entryPoints: [
    fileURLToPath(new URL("../src/components/Operations.tsx", import.meta.url)),
  ],
  bundle: true,
  write: false,
  platform: "node",
  format: "cjs",
  external: ["react"],
  logLevel: "silent",
});
const module = { exports: {} };
new Function("require", "module", "exports", outputFiles[0].text)(
  createRequire(import.meta.url),
  module,
  module.exports,
);
const {
  composeSpec,
  wslComposeSpec,
  rememberedProjectLabel,
  RememberedProjectDetails,
} = module.exports;
const inputs = {
  project: "shop",
  file: "C:\\fixture\\compose.json",
  directory: "C:\\fixture",
  overrides: [],
  profiles: [],
  migration: {
    source: {
      kind: "docker-desktop",
      project: "original-shop",
      daemon: "source-uuid",
    },
    target: "target-uuid",
  },
};
test("remembered labels use friendly source without leaking long provenance into picker", () => {
  assert.equal(rememberedProjectLabel(inputs), "shop · Docker Desktop");
  const longLabel = rememberedProjectLabel({
    ...inputs,
    project: "p".repeat(128),
    kind: "wsl",
    distribution: "Distro".repeat(40),
  });
  assert.ok(longLabel.length < 96);
  assert.ok(longLabel.includes("…"));
  assert.ok(!longLabel.includes("source-uuid"));
  assert.equal(
    rememberedProjectLabel({
      ...inputs,
      kind: "wsl",
      distribution: "Ubuntu",
      connection: { source: "/work/project" },
    }),
    "shop · Docker Desktop · WSL: Ubuntu",
  );
  assert.equal(
    rememberedProjectLabel({ ...inputs, migration: undefined }),
    "shop",
  );
  assert.equal(
    rememberedProjectLabel({
      ...inputs,
      migration: {
        ...inputs.migration,
        source: { ...inputs.migration.source, kind: "rancher-desktop" },
      },
    }),
    "shop · Rancher Desktop",
  );
});
test("selected details retain exact identity in native accessible disclosure and escape text", () => {
  const html = renderToStaticMarkup(
    createElement(RememberedProjectDetails, {
      inputs: {
        ...inputs,
        project: "<shop>",
        kind: "wsl",
        distribution: "Ubuntu",
        connection: {
          source: "/work/project",
          registration: "registration-uuid",
          signature: "2112:1374236",
        },
      },
    }),
  );
  assert.match(
    html,
    /<details class="operation-review-details"><summary>Saved project details<\/summary>/,
  );
  for (const value of [
    "&lt;shop&gt;",
    "original-shop",
    "source-uuid",
    "target-uuid",
    "registration-uuid",
    "2112:1374236",
    "/work/project",
  ])
    assert.ok(html.includes(value), value);
  assert.doesNotMatch(html, /<shop>/);
  assert.match(html, /unverified hint/);
});
test("reopened Windows Validate has explicit action and retains all source review without changing request/defaults", () => {
  const note = "Source-specific collision and mapping review";
  const spec = composeSpec("validate", inputs, note);
  assert.equal(spec.confirmLabel, "Validate");
  assert.ok(spec.description.length < 160);
  assert.ok(!spec.description.includes(note));
  for (const text of [
    note,
    "No transfer status is saved",
    "interpolation/include",
    "errors can contain file content",
    "does not reserve ports",
  ])
    assert.ok(spec.reviewDetails.includes(text), text);
  assert.deepEqual(spec.request, {
    kind: "compose",
    file: "",
    project: "",
    action: "validate",
  });
  assert.equal(spec.defaults.file, inputs.file);
  assert.deepEqual(spec.migration, inputs.migration);
  assert.equal(composeSpec("up", inputs).reviewDetails, undefined);
  assert.match(
    composeSpec("up", inputs).description,
    /may recreate containers/,
  );
});
test("Linux Validate uses same explicit action while preserving WSL path and connection warnings", () => {
  const spec = wslComposeSpec("Ubuntu", "/work/project", "validate", {
    signature: "1:2",
  });
  assert.equal(spec.confirmLabel, "Validate");
  assert.ok(spec.description.length < 180);
  for (const text of [
    "/work/project",
    "inside that root",
    "disconnected/replaced",
    "Keep a WSL session open",
  ])
    assert.ok(spec.reviewDetails.includes(text), text);
  assert.equal(spec.request.kind, "wslCompose");
  assert.equal(spec.request.action, "validate");
  assert.equal(
    wslComposeSpec("Ubuntu", "/work/project", "down", { signature: "1:2" })
      .reviewDetails,
    undefined,
  );
  assert.match(
    wslComposeSpec("Ubuntu", "/work/project", "down", { signature: "1:2" })
      .description,
    /retaining volumes/,
  );
});
