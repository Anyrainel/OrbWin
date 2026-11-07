// Exact copied lockfile, fresh scanner profile and public advisory DB. No fixes.
import { spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { readFileSync, writeFileSync, mkdirSync, copyFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";
const root = fileURLToPath(new URL("../", import.meta.url));
const folder = resolve(
  root,
  "artifacts/qualification",
  `p2-rust-audit-${randomUUID()}`,
);
mkdirSync(folder);
const hash = (p) => createHash("sha256").update(readFileSync(p)).digest("hex");
const source = resolve(root, "src-tauri/Cargo.lock"),
  lockSha256 = hash(source);
copyFileSync(source, resolve(folder, "Cargo.lock"));
const tool = resolve(
  root,
  ".tools/audits/cargo-audit-v0.22.2/bin/cargo-audit.exe",
);
const toolSha256 = hash(tool),
  profile = resolve(folder, "profile"),
  cargo = resolve(profile, ".cargo");
mkdirSync(cargo, { recursive: true });
const gitConfig = resolve(profile, "gitconfig");
writeFileSync(gitConfig, "", { flag: "wx" });
const env = {
  SystemRoot: process.env.SystemRoot,
  WINDIR: process.env.WINDIR,
  TEMP: process.env.TEMP,
  TMP: process.env.TMP,
  USERPROFILE: profile,
  HOME: profile,
  APPDATA: profile,
  LOCALAPPDATA: profile,
  XDG_CONFIG_HOME: profile,
  CARGO_HOME: cargo,
  PATH: resolve(process.env.SystemRoot, "System32"),
  GIT_CONFIG_NOSYSTEM: "1",
  GIT_CONFIG_GLOBAL: gitConfig,
  GIT_TERMINAL_PROMPT: "0",
};
const options = {
  cwd: folder,
  env,
  encoding: "utf8",
  windowsHide: true,
  timeout: 120000,
  maxBuffer: 16 << 20,
};
const version = spawnSync(tool, ["audit", "--version"], options);
assert.equal(version.status, 0);
assert.match(version.stdout, /0\.22\.2/);
const started = new Date().toISOString();
const result = spawnSync(
  tool,
  [
    "audit",
    "--json",
    "--file",
    resolve(folder, "Cargo.lock"),
    "--db",
    resolve(folder, "advisory-db"),
    "--url",
    "https://github.com/RustSec/advisory-db.git",
  ],
  options,
);
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
assert.equal(hash(source), lockSha256);
assert.equal(hash(resolve(folder, "Cargo.lock")), lockSha256);
assert.equal(hash(tool), toolSha256);
const completed =
  !result.error &&
  [0, 1].includes(result.status) &&
  !!report?.database &&
  !!report?.vulnerabilities;
const proof = {
  schema: 1,
  started,
  finished: new Date().toISOString(),
  toolVersion: version.stdout.trim(),
  toolSha256,
  lockSha256,
  rootLockUnchanged: true,
  scope:
    "All-platform exact Cargo lock graph, including build/transitive crates; installed binary correspondence and applicability separate",
  isolation:
    "Copied lock, fresh profile/CARGO_HOME, empty global Git config and no system Git config, no account/proxy env, explicit public RustSec URL; no fix/install/project mutation",
  status: result.status,
  error: result.error?.code,
  completed,
  database: report?.database,
  vulnerabilities: report?.vulnerabilities,
  warnings: report?.warnings,
  reportSha256: hash(resolve(folder, "report.json")),
};
writeFileSync(resolve(folder, "proof.json"), JSON.stringify(proof, null, 2), {
  flag: "wx",
});
console.log(
  JSON.stringify(
    {
      folder,
      completed,
      status: result.status,
      toolSha256,
      lockSha256,
      database: proof.database,
      vulnerabilities: report?.vulnerabilities?.count,
      warnings: Object.fromEntries(
        Object.entries(report?.warnings ?? {}).map(([kind, rows]) => [
          kind,
          rows.map((x) => ({
            id: x.advisory.id,
            package: x.package.name,
            version: x.package.version,
          })),
        ]),
      ),
    },
    null,
    2,
  ),
);
if (!completed) process.exitCode = 1;
