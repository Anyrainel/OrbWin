// Advisory queries for exact local binaries, never execution of those binaries.
// Scanner telemetry/profile/cache are isolated; no source/archive upload or fixes.
import { createHash, randomUUID } from "node:crypto";
import { createReadStream } from "node:fs";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { spawn } from "node:child_process";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";
const root = fileURLToPath(new URL("../", import.meta.url));
const folder = resolve(
  root,
  "artifacts/qualification",
  `p2-go-audit-${randomUUID()}`,
);
await mkdir(resolve(folder, "profile/go/telemetry"), { recursive: true });
await writeFile(resolve(folder, "profile/go/telemetry/mode"), "off", {
  flag: "wx",
});
const tool = resolve(root, ".tools/audits/govulncheck-v1.8.0/govulncheck.exe");
async function hash(path) {
  const h = createHash("sha256");
  for await (const data of createReadStream(path)) h.update(data);
  return h.digest("hex");
}
const toolSha256 = await hash(tool);
assert.equal(
  toolSha256,
  "1e6981feb46fe2026fa4ffe83059c86ba6cf961150bd2948e1c6e87f44c76f87",
);
const env = {
  SystemRoot: process.env.SystemRoot,
  WINDIR: process.env.WINDIR,
  TEMP: process.env.TEMP,
  TMP: process.env.TMP,
  APPDATA: resolve(folder, "profile"),
  LOCALAPPDATA: resolve(folder, "profile"),
  USERPROFILE: resolve(folder, "profile"),
  HOME: resolve(folder, "profile"),
  PATH: resolve(process.env.SystemRoot, "System32"),
  GOVULNDB: "https://vuln.go.dev",
  GOTOOLCHAIN: "local",
  GOENV: "off",
};
function run(args) {
  return new Promise((done) => {
    const child = spawn(tool, args, {
      cwd: folder,
      env,
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    const out = [],
      err = [];
    let size = 0,
      reason = null;
    const stop = (why) => {
      reason ??= why;
      child.kill();
    };
    const timer = setTimeout(() => stop("timeout"), 90000);
    for (const [stream, chunks] of [
      [child.stdout, out],
      [child.stderr, err],
    ])
      stream.on("data", (data) => {
        size += data.length;
        if (size > 32 * 1024 * 1024) stop("output-budget");
        else chunks.push(data);
      });
    child.on("error", (e) => {
      reason ??= e.code ?? "spawn-error";
    });
    child.on("close", (status, signal) => {
      clearTimeout(timer);
      done({
        status,
        signal,
        reason,
        stdout: Buffer.concat(out).toString(),
        stderr: Buffer.concat(err).toString(),
      });
    });
  });
}
const tools = JSON.parse(
  await readFile(resolve(root, "runtime-tools.lock.json")),
).tools;
const paths = [
  "artifacts/package/orbwin-cli.exe",
  "artifacts/package/orbwin-docker.exe",
  ...[
    "rdd.exe",
    "orbwin-session.exe",
    "docker-credential-orbwin.exe",
    "orbwin-fs",
    "orbwin-project",
    "orbwin-services",
    "orbwin-trust",
  ].map((x) => "artifacts/backend/" + x),
  ...tools.map((x) => "artifacts/runtime-tools/" + x.path),
];
const proof = {
  schema: 1,
  started: new Date().toISOString(),
  toolSha256,
  toolVersion: "govulncheck v1.8.0",
  database: "https://vuln.go.dev",
  scope:
    "Exact Go binaries at recorded paths; binary symbol analysis limitations apply. No installed/guest/platform or Rust/npm assessment implied.",
  isolation:
    "Fresh profile, telemetry off in isolated APPDATA/go/telemetry/mode, no credentials/proxy environment, public advisory metadata queries only",
  records: [],
};
async function save() {
  await writeFile(
    resolve(folder, "proof.json"),
    JSON.stringify(proof, null, 2),
  );
}
await save();
for (const path of paths) {
  const full = resolve(root, path),
    before = await hash(full),
    duplicate = proof.records.find((x) => x.sha256 === before);
  if (duplicate) {
    proof.records.push({ path, sha256: before, aliasOf: duplicate.path });
    await save();
    continue;
  }
  const started = new Date().toISOString();
  const extraction = await run(["-mode=extract", full]);
  let precision = null;
  try {
    const objects = extraction.stdout.trim().split(/\r?\n/).map(JSON.parse);
    assert.equal(extraction.status, 0);
    assert.equal(extraction.reason, null);
    assert.equal(objects.length, 2);
    assert.equal(objects[0].name, "govulncheck-extract");
    assert.equal(objects[0].version, "0.1.0");
    assert.match(objects[1].goVersion, /^go[0-9]+\.[0-9]+/);
    assert.ok(
      objects[1].modules === undefined || Array.isArray(objects[1].modules),
    );
    const symbols = objects[1].pkgSymbols ?? [];
    assert.ok(Array.isArray(symbols));
    precision = {
      extractedSymbols: symbols.length,
      observedDependencyModules: objects[1].modules?.length ?? 0,
      goVersion: objects[1].goVersion,
      mainModuleVersionKnown:
        !!objects[1].main?.Version && objects[1].main.Version !== "(devel)",
      findingBasis: symbols.length
        ? "extracted-symbols"
        : "advisory-module-metadata-fallback",
      limitation:
        "Function traces from module fallback do not establish binary presence; neither form proves runtime reachability",
    };
  } catch {}
  const result = await run(["-json", "-mode=binary", full]);
  const index = proof.records.length,
    report = `report-${index}.jsonl`,
    diagnostic = `stderr-${index}.txt`;
  const extractionReport = `extract-${index}.jsonl`;
  await writeFile(resolve(folder, extractionReport), extraction.stdout, {
    flag: "wx",
  });
  await writeFile(
    resolve(folder, `extract-stderr-${index}.txt`),
    extraction.stderr,
    { flag: "wx" },
  );
  await writeFile(resolve(folder, report), result.stdout, { flag: "wx" });
  await writeFile(resolve(folder, diagnostic), result.stderr, { flag: "wx" });
  let messages = [],
    parseError = false;
  try {
    // The protocol is a sequence of pretty-printed JSON objects, not JSONL.
    const text = result.stdout;
    let start = 0,
      depth = 0,
      inString = false,
      escaped = false;
    for (let i = 0; i < text.length; i++) {
      const c = text[i];
      if (inString) {
        if (escaped) escaped = false;
        else if (c === "\\") escaped = true;
        else if (c === '"') inString = false;
        continue;
      }
      if (c === '"') inString = true;
      else if (c === "{") {
        if (depth++ === 0) start = i;
      } else if (c === "}" && --depth === 0)
        messages.push(JSON.parse(text.slice(start, i + 1)));
      else if (depth === 0 && !/\s/.test(c)) throw Error("protocol");
    }
    if (depth !== 0 || inString) throw Error("truncated");
  } catch {
    parseError = true;
  }
  const config = messages.find((x) => x.config)?.config,
    sbom = messages.find((x) => x.SBOM || x.sbom),
    findings = messages.filter((x) => x.finding).map((x) => x.finding);
  const unchanged = (await hash(full)) === before;
  const completed =
    !result.reason &&
    !parseError &&
    [0, 3].includes(result.status) &&
    !!config &&
    !!sbom &&
    !!precision &&
    unchanged;
  const row = {
    path,
    sha256: before,
    started,
    finished: new Date().toISOString(),
    status: result.status,
    signal: result.signal,
    error: result.reason,
    parseError,
    unchanged,
    completed,
    config,
    precision,
    extractionReport,
    extractionSha256: await hash(resolve(folder, extractionReport)),
    findings,
    report,
    reportSha256: await hash(resolve(folder, report)),
    diagnostic,
  };
  proof.records.push(row);
  await save();
  console.log(
    JSON.stringify({
      path,
      completed,
      status: result.status,
      findings: findings.length,
      error: result.reason,
    }),
  );
  assert.equal(
    await hash(tool),
    toolSha256,
    "Scanner changed during assessment",
  );
}
proof.finished = new Date().toISOString();
proof.completed = proof.records.every((x) => x.completed || x.aliasOf);
await save();
console.log(
  JSON.stringify({
    folder,
    completed: proof.completed,
    records: proof.records.length,
  }),
);
if (!proof.completed) process.exitCode = 1;
