// Supplementary registry advisory scan of exact copied lockfiles. No installs,
// fixes, scripts, user/global npm config, account credentials or proxy env.
import { spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { readFileSync, writeFileSync, mkdirSync, copyFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";
const root = fileURLToPath(new URL("../", import.meta.url));
const folder = resolve(
  root,
  "artifacts/qualification",
  `p2-npm-audit-${randomUUID()}`,
);
mkdirSync(folder);
const hash = (p) => createHash("sha256").update(readFileSync(p)).digest("hex");
const files = ["package.json", "package-lock.json"];
const before = Object.fromEntries(
  files.map((name) => [name, hash(resolve(root, name))]),
);
for (const name of files)
  copyFileSync(resolve(root, name), resolve(folder, name));
const user = resolve(folder, "user.npmrc"),
  global = resolve(folder, "global.npmrc");
writeFileSync(user, "", { flag: "wx" });
writeFileSync(global, "", { flag: "wx" });
const cli = resolve(
  dirname(process.execPath),
  "node_modules/npm/bin/npm-cli.js",
);
const env = {
  SystemRoot: process.env.SystemRoot,
  WINDIR: process.env.WINDIR,
  TEMP: process.env.TEMP,
  TMP: process.env.TMP,
  APPDATA: folder,
  USERPROFILE: folder,
  PATH: `${dirname(process.execPath)};${process.env.SystemRoot}/System32`,
};
const options = {
  cwd: folder,
  env,
  encoding: "utf8",
  windowsHide: true,
  timeout: 60000,
  maxBuffer: 16 << 20,
};
const version = spawnSync(process.execPath, [cli, "--version"], options);
assert.equal(version.status, 0);
const args = [
  "audit",
  "--json",
  "--package-lock-only",
  "--ignore-scripts",
  "--registry=https://registry.npmjs.org",
  `--userconfig=${user}`,
  `--globalconfig=${global}`,
  `--cache=${resolve(folder, "cache")}`,
];
const started = new Date().toISOString();
const result = spawnSync(process.execPath, [cli, ...args], options);
writeFileSync(resolve(folder, "report.json"), result.stdout ?? "", {
  flag: "wx",
});
writeFileSync(resolve(folder, "stderr.txt"), result.stderr ?? "", {
  flag: "wx",
});
let report;
try {
  report = JSON.parse(result.stdout);
} catch {}
const after = Object.fromEntries(
  files.map((name) => [name, hash(resolve(root, name))]),
);
assert.deepEqual(after, before);
for (const name of files)
  assert.equal(hash(resolve(folder, name)), before[name]);
const completed =
  !result.error &&
  [0, 1].includes(result.status) &&
  report?.auditReportVersion === 2 &&
  report?.metadata?.vulnerabilities;
const proof = {
  schema: 1,
  started,
  finished: new Date().toISOString(),
  scope:
    "Supplementary public npm registry advisory scan, all locked frontend/build dependencies; applicability and other ecosystems remain separate",
  npmVersion: version.stdout.trim(),
  nodeVersion: process.version,
  npmCliSha256: hash(cli),
  lockfiles: before,
  rootLockfilesUnchanged: true,
  registry: "https://registry.npmjs.org",
  config:
    "Isolated exact lockfile copies and empty npm configs; no account/auth/proxy environment; no install/fix",
  status: result.status,
  error: result.error?.code,
  completed: Boolean(completed),
  counts: completed ? report.metadata.vulnerabilities : null,
  metadata: completed ? report.metadata.dependencies : null,
  databaseRevision:
    "Registry did not expose an immutable database revision; report and date retained",
  reportSha256: hash(resolve(folder, "report.json")),
};
writeFileSync(resolve(folder, "proof.json"), JSON.stringify(proof, null, 2), {
  flag: "wx",
});
console.log(JSON.stringify({ folder, ...proof }, null, 2));
if (!completed) process.exitCode = 1;
