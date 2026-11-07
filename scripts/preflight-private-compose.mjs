// Read-only production-client/plugin/cache/metadata check, never a fixture run.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  preflightPrivateCompose,
  safeStageFailure,
} from "./private-compose-stage.mjs";
assert.equal(process.env.ORBWIN_QUALIFY_OWNED, "1");
assert.ok(process.env.ORBWIN_QUALIFY_DOCKER_WRAPPER);
const root = fileURLToPath(new URL("../", import.meta.url));
const state = {
  wrapper: resolve(process.env.ORBWIN_QUALIFY_DOCKER_WRAPPER),
  privateConfig: resolve(
    process.env.LOCALAPPDATA,
    "OrbWinData/home/.rdorbwin/docker",
  ),
};
const raw = resolve(dirname(state.wrapper), "runtime/tools/bin/docker.exe");
const ctx = {
  root,
  state,
  execute(binary, args, options = {}) {
    if (binary === state.wrapper) {
      assert.ok(
        JSON.stringify(args) ===
          JSON.stringify(["compose", "version", "--short"]) ||
          JSON.stringify(args) ===
            JSON.stringify([
              "info",
              "--format",
              "{{json .ClientInfo.Plugins}}",
            ]),
        "Unexpected alias mutation refused",
      );
    } else {
      assert.equal(binary, raw);
      const prefix = [
        "--config",
        state.privateConfig,
        "--host",
        "npipe:////./pipe/docker-rancher-desktop-orbwin",
        "buildx",
        "--builder",
        "default",
      ];
      assert.deepEqual(args.slice(0, prefix.length), prefix);
      const tail = args.slice(prefix.length);
      assert.ok(
        JSON.stringify(tail) ===
          JSON.stringify(["du", "--format=json", "--timeout=10s"]) ||
          JSON.stringify(tail) ===
            JSON.stringify(["history", "ls", "--format=json", "--no-trunc"]),
        "Unexpected builder mutation refused",
      );
    }
    const result = spawnSync(binary, args, {
      env: {
        ...process.env,
        DOCKER_CONTEXT: "",
        DOCKER_TLS_VERIFY: "",
        DOCKER_CERT_PATH: "",
        DOCKER_API_VERSION: "",
        BUILDKIT_HOST: "",
        ...options.extraEnv,
      },
      windowsHide: true,
      encoding: "utf8",
      timeout: options.timeout ?? 30000,
      maxBuffer: 4 << 20,
    });
    assert.equal(result.error, undefined);
    assert.equal(
      result.status,
      0,
      "Read-only preflight command failed; output omitted",
    );
    return result;
  },
};
try {
  const result = preflightPrivateCompose(ctx);
  console.log(
    JSON.stringify({
      success: true,
      selectedPlugins: result.selectedPlugins,
      cacheRecords: result.cache.du.length,
      historyRecords: result.cache.history.length,
      selectionHashes: result.metadata.selection,
      metadataEntries: result.metadata.entries.length,
    }),
  );
} catch (error) {
  console.log(
    JSON.stringify({
      success: false,
      failure: safeStageFailure(error, {
        mode: "readonly-preflight",
        stage: "tools-cache-metadata",
      }),
    }),
  );
  process.exitCode = 1;
}
