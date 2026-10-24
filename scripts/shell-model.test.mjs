import test from "node:test";
import assert from "node:assert/strict";
import {
  applyState,
  kubernetesState,
  parseAppSettings,
} from "../src/components/settings/app-settings-model.ts";
import {
  bundleState,
  canStartEngine,
  engineState,
  wslState,
} from "../src/components/shell/engine-state.ts";
import { shortcutCommand } from "../src/components/shell/shortcuts.ts";
import {
  danglingImages,
  parseSize,
  stoppedContainers,
  unattachedVolumes,
} from "../src/components/settings/storage-model.ts";

const app = (spec, status = {}, generation = 3) =>
  JSON.stringify({
    apiVersion: "app.rancherdesktop.io/v1alpha1",
    kind: "App",
    metadata: { name: "app", generation },
    spec,
    status,
  });

test("App settings keep absent fields unknown and tolerate stderr noise", () => {
  const parsed = parseAppSettings(
    `warning: something\n${app({ running: true, kubernetes: { enabled: false } })}\n`,
  );
  assert.equal(parsed.running, true);
  assert.equal(parsed.kubernetesEnabled, false);
  assert.equal(parsed.cpus, undefined);
  assert.equal(kubernetesState(parsed), "disabled");
  assert.equal(applyState(parsed).kind, "unknown");
  assert.equal(
    kubernetesState(parseAppSettings(app({ running: true }))),
    "unknown",
  );
  assert.throws(() => parseAppSettings('{"kind":"Pod"}'));
  assert.throws(() => parseAppSettings("connection refused"));
});

test("Stale or false Settled conditions report a pending apply", () => {
  const settled = (status, observedGeneration) =>
    applyState(
      parseAppSettings(
        app(
          { running: true },
          {
            conditions: [
              {
                type: "Settled",
                status,
                reason: "WaitingForKubernetes",
                observedGeneration,
              },
            ],
          },
        ),
      ),
    );
  assert.equal(settled("True", 3).kind, "settled");
  assert.equal(settled("True", 2).kind, "applying");
  assert.deepEqual(settled("False", 3), {
    kind: "applying",
    reason: "WaitingForKubernetes",
    message: undefined,
  });
});

test("Kubernetes readiness ignores conditions from an older generation", () => {
  const state = (status, observedGeneration) =>
    kubernetesState(
      parseAppSettings(
        app(
          { kubernetes: { enabled: true } },
          {
            conditions: [
              { type: "KubernetesReady", status, observedGeneration },
            ],
          },
        ),
      ),
    );
  assert.equal(state("True", 3), "ready");
  assert.equal(state("False", 3), "starting");
  assert.equal(state("True", 1), "enabled");
});

test("Engine status classification never upgrades unknown failures", () => {
  const status = (diagnostic, available = false) => ({
    available,
    endpoint: "x",
    message: "m",
    diagnostic,
  });
  const offline = status(
    "error during connect: open //./pipe/docker-rancher-desktop-orbwin: The system cannot find the file specified.",
  );
  assert.equal(engineState(offline), "offline");
  assert.equal(bundleState(offline), "ready");
  assert.equal(canStartEngine(offline), true);
  const foreign = status(
    "The endpoint does not match the verified owned Linux engine. No inventory was accepted.",
  );
  assert.equal(engineState(foreign), "foreign");
  assert.equal(canStartEngine(foreign), false);
  const missing = status(
    "Owned backend evidence is unavailable: The system cannot find the path specified. (os error 3)",
  );
  assert.equal(bundleState(missing), "missing");
  assert.equal(canStartEngine(missing), false);
  assert.equal(engineState(status("something new")), "error");
  assert.equal(canStartEngine(status("something new")), false);
  assert.equal(
    canStartEngine(
      status("Bundled guest package pins differ from this application."),
    ),
    false,
  );
  assert.equal(bundleState(status("something new")), "unknown");
  assert.equal(engineState(status(undefined)), "preview");
  assert.equal(canStartEngine(status(undefined)), false);
  assert.equal(engineState(null), "unknown");
  assert.equal(engineState(status(undefined, true)), "running");
  assert.equal(wslState(null), "unknown");
  assert.equal(
    wslState([{ name: "WSL version", output: "", success: false }]),
    "blocked",
  );
});

test("Global shortcuts never fire from a terminal and spare text fields", () => {
  const key = (k, extra = {}) => ({
    key: k,
    ctrlKey: true,
    altKey: false,
    shiftKey: false,
    metaKey: false,
    ...extra,
  });
  const element = (tagName, inTerminal = false) => ({
    tagName,
    isContentEditable: false,
    closest: (selector) =>
      inTerminal && selector.includes(".xterm") ? {} : null,
  });
  const terminal = element("TEXTAREA", true);
  const input = element("INPUT");
  const body = element("BODY");
  assert.equal(shortcutCommand(key("k"), terminal), null);
  assert.equal(shortcutCommand(key("1"), terminal), null);
  assert.deepEqual(shortcutCommand(key("k"), input), { kind: "palette" });
  assert.equal(shortcutCommand(key("1"), input), null);
  assert.equal(shortcutCommand(key(","), input), null);
  assert.deepEqual(shortcutCommand(key("3"), body), { kind: "page", index: 2 });
  assert.deepEqual(shortcutCommand(key(","), body), { kind: "settings" });
  assert.equal(shortcutCommand(key("1", { shiftKey: true }), body), null);
  assert.equal(shortcutCommand(key("1", { ctrlKey: false }), body), null);
  assert.equal(shortcutCommand(key("0"), body), null);
});

test("Cleanup candidates come only from the snapshot and exclude managed items", () => {
  const containers = [
    {
      ID: "a".repeat(64),
      Names: "web",
      State: "running",
      Mounts: "data,cache",
    },
    { ID: "b".repeat(64), Names: "old", State: "exited", Mounts: "logs" },
    {
      ID: "c".repeat(64),
      Names: "k8s_pause",
      State: "exited",
      Labels: "io.kubernetes.pod.uid=1",
    },
  ];
  assert.deepEqual(
    stoppedContainers(containers).map((item) => item.label),
    ["old"],
  );
  assert.deepEqual(
    unattachedVolumes(
      [{ Name: "data" }, { Name: "logs" }, { Name: "orphan" }],
      containers,
    ).map((item) => item.target),
    ["orphan"],
  );
  assert.deepEqual(
    danglingImages([
      { ID: "sha256:" + "d".repeat(64), Repository: "<none>", Tag: "<none>" },
      { ID: "sha256:" + "e".repeat(64), Repository: "redis", Tag: "<none>" },
    ]).length,
    1,
  );
  assert.equal(parseSize("15.2MB"), 15.2e6);
  assert.equal(parseSize("1.5GB"), 1.5e9);
  assert.equal(parseSize("N/A"), undefined);
});
