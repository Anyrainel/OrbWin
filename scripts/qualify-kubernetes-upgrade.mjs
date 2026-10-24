// Explicit, sequential native-UI upgrade fixture. prepare never changes version;
// verify checks retained data after the reviewed native operation. cleanup only
// removes this fixture's verified namespace. No global kube context or reset.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve, basename } from "node:path";
import { createHash, randomUUID } from "node:crypto";
import assert from "node:assert/strict";
import { verifyBuild } from "./build-identity.mjs";

assert.equal(
  process.env.ORBWIN_QUALIFY_OWNED,
  "1",
  "Explicit owned-engine opt-in required",
);
const root = process.cwd();
const identity = JSON.parse(
  readFileSync("artifacts/qualification/runtime-identity.json"),
);
const build = verifyBuild(root);
assert.equal(identity.identityVerified, true);
assert.equal(identity.binarySha256, build.binarySha256);
assert.equal(identity.sourceCommit, build.sourceCommit);
assert.equal(
  identity.endpoint,
  "npipe:////./pipe/docker-rancher-desktop-orbwin",
);
const docker = resolve("artifacts/runtime-tools/bin/docker.exe");
const toolsLock = JSON.parse(readFileSync("runtime-tools.lock.json"));
assert.equal(
  createHash("sha256").update(readFileSync(docker)).digest("hex"),
  toolsLock.tools.find((tool) => tool.path === "bin/docker.exe").sha256,
);
const info = JSON.parse(
  execFileSync(
    docker,
    [
      "--config",
      resolve("artifacts/client/docker"),
      "--host",
      identity.endpoint,
      "info",
      "--format={{json .}}",
    ],
    {
      encoding: "utf8",
      timeout: 30000,
      windowsHide: true,
      env: {
        ...process.env,
        DOCKER_CONTEXT: "",
        DOCKER_TLS_VERIFY: "",
        DOCKER_CERT_PATH: "",
      },
    },
  ),
);
assert.equal(info.ID, identity.daemonId);
assert.equal(info.OSType, "linux");
const home = identity.home;
const env = {
  ...process.env,
  USERPROFILE: home,
  HOME: home,
  RDD_INSTANCE: "orbwin",
  RDD_DEVELOPER_MODE: "1",
};
const rdd = resolve("artifacts/backend/rdd.exe");
const management = resolve(
  home,
  "AppData/Local/rancher-desktop-orbwin/config.yaml",
);
const workload = resolve(home, "AppData/Local/rancher-desktop-orbwin/k3s.yaml");
const mode = process.argv[2];
assert.ok(
  ["prepare", "verify", "cleanup"].includes(mode),
  "Use prepare, verify, or cleanup",
);
const previous = process.argv[3];
if (mode !== "prepare")
  assert.match(previous ?? "", /^kubernetes-upgrade-[a-f0-9-]+\.json$/);
const owner = randomUUID();
const path = resolve(
  "artifacts/qualification",
  mode === "prepare" ? `kubernetes-upgrade-${owner}.json` : basename(previous),
);
const state =
  mode === "prepare"
    ? {
        owner,
        namespace: `orbwin-upgrade-${owner.slice(0, 8)}`,
        identity,
        steps: [],
        targetVersion: process.argv[3],
        success: false,
      }
    : JSON.parse(readFileSync(path));
assert.match(state.namespace, /^orbwin-upgrade-[a-f0-9]{8}$/);
assert.equal(state.identity.daemonId, identity.daemonId);
const previousBackend = state.currentBackendIdentity ?? state.identity;
let backendUpgrade;
if (previousBackend.binarySha256 !== identity.binarySha256) {
  assert.equal(mode, "verify");
  assert.equal(
    process.argv[4],
    "--backend-upgrade",
    "A changed backend requires explicit audited requalification",
  );
  assert.equal(state.verified, true);
  const proof = JSON.parse(
    readFileSync(resolve(identity.home, "evidence.json")),
  );
  assert.equal(proof.passed, true);
  assert.equal(proof.manifest.binarySha256, identity.binarySha256);
  assert.equal(proof.upgradeFrom.binarySha256, previousBackend.binarySha256);
  assert.equal(proof.daemonId, state.identity.daemonId);
  assert.equal(resolve(proof.home), resolve(identity.home));
  backendUpgrade = {
    from: previousBackend.binarySha256,
    to: identity.binarySha256,
    proofRunId: proof.runId,
  };
}
if (state.failure) {
  state.previousFailures ??= [];
  state.previousFailures.push({ message: state.failure, at: state.updated });
  delete state.failure;
}
function kubectl(config, args, input) {
  const start = performance.now();
  try {
    const output = execFileSync(
      rdd,
      ["kubectl", "--kubeconfig", config, "--request-timeout=60s", ...args],
      {
        env,
        input,
        encoding: "utf8",
        timeout: 180000,
        maxBuffer: 2 << 20,
        windowsHide: true,
      },
    );
    state.steps.push({
      args,
      milliseconds: performance.now() - start,
      success: true,
      output,
    });
    return output.trim();
  } catch (error) {
    state.steps.push({
      args,
      milliseconds: performance.now() - start,
      success: false,
      error: String(error),
      stderr: String(error.stderr ?? ""),
    });
    throw error;
  } finally {
    writeFileSync(path, JSON.stringify(state, null, 2));
  }
}
const app = () =>
  JSON.parse(kubectl(management, ["get", "apps/app", "-o", "json"]));
const kube = (args, input) => kubectl(workload, args, input);
const scoped = (args, input) =>
  kube(["--namespace", state.namespace, ...args], input);
function assertNamespace() {
  const namespace = JSON.parse(
    kube(["get", "namespace", state.namespace, "-o", "json"]),
  );
  assert.equal(
    namespace.metadata.labels["org.orbwin.qualification"],
    state.owner,
  );
  if (state.namespaceUid)
    assert.equal(namespace.metadata.uid, state.namespaceUid);
  return namespace;
}
function pod() {
  const list = JSON.parse(
    scoped(["get", "pods", "-l", "app=server", "-o", "json"]),
  );
  assert.equal(list.items.length, 1);
  return list.items[0];
}
try {
  if (mode === "prepare") {
    assert.match(state.targetVersion ?? "", /^1\.\d+\.\d+$/);
    const current = app();
    assert.equal(
      current.spec.kubernetes.enabled,
      true,
      "Enable the owned cluster first",
    );
    for (const kind of ["Settled", "KubernetesReady"])
      assert.ok(
        current.status.conditions.some(
          (c) =>
            c.type === kind &&
            c.status === "True" &&
            c.observedGeneration === current.metadata.generation,
        ),
      );
    state.beforeVersion = current.spec.kubernetes.version;
    state.observedBeforeVersion = JSON.parse(
      kube(["get", "--raw=/version"]),
    ).gitVersion;
    assert.ok(
      state.observedBeforeVersion.startsWith(`v${state.beforeVersion}+k3s`),
      "Desired and observed cluster versions differ before preparation",
    );
    assert.notEqual(state.targetVersion, state.beforeVersion);
    const namespace = {
      apiVersion: "v1",
      kind: "Namespace",
      metadata: {
        name: state.namespace,
        labels: { "org.orbwin.qualification": state.owner },
      },
    };
    kube(["create", "-f", "-"], JSON.stringify(namespace));
    state.namespaceUid = assertNamespace().metadata.uid;
    const manifest = {
      apiVersion: "v1",
      kind: "List",
      items: [
        {
          apiVersion: "v1",
          kind: "PersistentVolumeClaim",
          metadata: { name: "data" },
          spec: {
            accessModes: ["ReadWriteOnce"],
            resources: { requests: { storage: "16Mi" } },
          },
        },
        {
          apiVersion: "apps/v1",
          kind: "Deployment",
          metadata: { name: "server" },
          spec: {
            replicas: 1,
            selector: { matchLabels: { app: "server" } },
            template: {
              metadata: { labels: { app: "server" } },
              spec: {
                containers: [
                  {
                    name: "server",
                    image:
                      "nginx:1.29-alpine@sha256:5616878291a2eed594aee8db4dade5878cf7edcb475e59193904b198d9b830de",
                    ports: [{ containerPort: 80 }],
                    readinessProbe: {
                      tcpSocket: { port: 80 },
                      periodSeconds: 1,
                    },
                    volumeMounts: [
                      { name: "data", mountPath: "/usr/share/nginx/html" },
                    ],
                  },
                ],
                volumes: [
                  {
                    name: "data",
                    persistentVolumeClaim: { claimName: "data" },
                  },
                ],
              },
            },
          },
        },
        {
          apiVersion: "v1",
          kind: "Service",
          metadata: { name: "server" },
          spec: {
            selector: { app: "server" },
            ports: [{ port: 80, targetPort: 80 }],
          },
        },
      ],
    };
    scoped(["apply", "-f", "-"], JSON.stringify(manifest));
    scoped(["rollout", "status", "deployment/server", "--timeout=150s"]);
    const server = pod();
    state.podBefore = server.metadata.uid;
    state.sentinel = `orbwin-persistent-${state.owner}`;
    // Write once before upgrade. The container entrypoint never rewrites this file.
    scoped([
      "exec",
      server.metadata.name,
      "--",
      "sh",
      "-c",
      'printf "%s" "$1" > /usr/share/nginx/html/index.html; sync',
      "sh",
      state.sentinel,
    ]);
    assert.equal(
      scoped([
        "exec",
        server.metadata.name,
        "--",
        "cat",
        "/usr/share/nginx/html/index.html",
      ]),
      state.sentinel,
    );
    state.claimBefore = JSON.parse(scoped(["get", "pvc/data", "-o", "json"]));
    state.prepared = true;
    console.log(
      `Prepared ${state.namespace}; apply ${state.targetVersion} in the native UI, then verify ${basename(path)}.`,
    );
  } else if (mode === "verify") {
    assert.equal(state.prepared, true);
    assertNamespace();
    const current = app();
    assert.equal(current.spec.kubernetes.version, state.targetVersion);
    for (const kind of ["Settled", "KubernetesReady"])
      assert.ok(
        current.status.conditions.some(
          (c) =>
            c.type === kind &&
            c.status === "True" &&
            c.observedGeneration === current.metadata.generation,
        ),
      );
    const serverVersion = JSON.parse(kube(["get", "--raw=/version"]));
    assert.match(
      serverVersion.gitVersion,
      new RegExp(`^v${state.targetVersion.replaceAll(".", "\\.")}\\+k3s`),
    );
    scoped(["rollout", "status", "deployment/server", "--timeout=150s"]);
    const claim = JSON.parse(scoped(["get", "pvc/data", "-o", "json"]));
    assert.equal(claim.metadata.uid, state.claimBefore.metadata.uid);
    assert.equal(claim.spec.volumeName, state.claimBefore.spec.volumeName);
    assert.equal(
      scoped([
        "exec",
        pod().metadata.name,
        "--",
        "cat",
        "/usr/share/nginx/html/index.html",
      ]),
      state.sentinel,
    );
    state.actualVersion = serverVersion.gitVersion;
    // Exercise real admission without risking a data-version rollback if a
    // regression ever accepts the request. Dry-run still invokes admission.
    let rejected = false;
    try {
      kubectl(management, [
        "patch",
        "apps/app",
        "--type=merge",
        "--dry-run=server",
        "--patch",
        JSON.stringify({
          spec: { kubernetes: { version: state.beforeVersion } },
        }),
      ]);
    } catch (error) {
      assert.match(
        String(error.stderr ?? error),
        /downgrades and skipped minor versions are unsupported/,
      );
      rejected = true;
    }
    assert.equal(rejected, true, "Admission accepted a downgrade");
    const afterRejection = app();
    assert.equal(
      afterRejection.metadata.generation,
      current.metadata.generation,
    );
    assert.equal(afterRejection.spec.kubernetes.version, state.targetVersion);
    state.downgradeAdmissionRejected = true;
    state.verified = true;
    if (backendUpgrade) {
      state.backendRequalifications ??= [];
      state.backendRequalifications.push(backendUpgrade);
      state.currentBackendIdentity = identity;
    }
    state.success = true;
    console.log(
      `Upgrade verified: ${state.beforeVersion} -> ${state.actualVersion}; PVC UID, volume and sentinel retained.`,
    );
  } else {
    assertNamespace();
    kube([
      "delete",
      "namespace",
      state.namespace,
      "--wait=true",
      "--timeout=150s",
    ]);
    state.cleaned = true;
    console.log(`Removed only verified fixture namespace ${state.namespace}.`);
  }
} catch (error) {
  state.success = false;
  state.failure = String(error);
  throw error;
} finally {
  state.updated = new Date().toISOString();
  writeFileSync(path, JSON.stringify(state, null, 2));
}
