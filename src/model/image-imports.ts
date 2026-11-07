import type { ImageImportChoice } from "../backend";

export type ImageImportReview = {
  version: 1;
  source: string;
  sha256: string;
  bytes: number;
  images: {
    id: string;
    tags: string[] | null;
    os: string;
    architecture: string;
    variant?: string;
    layerCount: number;
  }[];
  tags: { reference: string; configIds: string[]; currentId: string }[];
};
const digest = (value: unknown): value is string =>
  typeof value === "string" && /^[a-f0-9]{64}$/.test(value);
const id = (value: unknown): value is string =>
  typeof value === "string" && /^sha256:[a-f0-9]{64}$/.test(value);
const bounded = (value: unknown, limit: number): value is string =>
  typeof value === "string" && value.length <= limit && !/[\0\r\n]/.test(value);
export function parseImageImportReview(text: string): ImageImportReview {
  const value = JSON.parse(text) as ImageImportReview;
  if (
    !value ||
    value.version !== 1 ||
    !bounded(value.source, 4096) ||
    !digest(value.sha256) ||
    !Number.isSafeInteger(value.bytes) ||
    value.bytes <= 0 ||
    value.bytes > 64 * 1024 ** 3 ||
    !Array.isArray(value.images) ||
    value.images.length < 1 ||
    value.images.length > 128 ||
    !Array.isArray(value.tags) ||
    value.tags.length > 512
  )
    throw new Error("Invalid image archive review.");
  const configs = new Set<string>();
  for (const image of value.images) {
    if (
      !image ||
      !id(image.id) ||
      configs.has(image.id) ||
      image.os !== "linux" ||
      !bounded(image.architecture, 64) ||
      !image.architecture ||
      (image.variant !== undefined && !bounded(image.variant, 64)) ||
      !Number.isSafeInteger(image.layerCount) ||
      image.layerCount < 0 ||
      image.layerCount > 4096 ||
      (image.tags !== null &&
        (!Array.isArray(image.tags) ||
          image.tags.length > 1024 ||
          image.tags.some((tag) => !bounded(tag, 512))))
    )
      throw new Error("Invalid image configuration in archive review.");
    configs.add(image.id);
  }
  const refs = new Set<string>();
  for (const tag of value.tags) {
    if (
      !tag ||
      !bounded(tag.reference, 512) ||
      !tag.reference ||
      refs.has(tag.reference) ||
      (tag.currentId !== "" && !id(tag.currentId)) ||
      !Array.isArray(tag.configIds) ||
      !tag.configIds.length ||
      tag.configIds.length > 128 ||
      new Set(tag.configIds).size !== tag.configIds.length ||
      tag.configIds.some((config) => !configs.has(config))
    )
      throw new Error("Invalid destination tag review.");
    refs.add(tag.reference);
  }
  return value;
}
export function imageImportChoices(
  review: ImageImportReview,
): ImageImportChoice[] {
  return review.tags.map((tag) => ({
    reference: tag.reference,
    configId: tag.configIds.length === 1 ? tag.configIds[0] : "",
    expectedId: tag.currentId,
    apply: !tag.currentId && tag.configIds.length === 1,
    replace: false,
  }));
}
export function imageImportReady(choices: ImageImportChoice[]) {
  return choices.every(
    (choice) =>
      !choice.apply ||
      (id(choice.configId) && (!choice.expectedId || choice.replace)),
  );
}
