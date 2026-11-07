import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  mkdtempSync,
  mkdirSync,
  writeFileSync,
  readFileSync,
  existsSync,
  rmSync,
  realpathSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { resolve, join, sep } from "node:path";
import { stageMaintenanceNotices } from "./maintenance-notices.mjs";
const hash = (value) => createHash("sha256").update(value).digest("hex");
function fixture(run) {
  const parent = realpathSync(tmpdir());
  const root = realpathSync(
    mkdtempSync(join(parent, "orbwin-maintenance-notices-")),
  );
  try {
    const notice = Buffer.from(
      "Preserved upstream copyright and license text.",
    );
    const archive = Buffer.from("Frozen source archive bytes for the fixture.");
    const recipe = Buffer.from("pkgname=probe");
    const input = Buffer.from("source input");
    mkdirSync(resolve(root, "third-party/maintenance-image/LICENSE"), {
      recursive: true,
    });
    mkdirSync(resolve(root, "artifacts/backend"), { recursive: true });
    writeFileSync(
      resolve(root, "third-party/maintenance-image/NOTICE.txt"),
      notice,
    );
    writeFileSync(
      resolve(root, "third-party/maintenance-image/LICENSE/component.txt"),
      notice,
    );
    writeFileSync(
      resolve(root, "artifacts/backend/maintenance-sources.tar.gz"),
      archive,
    );
    const file = (name, data) => ({
      file: name,
      sha256: hash(data),
      bytes: data.length,
    });
    const recipeRow = file("aports/probe/APKBUILD", recipe),
      inputRow = {
        ...file("distfiles/probe/source.tar", input),
        sha512: "b".repeat(128),
      };
    const pin = {
      schema: 1,
      imageRoot: "sha256:" + "a".repeat(64),
      imageSha256: "c".repeat(64),
      packages: [
        {
          name: "probe",
          origin: "probe",
          version: "1-r0",
          commit: "d".repeat(40),
          license: "MIT",
        },
      ],
      origins: [
        {
          origin: "probe",
          version: "1-r0",
          commit: "d".repeat(40),
          role: "installed-package-origin",
          recipeFiles: [recipeRow],
          inputs: [inputRow],
        },
      ],
      sourceFiles: [recipeRow, inputRow],
      notices: [
        file("NOTICE.txt", notice),
        file("LICENSE/component.txt", notice),
      ],
      noticeBindings: { probe: ["LICENSE/component.txt"] },
      sourceArchive: file("maintenance-sources.tar.gz", archive),
    };
    writeFileSync(
      resolve(root, "maintenance-image.lock.json"),
      JSON.stringify({ root: pin.imageRoot, sha256: pin.imageSha256 }),
    );
    const save = () =>
      writeFileSync(
        resolve(root, "maintenance-sources.lock.json"),
        JSON.stringify(pin),
      );
    save();
    return run({
      root,
      pin,
      save,
      archive,
      notice,
      destination: resolve(root, "stage"),
    });
  } finally {
    assert.equal(realpathSync(root), root);
    assert(
      root.startsWith(parent + sep) &&
        root.includes("orbwin-maintenance-notices-"),
    );
    rmSync(root, { recursive: true });
  }
}
test("verified sources and notices accompany the staged image inventory", () =>
  fixture(({ root, destination, archive, notice }) => {
    const result = stageMaintenanceNotices(root, destination);
    assert.equal(result.packages, 1);
    assert.equal(result.sourceFiles, 2);
    assert.deepEqual(
      readFileSync(resolve(destination, "SOURCES.tar.gz")),
      archive,
    );
    assert.deepEqual(
      readFileSync(resolve(destination, "LICENSE/component.txt")),
      notice,
    );
    assert(result.notices.some((row) => row.name === "SOURCE-INVENTORY.json"));
  }));
test("wrong image, incomplete package sources and unknown notice bindings refuse before staging", () => {
  for (const change of [
    (pin) => {
      pin.imageSha256 = "e".repeat(64);
    },
    (pin) => {
      pin.packages[0].version = "2-r0";
    },
    (pin) => {
      pin.origins[0].inputs[0].sha512 = "unverified";
    },
    (pin) => {
      pin.sourceFiles = [];
    },
    (pin) => {
      pin.noticeBindings.probe = ["LICENSE/missing.txt"];
    },
    (pin) => {
      pin.notices[0].file = "../outside";
    },
  ])
    fixture(({ root, destination, pin, save }) => {
      change(pin);
      save();
      assert.throws(() => stageMaintenanceNotices(root, destination));
      assert.equal(existsSync(destination), false);
    });
});
test("changed source/notice bytes never replace a previously staged artifact", () => {
  for (const name of [
    "third-party/maintenance-image/LICENSE/component.txt",
    "artifacts/backend/maintenance-sources.tar.gz",
  ])
    fixture(({ root, destination }) => {
      writeFileSync(resolve(root, name), "changed bytes");
      assert.throws(() => stageMaintenanceNotices(root, destination));
      assert.equal(existsSync(destination), false);
    });
  fixture(({ root, destination }) => {
    stageMaintenanceNotices(root, destination);
    writeFileSync(
      resolve(destination, "SOURCES.tar.gz"),
      "foreign staged bytes",
    );
    assert.throws(() => stageMaintenanceNotices(root, destination));
    assert.equal(
      readFileSync(resolve(destination, "SOURCES.tar.gz"), "utf8"),
      "foreign staged bytes",
    );
  });
});
