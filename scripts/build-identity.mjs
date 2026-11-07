import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { verifyRddForwarder } from "./rdd-forwarder-build.mjs";
import { verifyBackendGuestInputs } from "./backend-guest-input.mjs";
import {
  hasLimaBuildInput,
  verifyPortableLimaBuild,
} from "./lima-wsl2-build-input.mjs";
import { verifyPortableWinioBuild } from "./winio-build-input.mjs";
export const sha256 = (bytes) =>
  createHash("sha256").update(bytes).digest("hex");
export function verifyBuild(
  root,
  directory = resolve(root, "artifacts/backend"),
  { licensesDirectory = resolve(directory, "licenses") } = {},
) {
  const lock = JSON.parse(readFileSync(resolve(root, "upstream.lock.json")));
  const build = JSON.parse(
    readFileSync(resolve(directory, "build-manifest.json")),
  );
  if (
    build.guestPackagesLockSha256 !==
    sha256(readFileSync(resolve(root, "guest-packages.lock.json")))
  )
    throw new Error(
      "Backend guest package lock differs; rebuild before testing.",
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
    build.binarySha256 !== sha256(readFileSync(resolve(directory, "rdd.exe")))
  )
    throw new Error("Backend binary differs from its build manifest.");
  if (hasLimaBuildInput(root, build))
    verifyPortableLimaBuild(root, directory, build, { licensesDirectory });
  if(build.winioRuntimeOfferSHA256!==sha256(readFileSync(resolve(root,"winio-runtime-offer.json"))))throw new Error("Runtime source offer fixture differs from build");
  verifyPortableWinioBuild({ root, output: directory, portable: build.portableWinio, licensesDirectory });
  const forwarder = verifyRddForwarder(root, directory);
  if (JSON.stringify(build.forwarder) !== JSON.stringify(forwarder))
    throw new Error("Backend/forwarder build generations differ.");
  if (build.distroEmbedded)
    verifyBackendGuestInputs(root, directory, build, licensesDirectory);
  return build;
}
