import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync, lstatSync } from "node:fs";
import { resolve, join, dirname, isAbsolute } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");

// Compilation only: no helper executable is built or started here. The caller
// must copy this object into its frozen helper build and attest both XML/RC
// sources and compiler output. Never write a generated .syso into live source.
export function compileLanHelperResource(root, outputDirectory) {
  assert.ok(isAbsolute(root) && isAbsolute(outputDirectory));
  const sourceDirectory = join(root, "backend-tools/session/lan-helper");
  const inputs = Object.fromEntries(
    ["lan.manifest", "lan.rc"].map((name) => {
      const path = join(sourceDirectory, name),
        stat = lstatSync(path);
      assert.ok(
        stat.isFile() &&
          !stat.isSymbolicLink() &&
          stat.size > 0 &&
          stat.size <= 8192,
      );
      return [name, readFileSync(path)];
    }),
  );
  mkdirSync(outputDirectory); // A fresh caller-owned artifact directory only.
  for (const [name, bytes] of Object.entries(inputs))
    writeFileSync(join(outputDirectory, name), bytes);
  const compiler = join(root, ".tools/mingw/ucrt64/bin/windres.exe");
  const output = join(outputDirectory, "lan_windows_amd64.syso");
  const args = [
    "--target=pe-x86-64",
    "--language=0x409",
    "--input-format=rc",
    "--output-format=coff",
    "--input=lan.rc",
    "--output=lan_windows_amd64.syso",
  ];
  const result = spawnSync(compiler, args, {
    cwd: outputDirectory,
    windowsHide: true,
    encoding: "utf8",
    timeout: 30000,
    env: { ...process.env, PATH: dirname(compiler) + ";" + process.env.PATH },
  });
  writeFileSync(
    join(outputDirectory, "compile.stdout.txt"),
    result.stdout ?? "",
  );
  writeFileSync(
    join(outputDirectory, "compile.stderr.txt"),
    result.stderr ?? "",
  );
  assert.equal(result.status, 0, "LAN helper resource compilation failed");
  const bytes = readFileSync(output);
  assert.ok(
    bytes.length >= 60 && bytes.readUInt16LE(0) === 0x8664,
    "Expected amd64 COFF resource",
  );
  const sections = bytes.readUInt16LE(2),
    optional = bytes.readUInt16LE(16);
  assert.ok(
    sections > 0 &&
      sections <= 16 &&
      optional === 0 &&
      20 + sections * 40 <= bytes.length,
  );
  const names = Array.from({ length: sections }, (_, i) =>
    bytes
      .subarray(20 + 40 * i, 28 + 40 * i)
      .toString("ascii")
      .replace(/\0.*$/, ""),
  );
  assert.ok(names.includes(".rsrc"), "COFF output lacks resources");
  assert.ok(
    bytes.includes(inputs["lan.manifest"]),
    "Compiled resource differs from exact manifest bytes",
  );
  for (const [name, before] of Object.entries(inputs))
    assert.ok(
      readFileSync(join(sourceDirectory, name)).equals(before),
      "Resource source changed during compilation",
    );
  const proof = {
    schema: 1,
    scope:
      "amd64 resource compilation only; no helper executable, UI, native dialog, elevation or engine action",
    compilerSha256: hash(readFileSync(compiler)),
    args,
    sources: Object.fromEntries(
      Object.entries(inputs).map(([name, data]) => [name, hash(data)]),
    ),
    objectSha256: hash(bytes),
    machine: "amd64",
    sections: names,
    success: true,
  };
  writeFileSync(
    join(outputDirectory, "proof.json"),
    JSON.stringify(proof, null, 2),
  );
  return { output, proof };
}

if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  assert.equal(
    process.argv.length,
    3,
    "Pass a new absolute artifact output directory",
  );
  console.log(
    JSON.stringify(
      compileLanHelperResource(
        resolve(import.meta.dirname, ".."),
        process.argv[2],
      ),
    ),
  );
}
