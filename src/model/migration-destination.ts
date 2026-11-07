export function parseMigrationDestination(
  text: string,
  expected: {
    source: string;
    sourceVolume: string;
    destination: string;
    target: string;
  },
) {
  if (text.length > 65536)
    throw new Error("Destination review exceeded its response budget.");
  const value = JSON.parse(text);
  if (
    !value ||
    value.version !== 1 ||
    value.source !== expected.source ||
    value.sourceVolume !== expected.sourceVolume ||
    value.destination !== expected.destination ||
    value.target !== expected.target ||
    typeof value.restoreTransaction !== "string" ||
    !/^[a-f0-9]{32}$/.test(value.restoreTransaction) ||
    typeof value.manifestSha256 !== "string" ||
    !/^[a-f0-9]{64}$/.test(value.manifestSha256) ||
    !Array.isArray(value.exportTransactions) ||
    !value.exportTransactions.length ||
    value.exportTransactions.length > 256 ||
    new Set(value.exportTransactions).size !==
      value.exportTransactions.length ||
    value.exportTransactions.some(
      (v: unknown) => typeof v !== "string" || !/^[a-f0-9]{32}$/.test(v),
    ) ||
    ![value.createdAt, value.observedAt].every(
      (v) =>
        typeof v === "string" &&
        v.length <= 64 &&
        Number.isFinite(Date.parse(v)),
    )
  )
    throw new Error(
      "Destination review does not match the selected source and target.",
    );
  return value as {
    restoreTransaction: string;
    exportTransactions: string[];
    manifestSha256: string;
    observedAt: string;
  };
}
