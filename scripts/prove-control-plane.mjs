// Starts only the management API in a fresh task-owned home. No App or VM is
// created. The rdd-only controller selection cannot launch guest workloads.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { randomUUID, createHash } from "node:crypto";
const root = fileURLToPath(new URL("../", import.meta.url));
const home = resolve(root, `artifacts/control-plane/${randomUUID()}`);
const executable = resolve(root, "artifacts/backend/rdd.exe");
const realHome = process.env.USERPROFILE;
const digest = (path) =>
  existsSync(path)
    ? createHash("sha256").update(readFileSync(path)).digest("hex")
    : null;
const protectedFiles = [".docker/config.json", ".kube/config"].map((path) =>
  resolve(realHome, path),
);
const before = protectedFiles.map(digest);
mkdirSync(home, { recursive: true });
const env = {
  ...process.env,
  USERPROFILE: home,
  HOME: home,
  RDD_INSTANCE: "orbwin-proof",
  RDD_DEVELOPER_MODE: "1",
  DOCKER_CONFIG: resolve(home, "docker"),
  KUBECONFIG: resolve(home, "kube.config"),
};
const evidence = { home, guestCreated: false, steps: [], passed: false };
function run(args, seconds = 100) {
  const start = performance.now();
  try {
    const output = execFileSync(executable, args, {
      env,
      windowsHide: true,
      encoding: "utf8",
      timeout: seconds * 1000,
      maxBuffer: 1024 * 1024,
      stdio: ["ignore", "pipe", "pipe"],
    });
    evidence.steps.push({
      args,
      milliseconds: performance.now() - start,
      output,
    });
    return output;
  } catch (error) {
    evidence.steps.push({
      args,
      milliseconds: performance.now() - start,
      error: error.message,
      stderr: String(error.stderr ?? ""),
    });
    throw error;
  }
}
let started = false;
let failure;
try {
  run(["service", "create", "--controllers=rdd"]);
  // Verify saved arguments before any background process is started.
  const paths = JSON.parse(run(["service", "paths", "-o", "json"]));
  const argumentsFile = JSON.parse(readFileSync(paths.args_file));
  if (!argumentsFile.includes("rdd") || argumentsFile.includes("*"))
    throw new Error("Unexpected management-only controller selection");
  started = true;
  run(["service", "start", "--controllers=rdd", "--timeout=60s"], 70);
  if (run(["ctl", "get", "--raw=/readyz"]).trim() !== "ok")
    throw new Error("Management readiness did not pass");
  run([
    "ctl",
    "get",
    "configmap",
    "rdd-controller-manager",
    "-n",
    "rdd-system",
    "-o",
    "json",
  ]);
  run(["service", "status"]);
  evidence.passed = true;
} catch (error) {
  failure = error;
} finally {
  if (started) {
    try {
      run(["service", "stop", "--timeout=60s"], 70);
    } catch (error) {
      failure ??= error;
      evidence.passed = false;
    }
  }
  evidence.globalClientConfigurationUnchanged = protectedFiles.every(
    (file, index) => digest(file) === before[index],
  );
  if (!evidence.globalClientConfigurationUnchanged) {
    failure ??= new Error("Global client configuration changed");
    evidence.passed = false;
  }
  writeFileSync(
    resolve(home, "evidence.json"),
    JSON.stringify(evidence, null, 2),
  );
}
if (failure) throw failure;
console.log(
  `Management-only proof passed; service stopped. Evidence: ${resolve(home, "evidence.json")}`,
);
