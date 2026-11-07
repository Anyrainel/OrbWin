import assert from "node:assert/strict";
import test from "node:test";
import {
  engineProxyConfig,
  engineProxyReviewCurrent,
  readEngineProxySnapshot,
} from "../src/model/engine-proxy.ts";

function app() {
  const plan = {
    appUID: "app-owned",
    observedGeneration: 8,
    proxyRevision: 2,
    configDigest: "a".repeat(64),
    binding: {
      limaVMUID: "vm-owned",
      hostPID: 123,
      hostCreationTime: "a".repeat(16),
      transportEpoch: "b".repeat(32),
    },
    phase: "commit",
  };
  return {
    kind: "App",
    metadata: {
      name: "app",
      uid: "app-owned",
      resourceVersion: "42",
      generation: 9,
    },
    spec: {
      running: true,
      networkProxy: {
        version: 1,
        mode: "url",
        url: "https://proxy.example:8443",
        noProxy: [".example.test"],
      },
    },
    status: {
      conditions: [
        {
          type: "NetworkProxyReady",
          status: "True",
          reason: "Ready",
          observedGeneration: 9,
        },
      ],
      networkProxy: {
        appUID: "app-owned",
        observedGeneration: 9,
        proxyRevision: 2,
        configDigest: "a".repeat(64),
        mode: "url",
        managedEver: true,
        guestAppliedRevision: 2,
        plan,
      },
      networkProxyHost: {
        plan: structuredClone(plan),
        phase: "active",
        reason: "Ready",
      },
    },
  };
}

test("engine proxy readiness requires the current condition and exact guest/host permission", () => {
  assert.equal(readEngineProxySnapshot(app()).stage, "ready");
  for (const change of [
    (value) => (value.status.conditions[0].observedGeneration = 8),
    (value) => (value.status.networkProxy.guestAppliedRevision = 1),
    (value) =>
      (value.status.networkProxyHost.plan.binding.transportEpoch = "c".repeat(
        32,
      )),
    (value) => (value.status.networkProxyHost.plan.proxyRevision = 1),
    (value) => (value.status.networkProxyHost.phase = "prepared"),
    (value) => (value.status.networkProxyHost.reason = "RetirementUnknown"),
    (value) => (value.status.networkProxy.mode = "auto"),
    (value) => delete value.status.networkProxy.plan,
  ]) {
    const value = app();
    change(value);
    assert.notEqual(readEngineProxySnapshot(value).stage, "ready");
  }
  const value = app();
  value.metadata.generation = 10;
  value.status.conditions[0].observedGeneration = 10;
  value.status.networkProxy.observedGeneration = 10;
  assert.equal(
    readEngineProxySnapshot(value).stage,
    "ready",
    "unrelated App generation does not invalidate unchanged permission",
  );
});

test("fresh None is direct only with an explicit current untouched-policy observation", () => {
  const value = app();
  value.spec.networkProxy = { version: 1, mode: "none", url: "", noProxy: [] };
  Object.assign(value.status.networkProxy, {
    mode: "none",
    managedEver: false,
    guestAppliedRevision: 0,
    plan: null,
  });
  delete value.status.networkProxyHost;
  assert.equal(readEngineProxySnapshot(value).stage, "ready");
  delete value.spec.networkProxy;
  delete value.status.networkProxy;
  value.status.conditions = [];
  assert.equal(readEngineProxySnapshot(value).stage, "unmanaged");
  value.status.conditions = [
    {
      type: "NetworkProxyReady",
      status: "False",
      reason: "Unavailable",
      observedGeneration: 9,
    },
  ];
  assert.equal(readEngineProxySnapshot(value).stage, "unavailable");
});

test("proxy review is invalidated by exact App version change, not a guessed mode", () => {
  const snapshot = readEngineProxySnapshot(app());
  const review = {
    uid: snapshot.uid,
    resourceVersion: snapshot.resourceVersion,
    generation: snapshot.generation,
    config: engineProxyConfig("auto", "", ""),
  };
  assert.equal(engineProxyReviewCurrent(review, snapshot), true);
  for (const change of [
    { uid: "other" },
    { resourceVersion: "43" },
    { generation: 10 },
    { running: false },
  ])
    assert.equal(
      engineProxyReviewCurrent(review, { ...snapshot, ...change }),
      false,
    );
  assert.equal(engineProxyReviewCurrent(review, null), false);
});

test("proxy inputs preserve explicit canonical scope and refuse credentials and malformed bypass", () => {
  assert.deepEqual(
    engineProxyConfig(
      "url",
      "http://proxy.example:8080",
      ".example.test,10.0.0.0/8\n2001:db8::/32",
    ),
    {
      version: 1,
      mode: "url",
      url: "http://proxy.example:8080",
      noProxy: [".example.test", "10.0.0.0/8", "2001:db8::/32"],
    },
  );
  assert.equal(
    engineProxyConfig("url", "http://[::1]:8080", "").url,
    "http://[::1]:8080",
  );
  for (const endpoint of [
    "http://user:secret@proxy",
    "http://@proxy",
    "http://proxy/path",
    "http://proxy?x",
    "http://proxy#x",
    "http://proxy:080",
    "http://proxy\\foreign",
    "http://Proxy.EXAMPLE",
  ])
    assert.throws(() => engineProxyConfig("url", endpoint, ""));
  for (const bypass of [
    "10.0.0.1/8",
    "10.0.0.0/08",
    "2001:db8::1/32",
    "example:0",
    "../other",
    "example,example",
  ])
    assert.throws(() => engineProxyConfig("none", "", bypass));
  const value = app();
  value.spec.networkProxy.extra = "unsupported";
  assert.throws(() => readEngineProxySnapshot(value));
  value.metadata.uid = "";
  assert.throws(() => readEngineProxySnapshot(value));
});
