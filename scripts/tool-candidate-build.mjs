import assert from "node:assert/strict";
import { lstatSync, mkdirSync, realpathSync, readFileSync } from "node:fs";
import { dirname, isAbsolute, relative, resolve, sep } from "node:path";
import { freshOutputPath } from "./build-output.mjs";
import { createHash } from "node:crypto";

// Fixed compile-only recipes. Parse and validate existing inputs before creating
// a candidate; no default selected output or source repair is available.
export function toolCandidateArguments(root, args) {
  const values = new Map();
  for (let i = 0; i < args.length; i += 2) {
    const key = args[i],
      value = args[i + 1];
    assert.ok(
      ["--output-dir", "--source-dir"].includes(key) &&
        !values.has(key) &&
        value &&
        !value.startsWith("--"),
      "Explicit unique tool build selectors required",
    );
    values.set(key, value);
  }
  assert.equal(
    values.size,
    2,
    "Explicit fresh output and existing source required",
  );
  const source = values.get("--source-dir");
  assert.ok(isAbsolute(source), "Absolute existing tool source required");
  const selected = resolve(source);
  for (let p = selected; ; p = dirname(p)) {
    const stat = lstatSync(p);
    assert.ok(
      stat.isDirectory() && !stat.isSymbolicLink(),
      "Redirected tool source refused",
    );
    if (p === dirname(p)) break;
  }
  const exact = (value) =>
    process.platform === "win32" ? value.toLowerCase() : value;
  assert.equal(exact(realpathSync(selected)), exact(selected));
  const rel = relative(resolve(root), selected);
  assert.ok(
    rel && !isAbsolute(rel) && rel !== ".." && !rel.startsWith(`..${sep}`),
    "Tool source must be in this repository",
  );
  const directory = freshOutputPath(root, values.get("--output-dir"));
  const outputInSource = relative(selected, directory);
  assert.ok(
    outputInSource &&
      (isAbsolute(outputInSource) ||
        outputInSource === ".." ||
        outputInSource.startsWith(`..${sep}`)),
    "Fresh output must not modify the source tree",
  );
  return {
    directory,
    source: selected,
    sourceDirectory: rel.replaceAll("\\", "/"),
  };
}

export function createToolCandidate(selected) {
  mkdirSync(selected.directory); // exclusive; caller verified the exact source
  for (const name of ["gocache", "go-tmp"])
    mkdirSync(resolve(selected.directory, name));
  return {
    GOCACHE: resolve(selected.directory, "gocache"),
    GOTMPDIR: resolve(selected.directory, "go-tmp"),
    GOPROXY: "off",
    GOSUMDB: "off",
    GOPRIVATE: "",
    GONOPROXY: "",
    GONOSUMDB: "",
  };
}

export function toolBuildRecipePins(root) {
  return Object.fromEntries(
    ["scripts/tool-candidate-build.mjs", "scripts/build-output.mjs"].map(
      (name) => [
        name,
        createHash("sha256")
          .update(readFileSync(resolve(root, name)))
          .digest("hex"),
      ],
    ),
  );
}

// The source lock selects a candidate; the untrusted manifest never selects its
// own location. No fallback to an older output when an explicit candidate fails.
export function toolCandidateManifest(root, source, kind) {
  assert.ok(["compose", "buildx"].includes(kind));
  const name = source.manifest ?? `artifacts/${kind}/${kind}-manifest.json`;
  assert.ok(
    typeof name === "string" &&
      name.startsWith("artifacts/") &&
      name.endsWith(`/${kind}-manifest.json`) &&
      name
        .split("/")
        .every((p) => /^[A-Za-z0-9_.-]+$/.test(p) && p !== "." && p !== ".."),
    "Closed candidate manifest location required",
  );
  const result = resolve(root, name);
  for (let p = result; ; p = dirname(p)) {
    assert.ok(
      !lstatSync(p).isSymbolicLink(),
      "Redirected candidate manifest refused",
    );
    if (p === dirname(p)) break;
  }
  assert.ok(lstatSync(result).isFile());
  return result;
}
