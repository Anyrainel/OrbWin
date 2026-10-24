import test from "node:test";
import assert from "node:assert/strict";
import { isKubernetesContainer } from "../src/model/resources.ts";
test("Kubernetes display filtering requires returned ownership labels", () => {
  assert.equal(
    isKubernetesContainer({
      Names: "k8s_user-container",
      Labels: "com.docker.compose.project=dev",
    }),
    false,
  );
  assert.equal(
    isKubernetesContainer({
      Labels: "other=value,io.kubernetes.pod.uid=123,more=value",
    }),
    true,
  );
  assert.equal(
    isKubernetesContainer({
      Labels: { "io.kubernetes.pod.uid": "123", future: true },
    }),
    true,
  );
  assert.equal(
    isKubernetesContainer({ Labels: "not.io.kubernetes.pod.uid=123" }),
    false,
  );
});
