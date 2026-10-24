// Explicit sequential proof guest only; never selects a global kube context.
import { execFileSync, spawn } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import assert from "node:assert/strict";
import { verifyBuild } from "./build-identity.mjs";
if (process.env.ORBWIN_QUALIFY_OWNED !== "1")
  throw new Error("Owned-engine opt-in required");
const identity = JSON.parse(readFileSync("artifacts/setup-proof/runtime.json"));
assert.equal(identity.binarySha256, verifyBuild(process.cwd()).binarySha256);
assert.equal(
  identity.endpoint,
  "npipe:////./pipe/docker-rancher-desktop-orbwin-proof",
);
const kubeconfig = resolve(
  identity.home,
  "AppData/Local/rancher-desktop-orbwin-proof/k3s.yaml",
);
const env = {
  ...process.env,
  USERPROFILE: identity.home,
  HOME: identity.home,
  RDD_INSTANCE: "orbwin-proof",
  RDD_DEVELOPER_MODE: "1",
  KUBECONFIG: kubeconfig,
};
const namespace = `orbwin-terminal-${Date.now()}`;
const checks = [];
function kubectl(args, input) {
  return execFileSync(
    resolve("artifacts/backend/rdd.exe"),
    ["kubectl", "--kubeconfig", kubeconfig, "--request-timeout=60s", ...args],
    {
      env,
      input,
      encoding: "utf8",
      timeout: 90000,
      maxBuffer: 2 << 20,
      windowsHide: true,
    },
  ).trim();
}
async function session(uid, exercise) {
  const child = spawn(resolve("artifacts/backend/orbwin-session.exe"), [], {
    windowsHide: true,
    stdio: "pipe",
  });
  let stdout = "",
    stderr = "";
  child.stdout.on("data", (chunk) => {
    stdout = (stdout + chunk).slice(-131072);
  });
  child.stderr.on("data", (chunk) => {
    stderr = (stderr + chunk).slice(-131072);
  });
  child.stdin.on("error", () => {});
  const send = (message) => child.stdin.write(JSON.stringify(message) + "\n");
  const done = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code) => resolve(code));
  });
  const timer = setTimeout(() => child.kill(), 45000);
  async function waitFor(test) {
    const deadline = Date.now() + 15000;
    while (!test(stdout, stderr)) {
      if (child.exitCode !== null || Date.now() > deadline)
        throw new Error(`Session wait failed: ${stderr}\n${stdout}`);
      await new Promise((resolve) => setTimeout(resolve, 40));
    }
  }
  try {
    send({
      kind: "open",
      mode: "kubernetes",
      namespace,
      pod: "shell",
      podUid: uid,
      containerName: "shell",
      kubeconfig,
      shell: "sh",
      cols: 80,
      rows: 24,
    });
    await exercise({ send, waitFor });
    return { code: await done, stdout, stderr };
  } finally {
    clearTimeout(timer);
    if (child.exitCode === null) child.kill();
  }
}
let created = false,
  failure;
try {
  kubectl(["create", "namespace", namespace]);
  created = true;
  kubectl(
    ["apply", "-f", "-"],
    JSON.stringify({
      apiVersion: "v1",
      kind: "Pod",
      metadata: { name: "shell", namespace },
      spec: {
        containers: [
          {
            name: "shell",
            image:
              "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30",
            command: [
              "sh",
              "-c",
              "while true; do echo proof-log; sleep 2; done",
            ],
          },
        ],
      },
    }),
  );
  kubectl([
    "wait",
    "--namespace",
    namespace,
    "pod/shell",
    "--for=condition=Ready",
    "--timeout=60s",
  ]);
  const pod = JSON.parse(
    kubectl(["get", "pod/shell", "--namespace", namespace, "-o", "json"]),
  );
  const stale = await session("replaced-uid", async () => {});
  assert.equal(stale.code, 125);
  assert.match(stale.stderr, /pod was replaced/);
  checks.push("stale UID rejected");
  const live = await session(pod.metadata.uid, async ({ send, waitFor }) => {
    await waitFor((_, error) => error.includes("ORBWIN_SESSION_READY"));
    send({ kind: "resize", cols: 100, rows: 30 });
    // Avoid a race between independent input/resize streams.
    await new Promise((resolve) => setTimeout(resolve, 200));
    send({
      kind: "input",
      data: [...Buffer.from("stty size; printf 'terminal-%s\\n' verified\n")],
    });
    await waitFor(
      (output) =>
        output.includes("30 100") && output.includes("terminal-verified"),
    );
    send({ kind: "input", data: [...Buffer.from("sleep 30\n")] });
    await new Promise((resolve) => setTimeout(resolve, 300));
    send({ kind: "input", data: [3] });
    send({
      kind: "input",
      data: [...Buffer.from("printf 'after-%s\\n' interrupt; exit 7\n")],
    });
    await waitFor((output) => output.includes("after-interrupt"));
  });
  assert.equal(live.code, 7);
  checks.push("TTY input, resize, Ctrl-C and remote exit status");
  const cancelled = await session(
    pod.metadata.uid,
    async ({ send, waitFor }) => {
      await waitFor((_, error) => error.includes("ORBWIN_SESSION_READY"));
      send({ kind: "close" });
    },
  );
  assert.equal(cancelled.code, 125);
  const after = JSON.parse(
    kubectl(["get", "pod/shell", "--namespace", namespace, "-o", "json"]),
  );
  assert.equal(after.metadata.uid, pod.metadata.uid);
  assert.equal(after.status.phase, "Running");
  checks.push("closing terminal preserves running pod");
  assert.match(
    kubectl(["logs", "shell", "--namespace", namespace, "--tail=5"]),
    /proof-log/,
  );
  checks.push("real pod log output");
} catch (error) {
  failure = String(error);
} finally {
  if (created) {
    try {
      kubectl([
        "delete",
        "namespace",
        namespace,
        "--wait=true",
        "--timeout=60s",
      ]);
      checks.push("scoped cleanup");
    } catch (error) {
      failure = `${failure ?? ""}\nCleanup: ${error}`;
    }
  }
  writeFileSync(
    "artifacts/setup-proof/pod-terminal.json",
    JSON.stringify(
      { time: new Date().toISOString(), checks, success: !failure, failure },
      null,
      2,
    ),
  );
}
if (failure) throw new Error(failure);
console.log(checks.join("\n"));
