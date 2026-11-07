// Queries public advisory metadata and reads exact final binaries; never runs them.
import assert from "node:assert/strict";
import { createHash, randomUUID } from "node:crypto";
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { verifyBuildxSource } from "./buildx-provenance.mjs";
const root = fileURLToPath(new URL("../", import.meta.url)),
  folder = resolve(
    root,
    "artifacts/qualification",
    `p2-client-security-${randomUUID()}`,
  );
mkdirSync(resolve(folder, "profile/go/telemetry"), { recursive: true });
writeFileSync(resolve(folder, "profile/go/telemetry/mode"), "off", {
  flag: "wx",
});
const hash = (p) => createHash("sha256").update(readFileSync(p)).digest("hex"),
  lockPath = resolve(root, "runtime-tools.lock.json"),
  pins = JSON.parse(readFileSync(lockPath)),
  lockHash = hash(lockPath);
const scanner = resolve(
    root,
    ".tools/audits/govulncheck-v1.8.0/govulncheck.exe",
  ),
  scannerSha256 =
    "1e6981feb46fe2026fa4ffe83059c86ba6cf961150bd2948e1c6e87f44c76f87";
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
function objects(text) {
  const records = [];
  let depth = 0,
    start = -1,
    quote = false,
    escape = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quote) {
      if (escape) escape = false;
      else if (c === "\\") escape = true;
      else if (c === '"') quote = false;
    } else if (c === '"') quote = true;
    else if (c === "{") {
      if (depth++ === 0) start = i;
    } else if (c === "}" && --depth === 0)
      records.push(JSON.parse(text.slice(start, i + 1)));
  }
  assert.equal(depth, 0);
  assert(!quote);
  return records;
}
const proof = {
  schema: 1,
  started: new Date().toISOString(),
  runtimeLockSha256: lockHash,
  scanner: "govulncheck v1.8.0",
  scannerSha256,
  scope:
    "Exact final staged Windows/Linux Compose+Buildx binaries; metadata/symbol scan only, not engine/installed/runtime exposure or blanket crypto/advisory clearance",
  isolation:
    "Fresh profile, telemetry off, minimal child environment with public Go vulnerability DB only; no credential/proxy environment or inspected-binary execution",
  records: [],
};
const save = () =>
  writeFileSync(
    resolve(folder, "proof.json"),
    JSON.stringify(proof, null, 2) + "\n",
  );
function run(name, args) {
  const r = spawnSync(scanner, args, {
    cwd: folder,
    env,
    windowsHide: true,
    encoding: "utf8",
    maxBuffer: 32 << 20,
    timeout: 90000,
  });
  writeFileSync(resolve(folder, name), r.stdout ?? "");
  writeFileSync(resolve(folder, name + ".stderr"), r.stderr ?? "");
  assert.equal(r.status, 0, `Scanner ${name} failed`);
  return {
    messages: objects(r.stdout ?? ""),
    stdoutSha256: hash(resolve(folder, name)),
    stderrSha256: hash(resolve(folder, name + ".stderr")),
    status: r.status,
  };
}
try {
  assert.equal(hash(scanner), scannerSha256);
  verifyBuildxSource(root, pins, { buildMissing: false });
  const selected = pins.tools.filter((t) =>
    ["compose", "buildx"].includes(t.localSource?.name),
  );
  assert.equal(selected.length, 4);
  const fixedHttp2 = [
    "GO-2026-6603",
    "GO-2026-6610",
    "GO-2026-6611",
    "GO-2026-6612",
    "GO-2026-6617",
  ];
  for (const tool of selected) {
    const path = resolve(root, "artifacts/runtime-tools", tool.path);
    assert.equal(hash(path), tool.sha256);
    const name = `${tool.localSource.name}-${tool.localSource.target}`;
    const scan = run(name + ".scan.jsonl", ["-json", "-mode", "binary", path]),
      extraction = run(name + ".extract.json", ["-mode", "extract", path]);
    assert.equal(extraction.messages.length, 2);
    assert.equal(extraction.messages[0].name, "govulncheck-extract");
    const body = extraction.messages[1],
      symbols = body.pkgSymbols ?? [],
      findings = scan.messages.filter((m) => m.finding).map((m) => m.finding),
      ids = [...new Set(findings.map((f) => f.osv))].sort(),
      pk = (s) => s.pkg ?? s.Pkg ?? "";
    const counts = Object.fromEntries(
      ids.map((id) => {
        const f = findings.filter((f) => f.osv === id);
        return [
          id,
          {
            reportedFunctions: f.filter((f) => f.trace?.some((t) => t.function))
              .length,
            reportedPackages: f.filter((f) => f.trace?.some((t) => t.package))
              .length,
          },
        ];
      }),
    );
    const record = {
      path: `artifacts/runtime-tools/${tool.path}`,
      name: tool.localSource.name,
      target: tool.localSource.target,
      version: tool.version,
      sha256: tool.sha256,
      config: scan.messages.find((m) => m.config)?.config,
      go: body.goVersion,
      extractedSymbols: symbols.length,
      findingBasis: symbols.length
        ? "extracted-symbols with advisory attribution limitations"
        : "advisory-module-metadata-fallback; function fields do not establish presence",
      legacyOpenPGPSymbols: symbols.filter((s) =>
        pk(s).startsWith("golang.org/x/crypto/openpgp"),
      ).length,
      oldDockerDaemonSymbols: symbols.filter((s) =>
        pk(s).startsWith("github.com/docker/docker/daemon"),
      ).length,
      ids,
      counts,
      findings,
      scan: {
        stdoutSha256: scan.stdoutSha256,
        stderrSha256: scan.stderrSha256,
      },
      extraction: {
        stdoutSha256: extraction.stdoutSha256,
        stderrSha256: extraction.stderrSha256,
      },
      unchanged: hash(path) === tool.sha256,
    };
    proof.records.push(record);
    save();
    assert(record.unchanged);
    assert(
      !ids.some((id) => fixedHttp2.includes(id)),
      "Final binary still matches corrected HTTP2 advisory",
    );
    const expected =
      tool.localSource.name === "compose"
        ? ["GO-2026-5932"]
        : [
            "GO-2026-4883",
            "GO-2026-4887",
            "GO-2026-5617",
            "GO-2026-5668",
            "GO-2026-5746",
            "GO-2026-5932",
            "GO-2026-6253",
          ];
    assert.deepEqual(
      ids,
      expected.sort(),
      "Public DB findings changed; retain and review instead of accepting an outdated expectation",
    );
  }
  assert.equal(hash(scanner), scannerSha256);
  assert.equal(hash(lockPath), lockHash);
  proof.success = true;
} catch (error) {
  proof.success = false;
  proof.failure = { code: error.code ?? error.name, message: error.message };
  process.exitCode = 1;
} finally {
  proof.finished = new Date().toISOString();
  save();
  console.log(resolve(folder, "proof.json"));
}
