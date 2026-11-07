import test from "node:test";
import assert from "node:assert/strict";
import {
  imageImportChoices,
  imageImportReady,
  parseImageImportReview,
} from "../src/model/image-imports.ts";

const config = `sha256:${"a".repeat(64)}`;
const other = `sha256:${"b".repeat(64)}`;
const fixture = () => ({
  version: 1,
  source: "C:\\archive.tar",
  sha256: "1".repeat(64),
  bytes: 1024,
  images: [
    {
      id: config,
      tags: ["example/probe:test"],
      os: "linux",
      architecture: "amd64",
      layerCount: 1,
    },
  ],
  tags: [
    {
      reference: "docker.io/example/probe:test",
      configIds: [config],
      currentId: "",
    },
  ],
});
test("image import review defaults free tags to apply and conflicts to skip", () => {
  const review = parseImageImportReview(JSON.stringify(fixture()));
  assert.equal(imageImportChoices(review)[0].apply, true);
  review.tags[0].currentId = other;
  const choices = imageImportChoices(review);
  assert.equal(choices[0].apply, false);
  assert.equal(imageImportReady(choices), true);
  choices[0].apply = true;
  assert.equal(imageImportReady(choices), false);
  choices[0].replace = true;
  assert.equal(imageImportReady(choices), true);
  assert.equal(choices[0].expectedId, other);
});
test("multi-config tags need an explicit config selection", () => {
  const value = fixture();
  value.images.push({ ...value.images[0], id: other, architecture: "arm64" });
  value.tags[0].configIds.push(other);
  const choices = imageImportChoices(
    parseImageImportReview(JSON.stringify(value)),
  );
  assert.equal(choices[0].apply, false);
  assert.equal(choices[0].configId, "");
  choices[0].apply = true;
  assert.equal(imageImportReady(choices), false);
});
test("image import review rejects malformed, contradictory and unbounded metadata", () => {
  for (const change of [
    (value) => (value.sha256 = "changed"),
    (value) => value.images.push(value.images[0]),
    (value) => (value.images[0].layerCount = -1),
    (value) => (value.tags[0].currentId = "unknown"),
    (value) => (value.tags[0].configIds = [other]),
    (value) => (value.bytes = 65 * 1024 ** 3),
    (value) => value.tags.push(value.tags[0]),
  ]) {
    const value = fixture();
    change(value);
    assert.throws(() => parseImageImportReview(JSON.stringify(value)));
  }
});
