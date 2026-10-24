// Defensive parsing of `wsl --list --verbose` text. Headers are localized and
// WSL has emitted UTF-16 artifacts (NULs, BOM, stray CR) when redirected, so rows
// are recognised by shape (optional "*", name, state, numeric version) rather
// than by header text. Keep this file free of TypeScript-only runtime constructs
// so scripts/wsl-model.test.mjs can load it.
export type DistroRole = "engine" | "other-engine" | "user";
export type Distro = {
  name: string;
  state: string;
  version: number;
  isDefault: boolean;
  role: DistroRole;
};
export type ParsedDistros = { distros: Distro[]; ignored: string[] };

export function cleanWslText(raw: string): string {
  return raw
    .replace(/^﻿/, "")
    .replace(/\u0000/g, "")
    .replace(/\r/g, "");
}
export function roleOf(name: string): DistroRole {
  const lower = name.toLowerCase();
  if (lower.startsWith("lima-orbwin") || lower.startsWith("orbwin"))
    return "engine";
  // Name-based heuristic only: engines other than OrbWin's that use WSL.
  if (/^(rancher-desktop|lima-rd|docker-desktop)/.test(lower))
    return "other-engine";
  return "user";
}
export function parseDistributions(raw: string): ParsedDistros {
  const distros: Distro[] = [];
  const ignored: string[] = [];
  for (const line of cleanWslText(raw).split("\n")) {
    const text = line.trim();
    if (!text) continue;
    const isDefault = text.startsWith("*");
    const tokens = (isDefault ? text.slice(1) : text).trim().split(/\s+/);
    const version = tokens[tokens.length - 1];
    if (tokens.length >= 3 && /^[12]$/.test(version)) {
      const name = tokens.slice(0, -2).join(" ");
      distros.push({
        name,
        state: tokens[tokens.length - 2],
        version: Number(version),
        isDefault,
        role: roleOf(name),
      });
    } else ignored.push(text);
  }
  // The first non-matching line is normally the (localized) header. Anything
  // else that did not match is surfaced so unknown output is never hidden.
  return { distros, ignored: ignored.slice(distros.length ? 1 : 0) };
}

export type KeyValue = { key: string; value: string };
export function parseVersionInfo(raw: string): KeyValue[] {
  return cleanWslText(raw)
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const at = line.indexOf(":");
      return at > 0
        ? { key: line.slice(0, at).trim(), value: line.slice(at + 1).trim() }
        : { key: "", value: line };
    });
}
// Quote only when Windows command parsing would need it.
export function openCommand(name: string): string {
  return `wsl -d ${/[\s"&|<>^%]/.test(name) ? `"${name.replace(/"/g, "")}"` : name}`;
}
