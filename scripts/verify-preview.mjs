// Run against the task's already-running Vite preview. Probe only a generated
// harmless sentinel, never real backend keys or credentials.
import { mkdirSync, writeFileSync, unlinkSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { randomUUID } from "node:crypto";
const root = fileURLToPath(new URL("../", import.meta.url));
const name = `preview-deny-${randomUUID()}.txt`;
const path = resolve(root, "artifacts", name);
const sentinel = "orbwin-private-preview-probe";
mkdirSync(resolve(root, "artifacts"), { recursive: true });
writeFileSync(path, sentinel, { flag: "wx" });
const results = [];
try {
  const page = await fetch("http://127.0.0.1:1420/", {
    signal: AbortSignal.timeout(5000),
  });
  if (
    page.status !== 200 ||
    !(await page.text()).includes("<title>OrbWin</title>")
  )
    throw new Error("The OrbWin preview is not running on port 1420.");
  for (const url of [
    `http://127.0.0.1:1420/artifacts/${name}`,
    `http://127.0.0.1:1420/@fs/${path.replaceAll("\\", "/")}`,
  ]) {
    const response = await fetch(url, { signal: AbortSignal.timeout(5000) });
    if (response.status !== 403 || (await response.text()).includes(sentinel))
      throw new Error(
        "Private backend artifacts are accessible through the preview server.",
      );
    results.push({ url, status: response.status });
  }
  writeFileSync(
    resolve(root, "artifacts/preview-verification.json"),
    JSON.stringify(
      { checked: new Date().toISOString(), passed: true, results },
      null,
      2,
    ),
  );
} finally {
  unlinkSync(path);
}
console.log("Preview serves OrbWin and denies private backend artifacts.");
