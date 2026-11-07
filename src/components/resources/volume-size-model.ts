export type VolumeSize = {
  name: string;
  createdAt: string;
  bytes: number;
  references: number;
  measuredAt: string;
};
export function parseVolumeSize(
  text: string,
  name: string,
  createdAt: string,
): VolumeSize {
  if (text.length > 8192)
    throw new Error("Volume size output exceeded its limit.");
  const value = JSON.parse(text);
  if (
    !value ||
    value.name !== name ||
    value.createdAt !== createdAt ||
    !Number.isSafeInteger(value.bytes) ||
    value.bytes < 0 ||
    !Number.isSafeInteger(value.references) ||
    value.references < 0 ||
    value.references > 1_000_000 ||
    typeof value.measuredAt !== "string" ||
    !Number.isFinite(Date.parse(value.measuredAt))
  )
    throw new Error(
      "Volume size response does not match the selected volume or has invalid counters.",
    );
  return value;
}
