import test from "node:test";
import assert from "node:assert/strict";
import { kubernetesBuildVersion } from "./kubernetes-build-version.mjs";
test("kubectl build metadata reflects effective pinned modules, including replacements", () => {
  const result = kubernetesBuildVersion({
    Path: "k8s.io/component-base",
    Version: "v0.37.0",
    Replace: { Version: "v0.36.4" },
  });
  assert.equal(result.version, "v1.36.4");
  assert.equal(result.sourceVersion, "v0.36.4");
  assert.equal(kubernetesBuildVersion({ Version: "v0.35.3" }).minor, "35");
  for (const Version of ["v1.0.0", "v0.36.4;command", "unknown"])
    assert.throws(() => kubernetesBuildVersion({ Version }));
});
