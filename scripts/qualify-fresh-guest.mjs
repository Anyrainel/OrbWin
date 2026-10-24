// Sequential qualification of the task-created first-run fixture only.
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { randomUUID } from "node:crypto";
import { verifyBuild } from "./build-identity.mjs";

assert.equal(process.env.ORBWIN_QUALIFY_OWNED, "1");
const identity = JSON.parse(readFileSync("artifacts/setup-proof/runtime.json"));
const setup = JSON.parse(readFileSync("artifacts/setup-proof/result.json"));
assert.equal(
  setup.hadGuest,
  false,
  "This check requires the fresh setup record",
);
assert.equal(setup.setup.Ok, true);
assert.equal(identity.binarySha256, verifyBuild(process.cwd()).binarySha256);
assert.equal(
  identity.endpoint,
  "npipe:////./pipe/docker-rancher-desktop-orbwin-proof",
);
const running = execFileSync("wsl.exe", ["--list", "--running", "--quiet"]);
assert(
  !running
    .toString("utf16le")
    .split(/\r?\n/)
    .some((line) => line.trim() === "lima-orbwin"),
  "Product guest must already be stopped",
);
const run = randomUUID();
const namespace = `orbwin-fresh-${run.slice(0, 8)}`;
const marker = `orbwin-fresh-forward-${run}`;
const rdd = resolve("artifacts/backend/rdd.exe");
const management = resolve(
  identity.home,
  "AppData/Local/rancher-desktop-orbwin-proof/config.yaml",
);
const workload = resolve(
  identity.home,
  "AppData/Local/rancher-desktop-orbwin-proof/k3s.yaml",
);
const env = {
  ...process.env,
  USERPROFILE: identity.home,
  HOME: identity.home,
  RDD_INSTANCE: "orbwin-proof",
  RDD_DEVELOPER_MODE: "1",
  DOCKER_CONTEXT: "",
  DOCKER_HOST: identity.endpoint,
};
const evidence = {
  run,
  namespace,
  marker,
  identity,
  checks: [],
  passed: false,
};
function cli(args, input, seconds = 90) {
  return execFileSync(rdd, args, {
    env,
    input,
    encoding: "utf8",
    timeout: seconds * 1000,
    maxBuffer: 2 << 20,
    windowsHide: true,
  }).trim();
}
const kube = (args, input) =>
  cli(
    ["kubectl", "--kubeconfig", workload, "--request-timeout=60s", ...args],
    input,
    120,
  );
async function waitApi() {
  const deadline = Date.now() + 180000;
  for (;;) {
    try {
      assert.equal(
        cli(
          [
            "kubectl",
            "--kubeconfig",
            workload,
            "--request-timeout=5s",
            "get",
            "--raw=/readyz",
          ],
          undefined,
          10,
        ),
        "ok",
      );
      return;
    } catch (error) {
      const detail = String(error) + String(error.stderr ?? "");
      if (
        Date.now() >= deadline ||
        !/connectex|connection refused|deadline exceeded|ServiceUnavailable|InternalError|not ready/i.test(
          detail,
        )
      )
        throw error;
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
  }
}
async function waitWorkload() {
  const deadline = Date.now() + 120000;
  for (;;) {
    try {
      assert.equal(
        kube([
          "-n",
          namespace,
          "exec",
          "pod/server",
          "--",
          "cat",
          "/usr/share/nginx/html/index.html",
        ]),
        marker,
      );
      return;
    } catch (error) {
      const detail = String(error) + String(error.stderr ?? "");
      if (
        Date.now() >= deadline ||
        !/pod not found|pod does not exist|container not found|container .*not running|connectex|connection refused|NotFound|being terminated/i.test(
          detail,
        )
      )
        throw error;
      evidence.runtimeReadinessRetries =
        (evidence.runtimeReadinessRetries ?? 0) + 1;
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
  }
}
function packages() {
  return execFileSync(
    "wsl.exe",
    [
      "-d",
      "lima-orbwin-proof",
      "-u",
      "root",
      "--",
      "sh",
      "-c",
      "rpm -q socat libwrap0 && rpm -V socat libwrap0 && socat -V | head -n 2",
    ],
    { encoding: "utf8", timeout: 30000, windowsHide: true },
  ).trim();
}
async function forward(resource) {
  const child = spawn(
    rdd,
    [
      "kubectl",
      "--kubeconfig",
      workload,
      "-n",
      namespace,
      "port-forward",
      resource,
      "0:80",
      "--address=127.0.0.1",
    ],
    { env, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] },
  );
  let output = "";
  const collect = (bytes) => {
    output = (output + bytes).slice(-32768);
  };
  child.stdout.on("data", collect);
  child.stderr.on("data", collect);
  const done = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", resolve);
  });
  try {
    const deadline = Date.now() + 30000;
    while (!/Forwarding from 127\.0\.0\.1:(\d+)/.test(output)) {
      if (child.exitCode !== null || Date.now() > deadline)
        throw new Error(`Forward did not become ready: ${output}`);
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    const port = Number(output.match(/Forwarding from 127\.0\.0\.1:(\d+)/)[1]);
    const response = await fetch(`http://127.0.0.1:${port}/`, {
      signal: AbortSignal.timeout(10000),
    });
    assert.equal(response.status, 200);
    assert.equal(await response.text(), marker);
    evidence.checks.push({ resource, port, httpMarkerMatched: true });
  } finally {
    child.kill();
    await done;
  }
}
let namespaceUid;
try {
  console.log("Starting only the fresh proof guest…");
  cli(
    ["service", "start", "--controllers=*,-path-management", "--timeout=90s"],
    undefined,
    110,
  );
  console.log(
    "Enabling the fresh workload cluster and offline guest dependencies…",
  );
  cli(
    [
      "set",
      "kubernetes.enabled=true",
      "kubernetes.version=1.35.3",
      "--timeout=840s",
    ],
    undefined,
    900,
  );
  cli(
    [
      "ctl",
      "wait-condition",
      "limavms/orbwin-proof",
      "Running=True",
      "--reason=Started",
      "--since=startup",
      "--namespace=rancher-desktop",
      "--timeout=180s",
    ],
    undefined,
    200,
  );
  await waitApi();
  evidence.packagesBeforeRestart = packages();
  assert(evidence.packagesBeforeRestart.includes("socat-1.8.0.2-160000.2.3"));
  assert(evidence.packagesBeforeRestart.includes("libwrap0-7.6-160000.2.2"));
  const ns = JSON.parse(
    kube(
      ["create", "-f", "-", "-o", "json"],
      JSON.stringify({
        apiVersion: "v1",
        kind: "Namespace",
        metadata: {
          name: namespace,
          labels: { "org.orbwin.qualification": run },
        },
      }),
    ),
  );
  namespaceUid = ns.metadata.uid;
  evidence.namespaceUid = namespaceUid;
  const pod = {
    apiVersion: "v1",
    kind: "Pod",
    metadata: { name: "server", namespace, labels: { app: "server" } },
    spec: {
      containers: [
        {
          name: "server",
          image:
            "nginx:1.29-alpine@sha256:5616878291a2eed594aee8db4dade5878cf7edcb475e59193904b198d9b830de",
          command: [
            "sh",
            "-c",
            `printf %s '${marker}' > /usr/share/nginx/html/index.html; exec nginx -g 'daemon off;'`,
          ],
          ports: [{ containerPort: 80 }],
        },
      ],
    },
  };
  const service = {
    apiVersion: "v1",
    kind: "Service",
    metadata: { name: "server", namespace },
    spec: {
      selector: { app: "server" },
      ports: [{ port: 80, targetPort: 80 }],
    },
  };
  kube(
    ["create", "-f", "-"],
    JSON.stringify({ apiVersion: "v1", kind: "List", items: [pod, service] }),
  );
  cli(
    [
      "kubectl",
      "--kubeconfig",
      workload,
      "-n",
      namespace,
      "wait",
      "pod/server",
      "--for=condition=Ready",
      "--timeout=180s",
    ],
    undefined,
    200,
  );
  await waitWorkload();
  await forward("pod/server");
  await forward("service/server");
  console.log(
    "Fresh pod/service HTTP forwarding passed; checking an idempotent restart…",
  );
  cli(["service", "stop", "--timeout=60s"], undefined, 75);
  cli(
    ["service", "start", "--controllers=*,-path-management", "--timeout=90s"],
    undefined,
    110,
  );
  // A persisted Ready condition can precede the current boot's API. Require
  // the existing upstream current-start condition, then an actual readyz read.
  cli(
    [
      "ctl",
      "wait-condition",
      "limavms/orbwin-proof",
      "Running=True",
      "--reason=Started",
      "--since=startup",
      "--namespace=rancher-desktop",
      "--timeout=180s",
    ],
    undefined,
    200,
  );
  await waitApi();
  evidence.packagesAfterRestart = packages();
  assert.equal(evidence.packagesAfterRestart, evidence.packagesBeforeRestart);
  cli(
    [
      "kubectl",
      "--kubeconfig",
      workload,
      "-n",
      namespace,
      "wait",
      "pod/server",
      "--for=condition=Ready",
      "--timeout=180s",
    ],
    undefined,
    200,
  );
  await waitWorkload();
  await forward("service/server");
  evidence.passed = true;
} catch (error) {
  evidence.error = String(error);
} finally {
  if (namespaceUid) {
    try {
      const current = JSON.parse(
        kube(["get", "namespace", namespace, "-o", "json"]),
      );
      assert.equal(current.metadata.uid, namespaceUid);
      assert.equal(current.metadata.labels["org.orbwin.qualification"], run);
      kube(["delete", "namespace", namespace, "--wait=true", "--timeout=60s"]);
      evidence.namespaceCleanup = true;
    } catch (error) {
      evidence.cleanupError = String(error);
      evidence.passed = false;
    }
  }
  try {
    cli(["service", "stop", "--timeout=60s"], undefined, 75);
    evidence.proofStopped = true;
  } catch (error) {
    evidence.stopError = String(error);
    evidence.passed = false;
  }
  const file = resolve(`artifacts/qualification/fresh-guest-${run}.json`);
  writeFileSync(file, JSON.stringify(evidence, null, 2));
  console.log(`Evidence: ${file}`);
}
assert.equal(
  evidence.passed,
  true,
  evidence.error ?? evidence.cleanupError ?? evidence.stopError,
);
