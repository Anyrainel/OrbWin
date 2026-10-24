import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
export const sha256 = (bytes) =>
  createHash("sha256").update(bytes).digest("hex");
export function verifyBuild(root) {
  const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
  const build = JSON.parse(
    readFileSync(resolve(root, "artifacts/backend/build-manifest.json")),
  );
  if (
    build.sourceCommit !== lock.commit ||
    build.patches.length !== lock.patches.length
  )
    throw new Error(
      "Stale backend build; rebuild from the current source lock.",
    );
  for (let index = 0; index < lock.patches.length; index++) {
    const path = lock.patches[index];
    if (
      build.patches[index].path !== path ||
      build.patches[index].sha256 !== sha256(readFileSync(resolve(root, path)))
    )
      throw new Error(
        `Backend build differs from patch ${path}; rebuild before testing.`,
      );
  }
  if (
    build.binarySha256 !==
    sha256(readFileSync(resolve(root, "artifacts/backend/rdd.exe")))
  )
    throw new Error("Backend binary differs from its build manifest.");
  return build;
}
