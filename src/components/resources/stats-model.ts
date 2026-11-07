// Docker CLI values are rounded display counters. Do not turn them into precise
// rates or sum percentages of different per-container memory limits.
export type StatsSample = {
  cpu: number;
  memory: number;
  networkIn: number;
  networkOut: number;
  blockRead: number;
  blockWrite: number;
  containers: number;
};
export const sampleLimit = 60;
export const samplingIntervals = [3000, 5000, 10000, 30000] as const;

export function bytes(value: unknown): number {
  const match =
    typeof value === "string" &&
    value.trim().match(/^(\d+(?:\.\d+)?)\s*(B|kB|MB|GB|TB|KiB|MiB|GiB|TiB)$/);
  if (!match)
    throw new Error("The engine returned an unreadable byte counter.");
  const units: Record<string, number> = {
    B: 1,
    kB: 1e3,
    MB: 1e6,
    GB: 1e9,
    TB: 1e12,
    KiB: 1024,
    MiB: 1024 ** 2,
    GiB: 1024 ** 3,
    TiB: 1024 ** 4,
  };
  const result = Number(match[1]) * units[match[2]];
  if (!Number.isFinite(result) || result > Number.MAX_SAFE_INTEGER)
    throw new Error("The engine returned an oversized byte counter.");
  return result;
}
function pair(value: unknown) {
  const parts = typeof value === "string" ? value.split("/") : [];
  if (parts.length !== 2)
    throw new Error("The engine returned an unreadable counter pair.");
  return parts.map(bytes);
}
export function parseStats(
  output: string,
  engine: boolean,
  target?: string,
): StatsSample {
  if (output.length > 2 * 1024 * 1024)
    throw new Error("Stats exceeded the view's output limit.");
  const lines = output.trim() ? output.trim().split("\n") : [];
  if (lines.length > 4096 || (!engine && lines.length !== 1))
    throw new Error(
      "No current sample for this container. It may have stopped or been removed.",
    );
  const result: StatsSample = {
    cpu: 0,
    memory: 0,
    networkIn: 0,
    networkOut: 0,
    blockRead: 0,
    blockWrite: 0,
    containers: lines.length,
  };
  const ids = new Set<string>();
  for (const line of lines) {
    const row = JSON.parse(line);
    if (
      !row ||
      typeof row.ID !== "string" ||
      !/^[a-f0-9]{64}$/.test(row.ID) ||
      ids.has(row.ID)
    )
      throw new Error(
        "Stats returned an invalid or repeated container identity.",
      );
    ids.add(row.ID);
    if (target && row.ID !== target)
      throw new Error("Stats returned a different container identity.");
    if (!engine && row.PIDs === "0")
      throw new Error(
        "No active processes reported. This container may be stopped; previous samples are retained.",
      );
    if (
      typeof row.CPUPerc !== "string" ||
      !/^\d+(?:\.\d+)?%$/.test(row.CPUPerc)
    )
      throw new Error("The engine returned an unreadable CPU sample.");
    const cpu = Number(row.CPUPerc.slice(0, -1));
    if (!Number.isFinite(cpu))
      throw new Error("The engine returned an invalid CPU sample.");
    result.cpu += cpu;
    result.memory += pair(row.MemUsage)[0];
    const [incoming, outgoing] = pair(row.NetIO);
    const [read, write] = pair(row.BlockIO);
    result.networkIn += incoming;
    result.networkOut += outgoing;
    result.blockRead += read;
    result.blockWrite += write;
  }
  if (Object.values(result).some((value) => !Number.isFinite(value))) {
    throw new Error("The engine returned overflowing aggregate counters.");
  }
  return result;
}
export function formatBytes(value: number) {
  const index = Math.min(4, Math.floor(Math.log2(Math.max(1, value)) / 10));
  return `${(value / 1024 ** index).toFixed(1)} ${["B", "KiB", "MiB", "GiB", "TiB"][index]}`;
}
