import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  cpSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import {
  prepareColdPayloadInput,
  verifyColdPayloadArtifact,
  verifyColdPayloadInput,
} from "./session-cold-payload-input.mjs";

function fixture(t) {
  const root = realpathSync(
    mkdtempSync(join(tmpdir(), "orbwin-cold-payload-")),
  );
  t.after(() => {
    assert.equal(realpathSync(root), root);
    rmSync(root, { recursive: true });
  });
  const put = (name, bytes) => {
    const p = join(root, name);
    mkdirSync(dirname(p), { recursive: true });
    writeFileSync(p, bytes);
  };
  put("scripts/session-cold-payload-input.mjs", "fixed generator fixture");
  put(
    "backend-tools/session/internal/coldassets/compiled.go",
    "package coldassets\n",
  );
  const elf = Buffer.alloc(64);
  Buffer.from([0x7f, 69, 76, 70, 2, 1, 1]).copy(elf);
  elf.writeUInt16LE(62, 18);
  put("output/orbwin-services", elf);
  put("output/orbwin-trust", elf);
  put(
    "output/orbwin-session.exe",
    "synthetic manager " +
      elf.toString("base64") +
      createHash("sha256").update(elf).digest("hex"),
  );
  return { root, output: join(root, "output"), put };
}
test("cold overlay embeds exact fixed inputs without mutating source and refuses replay", (t) => {
  const { root, output } = fixture(t);
  const before = readFileSync(
    join(root, "backend-tools/session/internal/coldassets/compiled.go"),
  );
  const p = prepareColdPayloadInput(root, output);
  assert.equal(verifyColdPayloadInput(root, output, p.manifest).schema, 1);
  assert.ok(
    readFileSync(
      join(root, "backend-tools/session/internal/coldassets/compiled.go"),
    ).equals(before),
  );
  assert.throws(() => prepareColdPayloadInput(root, output));
});
test("changed embedded bytes, overlay map, recipe and ELF role are refused", (t) => {
  for (const name of [
    "output/orbwin-services",
    "output/orbwin-session.exe",
    "output/.cold-build-input/compiled.go",
    "output/.cold-build-input/overlay.json",
    "scripts/session-cold-payload-input.mjs",
    "backend-tools/session/internal/coldassets/compiled.go",
  ]) {
    const f = fixture(t);
    const p = prepareColdPayloadInput(f.root, f.output);
    f.put(name, "changed");
    assert.throws(
      () => verifyColdPayloadInput(f.root, f.output, p.manifest),
      name,
    );
  }
});

test("artifact verification relocates while build verification requires its original overlay", (t) => {
  const { root, output } = fixture(t);
  const p = prepareColdPayloadInput(root, output);
  const relocated = join(root, "relocated");
  cpSync(output, relocated, { recursive: true });
  assert.deepEqual(
    verifyColdPayloadArtifact(root, relocated, p.manifest),
    p.manifest,
  );
  assert.throws(
    () => verifyColdPayloadInput(root, relocated, p.manifest),
    /overlay mapping changed/,
  );
  rmSync(join(relocated, ".cold-build-input"), { recursive: true });
  assert.deepEqual(
    verifyColdPayloadArtifact(root, relocated, p.manifest),
    p.manifest,
  );
  assert.throws(() => verifyColdPayloadInput(root, relocated, p.manifest), {
    code: "ENOENT",
  });
  assert.deepEqual(
    verifyColdPayloadInput(root, output, p.manifest),
    p.manifest,
  );
});

test("relocated artifacts still refuse changed payloads, manager, provenance and sources", (t) => {
  for (const name of [
    "relocated/orbwin-services",
    "relocated/orbwin-trust",
    "relocated/orbwin-session.exe",
    "scripts/session-cold-payload-input.mjs",
    "backend-tools/session/internal/coldassets/compiled.go",
    "manifest",
  ]) {
    const { root, output, put } = fixture(t);
    const p = prepareColdPayloadInput(root, output);
    const relocated = join(root, "relocated");
    cpSync(output, relocated, {
      recursive: true,
      filter: (path) => !path.includes(".cold-build-input"),
    });
    if (name === "manifest") p.manifest.generatedSHA256 = "0".repeat(64);
    else put(name, "changed");
    assert.throws(
      () => verifyColdPayloadArtifact(root, relocated, p.manifest),
      name,
    );
  }
});
