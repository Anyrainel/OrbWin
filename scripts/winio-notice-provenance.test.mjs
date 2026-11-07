import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, writeFileSync, mkdirSync, cpSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fixture } from "./lima-wsl2-test-fixture.mjs";
import {
  classifyWinioNoticeData,
  winioNoticeVariantSuffixData,
  stageWinioSourceOffer,
  verifyPortableWinioBuild,
} from "./winio-build-input.mjs";
test("modified and independent same-version notices retain distinct source identities", () => {
  const path = "C:/fixture/winio",
    root = { Path: "github.com/Microsoft/go-winio", Version: "v0.6.4", Replace: { Path: path } };
  assert.equal(classifyWinioNoticeData(root, true, path), "orbwin-winio-custody");
  assert.equal(classifyWinioNoticeData({ Path: root.Path, Version: "v0.6.4" }, false, path), null);
  assert.notEqual(
    winioNoticeVariantSuffixData(undefined),
    winioNoticeVariantSuffixData("orbwin-winio-custody"),
  );
  assert.equal(winioNoticeVariantSuffixData("orbwin-wsl2"), "#orbwin-wsl2");
  assert.throws(() => winioNoticeVariantSuffixData("foreign"));
  for (const version of ["v0.6.2", "v0.6.3-0.20251027160822-ad3df93bed29"])
    assert.equal(classifyWinioNoticeData({ Path: root.Path, Version: version }, false, path), null);
  assert.throws(() => classifyWinioNoticeData({ ...root, Version: "v0.6.2" }, true, path));
  assert.throws(() => classifyWinioNoticeData(root, false, path));
});
test("all five source offer files copy exactly and relocation loses no provenance", (t) => {
  const f = fixture(t),
    p = f.prepare(),
    dest = join(f.root, "notices/winio-custody");
  stageWinioSourceOffer({
    root: f.root,
    output: f.output,
    portable: p.portableWinio,
    destination: dest,
  });
  for (const [n, d] of Object.entries(p.portableWinio.offerFiles)) {
    assert.equal(
      readFileSync(join(dest, n)).equals(readFileSync(join(f.output, "licenses/winio-custody", n))),
      true,
    );
  }
  assert.throws(
    () =>
      stageWinioSourceOffer({
        root: f.root,
        output: f.output,
        portable: p.portableWinio,
        destination: dest,
      }),
    /fresh/,
  );
  const relocated = join(f.root, "relocated/backend"),
    legal = join(f.root, "relocated/licenses");
  mkdirSync(relocated, { recursive: true });
  mkdirSync(legal, { recursive: true });
  cpSync(dest, join(legal, "winio-custody"), { recursive: true });
  assert.deepEqual(
    verifyPortableWinioBuild({
      root: f.root,
      output: relocated,
      portable: p.portableWinio,
      licensesDirectory: legal,
    }),
    p.portableWinio,
  );
  writeFileSync(join(legal, "winio-custody/LICENSE"), "changed");
  assert.throws(() =>
    verifyPortableWinioBuild({
      root: f.root,
      output: relocated,
      portable: p.portableWinio,
      licensesDirectory: legal,
    }),
  );
});
test("static expected runtime offer is complete, outside recipe cycle, and tamper refuses before output", (t) => {
  const f = fixture(t),
    lock = JSON.parse(readFileSync(join(f.root, "winio-custody.lock.json"))),
    expected = JSON.parse(readFileSync(join(f.root, "winio-runtime-offer.json")));
  assert.equal(Object.keys(lock.profile.recipeHashes).length, 6);
  assert.ok(!Object.hasOwn(lock.profile.recipeHashes, "winio-runtime-offer.json"));
  assert.equal(Object.keys(expected.offerFiles).length, 5);
  expected.offerFiles["manifest.json"] = "0".repeat(64);
  writeFileSync(join(f.root, "winio-runtime-offer.json"), JSON.stringify(expected));
  assert.throws(() => f.prepare(), /runtime source offer/);
  assert.equal(existsSync(f.output), false);
});
