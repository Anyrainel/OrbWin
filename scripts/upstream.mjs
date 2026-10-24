import { execFileSync } from "node:child_process";
import { readFileSync, existsSync, mkdirSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { assertOwnedChanges } from "./patch-policy.mjs";
import { createHash } from "node:crypto";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
const source = resolve(
  root,
  process.env.ORBWIN_UPSTREAM_DIR ?? ".upstream/rancher-desktop",
);
function git(args, options = {}) {
  return execFileSync("git", args, {
    cwd: source,
    encoding: "utf8",
    ...options,
  });
}
if (!existsSync(source)) {
  mkdirSync(dirname(source), { recursive: true });
  execFileSync(
    "git",
    ["clone", "--no-checkout", "--filter=blob:none", lock.repository, source],
    { stdio: "inherit" },
  );
  git(["-c", "core.autocrlf=false", "checkout", lock.commit]);
}
if (git(["rev-parse", "HEAD"]).trim() !== lock.commit) {
  throw new Error(
    "Upstream commit differs from lock. Preserve local work; use a separate checkout to evaluate upgrades.",
  );
}
const permitted = lock.patches.flatMap((patch) =>
  git(["apply", "--numstat", resolve(root, patch)])
    .trim()
    .split(/\r?\n/)
    .filter(Boolean)
    .map((line) => line.split("\t")[2]),
);
const changed = git(["diff", "--name-only", "HEAD"])
  .trim()
  .split(/\r?\n/)
  .filter(Boolean);
const untracked = git(["ls-files", "--others", "--exclude-standard"])
  .trim()
  .split(/\r?\n/)
  .filter(Boolean);
assertOwnedChanges([...changed, ...untracked], permitted);
const sourceHash = (path) => existsSync(resolve(source, path))
  ? createHash("sha256").update(readFileSync(resolve(source, path), "utf8").replace(/\r\n/g, "\n")).digest("hex")
  : null;
// Later patches can change the context of earlier ones. Reversing each patch
// independently is not a valid test of an applied stack. Accept only the exact
// final locked state or a clean pinned checkout; never guess through local drift.
const complete = Object.entries(lock.patchedFiles).every(([path, hash]) => sourceHash(path) === hash);
if (complete) {
  console.log("Complete patch stack matches locked source hashes.");
} else {
  if (changed.length || untracked.length) throw new Error("Patched source drift or incomplete patch stack. Preserve changes; use a separate clean checkout to replay the lock.");
  for (const patch of lock.patches) {
    const path = resolve(root, patch);
    // --check must pass before any mutation; never reset developer changes.
    git(["apply", "--check", path]);
    git(["apply", path]);
    console.log(`Applied: ${patch}`);
  }
}
git(["diff", "--check"]);
for (const [path, expected] of Object.entries(lock.patchedFiles)) {
  const actual = sourceHash(path);
  if (actual !== expected)
    throw new Error(
      `Patched source drift: ${path}. Preserve changes; reconcile the lock explicitly.`,
    );
}
if (process.argv[2] === "test") {
  const localGo = resolve(root, `.tools/go${lock.go.version}/go/bin/go.exe`);
  execFileSync(
    existsSync(localGo) ? localGo : "go",
    [
      "test",
      "-p",
      "4",
      "./pkg/instance",
      "./pkg/socketbridge",
      "./pkg/hostswitch",
      "./pkg/controllers/app/app/controllers",
    ],
    {
      cwd: resolve(source, "rdd"),
      stdio: "inherit",
    },
  );
}
console.log(
  `Development baseline verified: ${lock.commit}. Guest coexistence remains unqualified.`,
);
