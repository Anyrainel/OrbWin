import test from "node:test";
import assert from "node:assert/strict";
import {
  ageOf,
  classifyReadError,
  forwardUrls,
  groupByNamespace,
  parseList,
  podHealth,
  deploymentHealth,
} from "../src/components/cluster/model.ts";
import { toYaml } from "../src/components/cluster/yaml.ts";
import {
  openCommand,
  parseDistributions,
} from "../src/components/wsl/parse.ts";

const meta = (name, namespace = "default") => ({ name, namespace, uid: `${name}-uid` });
test("list parsing rejects objects without a UID", () => {
  assert.throws(() => parseList(JSON.stringify({ items: [{ metadata: { name: "a", namespace: "b" } }] })));
  assert.throws(() => parseList("not json"));
  assert.equal(parseList(JSON.stringify({ items: [{ metadata: meta("a") }] })).length, 1);
});
test("disabled cluster is distinguished from other failures", () => {
  assert.equal(classifyReadError("The owned workload cluster has no mirrored kubeconfig. Enable"), "disabled");
  assert.equal(classifyReadError("connection refused"), "failed");
});
test("pod health reflects waiting reasons and readiness", () => {
  const base = { metadata: meta("p"), spec: { containers: [{ name: "c" }] } };
  assert.deepEqual(
    podHealth({ ...base, status: { phase: "Running", containerStatuses: [{ ready: true, state: {} }] } }),
    { label: "Running", tone: "ok" },
  );
  assert.equal(
    podHealth({ ...base, status: { phase: "Running", containerStatuses: [{ ready: false, state: { waiting: { reason: "CrashLoopBackOff" } } }] } }).tone,
    "bad",
  );
  assert.equal(podHealth({ ...base, status: { phase: "Pending" } }).tone, "warn");
  assert.equal(podHealth({ metadata: { ...meta("p"), deletionTimestamp: "x" }, status: { phase: "Running" } }).label, "Terminating");
});
test("deployment health", () => {
  const d = (replicas, ready) => ({ metadata: meta("d"), spec: { replicas }, status: { readyReplicas: ready } });
  assert.equal(deploymentHealth(d(2, 2)).tone, "ok");
  assert.equal(deploymentHealth(d(2, 1)).tone, "warn");
  assert.equal(deploymentHealth(d(0, 0)).tone, "idle");
});
test("age and grouping", () => {
  const now = Date.parse("2026-01-10T00:00:00Z");
  assert.equal(ageOf("2026-01-09T23:59:30Z", now), "30s");
  assert.equal(ageOf("2026-01-09T20:00:00Z", now), "4h");
  assert.equal(ageOf("2025-12-31T00:00:00Z", now), "10d");
  assert.equal(ageOf(undefined, now), "—");
  const groups = groupByNamespace([{ metadata: meta("b", "z") }, { metadata: meta("a", "z") }, { metadata: meta("c", "k") }]);
  assert.deepEqual(groups.map(([ns, l]) => [ns, l.map((o) => o.metadata.name)]), [["k", ["c"]], ["z", ["a", "b"]]]);
});
test("forward output and YAML rendering", () => {
  assert.deepEqual(forwardUrls("Forwarding from 127.0.0.1:5000 -> 80\nForwarding from [::1]:5000 -> 80"), [{ local: 5000, remote: 80 }]);
  assert.equal(
    toYaml({ a: "x", b: ["1", { c: true, d: [] }], e: {}, f: "yes", g: "a: b" }),
    'a: x\nb:\n  - "1"\n  - c: true\n    d: []\ne: {}\nf: "yes"\ng: "a: b"',
  );
});
test("wsl list parsing tolerates UTF-16 residue, localized headers and defaults", () => {
  const raw = "﻿  NAME            STATE           VERSION\r\n* U\u0000buntu          Running         2\r\n  lima-orbwin     Stopped         2\r\n  Debian          Stopped         1\r\n  docker-desktop  Running         2\r\n";
  const { distros, ignored } = parseDistributions(raw);
  assert.deepEqual(distros.map((d) => [d.name, d.state, d.version, d.isDefault, d.role]), [
    ["Ubuntu", "Running", 2, true, "user"],
    ["lima-orbwin", "Stopped", 2, false, "engine"],
    ["Debian", "Stopped", 1, false, "user"],
    ["docker-desktop", "Running", 2, false, "other-engine"],
  ]);
  assert.deepEqual(ignored, []);
  assert.equal(parseDistributions("Das Subsystem ist nicht installiert.").distros.length, 0);
  assert.equal(openCommand("my distro"), 'wsl -d "my distro"');
});
