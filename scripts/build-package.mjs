// Build a reviewed fresh assembly only. Never install or start the product.
import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { buildOutput } from "./build-output.mjs";
import { packageTauriConfiguration } from "./package-input-config.mjs";
const root = fileURLToPath(new URL("../", import.meta.url)),
  args = process.argv.slice(2);
const index = args.indexOf("--input-dir");
assert.ok(index >= 0 && args[index + 1], "Explicit --input-dir required");
const input = args[index + 1];
args.splice(index, 2);
const prepared = packageTauriConfiguration(root, input); // verify BEFORE creating build output
const selected = buildOutput(root, args, ["--no-bundle"]);
const output = selected.directory,
  config = resolve(output, "tauri.package.conf.json");
prepared.config.build.frontendDist = resolve(output, "frontend");
writeFileSync(config, JSON.stringify(prepared.config, null, 2), { flag: "wx" });
// Compile only. Vite output belongs to this fresh build, never root dist or a
// selected package directory. No frontend or product role is started.
execFileSync(
  process.execPath,
  [resolve(root, "node_modules/typescript/bin/tsc"), "-b"],
  { cwd: root, windowsHide: true, stdio: "inherit" },
);
execFileSync(
  process.execPath,
  [
    resolve(root, "node_modules/vite/bin/vite.js"),
    "build",
    "--outDir",
    prepared.config.build.frontendDist,
  ],
  { cwd: root, windowsHide: true, stdio: "inherit" },
);
execFileSync(
  process.execPath,
  [
    resolve(root, "node_modules/@tauri-apps/cli/tauri.js"),
    "build",
    "--config",
    config,
    ...(selected.flags.has("--no-bundle")
      ? ["--no-bundle"]
      : ["--bundles", "nsis"]),
    "--",
    "--locked",
  ],
  {
    cwd: root,
    windowsHide: true,
    stdio: "inherit",
    env: {
      ...process.env,
      ORBWIN_COMPONENT_INPUT_DIR: prepared.componentInput,
      ORBWIN_TOOLS_INPUT_DIR: prepared.toolsInput,
      CARGO_TARGET_DIR: resolve(output, "target"),
    },
  },
);
