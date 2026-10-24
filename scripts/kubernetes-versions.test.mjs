import test from "node:test";
import assert from "node:assert/strict";
import {
  parseKubernetesVersions,
  versionChangeUnavailable,
} from "../src/components/settings/kubernetes-versions.ts";

test("backend version catalog retains exact releases and numeric ordering", () => {
  const raw = JSON.stringify({
    kind: "ConfigMap",
    metadata: { name: "k3s-versions" },
    data: {
      versions: JSON.stringify({
        "1.32.9": "v1.32.9+k3s1",
        "1.32.11": "v1.32.11+k3s3",
      }),
      channels: JSON.stringify({ stable: "1.32.11" }),
    },
  });
  const result = parseKubernetesVersions(raw);
  assert.equal(result[0].release, "v1.32.11+k3s3");
  assert.deepEqual(result[0].channels, ["stable"]);
  for (const bad of [
    raw.replace("k3s3", "k3s3;command"),
    raw.replace("v1.32.11+", "v1.33.11+"),
    raw.replace("ConfigMap", "App"),
  ])
    assert.throws(() => parseKubernetesVersions(bad));
});
test("version controls reject downgrades, skipped minors and unknown versions", () => {
  assert.equal(versionChangeUnavailable("1.33.9", "1.33.10"), undefined);
  assert.equal(versionChangeUnavailable("1.33.9", "1.34.6"), undefined);
  assert.match(versionChangeUnavailable("1.33.9", "1.33.8"), /Downgrades/);
  assert.match(versionChangeUnavailable("1.33.9", "1.35.3"), /one minor/);
  assert.match(versionChangeUnavailable("1.33.9", "2.0.0"), /one minor/);
  assert.match(versionChangeUnavailable("unknown", "1.34.6"), /verified/);
  assert.match(versionChangeUnavailable("1.33.9", "1.33.9"), /already/);
});
