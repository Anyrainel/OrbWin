// Version/checksum pins are inherited from the selected upstream tool manifest.
// Downloads are never executable until verified and atomically renamed.
import {
  readFileSync,
  existsSync,
  mkdirSync,
  writeFileSync,
  renameSync,
  unlinkSync,
} from "node:fs";
import { resolve, dirname } from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { fileURLToPath } from "node:url";
const root = fileURLToPath(new URL("../", import.meta.url));
const lock = JSON.parse(readFileSync(resolve(root, "runtime-tools.lock.json")));
const digest = (data) => createHash("sha256").update(data).digest("hex");
await Promise.all(
  lock.tools.map(async (tool) => {
    const path = resolve(root, "artifacts/runtime-tools", tool.path);
    mkdirSync(dirname(path), { recursive: true });
    if (existsSync(path)) {
      if (digest(readFileSync(path)) !== tool.sha256)
        throw new Error(`Existing staged tool differs from lock: ${tool.path}`);
      console.log(`Verified cached ${tool.path}`);
      return;
    }
    const response = await fetch(tool.url, {
      signal: AbortSignal.timeout(180000),
    });
    if (!response.ok)
      throw new Error(
        `Download failed for ${tool.path}: HTTP ${response.status}`,
      );
    const data = Buffer.from(await response.arrayBuffer());
    if (data.length > 256 * 1024 * 1024 || digest(data) !== tool.sha256)
      throw new Error(`Checksum/size rejection: ${tool.path}`);
    const temporary = path + ".partial-" + randomUUID();
    try {
      writeFileSync(temporary, data, { flag: "wx" });
      renameSync(temporary, path);
    } finally {
      if (existsSync(temporary)) unlinkSync(temporary);
    }
    console.log(`Downloaded and verified ${tool.path}`);
  }),
);
writeFileSync(
  resolve(root, "artifacts/runtime-tools/manifest.json"),
  JSON.stringify(lock, null, 2) + "\n",
);
