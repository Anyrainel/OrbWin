import { useCallback, useEffect, useRef, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import * as backend from "../../backend";
import { parseAppSettings, type AppSettings } from "./app-settings-model";

export type AppSettingsRead =
  | { kind: "idle" }
  | { kind: "loading"; previous: AppSettings | null }
  | { kind: "ready"; settings: AppSettings; readAt: number }
  | { kind: "error"; message: string; previous: AppSettings | null };

export function currentSettings(read: AppSettingsRead): AppSettings | null {
  if (read.kind === "ready") return read.settings;
  if (read.kind === "loading" || read.kind === "error") return read.previous;
  return null;
}

// Reads the rdd App resource through the existing read-only operation. The
// native adapter never auto-starts the management service for this read, so an
// offline backend yields an error state, not defaults. Overlapping requests are
// coalesced; a stale reply never overwrites a newer one.
export function useAppSettings() {
  const [read, setRead] = useState<AppSettingsRead>({ kind: "idle" });
  const generation = useRef(0);
  const last = useRef<AppSettings | null>(null);
  const reload = useCallback(async () => {
    if (!isTauri()) {
      setRead({
        kind: "error",
        message:
          "Backend configuration is read by the Windows desktop application. This browser preview has no backend connection.",
        previous: null,
      });
      return;
    }
    const id = ++generation.current;
    setRead({ kind: "loading", previous: last.current });
    let output = "";
    try {
      const outcome = await backend.run(
        crypto.randomUUID(),
        { kind: "readSettings" },
        (chunk) => {
          output = (output + chunk).slice(-1048576);
        },
      );
      if (id !== generation.current) return;
      if (!outcome.success) {
        const detail = output.trim().split(/\r?\n/).slice(-2).join(" ");
        setRead({
          kind: "error",
          message: `${outcome.message}${detail ? ` ${detail}` : ""}`,
          previous: last.current,
        });
        return;
      }
      const settings = parseAppSettings(output);
      last.current = settings;
      setRead({ kind: "ready", settings, readAt: Date.now() });
    } catch (error) {
      if (id === generation.current)
        setRead({
          kind: "error",
          message: String(error),
          previous: last.current,
        });
    }
  }, []);
  useEffect(() => {
    const changed = () => void reload();
    window.addEventListener("orbwin:cluster-changed", changed);
    window.addEventListener("orbwin:app-settings-changed", changed);
    return () => {
      window.removeEventListener("orbwin:cluster-changed", changed);
      window.removeEventListener("orbwin:app-settings-changed", changed);
    };
  }, [reload]);
  return { read, reload };
}
