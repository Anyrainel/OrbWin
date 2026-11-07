import test from "node:test";
import assert from "node:assert/strict";
import { resolve } from "node:path";
import {
  configurationForAssembly,
  packageTauriConfiguration,
} from "./package-input-config.mjs";
test("fresh assembly config replaces every selected-artifact resource without preparation replay", () => {
  const root = resolve("."),
    input = resolve("artifacts/fresh-review"),
    base = {
      build: { beforeBuildCommand: "npm run package:prepare && npm run build" },
      bundle: {
        resources: { "../artifacts/package/runtime/": "runtime/" },
        icon: ["icons/icon.ico"],
        windows: {
          nsis: {
            installerHooks: "installer-hooks.nsh",
            template: "installer-template.nsi",
            customLanguageFiles: { English: "installer-language.nsh" },
          },
        },
      },
    };
  const manifest = {
    applicationFiles: Object.fromEntries(
      [
        "orbwin-cli.exe",
        "orbwin-docker.exe",
        "cli-manifest.json",
        "completions/OrbWin.ps1",
      ].map((n) => [n, "data"]),
    ),
  };
  const config = configurationForAssembly(root, input, manifest, base);
  assert.equal(config.build.beforeBuildCommand, null);
  assert.equal(Object.keys(config.bundle.resources).length, 5);
  assert.ok(
    Object.keys(config.bundle.resources).every((p) => p.startsWith(input)),
  );
  assert.ok(
    Object.keys(base.bundle.resources).includes(
      "../artifacts/package/runtime/",
    ),
  );
  assert.throws(
    () => configurationForAssembly(root, input, { applicationFiles: {} }, base),
    /inventory/,
  );
  assert.throws(() => packageTauriConfiguration(root, "relative"), /absolute/);
});
