import { existsSync, mkdirSync, realpathSync } from "node:fs";
import { dirname, isAbsolute, relative, resolve, sep } from "node:path";

// An explicit candidate output is CREATE_NEW, under the real artifacts tree.
// It must never silently replace packaged/mapped development executables.
export function buildArguments(args, allowedFlags = []) {
  let requested;
  const flags = new Set();
  for (let index = 0; index < args.length; index++) {
    const argument = args[index];
    if (argument === "--output-dir" && requested === undefined) {
      requested = args[++index];
      if (!requested || requested.startsWith("--")) {
        throw new Error("--output-dir requires a fresh artifact directory");
      }
    } else if (allowedFlags.includes(argument) && !flags.has(argument)) {
      flags.add(argument);
    } else {
      throw new Error("Unknown or repeated build option");
    }
  }
  if (requested === undefined)
    throw new Error("An explicit fresh --output-dir is required");
  return { requested, flags };
}
export function buildTestArguments(buildOnly) {
  return buildOnly ? [] : [["test", "./..."]];
}
export function freshOutputPath(root, requested) {
  const artifactRoot = realpathSync(resolve(root, "artifacts"));
  const repositoryRoot = realpathSync(root);
  const artifactLocation = relative(repositoryRoot, artifactRoot);
  if (
    (process.platform === "win32"
      ? artifactLocation.toLowerCase()
      : artifactLocation) !== "artifacts"
  )
    throw new Error(
      "Repository artifacts must not redirect to another location",
    );
  const directory = resolve(root, requested);
  // Parent must already exist; realpath prevents a junction/symlink redirect.
  const parent = realpathSync(dirname(directory));
  const suffix = relative(artifactRoot, parent);
  if (isAbsolute(suffix) || suffix === ".." || suffix.startsWith(`..${sep}`)) {
    throw new Error(
      "Candidate output must be inside this repository's artifacts",
    );
  }
  const comparable = (value) =>
    process.platform === "win32" ? value.toLowerCase() : value;
  if (comparable(parent) !== comparable(dirname(directory))) {
    throw new Error("Candidate output parent must not redirect");
  }
  if (existsSync(directory)) {
    throw new Error(
      "Candidate output already exists; choose a fresh directory",
    );
  }
  return directory;
}
export function buildOutput(root, args, allowedFlags = []) {
  const { requested, flags } = buildArguments(args, allowedFlags);
  const directory = freshOutputPath(root, requested);
  mkdirSync(directory); // non-recursive/exclusive: a concurrent create refuses.
  return { directory, flags };
}
