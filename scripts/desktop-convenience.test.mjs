import test from "node:test";
import assert from "node:assert/strict";
import { buildSync } from "esbuild";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
function load(relative) {
  const { outputFiles } = buildSync({
    entryPoints: [fileURLToPath(new URL(relative, import.meta.url))],
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
  return module.exports;
}
test("preview sign-in settings present accessible opt-in and unavailable state without enabling writes", () => {
  const { DesktopConvenience } = load(
    "../src/components/settings/Settings.tsx",
  );
  const html = renderToStaticMarkup(createElement(DesktopConvenience));
  assert.match(html, /section aria-label="Tray and sign-in"/);
  assert.match(html, /<label[^>]*>.*<input type="checkbox" disabled=""/);
  assert.match(html, /Run OrbWin in the tray at sign-in/);
  assert.match(html, /unavailable in browser preview/);
  assert.match(html, /without starting, stopping or changing the engine/);
  assert.match(html, /<button[^>]*disabled=""[^>]*>Read sign-in preference/);
  assert.doesNotMatch(html, /checked=""|role="dialog"/);
});
test("preview backend cannot read or change native startup registration", async () => {
  const { desktopPreferences, desktopSetStartup } = load("../src/backend.ts");
  await assert.rejects(desktopPreferences(), /Windows desktop application/);
  await assert.rejects(desktopSetStartup(true), /Windows desktop application/);
  await assert.rejects(desktopSetStartup(false), /Windows desktop application/);
});
